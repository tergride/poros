#pragma once

#include <QSet>
#include <QTreeView>

class QSortFilterProxyModel;
class ServerTreeModel;

// The server list of the simple mode: cards of subscriptions painted by a delegate, clicks resolved here.
class ServerListView : public QTreeView {
    Q_OBJECT

public:
    explicit ServerListView(ServerTreeModel *model, QWidget *parent = nullptr);

    void setFilterText(const QString &text);

    // After a model reset: every group opens, except the ones the user closed.
    void restoreExpansion();

signals:
    void serverPicked(int profileId);
    void autoPicked(int groupId);
    void refreshRequested(int groupId);
    void groupMenuRequested(int groupId, const QPoint &globalPos);
    void addRequested();

protected:
    void mouseReleaseEvent(QMouseEvent *event) override;

private:
    ServerTreeModel *m_servers;
    QSortFilterProxyModel *m_filter;
    QSet<int> m_closedGroups;
};
