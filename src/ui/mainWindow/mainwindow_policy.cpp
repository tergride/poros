#include "include/ui/mainwindow.h"

#include "include/database/RoutesRepo.h"
#include "include/ui/utils/ConnectionModeController.h"
#include "include/ui/widget/AdminPromptDialog.h"
#include "include/ui/widget/ConnectionChoiceDialog.h"

#include <QMessageBox>
#include <QPushButton>
#include <QScopedValueRollback>

#include "include/engine/app/ExitController.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/sys/KillSwitch.hpp"
#include "include/ui/profile/dialog_vpn_auth.h"

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
    if (m_killSwitchDialog) {
        if (m_killSwitchDialog->property("privilege").toBool() == privilege) {
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
        if (prompt.choice() == AdminPromptDialog::Choice::Restart) restartWithRights(Engine::PrivilegeReason::KillSwitch);
        return;
    }

    auto *box = new QMessageBox(QMessageBox::Warning, tr("Kill switch is not active"), killSwitch->failureText(), QMessageBox::NoButton, this);
    box->setTextFormat(Qt::PlainText);
    box->setAttribute(Qt::WA_DeleteOnClose);
    box->setProperty("privilege", privilege);
    auto *fix = box->addButton(privilege ? tr("Restart as Administrator") : tr("Retry"), QMessageBox::AcceptRole);
    auto *disable = box->addButton(tr("Disable Kill Switch"), QMessageBox::DestructiveRole);
    auto *cancel = box->addButton(QMessageBox::Cancel);
    box->setDefaultButton(fix);
    box->setEscapeButton(cancel);
    m_killSwitchDialog = box;

    // Custom-button result codes changed in Qt 6.5, so only the clicked button pointer is trusted.
    connect(box, &QMessageBox::finished, this, [this, box, fix, disable, privilege] {
        const auto *clicked = box->clickedButton();
        if (m_killSwitchDialog == box) m_killSwitchDialog = nullptr;
        if (clicked == disable) {
            connection()->disableKillSwitch();
            refreshEmbeddedPages();
            return;
        }
        if (clicked != fix) return;
        if (!privilege) {
            Sys::KillSwitch::instance()->apply();
            return;
        }
        exitController()->requestExit(ExitReason::RestartElevated);
    });
    box->open();
}

void MainWindow::killSwitchProblemResolved() {
    if (m_killSwitchDialog) m_killSwitchDialog->close();
}

void MainWindow::startFailed(const StartRequest &, const QString &title, const QString &error) {
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

Engine::SlowOperation MainWindow::slowOperation(Engine::SlowOperationKind kind, int delayMs) {
    const QString text = kind == Engine::SlowOperationKind::Start
                             ? tr("Connecting is taking longer than usual. If nothing changes, restart Poros.")
                             : tr("Disconnecting is taking longer than usual. If nothing changes, restart Poros.");
    auto *timer = new QTimer(this);
    timer->setSingleShot(true);
    connect(timer, &QTimer::timeout, this, [this, text] {
        m_banner->present(text, {{tr("Restart Poros"), [] { MW_dialog_message(MwMessage::RestartProgram, {}); }}});
    });
    timer->start(delayMs);
    return Engine::SlowOperation([this, timer = QPointer<QTimer>(timer), text] {
        if (timer == nullptr) return;
        timer->stop();
        timer->deleteLater();
        // Another message may have taken the banner by now.
        if (m_banner->text() == text) m_banner->dismiss();
    });
}

Engine::PrivilegeDecision MainWindow::requestCorePrivileges(Engine::PrivilegeReason reason, int profileId) {
    using Engine::PrivilegeDecision;
    const QScopedValueRollback pending(m_pendingStart, profileId >= 0 ? profileId : m_pendingStart);
    AdminPromptDialog prompt(reason, this);
    prompt.exec();
    switch (prompt.choice()) {
        case AdminPromptDialog::Choice::TryWithout:
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

    return PrivilegeDecision::Cancel;
}

bool MainWindow::settleMode(const Engine::StartRequest &request) {
    if (!simpleModeShown()) return true;
    using Mode = ConnectionModeController::Mode;
    if (m_modes->mode() == Mode::Unset) {
        if (!request.interactive) {
            MW_show_log(tr("Not connecting: choose TUN or System first."));
            return false;
        }
        ConnectionChoiceDialog dialog(this);
        if (dialog.exec() != QDialog::Accepted) return false;
        m_modes->choose(dialog.choice());
    }
    const QScopedValueRollback pending(m_pendingStart, request.profileId);
    // The rights prompt can switch to System, which then needs no rights at all.
    return m_modes->apply(request.interactive) || (m_modes->mode() == Mode::System && m_modes->apply(request.interactive));
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

void MainWindow::exitProgram(ExitReason reason) {
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
    setModeSwitch(ConnectionModeController::Mode::Tun, false);
    setModeSwitch(ConnectionModeController::Mode::System, true);
}

void MainWindow::setModeSwitch(ConnectionModeController::Mode mode, bool on) {
    using Mode = ConnectionModeController::Mode;
    if (simpleModeShown()) {
        // One choice instead of two switches: the next start would undo a switch it was not told about.
        m_modes->choose(on ? mode : Mode::Unset);
        if (on) return;
    }
    if (mode == Mode::Tun) connection()->setTun(on);
    else connection()->setSystemProxy(on);
}

void MainWindow::showRightsFailure(Engine::PrivilegeReason reason) {
    QString text = tr("TLS spoofing did not work without administrator rights.");
    QList<FailureBanner::Action> actions{{tr("Restart with rights"), [this, reason] {
        // The start that just failed is the one to carry on with.
        const QScopedValueRollback pending(m_pendingStart, connection()->lastProfileId());
        restartWithRights(reason);
    }}};
    if (reason == Engine::PrivilegeReason::Tun) {
        text = tr("TUN did not start without administrator rights.");
        actions.append({tr("Use System"), [this] { useSystemInsteadOfTun(); }});
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
