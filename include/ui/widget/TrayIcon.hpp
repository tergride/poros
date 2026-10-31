#pragma once

#include <QIcon>
#include <QMenu>
#include <QObject>
#include <QSystemTrayIcon>

class TrayIcon : public QObject {
    Q_OBJECT
public:
    explicit TrayIcon(QObject *parent = nullptr);
    ~TrayIcon() override;

    void setIcon(const QIcon &icon);
    void setToolTip(const QString &text);
    void setContextMenu(QMenu *menu);
    void setVisible(bool visible);
    void hide() { setVisible(false); }
    bool isVisible() const;

signals:
    void activated(QSystemTrayIcon::ActivationReason reason);

private:
    QSystemTrayIcon *m_tray;
};
