#include "include/ui/widget/StartStopButton.hpp"

#include <QContextMenuEvent>
#include <QEvent>
#include <QPainter>
#include <QPainterPath>
#include <QPixmap>
#include <QPropertyAnimation>

#include "include/sys/KillSwitch.hpp"
#include "include/ui/setting/PorosTheme.h"

StartStopButton::StartStopButton(QWidget *parent) : QToolButton(parent) {
    setFocusPolicy(Qt::NoFocus);
    setSizePolicy(QSizePolicy::Fixed, QSizePolicy::Fixed);
    setAttribute(Qt::WA_Hover);

    m_dimAnim = new QPropertyAnimation(this, "dim", this);
    m_pressAnim = new QPropertyAnimation(this, "press", this);

    m_spinAnim = new QPropertyAnimation(this, "spin", this);
    m_spinAnim->setStartValue(0.0);
    m_spinAnim->setEndValue(360.0);
    m_spinAnim->setDuration(900);
    m_spinAnim->setLoopCount(-1);
    m_spinAnim->setEasingCurve(QEasingCurve::Linear);

    connect(this, &QAbstractButton::pressed, this, [this] { animate(m_pressAnim, 1.0, 110); });
    connect(this, &QAbstractButton::released, this, [this] { animate(m_pressAnim, 0.0, 160); });

    m_state = State::Disabled;
    applyState(false);
}

QSize StartStopButton::sizeHint() const {
    return {124, 38};
}

void StartStopButton::setState(State s) {
    if (s == m_state) return;
    m_state = s;
    applyState(true);
}

void StartStopButton::setMode(Mode m) {
    m_mode = m;
}

void StartStopButton::setLock(Lock l) {
    if (l == m_lock) return;
    m_lock = l;
    updateToolTip();
    update();
}

QString StartStopButton::label() const {
    switch (m_state) {
        case State::Connecting: return tr("Connecting…");
        case State::Disconnecting: return tr("Stopping…");
        case State::Running: return tr("Stop");
        default: return tr("Start");
    }
}

void StartStopButton::updateToolTip() {
    QString tip;
    switch (m_state) {
        case State::Disabled: tip = tr("Select a profile to start"); break;
        case State::Idle: tip = tr("Start"); break;
        case State::Connecting: tip = tr("Connecting…"); break;
        case State::Running: tip = tr("Stop"); break;
        case State::Disconnecting: tip = tr("Stopping…"); break;
    }
    switch (m_lock) {
        case Lock::Hidden: break;
        case Lock::Pending: tip += "\n" + tr("Kill switch: starting…"); break;
        case Lock::Blocking: tip += "\n" + tr("Kill switch: Tun is not running, so most traffic is blocked"); break;
        case Lock::Passing: tip += "\n" + tr("Kill switch: on — traffic only flows through Poros's Tun"); break;
        case Lock::Fault:
            tip += "\n" + (Sys::KillSwitch::instance()->failedForIdentity() ? tr("Kill switch is blocking traffic") : tr("Kill switch is not active"));
            break;
    }
    setToolTip(tip);
}

void StartStopButton::applyState(bool animated) {
    const bool interactive = (m_state == State::Idle || m_state == State::Running);
    setEnabled(interactive);
    setCursor(interactive ? Qt::PointingHandCursor : Qt::ArrowCursor);

    updateToolTip();

    const qreal dimTarget = (m_state == State::Disabled) ? 0.55 : 1.0;
    if (animated) {
        animate(m_dimAnim, dimTarget, 220);
    } else {
        m_dimAnim->stop();
        m_dim = dimTarget;
    }

    updateLoops();
    update();
}

void StartStopButton::animate(QPropertyAnimation *anim, const QVariant &to, int duration) {
    anim->stop();
    anim->setDuration(duration);
    anim->setEasingCurve(QEasingCurve::InOutCubic);
    anim->setStartValue(property(anim->propertyName().constData()));
    anim->setEndValue(to);
    anim->start();
}

void StartStopButton::setLoopRunning(QPropertyAnimation *anim, bool running) {
    if (running) {
        if (anim->state() != QAbstractAnimation::Running) anim->start();
        return;
    }
    anim->stop();
    if (anim == m_spinAnim) m_spin = 0.0;
    update();
}

void StartStopButton::updateLoops() {
    const bool spinning = m_state == State::Connecting || m_state == State::Disconnecting;
    setLoopRunning(m_spinAnim, m_shown && spinning);
}

void StartStopButton::showEvent(QShowEvent *e) {
    QToolButton::showEvent(e);
    m_shown = true;
    updateLoops();
}

void StartStopButton::hideEvent(QHideEvent *e) {
    m_shown = false;
    setLoopRunning(m_spinAnim, false);
    QToolButton::hideEvent(e);
}

bool StartStopButton::event(QEvent *e) {
    // QWidget::event drops a disabled widget's context menu before consulting the policy, and the button is disabled when nothing can start.
    if (e->type() == QEvent::ContextMenu && !isEnabled() && contextMenuPolicy() == Qt::CustomContextMenu) {
        emit customContextMenuRequested(static_cast<QContextMenuEvent *>(e)->pos());
        return true;
    }
    // The failure kind behind a Fault can change while the lock stays Fault.
    if (e->type() == QEvent::ToolTip && m_lock == Lock::Fault) updateToolTip();
    return QToolButton::event(e);
}

void StartStopButton::changeEvent(QEvent *e) {
    if (e->type() == QEvent::PaletteChange || e->type() == QEvent::ThemeChange) m_lockCache = QPixmap();
    QToolButton::changeEvent(e);
}

void StartStopButton::paintEvent(QPaintEvent *) {
    using namespace PorosTheme;
    QPainter p(this);
    p.setRenderHint(QPainter::Antialiasing, true);

    const QRectF box = QRectF(rect()).adjusted(0.5, 0.5, -0.5, -0.5);
    const QPointF centre = box.center();
    const qreal scale = 1.0 - 0.02 * m_press;
    p.translate(centre);
    p.scale(scale, scale);
    p.translate(-centre);
    p.setOpacity(m_dim);

    const bool hover = underMouse() && isEnabled();
    const bool busy = m_state == State::Connecting || m_state == State::Disconnecting;
    const bool primary = m_state == State::Idle || m_state == State::Disabled;
    const qreal radius = qMin<qreal>(10, height() * 0.26);
    if (primary) {
        p.setPen(Qt::NoPen);
        p.setBrush(m_state == State::Idle ? QBrush(PrimaryGradient(box)) : QBrush(QColor(S4)));
    } else {
        p.setPen(QPen(QColor(Line2), 1));
        p.setBrush(QColor(hover ? S4 : S3));
    }
    p.drawRoundedRect(box, radius, radius);
    if (primary && hover) {
        p.setPen(Qt::NoPen);
        p.setBrush(QColor(255, 255, 255, 26));
        p.drawRoundedRect(box, radius, radius);
    }

    const QColor ink = m_state == State::Disabled ? QColor(T2) : QColor(primary ? qRgb(255, 255, 255) : T1);
    QFont labelFont = font();
    labelFont.setWeight(QFont::DemiBold);
    if (height() > 44) labelFont.setPixelSize(15);
    p.setFont(labelFont);
    const QString text = label();
    const int iconSize = height() > 44 ? 18 : 15;
    constexpr int gap = 8;
    const int textWidth = QFontMetrics(labelFont).horizontalAdvance(text);
    const qreal left = centre.x() - (iconSize + gap + textWidth) / 2.0;
    const QRectF iconBox(left, centre.y() - iconSize / 2.0, iconSize, iconSize);

    if (busy) {
        p.setBrush(Qt::NoBrush);
        p.setPen(QPen(QColor(Line2), 2));
        p.drawEllipse(iconBox.adjusted(1, 1, -1, -1));
        QPen arc(QColor(Blue), 2);
        arc.setCapStyle(Qt::RoundCap);
        p.setPen(arc);
        p.drawArc(iconBox.adjusted(1, 1, -1, -1), static_cast<int>(-m_spin * 16), -110 * 16);
    } else {
        const QIcon icon = PorosTheme::Icon(m_state == State::Running ? QStringLiteral("stop") : QStringLiteral("play"), ink);
        icon.paint(&p, iconBox.toRect());
    }
    p.setPen(ink);
    p.drawText(QRectF(left + iconSize + gap, box.top(), textWidth + 2, box.height()), Qt::AlignVCenter | Qt::AlignLeft, text);

    if (m_lock != Lock::Hidden) {
        // Outside the press scale and m_dim: the lock must stay legible on a disabled button.
        p.resetTransform();
        p.setOpacity(1.0);
        paintLock(p, QRectF(rect()).adjusted(0, 0, -4, -4));
    }
}

QRectF StartStopButton::lockBox(const QRectF &area) const {
    const qreal D = qMin(area.width(), area.height());
    const qreal L = D * 0.31;
    const qreal margin = D * 0.01;
    return {area.right() - margin - L, area.bottom() - margin - L, L, L};
}

QColor StartStopButton::lockColor() const {
    const bool dark = palette().color(QPalette::Window).lightness() < 128;
    switch (m_lock) {
        case Lock::Pending: {
            QColor neutral = palette().color(QPalette::WindowText);
            neutral.setAlpha(255);
            return neutral;
        }
        case Lock::Blocking: return dark ? QColor(0xFF, 0xC1, 0x3B) : QColor(0xC7, 0x84, 0x00);
        case Lock::Fault: return dark ? QColor(0xFF, 0x5C, 0x5C) : QColor(0xD3, 0x2F, 0x2F);
        default: return dark ? QColor(0x4A, 0xD6, 0x7E) : QColor(0x1E, 0x8E, 0x45);
    }
}

qreal StartStopButton::lockOpacity() const {
    switch (m_lock) {
        case Lock::Pending: return 0.55;
        case Lock::Passing: return 0.75;
        default: return 1.0;
    }
}

void StartStopButton::paintLock(QPainter &p, const QRectF &area) {
    const QRectF box = lockBox(area);
    const qreal L = box.width();
    const qreal pad = L * 0.34;

    const QColor window = palette().color(QPalette::Window);
    const bool dark = window.lightness() < 128;
    const QColor fill = lockColor();

    const qreal dpr = devicePixelRatioF();
    if (m_lockCache.isNull() || m_lockKeyLock != m_lock || !qFuzzyCompare(m_lockKeySize, L) || !qFuzzyCompare(m_lockKeyDpr, dpr)) {
        const qreal side = L + 2 * pad;
        QPixmap pm(QSizeF(side * dpr, side * dpr).toSize());
        pm.setDevicePixelRatio(dpr);
        pm.fill(Qt::transparent);
        QPainter lp(&pm);
        lp.setRenderHint(QPainter::Antialiasing, true);
        lp.translate(pad, pad);

        const qreal sw = L * 0.14;
        const QRectF body(L * 0.11, L * 0.45, L * 0.78, L * 0.55);
        const qreal r = L * 0.215;
        const qreal cx = L / 2;
        QPainterPath shackle;
        shackle.moveTo(cx - r, body.top() + sw / 2);
        shackle.lineTo(cx - r, sw / 2 + r);
        shackle.arcTo(QRectF(cx - r, sw / 2, 2 * r, 2 * r), 180, -180);
        shackle.lineTo(cx + r, body.top() + sw / 2);
        QPainterPathStroker stroker;
        stroker.setWidth(sw);
        stroker.setCapStyle(Qt::FlatCap);
        stroker.setJoinStyle(Qt::RoundJoin);
        QPainterPath bodyPath;
        bodyPath.addRoundedRect(body, L * 0.12, L * 0.12);
        const QPainterPath outer = stroker.createStroke(shackle).united(bodyPath);

        QPainterPath mark;
        mark.setFillRule(Qt::WindingFill);
        if (m_lock == Lock::Blocking) {
            const qreal w = L * 0.09;
            mark.addRoundedRect(QRectF(cx - w / 2, body.top() + body.height() * 0.16, w, body.height() * 0.44), w / 2, w / 2);
            mark.addEllipse(QPointF(cx, body.top() + body.height() * 0.78), w * 0.62, w * 0.62);
        } else {
            const QPointF hole(cx, body.top() + body.height() * 0.40);
            mark.addEllipse(hole, L * 0.08, L * 0.08);
            mark.addRect(QRectF(cx - L * 0.035, hole.y(), L * 0.07, body.height() * 0.32));
        }

        // A cut-out in the window colour keeps the lock apart from the ring and its glow underneath.
        QColor cutout = window;
        cutout.setAlpha(255);
        lp.setPen(QPen(cutout, qMax(1.0, L * 0.08) * 2, Qt::SolidLine, Qt::RoundCap, Qt::RoundJoin));
        lp.setBrush(cutout);
        lp.drawPath(outer);

        lp.setPen(Qt::NoPen);
        lp.setBrush(fill);
        lp.drawPath(outer.subtracted(mark));

        if (m_lock == Lock::Fault) {
            lp.setPen(QPen(dark ? QColor(255, 255, 255, 220) : QColor(0x5A, 0x00, 0x00), qMax(1.0, L * 0.06),
                           Qt::SolidLine, Qt::RoundCap, Qt::RoundJoin));
            lp.setBrush(Qt::NoBrush);
            lp.drawPath(outer);
        }
        lp.end();

        m_lockCache = pm;
        m_lockKeyLock = m_lock;
        m_lockKeySize = L;
        m_lockKeyDpr = dpr;
    }

    p.setOpacity(lockOpacity());
    p.drawPixmap(box.topLeft() - QPointF(pad, pad), m_lockCache);
}
