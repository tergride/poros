#pragma once

#include <QPixmap>
#include <QStringList>

class QScreen;
class QWidget;

namespace ScreenQr {
    QPixmap GrabScreen(QScreen *screen, bool &ok);

    QStringList ScanScreens(QWidget *hideDuringScan, bool &captured);
} // namespace ScreenQr
