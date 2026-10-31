#pragma once

#include <QObject>
#include <QPalette>
#include <QColor>
#include <QHash>
#include <QIcon>
#include <QStyle>

#include <array>

// The Poros colours by role, for the widgets that paint themselves.
struct ThemeTokens {
    QColor surface;
    QColor onSurface;
    QColor accent;
    QColor muted;
    QColor tag;
    QColor danger;
    QColor success;
    QColor info;
    std::array<QColor, 5> chartSeries;
    QColor chartContext;
};

class ThemeManager : public QObject {
    Q_OBJECT
public:
    ThemeTokens tokens;

    void ApplyTheme();

    // QStyle::standardIcon loads from the system on every call: about 9 ms for a message box icon on Windows.
    QIcon standardIcon(QStyle::StandardPixmap pixmap);
signals:
    void themeChanged();

private:
    QHash<int, QIcon> standardIcons; // the current style's; ApplyTheme() reloads the message box icons
};

ThemeManager *themeManager();
