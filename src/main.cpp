#include <csignal>
#include <cstdio>
#include <memory>

#include <QApplication>
#include <QDeadlineTimer>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QUrl>
#include <QTranslator>
#include <QMessageBox>
#include <QLocalSocket>
#include <QLocalServer>
#include <QThread>
#include <QDateTime>
#include <3rdparty/WinCommander.hpp>

#include "include/global/Configs.hpp"
#include "include/global/Logger.hpp"
#include "include/global/Notice.hpp"

#include "include/engine/EngineHost.h"
#include "include/engine/app/Bootstrap.h"
#include "include/engine/app/ConfigDump.h"
#include "include/engine/app/DataDirLock.h"
#include "include/engine/app/ExitController.h"
#include "include/engine/app/ExitCodes.h"
#include "include/engine/app/InstanceServer.h"
#include "include/ui/app/GuiStartup.h"
#include "include/ui/mainwindow.h"
#include "include/stats/traffic/TrafficStatsManager.hpp"
#include "include/api/RPC.h"

#include "include/sys/windows/MiniDump.h"
#include "include/sys/windows/eventHandler.h"
#include "include/sys/windows/WinVersion.h"
#include <qfontdatabase.h>
#include <windows.h>

void signal_handler(int signum) {
    Q_UNUSED(signum)
    // The running teardown quits on its own; re-entering it from one of its nested loops would deadlock on mu_exit.
    if (Engine::Bootstrap::ExitInProgress()) return;
    if (auto *h = Engine::host()) h->shutdown();
    QCoreApplication::quit();
}

int main(int argc, char* argv[]) {
    Logging::InstallQtMessageHandler();

    // Ahead of the crash handler: its MessageBoxW would hang a displayless run.
    auto launch = Engine::Bootstrap::PreScan(argc, argv);
    if (launch.mode == Engine::Bootstrap::Mode::DumpConfigs) return Engine::RunConfigDump(argc, argv);

    Windows_SetCrashHandler(Windows_WindowStationVisible());

    QApplication::setAttribute(Qt::AA_DontUseNativeDialogs);
    QApplication::setQuitOnLastWindowClosed(false);
    QApplication a(argc, argv);

    SetNoticeSink([](const Notice &n) { ShowPassiveWarning(n.title, n.text); });
    Engine::Bootstrap::SetSoftwareNames();

    GuiStartup::LoadEmojiFont();
    GuiStartup::LoadUiFonts();

    QStringList arguments = QApplication::arguments();
    const Engine::Bootstrap::RelaunchParent relaunchParent(arguments);
    // Must run before the working directory moves below: argument paths may be relative to it.
    const QString launchDeeplink = Deeplink_ExtractFromArgs(arguments);
    const QStringList launchFiles = LaunchFiles_ExtractFromArgs(arguments, QDir::current());

    QDir::setCurrent(QApplication::applicationDirPath());
    if (QFile::exists("updater.old")) {
        QFile::remove("updater.old");
    }

    const auto dataDir = Engine::Bootstrap::ResolveDataDir(arguments, QDir(QCoreApplication::applicationDirPath()));
    const QDir &wd = dataDir.wd;
    if (!Engine::Bootstrap::DataDirIsUsable(dataDir)) {
        const QString text = QObject::tr("Poros keeps all its data in its own folder, but cannot write to:\n%1\n\nMove the Poros folder somewhere you can write to.")
                                 .arg(QDir::toNativeSeparators(dataDir.configDir));
        std::fputs(qPrintable(text + '\n'), stderr);
        if (!arguments.contains("-tray")) QMessageBox::critical(nullptr, "Poros", text);
        return 1;
    }
    Configs::SetBasePath(wd.absolutePath());

    const auto serverName = Engine::Bootstrap::InstanceServerName(wd);
    qDebug() << "server name: " << serverName;
    QStringList launchPayload;
    if (!launchDeeplink.isEmpty()) launchPayload << launchDeeplink;
    for (const auto &file : launchFiles) launchPayload << QUrl::fromLocalFile(file).toString();

    // Before the temp wipe and initDB: the owner of the lock owns the data dir.
    Engine::DataDirLock dataDirLock;
    const auto claim = Engine::Bootstrap::ClaimDataDir(dataDirLock, dataDir, serverName, launchPayload, relaunchParent);
    if (claim.outcome == Engine::Bootstrap::ClaimOutcome::HandedOff) return 0;
    if (claim.outcome == Engine::Bootstrap::ClaimOutcome::HeldByOther) {
        const auto pid = claim.ownerPid;
        // An autostart must not leave a box behind.
        if (!arguments.contains("-tray")) {
            const QString who = pid > 0 ? QObject::tr("Another Poros instance (PID %1)").arg(pid) : QObject::tr("Another Poros instance");
            QMessageBox::warning(nullptr, "Poros", QObject::tr("%1 is using this data directory:\n%2").arg(who, QDir::toNativeSeparators(wd.absolutePath())));
        }
        return 0;
    }

    // Listening before initDB lets a second launch hand off while the database opens; its payload waits in the pending buffers.
    // This user's processes only: what arrives here is opened as a link or a file.
    Engine::InstanceServer server(serverName, QLocalServer::UserAccessOption);
    if (!server.listen()) {
        // Logging::Init has not run yet, so the file log never sees this.
        const QString error = QString("Failed to start the instance server %1: %2").arg(serverName, server.errorString());
        std::fputs(qPrintable(error + '\n'), stderr);
        Engine::Bootstrap::WarnDroppedPayload(launchPayload);
        QMessageBox::critical(nullptr, "Poros", error);
        return 1;
    }
    // Not before this point: until the data dir is ours there is nothing to tear down, and the default action ends the waits above.
    server.setHandlers(GuiStartup::HandleLaunchLine, [](QLocalSocket *) { MW_dialog_message(MwMessage::Raise, {}); });
    server.installQuitCleanup();

    try {
        Engine::Bootstrap::OpenDataDir(dataDir, arguments);
    } catch (const std::exception &e) {
        // A damaged or locked database would otherwise end the process without a word.
        const QString text = QObject::tr("Poros could not open its database:\n%1\n\n%2\n\nMove poros.db out of that folder to start with empty data, or put back a copy you trust.")
                                 .arg(QDir::toNativeSeparators(QDir(dataDir.configDir).filePath(QStringLiteral("poros.db"))), QString::fromUtf8(e.what()));
        std::fputs(qPrintable(text + '\n'), stderr);
        if (!arguments.contains("-tray")) QMessageBox::critical(nullptr, "Poros", text);
        return 1;
    }
    Engine::Bootstrap::PrepareRuntime();

    LogThread = new QThread;
    LogThread->start();

    GuiStartup::SetupIconTheme();

    if (GuiStartup::RelaunchElevatedIfNeeded(server, arguments, launchDeeplink, launchFiles)) return 0;

    if (Configs::dataManager->settingsRepo->start_minimal) Configs::dataManager->settingsRepo->flag_tray = true;

    GuiStartup::LoadTranslator();

    // Must follow the single-instance check: opening the log earlier truncates the running instance's file and fakes a crash marker.
    Engine::Bootstrap::StartLogging(dataDir);
    GuiStartup::InstallSessionEndHooks(a, signal_handler);

    API::defaultClient = new API::Client();

    Engine::EngineHost::create();
    auto *window = UI_InitMainWindow();
    Engine::host()->init(window, window);
    // A launch during the exit waits for the data folder instead of handing its link to an instance that is going away.
    QObject::connect(Engine::host()->exitController(), &Engine::ExitController::shutdownStarted, &a, [&server] { server.close(); });

    Configs::dataManager->RunDeferredMaintenance();

    if (Logging::PreviousSessionCrashed()) {
        MW_show_log(QObject::tr("[Warn] Poros did not shut down cleanly last time. "
                                "Diagnostics were saved to: %1").arg(Logging::LogDir()));
    }

    // The Flush calls replay whatever arrived before the window existed.
    if (!launchDeeplink.isEmpty()) Deeplink_Submit(launchDeeplink);
    Deeplink_FlushPending();
    LaunchFiles_Submit(launchFiles);
    LaunchFiles_FlushPending();

    return QApplication::exec();
}
