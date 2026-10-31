#pragma once

#include <QToolButton>
#include <QColor>
#include <QPixmap>

class QPainter;
class QPropertyAnimation;

class StartStopButton : public QToolButton {
    Q_OBJECT
    Q_PROPERTY(qreal spin READ spin WRITE setSpin)
    Q_PROPERTY(qreal dim READ dim WRITE setDim)
    Q_PROPERTY(qreal press READ press WRITE setPress)

public:
    enum class State { Disabled, Idle, Connecting, Running, Disconnecting };
    Q_ENUM(State)

    // Mirrors the tray-icon modes computed in MainWindow::refresh_status.
    enum class Mode { Off, Core, SystemProxy, Tun };
    Q_ENUM(Mode)

    enum class Lock { Hidden, Pending, Blocking, Passing, Fault };
    Q_ENUM(Lock)

    explicit StartStopButton(QWidget *parent = nullptr);

    void setState(State s);
    State state() const { return m_state; }

    void setMode(Mode m);
    Mode mode() const { return m_mode; }

    void setLock(Lock l);
    Lock lock() const { return m_lock; }

    QSize sizeHint() const override;
    QSize minimumSizeHint() const override { return sizeHint(); }

    qreal spin() const { return m_spin; }
    void setSpin(qreal v) { m_spin = v; update(); }
    qreal dim() const { return m_dim; }
    void setDim(qreal v) { m_dim = v; update(); }
    qreal press() const { return m_press; }
    void setPress(qreal v) { m_press = v; update(); }

protected:
    void paintEvent(QPaintEvent *) override;
    void showEvent(QShowEvent *) override;
    void hideEvent(QHideEvent *) override;
    void changeEvent(QEvent *) override;
    bool event(QEvent *e) override;

private:
    void applyState(bool animated);
    void animate(QPropertyAnimation *anim, const QVariant &to, int duration);
    void setLoopRunning(QPropertyAnimation *anim, bool running);
    void updateLoops();
    void updateToolTip();
    QString label() const;
    QRectF lockBox(const QRectF &area) const;
    QColor lockColor() const;
    qreal lockOpacity() const;
    void paintLock(QPainter &p, const QRectF &area);

    State m_state = State::Disabled;
    Mode m_mode = Mode::Off;
    Lock m_lock = Lock::Hidden;
    bool m_shown = false;

    qreal m_spin = 0.0;
    qreal m_dim = 1.0;
    qreal m_press = 0.0;

    QPropertyAnimation *m_dimAnim = nullptr;
    QPropertyAnimation *m_pressAnim = nullptr;
    QPropertyAnimation *m_spinAnim = nullptr;

    QPixmap m_lockCache;
    Lock m_lockKeyLock = Lock::Hidden;
    qreal m_lockKeySize = 0.0;
    qreal m_lockKeyDpr = 0.0;
};
