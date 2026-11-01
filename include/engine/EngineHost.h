#pragma once

#include <memory>
#include <optional>

#include <QList>
#include <QObject>
#include <QStringList>

#include "include/engine/EngineTypes.h"

namespace Engine {
    class AutoSelectorController;
    class ConnectionManager;
    class CoreSupervisor;
    class EnginePolicy;
    class ExitController;
    class StatsService;
    class TestObserver;
    class TestRunner;
    class VpnAuthController;

    // Owns and wires every engine object. Main thread unless noted.
    class EngineHost final : public QObject {
        Q_OBJECT

    public:
        // Once, after initDB, DS_cores, the translator, Logging::Init and API::defaultClient; builds and wires, starts nothing.
        // Never destroyed: ~TestRunner joins its pool and the core process outlives the app object.
        static EngineHost *create();

        ~EngineHost() override;

        // Once the front end exists: binds its answers, then the random inbound port, the core IPC listener, the kill switch
        // and the core. observer may be null. The remembered profile starts once the core is up.
        void init(EnginePolicy *policy, TestObserver *observer);

        // Back to the unbound answers, for a front end that dies first.
        void detachFrontEnd();

        // Idempotent; ends with KillSwitch::shutdown(). interactive false: a failing or slow stop is logged, never shown.
        void shutdown(bool interactive = true);

        // Engine half of MwMessage::UpdateSettings; its dialogs appear only when interactive.
        void applySettingsChange(const QStringList &flags, bool interactive);

        // Stable for the process: forwards to the bound policy, or to the unbound answers while none is bound.
        EnginePolicy *policy() const;

        ConnectionManager *connection() const { return m_connection; }

        CoreSupervisor *supervisor() const { return m_coreSupervisor; }

        StatsService *stats() const { return m_stats; }

        TestRunner *tests() const { return m_tests.get(); }

        AutoSelectorController *autoSelector() const { return m_autoSelector.get(); }

        VpnAuthController *vpnAuth() const { return m_vpnAuth; }

        ExitController *exitController() const { return m_exit; }

        // After clearing, testResultsCleared carries the groups of the cleared profiles.
        int clearTestResults(const QList<int> &profileIds);

        // Empty when nothing runs, stats are off, or the last sample is older than maxAgeMs.
        std::optional<LiveRates> freshLiveRates(qint64 maxAgeMs) const;

    signals:
        // Where applySettingsChange used to refresh the status bar; slots run before its restart prompt.
        void statusChanged();

        // After clearTestResults() cleared at least one profile; the groups of the cleared profiles.
        void testResultsCleared(const QList<int> &groupIds);

    private:
        class PolicyRelay;
        class ObserverRelay;

        EngineHost() = default;

        void construct();

        void registerPeriodicJobs();

        std::unique_ptr<PolicyRelay> m_policy;
        std::unique_ptr<ObserverRelay> m_observer;
        StatsService *m_stats = nullptr;
        CoreSupervisor *m_coreSupervisor = nullptr;
        ConnectionManager *m_connection = nullptr;
        VpnAuthController *m_vpnAuth = nullptr;
        std::unique_ptr<TestRunner> m_tests;
        // After m_tests, so it is destroyed first.
        std::unique_ptr<AutoSelectorController> m_autoSelector;
        ExitController *m_exit = nullptr;
        bool m_initialized = false;
    };

    // Null before EngineHost::create().
    EngineHost *host();
} // namespace Engine
