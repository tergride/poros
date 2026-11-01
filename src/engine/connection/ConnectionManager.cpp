#include "include/engine/connection/ConnectionManager.h"

#include "include/api/RPC.h"
#include "include/configs/common/xrayStreamSetting.h"
#include "include/configs/generate.h"
#include "include/database/DatabaseManager.h"
#include "include/database/GroupsRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/engine/EnginePolicy.h"
#include "include/engine/EngineTr.h"
#include "include/engine/ThreadCheck.h"
#include "include/engine/autoselector/AutoSelectorController.h"
#include "include/engine/services/AssetUpdates.h"
#include "include/engine/services/StatsService.h"
#include "include/engine/supervisor/CoreSupervisor.h"
#include "include/engine/tests/TestRunner.h"
#include "include/engine/vpn/VpnAuthController.h"
#include "include/engine/vpn/VpnEndpoints.h"
#include "include/global/ExitLookup.hpp"
#include "include/global/Logger.hpp"
#include "include/global/RunningProfiles.hpp"
#include "include/global/Utils.hpp"
#include "include/sys/KillSwitch.hpp"

#include <QHostAddress>
#include <QJsonObject>
#include <QMutexLocker>

#include <algorithm>
#include <functional>
#include <memory>
#include <utility>

namespace Engine {
    ConnectionManager::ConnectionManager(EnginePolicy *policy, CoreSupervisor *coreSupervisor, StatsService *stats, QObject *parent)
        : QObject(parent), m_policy(policy), m_coreSupervisor(coreSupervisor), m_stats(stats) {
        connect(m_coreSupervisor, &CoreSupervisor::coreStarted, this, &ConnectionManager::onCoreStarted);
        connect(m_coreSupervisor, &CoreSupervisor::coreGaveUp, this, &ConnectionManager::onCoreGaveUp);
        m_lastProfileId = Configs::dataManager->settingsRepo->remember_id;
    }

    void ConnectionManager::attach(TestRunner *tests, AutoSelectorController *autoSelector, VpnAuthController *vpnAuth) {
        m_tests = tests;
        m_autoSelector = autoSelector;
        m_vpnAuth = vpnAuth;
    }

    void ConnectionManager::start(const StartRequest &request) {
        ENGINE_CHECK_MAIN_THREAD();
        if (Configs::dataManager->settingsRepo->prepare_exit) return;

        std::shared_ptr<Configs::Profile> ent = nullptr;
        if (request.profileId >= 0) {
            ent = Configs::dataManager->profilesRepo->GetProfile(request.profileId);
        }
        if (ent == nullptr) return;

        m_lastProfileId = ent->id;

        const auto group = Configs::dataManager->groupsRepo->GetGroup(ent->gid);
        if (group == nullptr || group->archive) return;

        StartRequest resolved = request;
        resolved.profileId = ent->id;

        auto *killSwitch = Sys::KillSwitch::instance();
        if (killSwitch->state() == Sys::KillSwitch::State::Arming) {
            // Only the latest request survives, so repeated clicks while arming start one profile.
            const bool queued = m_killSwitchDeferredStart.profileId >= 0;
            m_killSwitchDeferredStart = resolved;
            if (!queued) {
                killSwitch->whenSettled(this, [this](bool) {
                    if (const auto next = std::exchange(m_killSwitchDeferredStart, StartRequest{}); next.profileId >= 0) start(next);
                });
            }
            return;
        }
        if (!killSwitch->allowsStart()) {
            MW_show_log(MainWindowTr::tr("[Kill switch] Not starting %1: the kill switch is not active.").arg(ent->outbound->DisplayTypeAndName()));
            if (request.interactive) policy()->killSwitchProblem();
            return;
        }

        const int serial = ++m_startSerial;
        setStartPending(false);
        // It may ask the user, and a nested loop can bring a newer start or a stop.
        if (!policy()->settleMode(resolved) || serial != m_startSerial) return;

        // Ranking must run before the config is built and it blocks, so hop off the UI thread.
        const bool ranking = m_autoSelector->rankBeforeStart(ent, resolved.interactive, [=, this] {
            if (serial != m_startSerial) return;
            setStartPending(false);
            start(resolved);
        });
        if (ranking) {
            setStartPending(true);
            return;
        }

        if (resolved.interactive && !tunPrivileged() && ent->outbound->HasTLS() && ent->outbound->GetTLS()->enabled &&
            ent->outbound->GetTLS()->SpoofEffectivelyOn() && policy()->requestCorePrivileges(PrivilegeReason::TlsSpoof, ent->id) != PrivilegeDecision::TryWithout) {
            return;
        }

        const auto result = Configs::BuildSingBoxConfig(ent);
        if (!result->error.isEmpty()) {
            failStart(resolved, MainWindowTr::tr("BuildConfig return error"), result->error);
            return;
        }
        const auto logUnattended = [=](const QString &error) {
            if (!resolved.interactive) MW_show_log(error);
        };
        auto profile_start_stage2 = [=, this](const Engine::SlowOperation &slowStart) {
            libcore::LoadConfigReq req;
            req.core_config = QJsonObject2QString(result->coreConfig, true).toStdString();
            req.tun_ipv4_cidr = result->tunIPv4CIDR.toStdString();
            req.disable_stats = Configs::dataManager->settingsRepo->disable_traffic_stats;
            req.xray_config = QJsonObject2QString(result->xrayConfig, true).toStdString();
            req.need_xray = !result->xrayConfig.isEmpty();
            for (const auto &full : result->xrayFullConfigs) req.xray_full_configs.push_back(full.toStdString());
            if (req.need_xray || !req.xray_full_configs.empty()) {
                req.xray_outbound_dns_strategy = Configs::getXrayOutboundDomainStrategy().toStdString();
                if (auto selector = ent->AutoSelector(); selector != nullptr) {
                    // The idle window must outlast the probe interval or the sidecar restarts every round.
                    req.xray_lazy_start = true;
                    req.xray_idle_seconds = std::max(120, selector->intervalSec * 2);
                    // 0 = resident.
                    req.xray_full_idle_seconds = 0;
                }
            }
            bool rpcOK;
            const QString error = API::defaultClient->Start(&rpcOK, req);
            // Queued ahead of every dialog below, so none of them can be shown over the prompt.
            runOnUiThread([slowStart] { slowStart.dismiss(); });
            if (!rpcOK) return false;
            if (!error.isEmpty()) {
                if (Engine::AssetUpdates::HandleXrayGeoError(policy(), error, ent->outbound->DisplayTypeAndName(), resolved.interactive)) {
                    logUnattended(error);
                    return false;
                }
                if (error.contains("Fwpm", Qt::CaseInsensitive)) {
                    logUnattended(error);
                    if (!resolved.interactive) return false;
                    runOnUiThread([=, this] { policy()->strictRouteUnavailable(error); });
                    return false;
                }
                if (error.contains("configure tun interface")) {
                    logUnattended(error);
                    if (!resolved.interactive) return false;
                    if (error.contains("open existing adapter")) {
                        runOnUiThread([=, this] {
                            if (!policy()->offerLeftoverTunAdapterRetry(error)) return;
                            if (Configs::dataManager->settingsRepo->prepare_exit || connectionState() != ConnectionState::Idle) return;
                            start(StartRequest{ent->id});
                        });
                        return false;
                    }
                    runOnUiThread([=, this] {
                        if (policy()->offerCoreReset(error)) m_coreSupervisor->resetCore();
                    });
                    return false;
                }
                logUnattended(error);
                if (resolved.interactive) {
                    runOnUiThread([=, this] { policy()->startFailed(resolved, "LoadConfig return error", error); });
                }
                return false;
            }
            m_stats->beginRun(result->chainGroups);
            m_autoSelector->noteBuilt(ent, result->autoSelectors);

            Configs::dataManager->settingsRepo->UpdateStartedId(ent->id);
            // Must land after the stop this start may have run first: that stop clears the map.
            Stats::SetVpnEndpointProfiles(result->vpnEndpointProfiles);
            m_stats->setOutboundNames(result->outboundNames);
            Configs::SetRunningProfiles(result->involvedProfiles);
            setRunning(ent, !result->tunIPv4CIDR.isEmpty());
            if (Configs::dataManager->settingsRepo->spmode_system_proxy) applySystemProxy(true);

            const bool exitIsEndpoint = Stats::VpnExitEndpoint(ent) != nullptr;

            runOnUiThread([=, this] {
                // The profile object outlives the run, so an earlier run's answer would linger, for good with the lookup off.
                ent->runningCountryInfo.clear();
                clearRestartNeeded();
                m_vpnAuth->noteRunStarted();
                emit statusChanged();
                emit profileRowsChanged({ent->id});

                // "Only route advertised network" rejects this probe.
                if (exitIsEndpoint) return;

                if (Configs::dataManager->settingsRepo->exit_country_lookup) lookupExitCountry(ent);
            });

            return true;
        };

        if (!mu_starting.tryLock()) {
            failStart(resolved, QStringLiteral("Poros"), MainWindowTr::tr("Another profile is starting..."));
            return;
        }
        if (!mu_stopping.tryLock()) {
            mu_starting.unlock();
            failStart(resolved, QStringLiteral("Poros"), MainWindowTr::tr("Another profile is stopping..."));
            return;
        }
        mu_stopping.unlock();

        if (!Configs::dataManager->settingsRepo->core_running) {
            if (!m_coreSupervisor->canStartCore()) {
                mu_starting.unlock();
                failStart(resolved, QStringLiteral("Poros"), m_coreSupervisor->lastFailure());
                return;
            }
            deferStartToCore(resolved);
            MW_show_log(MainWindowTr::tr("Try to start the config, but the core has not listened to the RPC port, so restart it..."));
            m_coreSupervisor->restartNotListening(ent->id);
            mu_starting.unlock();
            return;
        }

        // Wintun spends 15 s on a leftover adapter before failing; a restart from the prompt inside that window strands a new one.
        const int slowStartMs = !result->tunIPv4CIDR.isEmpty() ? 20000 : 10000;
        const Engine::SlowOperation slowStart = resolved.interactive
            ? policy()->slowOperation(Engine::SlowOperationKind::Start, slowStartMs)
            : Engine::SlowOperation{};

        runOnUiThread([this] {
            m_profileConnecting = true;
            emit busyChanged();
        });

        runOnNewThread([=, this] {
            const bool switching = runningSnapshot() != nullptr;
            if (switching) {
                m_switchingProfile = true;
                stop({false, false, true, resolved.interactive});
                mu_stopping.lock();
                mu_stopping.unlock();
                m_switchingProfile = false;
            }
            MW_show_log(">>>>>>>> " + MainWindowTr::tr("Starting profile %1").arg(ent->outbound->DisplayTypeAndName()));
            const bool started = profile_start_stage2(slowStart);
            const auto markerAt = MonotonicMs();
            if (!started) {
                MW_show_log("<<<<<<<< " + MainWindowTr::tr("Failed to start profile %1").arg(ent->outbound->DisplayTypeAndName()));
            }
            mu_starting.unlock();
            runOnUiThread([=, this] {
                slowStart.dismiss();
                m_profileConnecting = false;
                emit busyChanged();
                if (started) {
                    emit runMarker(markerAt, switching ? RunMarker::Switched : RunMarker::Started,
                                   ent->outbound ? ent->outbound->DisplayName() : ent->name);
                } else if (switching) {
                    emit runMarker(markerAt, RunMarker::Stopped, {});
                }
                const bool restart = std::exchange(m_restartWhenSettled, false);
                if (std::exchange(m_stopWhenSettled, false)) {
                    if (started) stop({false, false, true, resolved.interactive});
                } else if (started && restart) {
                    start(StartRequest{ent->id, resolved.interactive});
                }
            });
        });
    }

    void ConnectionManager::stop(const StopOptions &options) {
        const bool crash = options.crash;
        const bool block = options.block;
        const bool manual = options.manual;
        const bool interactive = options.interactive;

        // The stop inside a profile switch is the start's own.
        if (manual && !crash && !m_switchingProfile) runOnUiThread([this] { cancelPendingStart(); });

        const auto current = runningSnapshot();
        if (current == nullptr) {
            // A crashed profile waits for the restarted core with nothing running; a user stop cancels that.
            if (manual && !crash) runOnUiThread([this] { m_coreSupervisor->cancelResume(); });
            return;
        }
        const auto id = current->id;

        auto profile_stop_stage2 = [=,this](const Engine::SlowOperation &slowStop) {
            if (m_tests->isTestingCurrent()) {
                bool ok;
                API::defaultClient->StopTests(&ok);
                if (!ok) MW_show_log("Failed to stop profile tests!");
            }
            if (!crash) {
                bool rpcOK;
                const QString error = API::defaultClient->Stop(&rpcOK);
                runOnUiThread([slowStop] { slowStop.dismiss(); });
                if (rpcOK && !error.isEmpty()) {
                    if (interactive) {
                        runOnUiThread([=,this] { policy()->stopFailed(error); });
                    } else {
                        MW_show_log(MainWindowTr::tr("Stop return error") + ": " + error);
                    }
                    return false;
                } else if (!rpcOK) {
                    return false;
                }
            }
            if (Configs::dataManager->settingsRepo->spmode_system_proxy) applySystemProxy(false);
            return true;
        };

        if (!mu_stopping.tryLock()) {
            return;
        }

        // stop() is reached from a worker thread as well as the UI one.
        m_stats->clearConnectionList();

        runOnUiThread([this] {
            m_profileDisconnecting = true;
            emit busyChanged();
        });

        m_autoSelector->noteStopped();

        runOnNewThread([=, this] {
            m_stats->endRun();

            // runOnUiThread is a no-op before qApp exists, so the teardown must not chase this.
            Engine::SlowOperation slowStop;
            if (interactive) {
                runOnUiThread([this, &slowStop] {
                    slowStop = policy()->slowOperation(Engine::SlowOperationKind::Stop, 5000);
                }, true);
            }

            // Snapshot: `running` is cleared below and a racing start can reassign it.
            const auto stopping = runningSnapshot();
            if (stopping != nullptr) {
                MW_show_log(">>>>>>>> " + MainWindowTr::tr("Stopping profile %1").arg(stopping->outbound->DisplayTypeAndName()));
            }
            if (!profile_stop_stage2(slowStop)) {
                MW_show_log("<<<<<<<< " + MainWindowTr::tr("Failed to stop, please restart the program."));
            }

            if (manual) Configs::dataManager->settingsRepo->UpdateStartedId(Configs::NoProfileId);
            // Cleared here too: a restart prompt armed before the UI-thread teardown would have no id to start.
            Configs::ClearRunningProfiles();
            m_stats->setOutboundNames({});
            setRunning(nullptr);
            const auto stoppedAt = MonotonicMs();

            runOnUiThread([=, this, &slowStop] {
                slowStop.dismiss();
                if (!m_switchingProfile) emit runMarker(stoppedAt, RunMarker::Stopped, {});

                m_profileDisconnecting = false;
                if (!m_switchingProfile) m_restartWhenSettled = false;
                clearRestartNeeded();
                m_vpnAuth->noteRunStopped(id, manual);
                emit statusChanged();
                emit profileRowsChanged({id});

                mu_stopping.unlock();
            }, true);
        }, block);
    }

    bool ConnectionManager::restartStartedProfile(bool interactive) {
        const int startedId = Configs::dataManager->settingsRepo->started_id;
        if (startedId < 0) return false;
        start(StartRequest{startedId, interactive});
        return true;
    }

    void ConnectionManager::applyPendingRestart() {
        const int startedId = Configs::dataManager->settingsRepo->started_id;
        clearRestartNeeded();
        if (startedId >= 0) start(StartRequest{startedId});
    }

    void ConnectionManager::failStart(const StartRequest &request, const QString &title, const QString &error) {
        if (request.interactive) {
            policy()->startFailed(request, title, error);
        } else {
            MW_show_log(title + ": " + error);
        }
    }

    void ConnectionManager::cancelPendingStart() {
        ++m_startSerial;
        m_killSwitchDeferredStart = {};
        setStartPending(false);
        m_restartWhenSettled = false;
        m_stopWhenSettled = m_profileConnecting;
    }

    void ConnectionManager::setStartPending(bool pending) {
        if (m_startPending == pending) return;
        m_startPending = pending;
        emit busyChanged();
    }

    void ConnectionManager::deferStartToCore(const StartRequest &request) {
        m_coreStartRequest = request;
    }

    void ConnectionManager::onCoreStarted(int profileId) {
        auto &settings = Configs::dataManager->settingsRepo;
        // A relaunch carries the session on, so it brings back what remember_enable would. The exit saved it just before.
        const bool resume = settings->remember_enable || std::exchange(settings->flag_restart_profile, -1) >= 0;
        if (resume && settings->remember_system_proxy) {
            setSystemProxy(true, ModeChange{.save = false});
        }
        if ((resume && settings->remember_tun) || settings->flag_restart_tun_on) {
            setTun(true, ModeChange{.save = settings->flag_restart_tun_on});
            settings->flag_restart_tun_on = false;
        }
        const auto request = std::exchange(m_coreStartRequest, StartRequest{});
        if (profileId >= 0) start(request.profileId == profileId ? request : StartRequest{profileId});
        emit statusChanged();
    }

    void ConnectionManager::onCoreGaveUp() {
        m_coreStartRequest = {};
    }

    void ConnectionManager::lookupExitCountry(const std::shared_ptr<Configs::Profile> &ent) {
        ENGINE_CHECK_MAIN_THREAD();
        const QString provider = Configs::dataManager->settingsRepo->exit_lookup_provider;
        // Off the UI thread: HttpGet spins a nested event loop.
        runOnNewThread([=, this] {
            QString error;
            const auto info = Configs_network::LookupExit(provider, &error);
            if (!info.hasCountry()) {
                LOG_DEBUG(QStringLiteral("[ExitLookup] no exit country: %1").arg(error));
                return;
            }
            const QString text = Configs_network::ExitLocationText(info);
            runOnUiThread([=, this] {
                if (runningSnapshot() != ent) return;
                ent->runningCountryInfo = text;
                emit statusChanged();
            });
        });
    }

    int ConnectionManager::resolveLastProfile() const {
        const auto &settings = Configs::dataManager->settingsRepo;
        for (const int id : {settings->started_id, m_lastProfileId, settings->remember_id}) {
            if (id >= 0 && Configs::dataManager->profilesRepo->GetProfile(id) != nullptr) return id;
        }
        return -1;
    }

    int ConnectionManager::lastProfileId() const {
        return m_lastProfileId;
    }

    ConnectionState ConnectionManager::connectionState() const {
        if (m_profileConnecting || m_startPending) return ConnectionState::Connecting;
        if (m_profileDisconnecting) return ConnectionState::Stopping;
        return runningSnapshot() != nullptr ? ConnectionState::Running : ConnectionState::Idle;
    }

    std::shared_ptr<Configs::Profile> ConnectionManager::runningSnapshot() const {
        QMutexLocker locker(&m_runningMutex);
        return m_running;
    }

    void ConnectionManager::setRunning(std::shared_ptr<Configs::Profile> profile, bool tun) {
        QMutexLocker locker(&m_runningMutex);
        m_running.swap(profile);
        m_runningTun = m_running != nullptr && tun;
    }

    bool ConnectionManager::runningTun() const {
        QMutexLocker locker(&m_runningMutex);
        return m_runningTun;
    }

} // namespace Engine
