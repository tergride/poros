// What the window shows of Poros's own updates; the work itself is in Engine::ReleaseUpdates.
#include "include/ui/mainwindow.h"

#include <QMessageBox>

#include "include/engine/services/ReleaseUpdates.h"
#include "include/ui/widget/AdminPromptDialog.h"

void MainWindow::setupReleaseUpdates() {
    auto *updates = Engine::ReleaseUpdates::instance();
    connect(updates, &Engine::ReleaseUpdates::checked, this, [this, updates](bool manual, const QString &error) {
        if (!error.isEmpty()) {
            MW_show_log(tr("Update check failed: %1").arg(error));
            if (manual) MessageBoxWarning(tr("Update"), error);
            return;
        }
        const auto version = updates->readyVersion();
        if (version.isEmpty()) {
            if (manual) MessageBoxInfo(tr("Update"), tr("You have the latest version."));
        } else if (manual || version != m_announcedUpdate) {
            // A dismissed banner stays dismissed until the user asks again.
            m_announcedUpdate = version;
            showReleaseUpdate();
        }
    });
    connect(updates, &Engine::ReleaseUpdates::installFailed, this, [this](const QString &error) {
        MW_show_log(tr("The update was not installed: %1").arg(error));
        m_banner->present(tr("The update was not installed, so Poros stays on this version. Details are in the log."), {});
    }, Qt::QueuedConnection);
    updates->reportFailedInstall();
}

void MainWindow::CheckUpdate() {
    Engine::ReleaseUpdates::instance()->check(true);
}

void MainWindow::showReleaseUpdate() {
    auto text = tr("Poros %1 is ready to install. It takes a restart, which drops the connection for a moment.")
                    .arg(Engine::ReleaseUpdates::instance()->readyVersion());
    // The guard stops with Poros: the update replaces PorosCore.exe, which runs it.
    if (Configs::dataManager->settingsRepo->kill_switch) {
        text += u' ' + tr("While Poros restarts, the Kill switch is off and programs can reach the internet without it.");
    }
    m_banner->present(text,
                      {{tr("Restart and update"), [this] {
                            m_restartProfileId = Configs::dataManager->settingsRepo->started_id;
                            exit_reason = ExitReason::RunUpdater;
                            on_menu_exit_triggered();
                        }},
                       {tr("What's new"), [this] {
                            showReleaseNotes();
                            showReleaseUpdate();
                        }}},
                      true);
}

void MainWindow::showReleaseNotes() {
    const auto *updates = Engine::ReleaseUpdates::instance();
    QMessageBox box(QMessageBox::Information, tr("What's new in Poros %1").arg(updates->readyVersion()), updates->readyNotes(), QMessageBox::Close, this);
    box.setTextFormat(Qt::MarkdownText);
    box.exec();
}
