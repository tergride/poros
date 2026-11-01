#pragma once

#include <QtGlobal>

namespace Engine {
    enum class ConnectionState {
        Idle,
        Connecting,
        Running,
        Stopping,
    };

    struct StartRequest {
        int profileId = -1;
        // Unattended callers get every failure through the log, never a dialog.
        bool interactive = true;
    };

    struct StopOptions {
        bool crash = false;
        bool block = false;
        bool manual = false;
        bool interactive = true;
    };

    struct ModeChange {
        bool save = true;
        // Unattended callers get no dialog or elevation prompt, and their restart is an unattended StartRequest.
        bool interactive = true;
        bool restart = true;
    };

    // Where the traffic chart marks a run.
    enum class RunMarker {
        Started,
        Switched,
        Stopped,
    };

    // Why the running profile should restart to apply a change; the view translates it.
    enum class RestartReason {
        Settings,
        Routing,
        Profile,
        Group,
    };

    // What needs more rights than the process has.
    enum class PrivilegeReason {
        Tun,
        KillSwitch,
        TlsSpoof,
    };

    // Restart: the front end relaunches elevated, so the attempt ends here. TryWithout: go on and let it fail by itself.
    enum class PrivilegeDecision {
        Restart,
        TryWithout,
        Cancel,
    };

    enum class ExitReason {
        None,
        RunUpdater,
        Restart,
        RestartWithTun,
        RestartElevated,
    };

    // Bytes per second.
    struct LiveRates {
        qint64 proxyUp = 0;
        qint64 proxyDown = 0;
        qint64 directUp = 0;
        qint64 directDown = 0;
    };

    enum class TestKind {
        Latency,
        Ip,
        Speed,
    };
} // namespace Engine
