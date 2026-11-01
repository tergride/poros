#include "include/ui/app/GuiStartup.h"

#include <QApplication>
#include <QDeadlineTimer>
#include <QDir>
#include <QFileInfo>
#include <QFontDatabase>
#include <QIcon>
#include <QLocale>
#include <QLocalSocket>
#include <QTranslator>
#include <QUrl>

#include "include/engine/app/InstanceServer.h"
#include "include/engine/app/StartupElevation.h"
#include "include/global/Configs.hpp"
#include "include/global/Logger.hpp"
#include "include/global/Notice.hpp"

#include <3rdparty/WinCommander.hpp>
#include "include/sys/windows/eventHandler.h"
#include "include/sys/windows/WinVersion.h"

namespace {

    QTranslator *guiStartupTranslator = nullptr;

    void guiStartupLoadTranslate(const QString &locale) {
        QT_TRANSLATE_NOOP("QPlatformTheme", "Cancel");
        QT_TRANSLATE_NOOP("QPlatformTheme", "Apply");
        QT_TRANSLATE_NOOP("QPlatformTheme", "Yes");
        QT_TRANSLATE_NOOP("QPlatformTheme", "&Yes");
        QT_TRANSLATE_NOOP("QPlatformTheme", "&No");
        QT_TRANSLATE_NOOP("QPlatformTheme", "No");
        QT_TRANSLATE_NOOP("QPlatformTheme", "OK");
        QT_TRANSLATE_NOOP("QPlatformTheme", "Close");
        QT_TRANSLATE_NOOP("QPlatformTheme", "Save");
        QT_TRANSLATE_NOOP("QPlatformTheme", "Discard");
        QT_TRANSLATE_NOOP("QPlatformTheme", "Reset");
        QT_TRANSLATE_NOOP("QPlatformTheme", "Restore Defaults");
        if (guiStartupTranslator != nullptr) {
            guiStartupTranslator->deleteLater();
        }
        guiStartupTranslator = new QTranslator;
        QLocale::setDefault(QLocale(locale));
        const QString diskPath = QCoreApplication::applicationDirPath()+"/translations/" + locale + ".qm";
        const QString qrcPath = ":/translations/" + locale + ".qm";
        bool loadOK=false;
        if (QFileInfo::exists(diskPath)) {
            loadOK = guiStartupTranslator->load(diskPath);
        }
        if (!loadOK) {
            loadOK = guiStartupTranslator->load(qrcPath);
        }
        if (loadOK) {
            QCoreApplication::installTranslator(guiStartupTranslator);
        }
    }

    // Flags carry over; the deeplink and files go last as absolute paths, since the working directory has moved since launch.
    QStringList ElevatedRelaunchArguments(const QStringList &arguments, const QString &deeplink, const QStringList &files) {
        QStringList out;
        for (qsizetype i = 1; i < arguments.size(); i++) {
            const auto &arg = arguments.at(i);
            if (!arg.startsWith('-')) continue;
            out << arg;
            if (arg == "-appdata" && i + 1 < arguments.size() && !arguments.at(i + 1).startsWith('-')) out << arguments.at(++i);
        }
        out << QString("-wait_pid=%1").arg(QCoreApplication::applicationPid());
        if (!deeplink.isEmpty()) out << deeplink;
        return out + files;
    }
} // namespace

namespace GuiStartup {
    void LoadEmojiFont() {
        int fontId = QFontDatabase::addApplicationFont(WinVersion::IsBuildNumGreaterOrEqual(BuildNumber::Windows_11_22H2) ? ":/font/notoEmoji" : ":/font/Twemoji");
        if (fontId >= 0)
        {
            QStringList fontFamilies = QFontDatabase::applicationFontFamilies(fontId);
            QFontDatabase::setApplicationEmojiFontFamilies(fontFamilies);
        } else
        {
            qDebug() << "could not load emoji font!";
        }
    }

    void LoadUiFonts() {
        for (const auto &font : {QStringLiteral(":/font/Onest"), QStringLiteral(":/font/Unbounded"), QStringLiteral(":/font/JetBrainsMono")}) {
            if (QFontDatabase::addApplicationFont(font) < 0) qDebug() << "could not load font" << font;
        }
    }

    void HandleLaunchLine([[maybe_unused]] QLocalSocket *socket, const QString &line) {
        if (IsAppDeeplink(line)) {
            Deeplink_Submit(line);
        } else if (line.startsWith("file://")) {
            LaunchFiles_Submit({QUrl(line).toLocalFile()});
        }
    }

    void SetupIconTheme() {
        QIcon::setFallbackSearchPaths(QStringList{
            ":/icon",
        });

        if (QIcon::themeName().isEmpty()) {
            QIcon::setThemeName("breeze");
        }
    }

    bool RelaunchElevatedIfNeeded(Engine::InstanceServer &server, const QStringList &arguments, const QString &launchDeeplink,
                                  const QStringList &launchFiles) {
        const auto &settings = Configs::dataManager->settingsRepo;
        // Read first: IsAdmin() overwrites windows_set_admin with the state of this process.
        const bool needed = Engine::NeedsElevationAtStartup({settings->windows_set_admin, settings->disable_run_admin, settings->remember_enable,
                                                             settings->remember_tun, settings->kill_switch});
        if (needed && !Configs::IsAdmin())
        {
            Configs::dataManager->settingsRepo->windows_set_admin = false; // so that if permission denied, we will run as user on the next run
            Configs::dataManager->settingsRepo->Save();
            // Hand-offs already accepted are read into the pending buffers first; closing the server would drop them.
            const QDeadlineTimer pump(200);
            while (!pump.hasExpired()) QCoreApplication::processEvents(QEventLoop::AllEvents, 20);
            // Not listening during the UAC prompt, so a new launch waits for the elevated child; a declined prompt carries on as user.
            server.close();
            const QString pendingDeeplink = Deeplink_TakePending();
            const QStringList pendingFiles = LaunchFiles_TakePending();
            if (!launchDeeplink.isEmpty() && !pendingDeeplink.isEmpty()) LOG_WARN("dropped the handed-off deeplink: " + pendingDeeplink);
            const auto elevatedArguments = ElevatedRelaunchArguments(arguments, launchDeeplink.isEmpty() ? pendingDeeplink : launchDeeplink,
                                                                     launchFiles + pendingFiles);
            if (WinCommander::runProcessElevated(QApplication::applicationFilePath(), elevatedArguments, "", 1, false) == 0) return true;
            if (!server.listen()) LOG_WARN("could not restart the instance server: " + server.errorString());
            Deeplink_Submit(pendingDeeplink);
            LaunchFiles_Submit(pendingFiles);
        }
        return false;
    }

    void LoadTranslator() {
        QString locale;
#ifdef POROS_UI_TOUR
        if (qEnvironmentVariableIsSet("POROS_TOUR_LOCALE")) {
            guiStartupLoadTranslate(qEnvironmentVariable("POROS_TOUR_LOCALE"));
            return;
        }
#endif
        switch (Configs::dataManager->settingsRepo->language) {
            case 1: // English
                break;
            case 4:
                locale = "ru_RU";
                break;
            default:
                // Any Russian locale gets the one translation; another language has no file and stays in English, with its own formats.
                locale = QLocale().language() == QLocale::Russian ? QStringLiteral("ru_RU") : QLocale().name();
        }
        QGuiApplication::tr("QT_LAYOUT_DIRECTION");
        guiStartupLoadTranslate(locale);
    }

    void InstallSessionEndHooks([[maybe_unused]] QApplication &app, [[maybe_unused]] void (*onSessionEnd)(int)) {
        auto eventFilter = new PowerOffTaskkillFilter(onSessionEnd);
        app.installNativeEventFilter(eventFilter);

    }

} // namespace GuiStartup
