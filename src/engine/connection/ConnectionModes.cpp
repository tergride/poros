#include "include/engine/connection/ConnectionManager.h"

#include "include/api/RPC.h"
#include "include/database/DatabaseManager.h"
#include "include/database/entities/Profile.h"
#include "include/engine/EnginePolicy.h"
#include "include/engine/EngineTr.h"
#include "include/engine/ThreadCheck.h"
#include "include/engine/supervisor/CoreSupervisor.h"
#include "include/global/Configs.hpp"
#include "include/global/LocalNetwork.hpp"
#include "include/global/Notice.hpp"
#include "include/global/Utils.hpp"
#include "include/sys/KillSwitch.hpp"
#include "include/sys/SystemProxy.hpp"

#include <QThread>
#include <QTimer>

#include <algorithm>
#include <utility>

namespace {
    // Profile start and stop call in from their own threads.
    QThread *connectionModesSystemProxyThread() {
        static auto *thread = [] {
            auto *t = new QThread;
            t->start();
            return t;
        }();
        return thread;
    }

}

namespace Engine {
    bool ConnectionManager::ensureTunPrivileges(bool interactive) {
        if (Configs::dataManager->settingsRepo->disable_privilege_req)
        {
            MW_show_log(MainWindowTr::tr("User opted for no privilege req, some features may not work"));
            return true;
        }
        if (tunPrivileged()) return true;
        if (!interactive) return false;
        return policy()->requestCorePrivileges(PrivilegeReason::Tun) == PrivilegeDecision::TryWithout;
    }

    bool ConnectionManager::tunPrivileged() {
        ENGINE_CHECK_MAIN_THREAD();
        return Configs::IsAdmin();
    }

    void ConnectionManager::applySystemProxy(bool enable, bool wait) {
        const auto &settings = Configs::dataManager->settingsRepo;
        const auto host = LocalNetwork::InboundConnectHost();
        const auto port = settings->inbound_socks_port;
        runOnThread([=] {
            QString error;
            if (!enable) {
                error = SystemProxy_Clear();
            } else if (Configs::dataManager->settingsRepo->spmode_system_proxy) {
                // Rechecked: a profile start can queue this after the box was unchecked.
                error = SystemProxy_Apply(host, port);
            }
            if (!error.isEmpty()) MW_show_log(MainWindowTr::tr("System proxy: %1").arg(error));
        }, connectionModesSystemProxyThread(), wait);
    }

    bool ConnectionManager::setSystemProxy(bool enable, const ModeChange &change) {
        ENGINE_CHECK_MAIN_THREAD();
        const auto &settings = Configs::dataManager->settingsRepo;
        settings->spmode_system_proxy = enable;
        bool restarted = false;
        if (const auto running = runningSnapshot()) {
            applySystemProxy(enable);
            if (!enable && settings->reset_proxy_on_disable_sp && change.restart) {
                start(StartRequest{running->id, change.interactive});
                restarted = true;
            }
        }

        if (change.save) {
            settings->remember_system_proxy = enable;
            settings->Save();
        }

        emit statusChanged();
        return restarted;
    }

    bool ConnectionManager::setTun(bool enable, const ModeChange &change) {
        ENGINE_CHECK_MAIN_THREAD();
        const auto &settings = Configs::dataManager->settingsRepo;
        if (enable == settings->spmode_vpn) return false;

        if (enable && !tunPrivileged() && !ensureTunPrivileges(change.interactive)) {
            emit statusChanged();
            return false;
        }

        if (change.save) {
            // Written here, after the elevation check, so a failed enable is not remembered.
            settings->remember_tun = enable;
            settings->Save();
        }

        settings->spmode_vpn = enable;
        emit statusChanged();

        if (!change.restart || settings->started_id < 0) return false;
        start(StartRequest{settings->started_id, change.interactive});
        return true;
    }

    bool ConnectionManager::chooseRoute(int routeId, bool interactive) {
        ENGINE_CHECK_MAIN_THREAD();
        const auto &settings = Configs::dataManager->settingsRepo;
        if (settings->current_route_id == routeId) return false;
        settings->current_route_id = routeId;
        settings->Save();
        if (settings->started_id < 0) return false;
        start(StartRequest{settings->started_id, interactive});
        return true;
    }

    void ConnectionManager::attachKillSwitch() {
        connect(Sys::KillSwitch::instance(), &Sys::KillSwitch::stateChanged, this, [this] { onKillSwitchStateChanged(); });
        QTimer::singleShot(0, this, [this] { onKillSwitchStateChanged(); });
    }

    void ConnectionManager::onKillSwitchStateChanged() {
        const auto state = Sys::KillSwitch::instance()->state();
        const bool failed = state == Sys::KillSwitch::State::Failed;
        const bool enteredFailed = failed && !m_killSwitchWasFailed;
        m_killSwitchWasFailed = failed;

        emit statusChanged();
        if (!failed) policy()->killSwitchProblemResolved();
        if (enteredFailed) policy()->killSwitchProblem();
    }

    void ConnectionManager::disableKillSwitch() {
        Configs::dataManager->settingsRepo->kill_switch = false;
        Configs::dataManager->settingsRepo->Save();
        Sys::KillSwitch::instance()->apply();
    }

    void ConnectionManager::noteRestartNeeded(RestartReason reason) {
        ENGINE_CHECK_MAIN_THREAD();
        if (Configs::dataManager->settingsRepo->started_id < 0 || m_restartReasons.contains(reason)) return;
        m_restartReasons << reason;
        emit restartReasonsChanged();
    }

    void ConnectionManager::clearRestartNeeded() {
        if (m_restartReasons.isEmpty()) return;
        m_restartReasons.clear();
        emit restartReasonsChanged();
    }

    QList<RestartReason> ConnectionManager::restartReasons() const {
        return m_restartReasons;
    }

    bool ConnectionManager::restartPending() const {
        return !m_restartReasons.isEmpty();
    }
} // namespace Engine
