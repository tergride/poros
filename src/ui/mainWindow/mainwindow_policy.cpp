#include "include/ui/mainwindow.h"

#include "include/database/RoutesRepo.h"
#include "include/ui/utils/ConnectionModeController.h"
#include "include/ui/widget/AdminPromptDialog.h"

#include <QMessageBox>
#include <QPushButton>

#include "include/engine/app/ExitController.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/sys/KillSwitch.hpp"
#include "include/ui/profile/dialog_vpn_auth.h"
#include "include/ui/utils/RestartPrompt.h"

#ifdef Q_OS_LINUX
#include "include/sys/linux/LinuxCap.h"
#endif
#ifdef Q_OS_MACOS
#include "include/sys/macos/MacOS.h"
#endif

void MainWindow::geoAssetMissingCategory(const QString &contextName, const QString &needed, const QString &whichFile) {
    MessageBoxWarning(
        tr("Geo asset missing category"),
        tr("The Xray config \"%1\" needs \"%2\", but the installed %3 does "
           "not contain it.\n\n"
           "Re-downloading from the same source will not fix this — the data "
           "file does not include that category. Set the GeoIP/GeoSite asset "
           "URL in Settings to a source that provides \"%2\", then delete %3 "
           "from the app folder and download it again.")
            .arg(contextName, needed, whichFile));
}

bool MainWindow::confirmGeoAssetDownload(const QString &contextName) {
    return QMessageBox::question(this, tr("Geo asset files required"),
            tr("The Xray config \"%1\" uses geoip/geosite routing rules, but the "
               "required data files (geoip.dat / geosite.dat) are not installed.\n\n"
               "Download them now?").arg(contextName)) == QMessageBox::Yes;
}

void MainWindow::geoAssetDownloadFinished(const QString &error) {
    if (!error.isEmpty()) {
        MessageBoxWarning(tr("Geo asset download failed"), error);
    } else {
        QMessageBox::information(this, tr("Geo assets installed"),
            tr("Geo data files were downloaded successfully.\n\n"
               "Please try again."));
    }
}

void MainWindow::killSwitchProblem() {
    auto *killSwitch = Sys::KillSwitch::instance();
    if (killSwitch->state() != Sys::KillSwitch::State::Failed) return;
    const bool privilege = killSwitch->failedForPrivileges();
    const bool identity = killSwitch->failedForIdentity();
    if (m_killSwitchDialog) {
        if (m_killSwitchDialog->property("privilege").toBool() == privilege && m_killSwitchDialog->property("identity").toBool() == identity) {
            m_killSwitchDialog->setText(killSwitch->failureText());
            return;
        }
        m_killSwitchDialog->close();
    }

    if (privilege && !m_killSwitchPromptOpen) {
        m_killSwitchPromptOpen = true;
        AdminPromptDialog prompt(Engine::PrivilegeReason::KillSwitch, this);
        prompt.exec();
        m_killSwitchPromptOpen = false;
        if (prompt.choice() == AdminPromptDialog::Choice::Restart) {
            restartWithRights(Engine::PrivilegeReason::KillSwitch);
        } else if (prompt.choice() == AdminPromptDialog::Choice::TryWithout) {
            m_triedWithoutRights = Engine::PrivilegeReason::KillSwitch;
            killSwitch->apply();
        }
        return;
    }

    auto *box = new QMessageBox(QMessageBox::Warning, identity ? tr("Kill switch is blocking traffic") : tr("Kill switch is not active"),
                                killSwitch->failureText(), QMessageBox::NoButton, this);
    box->setTextFormat(Qt::PlainText);
    box->setAttribute(Qt::WA_DeleteOnClose);
    box->setProperty("privilege", privilege);
    box->setProperty("identity", identity);
    // The guard still blocks and the core cannot join its group: neither a retry nor more privileges changes that.
    QPushButton *fix = nullptr;
    if (!identity) {
#ifdef Q_OS_WIN
        fix = box->addButton(privilege ? tr("Restart as Administrator") : tr("Retry"), QMessageBox::AcceptRole);
#else
        fix = box->addButton(privilege ? tr("Grant Privileges") : tr("Retry"), QMessageBox::AcceptRole);
#endif
    }
    auto *disable = box->addButton(tr("Disable Kill Switch"), QMessageBox::DestructiveRole);
    auto *cancel = box->addButton(QMessageBox::Cancel);
    box->setDefaultButton(fix != nullptr ? fix : cancel);
    box->setEscapeButton(cancel);
    m_killSwitchDialog = box;

    // Custom-button result codes changed in Qt 6.5, so only the clicked button pointer is trusted.
    connect(box, &QMessageBox::finished, this, [this, box, fix, disable, privilege] {
        const auto *clicked = box->clickedButton();
        if (m_killSwitchDialog == box) m_killSwitchDialog = nullptr;
        if (clicked == disable) {
            connection()->disableKillSwitch();
            return;
        }
        if (fix == nullptr || clicked != fix) return;
        if (!privilege) {
            Sys::KillSwitch::instance()->apply();
            return;
        }
#ifdef Q_OS_WIN
        exitController()->requestExit(ExitReason::RestartElevated, false);
#else
        // Otherwise the core restarts once it is privileged, and onCoreStarted retries.
        if (connection()->ensureTunPrivileges(true)) Sys::KillSwitch::instance()->apply();
#endif
    });
    box->open();
}

void MainWindow::killSwitchProblemResolved() {
    if (m_killSwitchDialog) m_killSwitchDialog->close();
}

void MainWindow::extraCoreNotPermitted() {
    ShowPassiveWarning(tr("Kill switch"),
                       tr("The kill switch could not be updated to allow this profile's extra core, so the profile was not started. See the log for details."));
}

void MainWindow::startFailed(const StartRequest &, StartOutcome, const QString &title, const QString &error) {
    if (error.contains(QLatin1String("initial rule-set"), Qt::CaseInsensitive)) {
        showRuleSetFailure();
        return;
    }
    if (error.contains(QLatin1String("divert"), Qt::CaseInsensitive) && !Configs::KnownAdmin().value_or(false)) {
        showRightsFailure(Engine::PrivilegeReason::TlsSpoof);
        return;
    }
    MessageBoxWarning(title, error);
}

void MainWindow::strictRouteUnavailable(const QString &error) {
    MessageBoxWarning(
        tr("Strict routing unavailable"),
        tr("Windows could not enable strict routing. Open Tun Settings, "
           "disable Strict Route, and start the profile again.\n\n"
           "Disabling Strict Route may cause DNS leaks.\n\nError: %1").arg(error));
}

bool MainWindow::offerCoreReset(const QString &error) {
    // TUN simply cannot open without rights: a reset would not help, and the choice to try was the user's.
    if (!Configs::KnownAdmin().value_or(false)) {
        showRightsFailure(Engine::PrivilegeReason::Tun);
        return false;
    }
    QMessageBox msg(
        QMessageBox::Information,
        tr("Tun device misbehaving"),
        tr("If you have trouble starting VPN, you can force reset Core process here and then try starting the profile again. The error is %1").arg(error),
        QMessageBox::NoButton,
        this
    );
    auto reset = msg.addButton(tr("Reset"), QMessageBox::ActionRole);
    auto cancel = msg.addButton(tr("Cancel"), QMessageBox::ActionRole);

    msg.setDefaultButton(cancel);
    msg.setEscapeButton(cancel);

    msg.exec();
    return msg.clickedButton() == reset;
}

bool MainWindow::offerLeftoverTunAdapterRetry(const QString &error) {
    QMessageBox msg(
        QMessageBox::Information,
        tr("Leftover Tun adapter"),
        tr("Windows was still holding the Tun adapter from an earlier session that did not close cleanly. "
           "Starting the profile again usually works; if it keeps failing, restart Windows.\n\nError: %1").arg(error),
        QMessageBox::NoButton,
        this
    );
    auto startAgain = msg.addButton(tr("Start Again"), QMessageBox::AcceptRole);
    auto cancel = msg.addButton(tr("Cancel"), QMessageBox::RejectRole);

    msg.setDefaultButton(startAgain);
    msg.setEscapeButton(cancel);

    msg.exec();
    return msg.clickedButton() == startAgain;
}

void MainWindow::stopFailed(const QString &error) {
    MessageBoxWarning(tr("Stop return error"), error);
}

Engine::SlowOperation MainWindow::slowOperation(Engine::SlowOperationKind, int delayMs) {
    const QPointer<RestartPrompt> restartPrompt =
        new RestartPrompt(this, tr("If there is no response for a long time, it is recommended to restart the software."), delayMs);
    return Engine::SlowOperation([restartPrompt] { if (restartPrompt) restartPrompt->dismiss(); });
}

Engine::PrivilegeDecision MainWindow::requestCorePrivileges(Engine::PrivilegeReason reason) {
    using Engine::PrivilegeDecision;
#ifdef NKR_ELEVATION_HINT
    MessageBoxWarning(software_name, tr("This installation cannot grant the core privileges by itself.") + "\n\n" + NKR_ELEVATION_HINT);
    return PrivilegeDecision::Cancel;
#endif
#ifdef Q_OS_LINUX
    if (!Linux_HavePkexec()) {
        MessageBoxWarning(software_name, "Please install \"pkexec\" first.");
        return PrivilegeDecision::Cancel;
    }
    auto n = QMessageBox::warning(GetMessageBoxParent(), software_name, tr("Please give the core root privileges"), QMessageBox::Yes | QMessageBox::No);
    if (n == QMessageBox::Yes) {
        runOnNewThread([=,this]
        {
            auto chownArgs = QString("root:root " + Configs::FindCoreRealPath());
            auto ret = Linux_Run_Command("chown", chownArgs);
            if (ret != 0) {
                MW_show_log(QString("Failed to run chown %1 code is %2").arg(chownArgs).arg(ret));
            }
            auto chmodArgs = QString("u+s " + Configs::FindCoreRealPath());
            ret = Linux_Run_Command("chmod", chmodArgs);
            if (ret == 0) {
                StopVPNProcess();
            } else {
                MW_show_log(QString("Failed to run chmod %1").arg(chmodArgs));
            }
        });
        return PrivilegeDecision::Restart;
    }
#endif
#ifdef Q_OS_WIN
    AdminPromptDialog prompt(reason, this);
    prompt.exec();
    switch (prompt.choice()) {
        case AdminPromptDialog::Choice::TryWithout:
            m_triedWithoutRights = reason;
            return PrivilegeDecision::TryWithout;
        case AdminPromptDialog::Choice::UseSystem:
            useSystemInsteadOfTun();
            return PrivilegeDecision::Cancel;
        case AdminPromptDialog::Choice::Restart:
            restartWithRights(reason);
            return PrivilegeDecision::Restart;
        case AdminPromptDialog::Choice::Cancel:
            break;
    }
#endif

#ifdef Q_OS_MACOS
    if (Configs::isSetuidSet(Configs::FindCoreRealPath().toStdString()))
    {
        StopVPNProcess();
        return PrivilegeDecision::TryWithout;
    }
    auto n = QMessageBox::warning(GetMessageBoxParent(), software_name, tr("Please give the core root privileges"), QMessageBox::Yes | QMessageBox::No);
    if (n == QMessageBox::Yes)
    {
        auto Command = QString("sudo chown root:wheel '%1' && sudo chmod u+s '%1'").arg(Configs::FindCoreRealPath());
        auto ret = Mac_Run_Command(Command);
        if (ret == 0) {
            MessageBoxInfo(tr("Requesting permission"), tr("Please Enter your password in the opened terminal, then try again"));
        } else {
            MW_show_log(QString("Failed to run %1 with %2").arg(Command).arg(ret));
        }
    }
#endif
    return PrivilegeDecision::Cancel;
}

void MainWindow::systemProxyNeedsMixedInbound() {
    MessageBoxWarning("Invalid Operation", "Cannot set system proxy when mixed inbound is disabled.");
}

bool MainWindow::vpnPromptOpen() const {
    return m_vpnAuthDialog != nullptr;
}

void MainWindow::vpnChallenge(const VpnAuthChallenge &challenge) {
    auto *dialog = new DialogVpnAuth(this, vpnAuth(), challenge);
    dialog->setAttribute(Qt::WA_DeleteOnClose);
    m_vpnAuthDialog = dialog;
    dialog->show();
    ActivateWindow(dialog);
}

void MainWindow::vpnCredentialsRejected(int, const VpnAuthChallenge &challenge,
                                        const std::function<void(const QString &username, const QString &password)> &retryWith) {
    auto *dialog = new DialogVpnAuth(this, vpnAuth(), challenge, true);
    dialog->setAttribute(Qt::WA_DeleteOnClose);
    m_vpnAuthDialog = dialog;
    connect(dialog, &QDialog::accepted, this, [dialog, retryWith] {
        retryWith(dialog->enteredUsername(), dialog->enteredPassword());
    });
    dialog->show();
    ActivateWindow(dialog);
}

void MainWindow::vpnChallengesCleared() {
    if (m_vpnAuthDialog != nullptr) m_vpnAuthDialog->close();
}

void MainWindow::settingsNeedTunRestart() {
    MessageBoxWarning(tr("Tun Settings changed"), tr("Restart Tun to take effect."));
}

bool MainWindow::confirmRestartProgram() {
    return QMessageBox::warning(GetMessageBoxParent(), tr("Settings changed"), tr("Restart the program to take effect."), QMessageBox::Yes | QMessageBox::No) == QMessageBox::Yes;
}

void MainWindow::testSessionBusy(const QString &text) {
    MessageBoxWarning(software_name, text);
}

void MainWindow::exitProgram(ExitReason reason, bool unattended) {
    if (unattended) {
        request_unattended_exit(reason);
        return;
    }
    exit_reason = reason;
    on_menu_exit_triggered();
}

void MainWindow::restartWithRights(Engine::PrivilegeReason reason) {
    const auto &settings = Configs::dataManager->settingsRepo;
    // Whatever was being started, or what runs now: the new process picks it up by itself.
    m_restartProfileId = m_pendingStart >= 0 ? m_pendingStart : settings->started_id;
    exit_reason = reason == Engine::PrivilegeReason::Tun ? ExitReason::RestartWithTun : ExitReason::RestartElevated;
    on_menu_exit_triggered();
}

void MainWindow::useSystemInsteadOfTun() {
    if (Configs::dataManager->settingsRepo->ui_mode != QLatin1String("advanced")) m_modes->choose(ConnectionModeController::Mode::System);
    else connection()->setSystemProxy(true);
}

void MainWindow::showRightsFailure(Engine::PrivilegeReason reason) {
    QString text;
    QList<FailureBanner::Action> actions{{tr("Restart with rights"), [this, reason] { restartWithRights(reason); }}};
    switch (reason) {
        case Engine::PrivilegeReason::Tun:
            text = tr("TUN did not start without administrator rights.");
            actions.append({tr("Use System"), [this] { useSystemInsteadOfTun(); }});
            break;
        case Engine::PrivilegeReason::KillSwitch:
            text = tr("The kill switch did not turn on without administrator rights.");
            break;
        case Engine::PrivilegeReason::TlsSpoof:
            text = tr("TLS spoofing did not work without administrator rights.");
            break;
    }
    m_banner->present(text, actions);
}

void MainWindow::showRuleSetFailure() {
    QList<FailureBanner::Action> actions;
    for (const auto &route : Configs::dataManager->routesRepo->GetAllRouteProfiles()) {
        if (route->builtinKey != QLatin1String("all")) continue;
        actions.append({tr("Use %1").arg(route->name), [this, id = route->id] {
            connection()->chooseRoute(id);
            refreshRouteChoice();
            refreshSimplePortal();
        }});
    }
    m_banner->present(tr("Could not download the site lists this preset needs. Check the connection, or use a preset that does not need them."), actions);
}
