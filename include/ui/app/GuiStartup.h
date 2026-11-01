#pragma once

#include <QString>
#include <QStringList>

class QApplication;
class QLocalSocket;

namespace Engine {
    class InstanceServer;
}

// The window-only half of startup, called from main() in order.
namespace GuiStartup {

    void LoadEmojiFont();

    void LoadUiFonts();

    void HandleLaunchLine(QLocalSocket *socket, const QString &line);

    void SetupIconTheme();

    // True when an elevated copy was started and this process must return 0.
    bool RelaunchElevatedIfNeeded(Engine::InstanceServer &server, const QStringList &arguments, const QString &launchDeeplink,
                                  const QStringList &launchFiles);

    void LoadTranslator();

    void InstallSessionEndHooks(QApplication &app, void (*onSessionEnd)(int));

} // namespace GuiStartup
