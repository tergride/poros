#include "include/ui/utils/ScreenQrScanner.h"

#include <QEventLoop>
#include <QFile>
#include <QGuiApplication>
#include <QScreen>
#include <QTimer>
#include <QUuid>
#include <QWidget>

#include "3rdparty/QrDecoder.h"
#include "include/ui/mainwindow.h"

namespace ScreenQr {
    namespace {
        constexpr int CAPTURE_DELAY_MS = 2500;

        bool IsWayland() {
            return qEnvironmentVariable("XDG_SESSION_TYPE") == "wayland"
                   || qEnvironmentVariable("WAYLAND_DISPLAY").contains("wayland", Qt::CaseInsensitive);
        }
    }

    QPixmap GrabScreen(QScreen *screen, bool &ok) {
        QPixmap p;
        if (screen == nullptr) {
            ok = false;
            return p;
        }
        const QRect geom = screen->geometry();
        return screen->grabWindow(0, geom.x(), geom.y(), geom.width(), geom.height());
    }

    QStringList ScanScreens(QWidget *hideDuringScan, bool &captured) {
        captured = false;

        const bool restore = hideDuringScan != nullptr && hideDuringScan->isVisible();
        if (restore) hideDuringScan->hide();

        // Spin rather than sleep: a blocking wait can stop the hide from painting at all.
        QEventLoop wait;
        QTimer::singleShot(CAPTURE_DELAY_MS, &wait, &QEventLoop::quit);
        wait.exec();

        QList<QPixmap> shots;
        const auto screens = IsWayland() ? QList<QScreen *>{QGuiApplication::primaryScreen()}
                                         : QGuiApplication::screens();
        for (QScreen *screen : screens) {
            bool ok = true;
            QPixmap shot = GrabScreen(screen, ok);
            if (ok && !shot.isNull()) {
                captured = true;
                shots.append(shot);
            }
        }

        if (restore) hideDuringScan->show();

        QStringList payloads;
        for (const auto &shot : shots) {
            for (const auto &text : QrDecoder().decode(shot.toImage())) {
                if (!text.isEmpty() && !payloads.contains(text)) payloads << text;
            }
        }
        return payloads;
    }
} // namespace ScreenQr
