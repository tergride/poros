#pragma once

#include <QColor>
#include <QFont>
#include <QIcon>
#include <QLinearGradient>
#include <QMap>
#include <QRectF>
#include <QString>

// The one source of the Poros colours: poros.qss refers to them as @name, painted widgets through these constants.
namespace PorosTheme {
    inline constexpr QRgb Blue = qRgb(0x27, 0x90, 0xFF);
    inline constexpr QRgb Violet = qRgb(0x7B, 0x61, 0xFF);
    inline constexpr QRgb Orchid = qRgb(0xC4, 0x6B, 0xFF);
    inline constexpr QRgb Pink = qRgb(0xF2, 0xA6, 0xFF);

    // Surfaces, from behind the window to raised and selected.
    inline constexpr QRgb S0 = qRgb(0x07, 0x0B, 0x17);
    inline constexpr QRgb S1 = qRgb(0x0B, 0x10, 0x20);
    inline constexpr QRgb S2 = qRgb(0x0F, 0x17, 0x30);
    inline constexpr QRgb S3 = qRgb(0x0F, 0x1B, 0x3D);
    inline constexpr QRgb S4 = qRgb(0x17, 0x26, 0x50);
    inline constexpr QRgb S5 = qRgb(0x1F, 0x32, 0x69);
    inline constexpr QRgb Line = qRgb(0x1E, 0x2A, 0x4E);
    inline constexpr QRgb Line2 = qRgb(0x2F, 0x41, 0x75);

    inline constexpr QRgb T1 = qRgb(0xE6, 0xF0, 0xFF);
    inline constexpr QRgb T2 = qRgb(0xB0, 0xBF, 0xDC);
    inline constexpr QRgb T3 = qRgb(0x8C, 0x9D, 0xC2);
    inline constexpr QRgb TextDisabled = qRgb(0x5B, 0x6B, 0x8F);

    // Fills under white text are darker than the palette blue, to hold 4.5:1.
    inline constexpr QRgb Act = qRgb(0x1E, 0x6F, 0xE0);
    inline constexpr QRgb GradMid = qRgb(0x5B, 0x52, 0xEE);
    inline constexpr QRgb GradEnd = qRgb(0x8A, 0x4B, 0xE8);

    // States stay outside the brand hues so a status never reads as decoration.
    inline constexpr QRgb Ok = qRgb(0x3F, 0xE0, 0xC5);
    inline constexpr QRgb Warn = qRgb(0xFF, 0xB4, 0x54);
    inline constexpr QRgb Bad = qRgb(0xFF, 0x7F, 0x93);

    // Pairs of @name and hex for the stylesheet.
    QMap<QString, QString> StyleTokens();

    QFont HeadingFont(int pixelSize);
    QFont MonoFont(int pixelSize);

    // The primary button's fill and the logo's "road" line, across rect.
    QLinearGradient PrimaryGradient(const QRectF &rect);
    QLinearGradient RoadGradient(const QRectF &rect);

    // An icon from res/poros/icons, tinted per mode and state. An invalid color follows the theme's text colours.
    QIcon Icon(const QString &name, const QColor &color = {});
} // namespace PorosTheme
