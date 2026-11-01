#pragma once

#include <QFrame>
#include <QUrl>
#include <memory>

namespace Configs {
    class Group;
}

class QAction;
class QLabel;
class QMenu;
class QProgressBar;
class QPushButton;
class QResizeEvent;
class QToolButton;
class QWidget;

// The line above the profile table: what the current group is, its subscription's state, and the group's actions.
class SubscriptionInfoCard : public QFrame {
    Q_OBJECT
public:
    explicit SubscriptionInfoCard(QWidget *parent = nullptr);

    void setGroup(const std::shared_ptr<Configs::Group> &group);
    void setActions(QAction *update, QAction *edit, QMenu *more);

    static QString expiryText(qint64 expire);

protected:
    void resizeEvent(QResizeEvent *event) override;

private:
    void updateData();
    void showTitle();

    std::shared_ptr<Configs::Group> m_group;
    QUrl m_webUrl;
    QUrl m_supportUrl;

    QLabel *m_titleLabel = nullptr;
    QLabel *m_detailLabel = nullptr;
    QWidget *m_quota = nullptr;
    QLabel *m_quotaLabel = nullptr;
    QLabel *m_expiryLabel = nullptr;
    QProgressBar *m_progressBar = nullptr;
    QToolButton *m_announceButton = nullptr;
    QToolButton *m_portalButton = nullptr;
    QToolButton *m_supportButton = nullptr;
    QPushButton *m_updateButton = nullptr;
    QToolButton *m_editButton = nullptr;
    QToolButton *m_moreButton = nullptr;
};
