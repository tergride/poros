#pragma once

#include <atomic>
#include <memory>

#include <QElapsedTimer>
#include <QList>
#include <QMutex>
#include <QObject>
#include <QString>

#include "include/engine/EngineTypes.h"

namespace Configs {
    class Profile;
}

namespace Engine {
    class AutoSelectorController;
    class CoreSupervisor;
    class EnginePolicy;
    class StatsService;
    class TestRunner;
    class VpnAuthController;

    // Starts, stops and switches the running profile, applies the Tun/system-proxy/route modes and reacts to the kill switch.
    // Lives on the main thread and emits only there; main thread unless noted.
    class ConnectionManager final : public QObject {
        Q_OBJECT

    public:
        ConnectionManager(EnginePolicy *policy, CoreSupervisor *coreSupervisor, StatsService *stats, QObject *parent = nullptr);

        // Before the event loop runs; all three outlive every start and stop.
        void attach(TestRunner *tests, AutoSelectorController *autoSelector, VpnAuthController *vpnAuth);

        // Once, before the first KillSwitch::apply(), so the state changes apply() emits reach the handler.
        void attachKillSwitch();

        void start(const StartRequest &request);

        // Any thread, a start worker included; runs on the caller. block: the caller spins a nested loop until the main-thread
        // teardown ran.
        void stop(const StopOptions &options);

        bool restartStartedProfile(bool interactive = true);

        void applyPendingRestart();

        bool tunPrivileged();

        // Any thread. The running config carries a tun inbound.
        bool runningTun() const;

        ConnectionState connectionState() const;

        // Any thread.
        std::shared_ptr<Configs::Profile> runningSnapshot() const;

        int resolveLastProfile() const;

        int lastProfileId() const;

        // ConnectionModes.cpp. Each returns whether it restarted the profile.
        bool setSystemProxy(bool enable, const ModeChange &change = {});

        bool setTun(bool enable, const ModeChange &change = {});

        bool chooseRoute(int routeId, bool interactive = true);

        // Any thread. Queued on one worker thread in call order; wait spins a nested loop until this change ran.
        void applySystemProxy(bool enable, bool wait = false);

        bool ensureTunPrivileges(bool interactive);

        void disableKillSwitch();

        // A no-op while nothing was started.
        void noteRestartNeeded(RestartReason reason);

        void clearRestartNeeded();

        QList<RestartReason> restartReasons() const;

        bool restartPending() const;

    signals:
        // View hints, emitted where the old code refreshed a view; slots must not block, open a dialog, start or stop.
        void statusChanged();

        void busyChanged();

        void profileRowsChanged(const QList<int> &profileIds);

        void runMarker(qint64 monoMs, Engine::RunMarker kind, const QString &name);

        // Only when the set of pending reasons changed.
        void restartReasonsChanged();

    private:
        static constexpr qint64 kGuardRestartWindowMs = 15000;

        EnginePolicy *policy() const { return m_policy; }

        void onCoreStarted(int profileId);

        void onCoreGaveUp();

        void failStart(const StartRequest &request, const QString &title, const QString &error);

        void deferStartToCore(const StartRequest &request);

        // tun: the running config carries a tun inbound (generated Tun or a custom config's).
        void setRunning(std::shared_ptr<Configs::Profile> profile, bool tun = false);

        void lookupExitCountry(const std::shared_ptr<Configs::Profile> &ent);

        // ConnectionModes.cpp.
        void onKillSwitchStateChanged();

        bool guardRestartPending() const;

        void restartCoreForGuard(const StartRequest &request);

        EnginePolicy *m_policy;
        CoreSupervisor *m_coreSupervisor;
        StatsService *m_stats;
        TestRunner *m_tests = nullptr;
        AutoSelectorController *m_autoSelector = nullptr;
        VpnAuthController *m_vpnAuth = nullptr;

        // Written by the start/stop workers; read through runningSnapshot().
        mutable QMutex m_runningMutex;
        std::shared_ptr<Configs::Profile> m_running;
        bool m_runningTun = false;

        // Main thread only.
        int m_lastProfileId = -1;
        bool m_profileConnecting = false;
        bool m_profileDisconnecting = false;
        StartRequest m_killSwitchDeferredStart;
        // coreStarted carries only a profile id, so the rest of the request waits here.
        StartRequest m_coreStartRequest;
        QElapsedTimer m_guardCoreRestart;
        bool m_killSwitchWasFailed = false;
        bool m_killSwitchWasArmed = false;
        QList<RestartReason> m_restartReasons;

        // Locked and unlocked on different threads by design; no owner tracking.
        QMutex mu_starting;
        QMutex mu_stopping;
        // Set while start() stops the running profile, so that stop is marked as part of the switch.
        std::atomic<bool> m_switchingProfile{false};

    };
} // namespace Engine
