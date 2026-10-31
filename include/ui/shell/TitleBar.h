#pragma once

#include <QPixmap>
#include <QWidget>

class QAbstractButton;
class QLabel;
class QToolButton;

// The window's own title bar: logo, app menu, mode and rights chips, window buttons.
class TitleBar : public QWidget {
    Q_OBJECT

public:
    explicit TitleBar(QWidget *parent = nullptr);

    void setMenuVisible(bool visible);
    void setModeText(const QString &text);
    void setAdminVisible(bool visible);
    void setMaximized(bool maximized);

    // Windows 11 opens Snap Layouts only over a non-client maximize button, which also hides the hover from Qt.
    void setMaximizeHover(bool hover);
    QAbstractButton *maximizeButton() const;

    // Whether a press at this point of the bar moves the window; the controls keep theirs.
    bool isCaption(const QPoint &pos) const;

signals:
    void menuRequested(const QPoint &globalPos);
    void minimizeRequested();
    void maximizeToggled();
    void closeRequested();

protected:
    void paintEvent(QPaintEvent *event) override;

private:
    QToolButton *m_menu;
    QLabel *m_mode;
    QLabel *m_admin;
    QToolButton *m_minimize;
    QToolButton *m_maximize;
    QToolButton *m_close;
    QPixmap m_mark;
    QPixmap m_wordmark;
};
