#pragma once

#include <QAbstractButton>
#include <QPainter>

#include "include/ui/setting/PorosTheme.h"

class SimpleSettingsNavButton : public QAbstractButton {
public:
    SimpleSettingsNavButton(const QString &iconName, const QString &text, QWidget *parent) : QAbstractButton(parent) {
        setIcon(PorosTheme::Icon(iconName));
        setText(text);
        setCheckable(true);
        setAttribute(Qt::WA_Hover);
        setCursor(Qt::PointingHandCursor);
        setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
    }

    QSize sizeHint() const override { return {208, 36}; }

protected:
    void paintEvent(QPaintEvent *) override {
        using namespace PorosTheme;
        QPainter painter(this);
        painter.setRenderHint(QPainter::Antialiasing);
        const bool hover = underMouse();
        if (isChecked() || hover) {
            painter.setPen(Qt::NoPen);
            painter.setBrush(QColor(isChecked() ? S4 : S3));
            painter.drawRoundedRect(rect(), 8, 8);
        }
        constexpr int iconSize = 16;
        icon().paint(&painter, QRect(10, (height() - iconSize) / 2, iconSize, iconSize), Qt::AlignCenter,
                     isChecked() ? QIcon::Selected : hover ? QIcon::Active : QIcon::Normal);
        painter.setPen(QColor(isChecked() ? qRgb(255, 255, 255) : hover ? T1 : T2));
        painter.drawText(rect().adjusted(36, 0, -6, 0), Qt::AlignVCenter | Qt::AlignLeft, text());
    }
};
