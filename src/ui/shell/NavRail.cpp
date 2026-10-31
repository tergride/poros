#include "include/ui/shell/NavRail.h"

#include <QAbstractButton>
#include <QPainter>
#include <QToolButton>
#include <QVBoxLayout>

#include "include/ui/setting/PorosTheme.h"

namespace {
    class NavRailButton : public QAbstractButton {
    public:
        NavRailButton(const QString &iconName, const QString &text, QWidget *parent) : QAbstractButton(parent) {
            setIcon(PorosTheme::Icon(iconName));
            setText(text);
            setCheckable(true);
            setAttribute(Qt::WA_Hover);
            setFocusPolicy(Qt::TabFocus);
            setCursor(Qt::PointingHandCursor);
            setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
        }

        QSize sizeHint() const override { return {156, 38}; }

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
            const auto mode = isChecked() ? QIcon::Selected : hover ? QIcon::Active : QIcon::Normal;
            icon().paint(&painter, QRect(10, (height() - iconSize) / 2, iconSize, iconSize), Qt::AlignCenter, mode);
            painter.setPen(QColor(isChecked() ? qRgb(255, 255, 255) : hover ? T1 : T2));
            painter.drawText(rect().adjusted(36, 0, -6, 0), Qt::AlignVCenter | Qt::AlignLeft, text());
        }
    };
} // namespace

NavRail::NavRail(QWidget *parent) : QWidget(parent) {
    setAttribute(Qt::WA_StyledBackground);
    setFixedWidth(176);
    m_column = new QVBoxLayout(this);
    m_column->setContentsMargins(10, 10, 10, 10);
    m_column->setSpacing(2);
    m_column->addStretch();

    auto *search = new QToolButton(this);
    search->setObjectName(QStringLiteral("railSearch"));
    search->setIcon(PorosTheme::Icon(QStringLiteral("search")));
    search->setIconSize({14, 14});
    search->setText(tr("Search") + QStringLiteral("   Ctrl K"));
    search->setToolButtonStyle(Qt::ToolButtonTextBesideIcon);
    search->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
    search->setFocusPolicy(Qt::NoFocus);
    connect(search, &QToolButton::clicked, this, &NavRail::searchRequested);
    m_column->addWidget(search);
}

int NavRail::addSection(const QString &icon, const QString &text) {
    const int index = m_buttons.size();
    auto *button = new NavRailButton(icon, text, this);
    connect(button, &QAbstractButton::clicked, this, [this, index] {
        setCurrent(index);
        emit currentChanged(index);
    });
    m_column->insertWidget(index, button);
    m_buttons << button;
    return index;
}

void NavRail::setCurrent(int index) {
    m_current = index;
    for (int i = 0; i < m_buttons.size(); i++) m_buttons[i]->setChecked(i == index);
}
