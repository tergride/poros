#pragma once

#include <QSortFilterProxyModel>
#include <QString>

class ConnectionsTreeModel;

namespace ConnectionsTree {
struct ProcessGroupItem;
struct ConnectionLeafItem;
}

class ConnectionsTreeFilterProxyModel : public QSortFilterProxyModel {
    Q_OBJECT
public:
    explicit ConnectionsTreeFilterProxyModel(QObject *parent = nullptr);

    void setSearch(const QString &text);

    ConnectionsTreeModel *treeModel() const;

protected:
    bool filterAcceptsRow(int sourceRow, const QModelIndex &sourceParent) const override;

private:
    bool leafMatches(const ConnectionsTree::ProcessGroupItem *group, const ConnectionsTree::ConnectionLeafItem *leaf) const;

    QString m_search;
};
