#pragma once

#include <QFrame>
#include <QPixmap>

class QLabel;
class QPropertyAnimation;
class QPushButton;
class SegmentSwitch;
class StartStopButton;

// The left side of the simple main screen: the portal art lights up with the connection.
class PortalPanel : public QFrame {
    Q_OBJECT
    Q_PROPERTY(qreal glow READ glow WRITE setGlow)

public:
    enum class State { Off, Busy, On };

    explicit PortalPanel(QWidget *parent = nullptr);

    StartStopButton *startButton() const { return m_start; }
    SegmentSwitch *modeSwitch() const { return m_mode; }

    void setState(State state, const QString &text);
    void setServer(const QString &title, const QString &meta);
    // Empty down hides the row: it only means something while connected.
    void setStats(const QString &down, const QString &up, const QString &exit);
    void setModeHint(const QString &text);
    void setPreset(const QString &name);

    qreal glow() const { return m_glow; }
    void setGlow(qreal glow);

signals:
    void presetClicked();

protected:
    void paintEvent(QPaintEvent *event) override;
    void resizeEvent(QResizeEvent *event) override;

private:
    void renderArt();

    QLabel *m_state;
    QLabel *m_title;
    QLabel *m_meta;
    QWidget *m_stats;
    QLabel *m_down;
    QLabel *m_up;
    QLabel *m_exit;
    StartStopButton *m_start;
    SegmentSwitch *m_mode;
    QLabel *m_hint;
    QPushButton *m_preset;

    QPixmap m_art;
    QPixmap m_artDim;
    qreal m_glow = 0;
    QPropertyAnimation *m_glowAnimation;
};
