#include "include/ui/utils/ConnectionModeController.h"

#include "include/engine/connection/ConnectionManager.h"
#include "include/global/Configs.hpp"

namespace {
    QString connectionModeName(ConnectionModeController::Mode mode) {
        switch (mode) {
            case ConnectionModeController::Mode::Tun: return QStringLiteral("tun");
            case ConnectionModeController::Mode::System: return QStringLiteral("system");
            default: return {};
        }
    }
} // namespace

ConnectionModeController::Mode ConnectionModeController::Resolve(const QString &saved, bool tunOn, bool systemOn) {
    if (saved == QLatin1String("tun")) return Mode::Tun;
    if (saved == QLatin1String("system")) return Mode::System;
    if (tunOn == systemOn) return Mode::Unset;
    return tunOn ? Mode::Tun : Mode::System;
}

ConnectionModeController::ConnectionModeController(Engine::ConnectionManager *connection, QObject *parent)
    : QObject(parent), m_connection(connection) {}

ConnectionModeController::Mode ConnectionModeController::mode() const {
    const auto &settings = Configs::dataManager->settingsRepo;
    return Resolve(settings->simple_conn_mode, settings->spmode_vpn, settings->spmode_system_proxy);
}

ConnectionModeController::Mode ConnectionModeController::liveMode() const {
    if (m_connection->runningSnapshot() == nullptr) return mode();
    const auto &settings = Configs::dataManager->settingsRepo;
    return settings->spmode_vpn ? Mode::Tun : settings->spmode_system_proxy ? Mode::System : Mode::Unset;
}

void ConnectionModeController::save(Mode mode) {
    auto &settings = Configs::dataManager->settingsRepo;
    settings->simple_conn_mode = connectionModeName(mode);
    settings->Save();
    emit modeChanged();
}

void ConnectionModeController::choose(Mode mode) {
    const Mode before = this->mode();
    save(mode);
    if (mode == Mode::Unset || m_connection->connectionState() == Engine::ConnectionState::Idle) return;

    const auto &settings = Configs::dataManager->settingsRepo;
    if (mode == Mode::Tun) {
        m_connection->setTun(true);
        // Refused: stay on what still works, and let the switch show it.
        if (!settings->spmode_vpn) {
            save(before);
            return;
        }
        m_connection->setSystemProxy(false, Engine::ModeChange{.restart = false});
    } else {
        m_connection->setSystemProxy(true, Engine::ModeChange{.restart = false});
        m_connection->setTun(false);
    }
}

bool ConnectionModeController::apply(bool interactive) {
    const auto &settings = Configs::dataManager->settingsRepo;
    const Engine::ModeChange noRestart{.interactive = interactive, .restart = false};
    switch (mode()) {
        case Mode::Tun:
            if (!settings->spmode_vpn) m_connection->setTun(true, noRestart);
            if (!settings->spmode_vpn) return false;
            if (settings->spmode_system_proxy) m_connection->setSystemProxy(false, noRestart);
            return true;
        case Mode::System:
            if (settings->spmode_vpn) m_connection->setTun(false, noRestart);
            if (!settings->spmode_system_proxy) m_connection->setSystemProxy(true, noRestart);
            return true;
        default:
            return false;
    }
}

void ConnectionModeController::adopt() {
    const auto &settings = Configs::dataManager->settingsRepo;
    // The one switch left on is what the user last asked for, whatever was saved before.
    const Mode single = Resolve({}, settings->spmode_vpn, settings->spmode_system_proxy);
    const Mode resolved = single != Mode::Unset ? single : mode();
    if (resolved != Mode::Unset) save(resolved);
    if (resolved == Mode::Unset || m_connection->runningSnapshot() == nullptr) return;
    // Both on: the saved choice decides. Dropping a switch restarts only if the engine says it must.
    if (settings->spmode_vpn && settings->spmode_system_proxy) {
        if (resolved == Mode::Tun) m_connection->setSystemProxy(false, Engine::ModeChange{.restart = false});
        else m_connection->setTun(false);
    } else if (single == Mode::Unset) {
        choose(resolved);
    }
}
