#include "include/ui/utils/ConnectionsTreeFilterProxyModel.h"
#include "include/ui/utils/ConnectionsTreeModel.h"

#include <algorithm>

ConnectionsTreeFilterProxyModel::ConnectionsTreeFilterProxyModel(QObject *parent)
    : QSortFilterProxyModel(parent) {
    setDynamicSortFilter(true);
}

ConnectionsTreeModel *ConnectionsTreeFilterProxyModel::treeModel() const {
    return qobject_cast<ConnectionsTreeModel *>(sourceModel());
}

void ConnectionsTreeFilterProxyModel::setSearch(const QString &text) {
    if (m_search == text) return;
    m_search = text;
    invalidateRowsFilter();
}

bool ConnectionsTreeFilterProxyModel::leafMatches(const ConnectionsTree::ProcessGroupItem *group,
                                                  const ConnectionsTree::ConnectionLeafItem *leaf) const {
    const auto found = [this](const QString &field) { return field.contains(m_search, Qt::CaseInsensitive); };
    return found(ConnectionsTreeModel::displayProcessName(group->processName)) || found(leaf->destText) || found(leaf->meta.outboundName)
           || found(leaf->meta.outbound) || found(leaf->protocolText) || found(leaf->meta.sourceDisplay);
}

bool ConnectionsTreeFilterProxyModel::filterAcceptsRow(int sourceRow, const QModelIndex &sourceParent) const {
    if (m_search.isEmpty()) return true;
    const auto *model = treeModel();
    if (model == nullptr) return true;

    const auto *group = model->groupAt(sourceParent.isValid() ? sourceParent.row() : sourceRow);
    if (group == nullptr) return false;
    if (!sourceParent.isValid()) {
        return std::any_of(group->children.begin(), group->children.end(),
                           [&](const auto &leaf) { return leafMatches(group, leaf.get()); });
    }
    return sourceRow >= 0 && sourceRow < static_cast<int>(group->children.size())
           && leafMatches(group, group->children[sourceRow].get());
}
