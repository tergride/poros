#pragma once

#include <QIcon>

namespace Icon {

    enum class TrayIconStatus {
        None,
        Connecting,
        Running,
        SystemProxy,
        Vpn,
    };

    QIcon GetTrayIcon(TrayIconStatus status);
} // namespace Icon
