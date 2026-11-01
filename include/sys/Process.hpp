#pragma once

#include <atomic>
#include <memory>
#include <QProcess>

namespace Configs_sys {
    // Before start(): a new process group without a console window, so a console's Ctrl+C or close reaches only Poros, which
    // stops the core and the guard in order.
    void IsolateChild(QProcess *process);

    // The PorosCore process; Engine::CoreSupervisor connects its handlers and owns the restart policy.
    class CoreProcess : public QProcess
    {
    public:
        QString tag;
        QString program;
        QStringList arguments;

        ~CoreProcess();

        // start & kill is one time

        void Start();

        // Waits up to waitMs for the process to exit.
        void Kill(int waitMs = 30000);

        CoreProcess(const QString &core_path, const QString &socketName, bool debugMode);

        void Restart();

        // Bumped by each Start(), so an exit or connection can be matched to the attempt that produced it.
        std::atomic<quint64> generation = 0;

        // DS_cores only; failed_to_start is reset by each Start().
        bool failed_to_start = false;
        bool restarting = false;

    private:
        QString m_socketName;
        bool m_debugMode = false;

    protected:
        bool started = false;
    };
} // namespace Configs_sys
