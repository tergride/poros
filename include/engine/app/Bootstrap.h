#pragma once

#include <functional>

#include <QDir>
#include <QString>
#include <QStringList>

namespace Engine {
    class DataDirLock;
}

namespace Engine::Bootstrap {
    enum class Mode {
        Gui,
        DumpConfigs,
    };

    struct Launch {
        Mode mode = Mode::Gui;
    };

    // Runs before any Qt object exists, so it only looks at raw argv.
    Launch PreScan(int argc, char **argv);

    // software_name and software_core_name; before anything titles a dialog or notice.
    void SetSoftwareNames();

    struct DataDir {
        QDir wd;
        bool useAppdata = false;
        // As given after -appdata; empty when the flag had no value.
        QString appdataDir;
        QString configDir;
    };

    // Sets the application name first, since AppConfigLocation depends on it. Creates wd and wd/config; a relative -appdata resolves against relativeBase.
    DataDir ResolveDataDir(const QStringList &arguments, const QDir &relativeBase);

    // Whether <config> can be written; there is no fallback outside the program folder.
    bool DataDirIsUsable(const DataDir &dataDir);

    QString InstanceServerName(const QDir &wd);

    // The instance that relaunched us (-wait_pid=); it keeps the data dir lock and the socket until it exits.
    class RelaunchParent {
    public:
        // Stripped so the flag never lands in the saved argv; waits up to 30 s for the exit.
        explicit RelaunchParent(QStringList &arguments);
        ~RelaunchParent();

        RelaunchParent(const RelaunchParent &) = delete;
        RelaunchParent &operator=(const RelaunchParent &) = delete;

        [[nodiscard]] bool relaunched() const { return pid > 0; }

        // Once it is gone, whoever holds the lock is another instance, so the normal hand-off applies again.
        [[nodiscard]] bool gone() const;

        qint64 pid = 0;

    private:
        void *handle = nullptr;
    };

    void WarnDroppedPayload(const QStringList &payload);

    enum class ClaimOutcome {
        Owned,
        HandedOff,
        HeldByOther,
    };

    struct Claim {
        ClaimOutcome outcome = ClaimOutcome::Owned;
        qint64 ownerPid = 0;
    };

    // Owned also covers a lock that could not be taken at all: that run continues unlocked.
    Claim ClaimDataDir(DataDirLock &lock, const DataDir &dataDir, const QString &serverName, const QStringList &payload,
                       const RelaunchParent &relaunchParent);

    // CWD to <config>, temp wipe, start epoch, initDB, log level, stats DB, launch flags.
    void OpenDataDir(const DataDir &dataDir, const QStringList &arguments);

    // The DS_cores thread the core process lives on.
    void PrepareRuntime();

    // Must follow the lock: Init truncates the log and writes the crash marker.
    void StartLogging(const DataDir &dataDir);

    // False before initDB.
    bool ExitInProgress();
} // namespace Engine::Bootstrap
