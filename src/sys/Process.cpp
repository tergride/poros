#include "include/sys/Process.hpp"
#include "include/global/Configs.hpp"
#include "include/sys/KillSwitch.hpp"

namespace Configs_sys {
    namespace {
        // CREATE_NEW_PROCESS_GROUP and CREATE_NO_WINDOW from <winbase.h>, which this portable TU does not include.
        constexpr unsigned long kProcessNewProcessGroup = 0x00000200;
        constexpr unsigned long kProcessNoWindow = 0x08000000;
    }

    void IsolateChild(QProcess *process) {
        // Qt adds CREATE_NO_WINDOW only while we have no console: after AttachConsole a Ctrl+C or a console close would
        // reach the core and the guard (which disarms) before our ordered teardown.
        process->setCreateProcessArgumentsModifier([](QProcess::CreateProcessArguments *args) {
            args->flags |= kProcessNewProcessGroup | kProcessNoWindow;
        });
    }

    CoreProcess::~CoreProcess() {
    }

    void CoreProcess::Kill(int waitMs) {
        kill();
        waitForFinished(waitMs);
    }

    CoreProcess::CoreProcess(const QString &core_path, const QString &socketName, bool debugMode)
        : m_socketName(socketName), m_debugMode(debugMode) {
        program = core_path;
    }

    void CoreProcess::Start() {
        if (started) return;
        started = true;
        ++generation;
        failed_to_start = false;

        auto env = QProcessEnvironment::systemEnvironment();
        env.insert("THRONE_CORE_SOCKET", m_socketName);
        // Turns an unrecovered Go panic into a real abort, so all goroutine stacks are dumped and WER captures the core too.
        env.insert("GOTRACEBACK", "crash");
        if (m_debugMode) env.insert("THRONE_CORE_DEBUG", "1");
        // Points Xray's asset loader at our writable config dir, so a geoip.dat/geosite.dat downloaded later is found with no core restart.
        env.insert("XRAY_LOCATION_ASSET", Configs::GetBasePath());
        // Never inherited: only guard_identity decides, or an identity exit would repeat with the flag already handled.
        env.remove("THRONE_GUARD");
        guard_identity = Configs::dataManager->settingsRepo->kill_switch && !Sys::KillSwitch::instance()->identityUnsupported();
        if (guard_identity) env.insert("THRONE_GUARD", "1");
        setProcessEnvironment(env);
        IsolateChild(this);
        start(program, {});
    }

    void CoreProcess::Restart() {
        restarting = true;
        kill();
        waitForFinished(500);
        started = false;
        Start();
        restarting = false;
    }

} // namespace Configs_sys
