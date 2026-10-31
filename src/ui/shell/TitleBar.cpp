#include "include/ui/shell/TitleBar.h"

#include <QHBoxLayout>
#include <QLabel>
#include <QPainter>
#include <QStyle>
#include <QToolButton>

#include "include/ui/setting/PorosTheme.h"

namespace {
    constexpr int titleBarHeight = 44;
    constexpr int titleBarLogoEnd = 118;

    QToolButton *titleBarWindowButton(const QString &name, const QString &icon, QWidget *parent) {
        auto *button = new QToolButton(parent);
        button->setObjectName(name);
        button->setProperty("role", QStringLiteral("window"));
        button->setIcon(PorosTheme::Icon(icon));
        button->setIconSize({16, 16});
        button->setFixedSize(46, titleBarHeight);
        button->setFocusPolicy(Qt::NoFocus);
        return button;
    }

    QLabel *titleBarChip(const QString &name, QWidget *parent) {
        auto *chip = new QLabel(parent);
        chip->setObjectName(name);
        chip->setProperty("role", QStringLiteral("chip"));
        chip->setAlignment(Qt::AlignCenter);
        chip->setFixedHeight(26);
        return chip;
    }
} // namespace

TitleBar::TitleBar(QWidget *parent)
    : QWidget(parent), m_mark(QStringLiteral(":/poros/img/mark.png")), m_wordmark(QStringLiteral(":/poros/img/wordmark.png")) {
    setAttribute(Qt::WA_StyledBackground);
    setFixedHeight(titleBarHeight);

    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(titleBarLogoEnd, 0, 0, 0);
    row->setSpacing(4);

    m_menu = new QToolButton(this);
    m_menu->setObjectName(QStringLiteral("appMenuButton"));
    m_menu->setIcon(PorosTheme::Icon(QStringLiteral("menu")));
    m_menu->setIconSize({16, 16});
    m_menu->setText(tr("Menu"));
    m_menu->setToolButtonStyle(Qt::ToolButtonTextBesideIcon);
    m_menu->setFocusPolicy(Qt::NoFocus);
    connect(m_menu, &QToolButton::clicked, this, [this] { emit menuRequested(m_menu->mapToGlobal(QPoint(0, m_menu->height() + 2))); });
    row->addWidget(m_menu);
    row->addStretch();

    m_admin = titleBarChip(QStringLiteral("adminChip"), this);
    m_admin->setText(tr("Administrator"));
    m_admin->hide();
    row->addWidget(m_admin);

    m_mode = titleBarChip(QStringLiteral("modeChip"), this);
    row->addWidget(m_mode);
    row->addSpacing(8);

    m_minimize = titleBarWindowButton(QStringLiteral("minimizeButton"), QStringLiteral("min"), this);
    m_maximize = titleBarWindowButton(QStringLiteral("maximizeButton"), QStringLiteral("max"), this);
    m_close = titleBarWindowButton(QStringLiteral("closeButton"), QStringLiteral("close"), this);
    row->setSpacing(0);
    row->addWidget(m_minimize);
    row->addWidget(m_maximize);
    row->addWidget(m_close);
    connect(m_minimize, &QToolButton::clicked, this, &TitleBar::minimizeRequested);
    connect(m_maximize, &QToolButton::clicked, this, &TitleBar::maximizeToggled);
    connect(m_close, &QToolButton::clicked, this, &TitleBar::closeRequested);
}

void TitleBar::setMenuVisible(bool visible) {
    m_menu->setVisible(visible);
    m_mode->setVisible(visible);
}

void TitleBar::setModeText(const QString &text) {
    m_mode->setText(text);
}

void TitleBar::setAdminVisible(bool visible) {
    m_admin->setVisible(visible);
}

void TitleBar::setMaximized(bool maximized) {
    m_maximize->setToolTip(maximized ? tr("Restore") : tr("Maximize"));
}

void TitleBar::setMaximizeHover(bool hover) {
    m_maximize->setProperty("hovered", hover);
    m_maximize->style()->unpolish(m_maximize);
    m_maximize->style()->polish(m_maximize);
}

QAbstractButton *TitleBar::maximizeButton() const {
    return m_maximize;
}

bool TitleBar::isCaption(const QPoint &pos) const {
    return qobject_cast<const QToolButton *>(childAt(pos)) == nullptr;
}

void TitleBar::paintEvent(QPaintEvent *event) {
    QWidget::paintEvent(event);
    QPainter painter(this);
    painter.setRenderHint(QPainter::SmoothPixmapTransform);
    const auto scaledWidth = [](const QPixmap &pixmap, int height) { return height * pixmap.width() / qMax(1, pixmap.height()); };
    constexpr int markHeight = 22;
    constexpr int wordmarkHeight = 10;
    const int markWidth = scaledWidth(m_mark, markHeight);
    painter.drawPixmap(QRect(12, (height() - markHeight) / 2, markWidth, markHeight), m_mark);
    painter.drawPixmap(QRect(12 + markWidth + 9, (height() - wordmarkHeight) / 2, scaledWidth(m_wordmark, wordmarkHeight), wordmarkHeight), m_wordmark);
}
