#include "include/ui/mainwindow.h"

#include "include/ui/advanced/AdvancedPage.h"
#include "include/ui/advanced/SettingsSection.h"
#include "NkrVersion.h"

#include <QApplication>
#include <QDesktopServices>
#include <QDir>
#include <QDirIterator>
#include <QFile>
#include <QJsonArray>
#include <QJsonObject>
#include <QMenu>
#include <QMessageBox>
#include <QPushButton>
#include <QThread>

#include "include/api/RPC.h"
#include "include/configs/generate.h"
#include "include/engine/EngineHost.h"
#include "include/engine/ThreadCheck.h"
#include "include/engine/app/ExitController.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/engine/services/AssetUpdates.h"
#include "include/engine/services/ReleaseUpdates.h"
#include "include/engine/supervisor/CoreSupervisor.h"
#include "include/global/Configs.hpp"
#include "include/global/HTTPRequestHelper.hpp"
#include "include/global/LocalNetwork.hpp"
#include "include/global/Logger.hpp"
#include "include/sys/KillSwitch.hpp"
#include "include/sys/SystemProxy.hpp"
#include "include/ui/mainWindow/MainWindowInternal.h"

#include "include/ui/group/dialog_manage_groups.h"
#include "include/ui/setting/dialog_basic_settings.h"
#include "include/ui/setting/dialog_integration.h"
#include "include/ui/setting/dialog_manage_routes.h"
#include "include/ui/setting/dialog_otp_manager.h"
#include "include/ui/setting/dialog_preset_settings.h"
#include "include/ui/setting/dialog_vpn_settings.h"

#include "3rdparty/WinCommander.hpp"
#include "include/sys/windows/WinVersion.h"

void MainWindow::on_menu_basic_settings_triggered() {
    showAdvancedSection(AdvancedPage::Settings);
    m_settingsSection->showDialog(m_basicSettings);
}

void MainWindow::on_menu_manage_groups_triggered() {
    USE_DIALOG(DialogManageGroups)
}

void MainWindow::on_menu_routing_settings_triggered() {
    showAdvancedSection(AdvancedPage::Routing);
}

void MainWindow::updateRuleSets() {
    Engine::AssetUpdates::UpdateRuleSets([](const Engine::AssetUpdates::RuleSetResult &result) {
        if (!result.coreReachable) {
            MessageBoxWarning(tr("Update Rule-Sets"), result.error);
            return;
        }
        QStringList problems = result.failures;
        if (!result.error.isEmpty()) problems.prepend(result.error);
        const auto summary = tr("%n remote rule-set(s) refreshed", nullptr, result.updated);
        if (!problems.isEmpty()) {
            MessageBoxWarning(tr("Update Rule-Sets"), summary + "\n\n" + problems.join("\n"));
        } else {
            MessageBoxInfo(tr("Update Rule-Sets"), summary);
        }
    });
}

void MainWindow::on_menu_vpn_settings_triggered() {
    showAdvancedSection(AdvancedPage::Settings);
    m_settingsSection->showDialog(m_vpnSettings, -1);
}

void MainWindow::on_menu_preset_settings_triggered() {
    showAdvancedSection(AdvancedPage::Settings);
    m_settingsSection->showDialog(m_presetSettings, -1);
}

void MainWindow::on_menu_otp_manager_triggered() {
    showAdvancedSection(AdvancedPage::Tools);
    m_toolTabs->setCurrentIndex(0);
}

void MainWindow::on_menu_integration_settings_triggered() {
    showAdvancedSection(AdvancedPage::Settings);
    m_settingsSection->showDialog(m_hotkeySettings);
}

void MainWindow::saveWindowState() {
    auto* settings = Configs::dataManager->settingsRepo.get();

    settings->mainWindowGeometry = this->saveGeometry().toBase64(QByteArray::Base64Encoding);
    if (!isMaximized()) {
        auto news = QString("%1x%2").arg(size().width()).arg(size().height());
        if (settings->mw_size != news) settings->mw_size = news;
    }
}

void MainWindow::on_commitDataRequest() {
    saveWindowState();
    exitController()->persistSessionState();
}

void MainWindow::on_menu_exit_triggered() {
    // A teardown already running quits on its own; a second request would cut its nested waits short.
    if (Configs::dataManager->settingsRepo->prepare_exit || exitController()->exitInProgress()) return;
    exitController()->setExitInProgress(true);
    // A request made during the teardown below may overwrite exit_reason.
    const auto reason = exit_reason;
    const bool restart = reason == ExitReason::Restart || reason == ExitReason::RestartWithTun ||
                         reason == ExitReason::RestartElevated || reason == ExitReason::RunUpdater;
    const auto program = QApplication::applicationFilePath();
    QStringList arguments;
    if (restart) {
        arguments = Configs::dataManager->settingsRepo->argv;
        if (arguments.length() > 0) {
            arguments.removeFirst();
            arguments.removeAll("-tray");
            arguments.removeAll("-flag_restart_tun_on");
        }
        arguments.erase(std::remove_if(arguments.begin(), arguments.end(), [](const QString &argument) {
            return argument.startsWith(QLatin1String("-flag_restart_profile="));
        }), arguments.end());
        if (reason == ExitReason::RestartWithTun) arguments << "-flag_restart_tun_on";
        if (m_restartProfileId >= 0) arguments << QString("-flag_restart_profile=%1").arg(m_restartProfileId);
    }

    bool relaunched = false;
    if (reason == ExitReason::RestartWithTun || reason == ExitReason::RestartElevated) {
        // Asked before the teardown so a declined UAC prompt leaves this instance running; the new one waits for this pid to exit.
        const auto elevatedArguments = arguments + QStringList{QString("-wait_pid=%1").arg(QCoreApplication::applicationPid())};
        const bool launched = WinCommander::runProcessElevated(program, elevatedArguments, QApplication::applicationDirPath(), 1, false) == 0;
        if (!launched) {
            exitController()->setExitInProgress(false);
            exit_reason = ExitReason::None;
            MW_show_log(tr("Restart as administrator was cancelled, Poros keeps running"));
            return;
        }
        relaunched = true;
    }

    Engine::host()->shutdown();
    if (reason == ExitReason::RunUpdater) {
        QDir::setCurrent(QApplication::applicationDirPath());
        QFile::copy("./updater.exe", "./updater.old");
        QProcess::startDetached("./updater.old", Engine::ReleaseUpdates::instance()->installArguments(arguments));
    } else if (restart && !relaunched) {
        QDir::setCurrent(QApplication::applicationDirPath());
        // Until this process exits it holds the data dir lock and still answers on the instance socket.
        arguments << QString("-wait_pid=%1").arg(QCoreApplication::applicationPid());
        QProcess::startDetached(program, arguments);
    }
    QCoreApplication::quit();
}

void MainWindow::toggle_system_proxy() {
    auto currentState = Configs::dataManager->settingsRepo->spmode_system_proxy;
    if (currentState) {
        connection()->setSystemProxy(false);
    } else {
        connection()->setSystemProxy(true);
    }
}

void MainWindow::toggle_tun() {
    const auto state = connection()->connectionState();
    if (state == ConnectionState::Connecting || state == ConnectionState::Stopping) return;
    connection()->setTun(!Configs::dataManager->settingsRepo->spmode_vpn);
}

bool MainWindow::StopVPNProcess() {
    supervisor()->killForPrivilegeChange();
    return true;
}

void MainWindow::show_startstop_menu() {
    if (!Configs::dataManager->settingsRepo->kill_switch) return;
    QMenu menu(this);
    connect(menu.addAction(tr("Disable Kill Switch")), &QAction::triggered, this, [this] { confirm_disable_kill_switch(); });
    auto *button = ui->toolButton_startstop;
    menu.exec(button->mapToGlobal(QPoint(0, button->height())));
}

void MainWindow::confirm_disable_kill_switch() {
    auto *box = new QMessageBox(QMessageBox::Warning, tr("Disable kill switch"),
                                tr("Turn off the kill switch? Traffic that does not go through Poros will no longer be blocked."),
                                QMessageBox::NoButton, this);
    box->setAttribute(Qt::WA_DeleteOnClose);
    auto *disable = box->addButton(tr("Disable Kill Switch"), QMessageBox::DestructiveRole);
    auto *cancel = box->addButton(QMessageBox::Cancel);
    // Cancel is the default, so a stray Enter cannot complete the two-step disable.
    box->setDefaultButton(cancel);
    box->setEscapeButton(cancel);
    connect(box, &QMessageBox::finished, this, [this, box, disable] {
        if (box->clickedButton() != disable) return;
        connection()->disableKillSwitch();
        refreshEmbeddedPages();
    });
    box->open();
}
