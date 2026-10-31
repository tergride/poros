#pragma once

#include <QWidget>

class PortalPanel;
class QLineEdit;
class QPushButton;
class QStackedWidget;
class ServerListView;
class ServerTreeModel;

// The simple mode's main screen: the portal on the left, the subscriptions on the right.
class SimpleHome : public QWidget {
    Q_OBJECT

public:
    explicit SimpleHome(ServerTreeModel *model, QWidget *parent = nullptr);

    PortalPanel *portal() const { return m_portal; }
    ServerListView *list() const { return m_list; }

    // Follows whether there is any subscription to list.
    void syncEmptyState();

signals:
    void testRequested();
    void addRequested(const QString &link);
    void wizardRequested();

private:
    ServerTreeModel *m_model;
    PortalPanel *m_portal;
    ServerListView *m_list;
    QStackedWidget *m_body;
    QLineEdit *m_search;
    QLineEdit *m_emptyLink;
};
