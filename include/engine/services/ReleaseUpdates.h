#pragma once

#include <QMutex>
#include <QObject>
#include <QStringList>

#include <atomic>
#include <optional>

namespace Engine {
    // latest.json, which script/release_windows.ps1 publishes next to the archive.
    struct ReleaseFeed {
        QString version;
        QString file;
        qint64 size = 0;
        QString sha256;
        QString notesRu;
        QString notesEn;
    };

    // Nothing from the feed reaches a path or a URL unchecked: nullopt for anything but a well-formed release.
    std::optional<ReleaseFeed> ParseReleaseFeed(const QByteArray &json);

    // Numbers separated by dots; a missing part counts as 0.
    bool IsNewerVersion(const QString &candidate, const QString &current);

    // Updates of Poros itself from the releases of its own repository. A new version is downloaded at once and installed only
    // when the user asks, because the install restarts Poros and drops the connection.
    class ReleaseUpdates final : public QObject {
        Q_OBJECT

    public:
        static ReleaseUpdates *instance();

        // A build without a version is a local one, and without updater.exe nothing can replace the files.
        static bool Available();

        // Blocks on the network: call it off the main thread. Ends with checked().
        void check(bool manual);

        // Reports an install that failed, whether or not automatic checks are on. For the start of the program, once installFailed is connected.
        void reportFailedInstall();

        // Empty until a verified archive waits to be installed.
        QString readyVersion() const;

        // In the interface language, as Markdown.
        QString readyNotes() const;

        // For updater.exe; programArguments go on to the new Poros.
        QStringList installArguments(const QStringList &programArguments) const;

    signals:
        // manual: the user asked and expects an answer. error is empty when the check itself worked.
        void checked(bool manual, const QString &error);

        // The last install did not go through, and this is the old version running again.
        void installFailed(const QString &error);

    private:
        ReleaseUpdates() = default;

        QString run();

        std::atomic<bool> m_checking{false};
        std::atomic<bool> m_installFailed{false};
        mutable QMutex m_mutex;
        std::optional<ReleaseFeed> m_ready;
    };
} // namespace Engine
