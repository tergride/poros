#include "include/ui/simple/PortalPanel.h"

#include <QHBoxLayout>
#include <QLabel>
#include <QPainter>
#include <QPropertyAnimation>
#include <QPushButton>
#include <QStyle>
#include <QVBoxLayout>

#include "include/ui/setting/PorosTheme.h"
#include "include/ui/shell/SegmentSwitch.h"
#include "include/ui/widget/StartStopButton.hpp"

namespace {
    constexpr int portalArtHeight = 380;

    QLabel *portalStat(const QString &caption, QWidget *parent, QHBoxLayout *row) {
        auto *card = new QFrame(parent);
        card->setProperty("role", QStringLiteral("stat"));
        auto *column = new QVBoxLayout(card);
        column->setContentsMargins(10, 8, 10, 8);
        column->setSpacing(2);
        auto *label = new QLabel(caption, card);
        label->setProperty("role", QStringLiteral("hint"));
        column->addWidget(label);
        auto *value = new QLabel(card);
        value->setProperty("role", QStringLiteral("mono"));
        column->addWidget(value);
        row->addWidget(card, 1);
        return value;
    }

    // saturate(.45) brightness(.42), as the mockup dims the art.
    QImage portalDimmed(QImage image) {
        image = image.convertToFormat(QImage::Format_ARGB32);
        for (int y = 0; y < image.height(); y++) {
            auto *line = reinterpret_cast<QRgb *>(image.scanLine(y));
            for (int x = 0; x < image.width(); x++) {
                const int gray = qGray(line[x]);
                const auto channel = [gray](int value) { return qBound(0, static_cast<int>((gray + (value - gray) * 0.45) * 0.42), 255); };
                line[x] = qRgb(channel(qRed(line[x])), channel(qGreen(line[x])), channel(qBlue(line[x])));
            }
        }
        return image;
    }
} // namespace

PortalPanel::PortalPanel(QWidget *parent) : QFrame(parent) {
    setFixedWidth(384);
    auto *column = new QVBoxLayout(this);
    column->setContentsMargins(22, 22, 22, 22);
    column->setSpacing(14);
    column->addStretch();

    m_state = new QLabel(this);
    m_state->setObjectName(QStringLiteral("portalState"));
    column->addWidget(m_state);

    auto *server = new QVBoxLayout;
    server->setSpacing(4);
    m_title = new QLabel(this);
    m_title->setObjectName(QStringLiteral("portalTitle"));
    m_title->setProperty("role", QStringLiteral("heading"));
    m_title->setWordWrap(true);
    m_title->setTextFormat(Qt::PlainText);
    server->addWidget(m_title);
    m_meta = new QLabel(this);
    m_meta->setProperty("role", QStringLiteral("hint"));
    m_meta->setTextFormat(Qt::PlainText);
    server->addWidget(m_meta);
    column->addLayout(server);

    m_stats = new QWidget(this);
    auto *stats = new QHBoxLayout(m_stats);
    stats->setContentsMargins(0, 0, 0, 0);
    stats->setSpacing(8);
    m_down = portalStat(tr("Download"), m_stats, stats);
    m_up = portalStat(tr("Upload"), m_stats, stats);
    m_exit = portalStat(tr("Exit"), m_stats, stats);
    m_stats->hide();
    column->addWidget(m_stats);

    auto *actions = new QHBoxLayout;
    actions->setSpacing(10);
    m_start = new StartStopButton(this);
    m_start->setFixedHeight(52);
    m_start->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
    m_start->setMinimumWidth(0);
    actions->addWidget(m_start, 1);
    m_mode = new SegmentSwitch({QStringLiteral("TUN"), QStringLiteral("System")}, this);
    m_mode->setFixedHeight(52);
    actions->addWidget(m_mode);
    column->addLayout(actions);

    m_hint = new QLabel(this);
    m_hint->setProperty("role", QStringLiteral("hint"));
    m_hint->setWordWrap(true);
    column->addWidget(m_hint);

    m_preset = new QPushButton(this);
    m_preset->setObjectName(QStringLiteral("portalPreset"));
    m_preset->setIcon(PorosTheme::Icon(QStringLiteral("layers")));
    m_preset->setIconSize({16, 16});
    m_preset->setCursor(Qt::PointingHandCursor);
    connect(m_preset, &QPushButton::clicked, this, &PortalPanel::presetClicked);
    column->addWidget(m_preset);

    m_glowAnimation = new QPropertyAnimation(this, "glow", this);
    m_glowAnimation->setDuration(700);
    m_glowAnimation->setEasingCurve(QEasingCurve::InOutCubic);
}

void PortalPanel::setState(State state, const QString &text) {
    m_state->setText(QStringLiteral("●  ") + text);
    const QString name = state == State::On ? QStringLiteral("on") : state == State::Busy ? QStringLiteral("busy") : QStringLiteral("off");
    if (m_state->property("state").toString() != name) {
        m_state->setProperty("state", name);
        m_state->style()->unpolish(m_state);
        m_state->style()->polish(m_state);
    }
    const qreal target = state == State::On ? 1.0 : state == State::Busy ? 0.45 : 0.0;
    if (m_glowAnimation->endValue().toReal() == target && m_glowAnimation->state() == QAbstractAnimation::Running) return;
    if (qFuzzyCompare(1.0 + m_glow, 1.0 + target)) return;
    m_glowAnimation->stop();
    m_glowAnimation->setStartValue(m_glow);
    m_glowAnimation->setEndValue(target);
    m_glowAnimation->start();
}

void PortalPanel::setServer(const QString &title, const QString &meta) {
    m_title->setText(title);
    m_meta->setText(meta);
}

void PortalPanel::setStats(const QString &down, const QString &up, const QString &exit) {
    m_stats->setVisible(!down.isEmpty());
    m_down->setText(down);
    m_up->setText(up);
    m_exit->setText(exit);
}

void PortalPanel::setModeHint(const QString &text) {
    m_hint->setText(text);
}

void PortalPanel::setPreset(const QString &name) {
    m_preset->setText(tr("Preset: %1").arg(name));
}

void PortalPanel::setGlow(qreal glow) {
    m_glow = glow;
    update(0, 0, width(), portalArtHeight);
}

void PortalPanel::resizeEvent(QResizeEvent *event) {
    QFrame::resizeEvent(event);
    m_art = {};
}

void PortalPanel::renderArt() {
    const qreal ratio = devicePixelRatioF();
    const QSize target = QSize(width(), portalArtHeight) * ratio;
    const QImage source(QStringLiteral(":/poros/img/portal.jpg"));
    const QImage scaled = source.scaled(target, Qt::KeepAspectRatioByExpanding, Qt::SmoothTransformation)
                              .copy((source.scaled(target, Qt::KeepAspectRatioByExpanding).width() - target.width()) / 2, 0, target.width(), target.height());
    m_art = QPixmap::fromImage(scaled);
    m_art.setDevicePixelRatio(ratio);
    m_artDim = QPixmap::fromImage(portalDimmed(scaled));
    m_artDim.setDevicePixelRatio(ratio);
}

void PortalPanel::paintEvent(QPaintEvent *event) {
    QFrame::paintEvent(event);
    if (m_art.isNull() || !qFuzzyCompare(m_art.devicePixelRatio(), devicePixelRatioF())) renderArt();
    using namespace PorosTheme;
    QPainter painter(this);
    painter.drawPixmap(0, 0, m_artDim);
    painter.setOpacity(m_glow);
    painter.drawPixmap(0, 0, m_art);
    painter.setOpacity(1);
    QLinearGradient fade(0, 0, 0, portalArtHeight);
    QColor clear(S0);
    clear.setAlpha(0);
    fade.setColorAt(0.4, clear);
    fade.setColorAt(0.98, QColor(S0));
    painter.fillRect(0, 0, width(), portalArtHeight, fade);
    painter.fillRect(width() - 1, 0, 1, height(), QColor(Line));
}
