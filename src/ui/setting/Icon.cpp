#include "include/ui/setting/Icon.hpp"

#include "include/global/Configs.hpp"

#include <QHash>

namespace {
    QHash<Icon::TrayIconStatus, QIcon> g_trayIcons;

    QString statusName(Icon::TrayIconStatus status) {
        switch (status) {
            case Icon::TrayIconStatus::None: return QStringLiteral("Off");
            case Icon::TrayIconStatus::Connecting: return QStringLiteral("Connecting");
            case Icon::TrayIconStatus::Running: return QStringLiteral("Poros");
            case Icon::TrayIconStatus::SystemProxy: return QStringLiteral("Proxy");
            case Icon::TrayIconStatus::Vpn: return QStringLiteral("Tun");
        }
        MW_show_log("Icon::GetTrayIcon: Unknown status");
        return QStringLiteral("Off");
    }
} // namespace

QIcon Icon::GetTrayIcon(TrayIconStatus status) {
    if (const auto it = g_trayIcons.constFind(status); it != g_trayIcons.cend()) return it.value();

    const QIcon icon(QStringLiteral(":/tray/") + statusName(status) + QStringLiteral(".png"));
    g_trayIcons.insert(status, icon);
    return icon;
}
