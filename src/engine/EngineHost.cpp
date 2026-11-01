#include "include/engine/EngineHost.h"

#include <atomic>
#include <memory>

#include "include/configs/generate.h"
#include "include/configs/sub/GroupUpdater.hpp"
#include "include/configs/sub/RouteUpdater.hpp"
#include "include/engine/EngineEvents.h"
#include "include/engine/EnginePolicy.h"
#include "include/engine/EngineTr.h"
#include "include/engine/Lifecycle.h"
#include "include/engine/ThreadCheck.h"
#include "include/engine/app/ExitController.h"
#include "include/engine/autoselector/AutoSelectorController.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/engine/UnboundPolicy.h"
#include "include/engine/services/ProfileService.h"
#include "include/engine/services/ReleaseUpdates.h"
#include "include/engine/services/StatsService.h"
#include "include/engine/supervisor/CoreSupervisor.h"
#include "include/engine/tests/TestObserver.h"
#include "include/engine/tests/TestRunner.h"
#include "include/engine/vpn/VpnAuthController.h"
#include "include/global/Configs.hpp"
#include "include/global/DeviceDetailsHelper.hpp"
#include "include/global/Logger.hpp"
#include "include/global/PeriodicRunner.hpp"
#include "include/sys/AutoRun.hpp"
#include "include/sys/KillSwitch.hpp"

namespace Engine {
    namespace {
        EngineHost *engineHostInstance = nullptr;
    } // namespace

    class EngineHost::PolicyRelay final : public EnginePolicy {
    public:
        void bind(EnginePolicy *policy) { m_target = policy; }

        void geoAssetMissingCategory(const QString &contextName, const QString &needed, const QString &whichFile) override {
            target(__func__)->geoAssetMissingCategory(contextName, needed, whichFile);
        }

        bool confirmGeoAssetDownload(const QString &contextName) override {
            return target(__func__)->confirmGeoAssetDownload(contextName);
        }

        void geoAssetDownloadFinished(const QString &error) override {
            target(__func__)->geoAssetDownloadFinished(error);
        }

        void killSwitchProblem() override {
            target(__func__)->killSwitchProblem();
        }

        void killSwitchProblemResolved() override {
            target(__func__)->killSwitchProblemResolved();
        }

        void startFailed(const StartRequest &request, const QString &title, const QString &error) override {
            target(__func__)->startFailed(request, title, error);
        }

        void strictRouteUnavailable(const QString &error) override {
            target(__func__)->strictRouteUnavailable(error);
        }

        bool offerCoreReset(const QString &error) override {
            return target(__func__)->offerCoreReset(error);
        }

        bool offerLeftoverTunAdapterRetry(const QString &error) override {
            return target(__func__)->offerLeftoverTunAdapterRetry(error);
        }

        void stopFailed(const QString &error) override {
            target(__func__)->stopFailed(error);
        }

        SlowOperation slowOperation(SlowOperationKind kind, int delayMs) override {
            return target(__func__)->slowOperation(kind, delayMs);
        }

        PrivilegeDecision requestCorePrivileges(PrivilegeReason reason, int profileId) override {
            return target(__func__)->requestCorePrivileges(reason, profileId);
        }

        bool settleMode(const StartRequest &request) override {
            return target(__func__)->settleMode(request);
        }

        bool vpnPromptOpen() const override {
            return target(__func__)->vpnPromptOpen();
        }

        void vpnChallenge(const VpnAuthChallenge &challenge) override {
            target(__func__)->vpnChallenge(challenge);
        }

        void vpnCredentialsRejected(int profileId, const VpnAuthChallenge &challenge,
                                    const std::function<void(const QString &username, const QString &password)> &retryWith) override {
            target(__func__)->vpnCredentialsRejected(profileId, challenge, retryWith);
        }

        void vpnChallengesCleared() override {
            target(__func__)->vpnChallengesCleared();
        }

        void settingsNeedTunRestart() override {
            target(__func__)->settingsNeedTunRestart();
        }

        bool confirmRestartProgram() override {
            return target(__func__)->confirmRestartProgram();
        }

        void testSessionBusy(const QString &text) override {
            target(__func__)->testSessionBusy(text);
        }

        void exitProgram(ExitReason reason) override {
            target(__func__)->exitProgram(reason);
        }

    private:
        EnginePolicy *target(const char *method) const {
            if (m_target != nullptr) return m_target;
            if (!m_warnedUnbound.exchange(true)) {
                LOG_WARN(QStringLiteral("POLICY-UNBOUND %1 got the unbound answer: no front end is bound").arg(QLatin1String(method)));
            }
            return &m_fallback;
        }

        EnginePolicy *m_target = nullptr;
        mutable UnboundPolicy m_fallback;
        mutable std::atomic<bool> m_warnedUnbound{false};
    };

    class EngineHost::ObserverRelay final : public TestObserver {
    public:
        void bind(TestObserver *observer) { m_target.store(observer); }

        void testPanelSeeded(TestKind kind, int totalProfiles) override {
            if (auto *target = m_target.load()) target->testPanelSeeded(kind, totalProfiles);
        }

        void testProgressed() override {
            if (auto *target = m_target.load()) target->testProgressed();
        }

        void testPanelsCleared() override {
            if (auto *target = m_target.load()) target->testPanelsCleared();
        }

        void testViewChanged(bool force) override {
            if (auto *target = m_target.load()) target->testViewChanged(force);
        }

        void speedTestProgress(const QString &profileName, const libcore::SpeedTestResult &result) override {
            if (auto *target = m_target.load()) target->speedTestProgress(profileName, result);
        }

        void testedProfilesChanged(const QList<int> &profileIds) override {
            if (auto *target = m_target.load()) target->testedProfilesChanged(profileIds);
        }

    private:
        std::atomic<TestObserver *> m_target{nullptr};
    };

    EngineHost::~EngineHost() {
        engineHostInstance = nullptr;
        detachFrontEnd();
        Subscription::updater()->SetUrlTester(nullptr);
        Lifecycle::SetStopHandler({});
        m_autoSelector.reset();
        m_tests.reset();
        delete m_exit;
        delete m_vpnAuth;
        delete m_connection;
        delete m_coreSupervisor;
        delete m_stats;
    }

    EngineHost *EngineHost::create() {
        ENGINE_CHECK_MAIN_THREAD();
        if (engineHostInstance != nullptr) {
            LOG_WARN("EngineHost::create() called again; keeping the existing host");
            return engineHostInstance;
        }
        Sys::KillSwitch::instance();
        Poros::PeriodicRunner::instance();
        Subscription::updater();
        EngineEvents::instance();
        auto *created = new EngineHost;
        engineHostInstance = created;
        created->construct();
        return created;
    }

    void EngineHost::construct() {
        m_policy = std::make_unique<PolicyRelay>();
        m_observer = std::make_unique<ObserverRelay>();
        m_stats = new StatsService(this);
        m_coreSupervisor = new CoreSupervisor(this);
        m_connection = new ConnectionManager(policy(), m_coreSupervisor, m_stats, this);
        m_vpnAuth = new VpnAuthController(policy(), m_connection, this);
        Lifecycle::SetStopHandler([c = m_connection](bool crash, bool block, bool manual, bool interactive) { c->stop({crash, block, manual, interactive}); });
        m_tests = std::make_unique<TestRunner>(policy(), m_observer.get(), [c = m_connection] { return c->runningSnapshot(); });
        m_autoSelector = std::make_unique<AutoSelectorController>(m_connection, m_tests.get());
        m_connection->attach(m_tests.get(), m_autoSelector.get(), m_vpnAuth);
        // In this order: the traffic and connection loops spawn before the selector monitor, as before.
        connect(m_coreSupervisor, &CoreSupervisor::rpcConnected, m_stats, &StatsService::startLoops);
        connect(m_coreSupervisor, &CoreSupervisor::rpcConnected, m_autoSelector.get(), &AutoSelectorController::startMonitorLoop);
        m_exit = new ExitController(policy(), m_connection, m_coreSupervisor, this);

        connect(EngineEvents::instance(), &EngineEvents::subscriptionFinished, this, [](int importedCount, bool quiet) {
            if (quiet) return;
            // The job's own count: the global may already belong to the next job.
            MW_show_log(MainWindowTr::tr("Imported %1 profile(s)").arg(importedCount >= 0 ? importedCount : Configs::dataManager->settingsRepo->imported_count));
        });
        connect(EngineEvents::instance(), &EngineEvents::subscriptionReplacedRunning, this, [this](int profileId) {
            // Whatever the user started since the refresh stopped the old profile wins.
            if (m_connection->connectionState() != ConnectionState::Idle) return;
            m_connection->start(StartRequest{profileId, false});
        });

        runOnNewThread([] { GetDeviceDetails(); });

        Subscription::updater()->SetUrlTester([this](const QList<int> &profileIDs, bool interactive, const Subscription::GroupUpdater::Finish &done) {
            m_tests->queueUrlTests(profileIDs, done, interactive);
        });

        registerPeriodicJobs();
    }

    void EngineHost::init(EnginePolicy *frontEnd, TestObserver *testObserver) {
        ENGINE_CHECK_MAIN_THREAD();
        if (m_initialized) {
            LOG_WARN("EngineHost::init() called again; ignored");
            return;
        }
        m_initialized = true;
        m_policy->bind(frontEnd);
        m_observer->bind(testObserver);

        // Not in construct(): MkPort's failure line needs the front end's log sink; the emit repaints the port the window drew.
        if (Configs::dataManager->settingsRepo->random_inbound_port)
        {
            Configs::dataManager->settingsRepo->inbound_socks_port = MkPort(Configs::dataManager->settingsRepo->inbound_address);
            emit statusChanged();
        }

        // On failure the supervisor posts the notice, and start() and every profile start refuse to run.
        m_coreSupervisor->listen();

        // Before apply(): a state change inside apply() must reach the handler.
        m_connection->attachKillSwitch();

        // Must precede the first coreStarted, or a remembered profile would start before the guard is even arming.
        Sys::KillSwitch::instance()->apply();

        const auto &startSettings = Configs::dataManager->settingsRepo;
        // A relaunch for rights carries on with the start that asked for them.
        if (startSettings->flag_restart_profile >= 0) m_coreSupervisor->start(startSettings->flag_restart_profile);
        else m_coreSupervisor->start(startSettings->remember_enable && startSettings->remember_id >= 0 ? startSettings->remember_id : -1);
    }

    void EngineHost::detachFrontEnd() {
        if (m_policy) m_policy->bind(nullptr);
        if (m_observer) m_observer->bind(nullptr);
    }

    void EngineHost::shutdown(bool interactive) {
        ENGINE_CHECK_MAIN_THREAD();
        m_exit->shutdown(interactive);
    }

    void EngineHost::applySettingsChange(const QStringList &flags, bool interactive) {
        ENGINE_CHECK_MAIN_THREAD();
        const auto changed = [&](const QString &flag) { return flags.contains(flag); };
        auto &settings = Configs::dataManager->settingsRepo;

        if (changed(MwArg::ChoosePort)) {
            settings->inbound_socks_port = MkPort(settings->inbound_address);
            if (settings->spmode_system_proxy) {
                const ModeChange change{.interactive = interactive};
                m_connection->setSystemProxy(false, change);
                m_connection->setSystemProxy(true, change);
            }
        }
        if (changed(MwArg::DisableAdmin)) {
            AutoRun_FixTaskIfNeeded();
        }
        auto suggestRestartProxy = settings->Save();
        Poros::PeriodicRunner::instance()->CheckNow();
        if (changed(MwArg::KillSwitch) || changed(MwArg::Vpn)) Sys::KillSwitch::instance()->apply();
        if (changed(MwArg::Route)) {
            settings->Save();
            suggestRestartProxy = true;
        }
        if (changed(MwArg::NeedRestart)) {
            suggestRestartProxy = false;
        }
        if (changed(MwArg::Vpn) && settings->spmode_vpn) {
            if (interactive) policy()->settingsNeedTunRestart();
            else MW_show_log(MainWindowTr::tr("Tun Settings changed") + ": " + MainWindowTr::tr("Restart Tun to take effect."));
        }
        if (changed(MwArg::ChoosePort) || suggestRestartProxy) {
            m_connection->noteRestartNeeded(changed(MwArg::Route) ? Engine::RestartReason::Routing : Engine::RestartReason::Settings);
        }
        emit statusChanged();
        if (changed(MwArg::NeedRestart)) {
            if (!interactive) {
                MW_show_log(MainWindowTr::tr("Settings changed") + ": " + MainWindowTr::tr("Restart the program to take effect."));
            } else if (policy()->confirmRestartProgram()) {
                m_exit->requestExit(ExitReason::Restart);
            }
        }
    }

    EnginePolicy *EngineHost::policy() const {
        return m_policy.get();
    }

    int EngineHost::clearTestResults(const QList<int> &profileIds) {
        QList<int> gids;
        const int n = ProfileService::ClearTestResults(profileIds, &gids);
        if (n > 0) emit testResultsCleared(gids);
        return n;
    }

    std::optional<LiveRates> EngineHost::freshLiveRates(qint64 maxAgeMs) const {
        if (m_connection->runningSnapshot() == nullptr) return std::nullopt;
        return m_stats->freshLiveRates(maxAgeMs);
    }

    void EngineHost::registerPeriodicJobs() {
        auto* runner = Poros::PeriodicRunner::instance();
        // Interval is sign-encoded in settings (negative = disabled); < 30 min counts as off.
        const auto minutesOf = [](int v) { return v >= 30 ? v : 0; };
        // Every poll while enabled: each group keeps its own schedule, persisted as its sub_last_update.
        runner->Add({
            {},
            [minutesOf] { return minutesOf(Configs::dataManager->settingsRepo->sub_auto_update) > 0 ? 1 : 0; },
            nullptr,
            nullptr,
            [] { Subscription::updater()->CheckAutoUpdate(); },
        });
        runner->Add({
            MainWindowTr::tr("routing profiles"),
            [minutesOf] { return minutesOf(Configs::dataManager->settingsRepo->route_auto_update); },
            [] { return Configs::dataManager->settingsRepo->route_auto_update_last; },
            [](qint64 t) {
                Configs::dataManager->settingsRepo->route_auto_update_last = t;
                Configs::dataManager->settingsRepo->Save();
            },
            [] { UI_update_all_remote_routes(true); },
        });
        // Not persisted: with the setting on, every launch checks, and then every four hours. Read on every tick, so switching it needs no restart.
        auto lastReleaseCheck = std::make_shared<qint64>(0);
        runner->Add({
            {},
            [] { return ReleaseUpdates::Available() && Configs::dataManager->settingsRepo->auto_update_app ? 240 : 0; },
            [lastReleaseCheck] { return *lastReleaseCheck; },
            [lastReleaseCheck](qint64 t) { *lastReleaseCheck = t; },
            [] { runOnNewThread([] { ReleaseUpdates::instance()->check(false); }); },
        });
    }

    EngineHost *host() {
        return engineHostInstance;
    }
} // namespace Engine
