#include "include/ui/utils/ProfilesFilterProxyModel.h"
#include "include/ui/utils/ProfilesTableModel.h"

namespace {
    const QString portPrefix = QStringLiteral("port=");
}

ProfilesFilterProxyModel::ProfilesFilterProxyModel(QObject *parent)
    : QSortFilterProxyModel(parent) {
    setDynamicSortFilter(true);
}

ProfilesTableModel *ProfilesFilterProxyModel::profilesModel() const {
    return qobject_cast<ProfilesTableModel *>(sourceModel());
}

bool ProfilesFilterProxyModel::hasActiveFilter() const {
    return !m_search.isEmpty();
}

void ProfilesFilterProxyModel::setSearch(const QString &text) {
    if (m_search == text) return;
    m_search = text;
    invalidateRowsFilter();
}

int ProfilesFilterProxyModel::toSourceRow(int proxyRow) const {
    const QModelIndex idx = mapToSource(index(proxyRow, 0));
    return idx.isValid() ? idx.row() : -1;
}

int ProfilesFilterProxyModel::toProxyRow(int sourceRow) const {
    if (!sourceModel()) return -1;
    const QModelIndex idx = mapFromSource(sourceModel()->index(sourceRow, 0));
    return idx.isValid() ? idx.row() : -1;
}

bool ProfilesFilterProxyModel::portMatches(int port) const {
    const QString val = m_search.mid(portPrefix.size());
    if (!val.contains(':')) return val.isEmpty() ? false : port == val.toInt();
    const QStringList parts = val.split(':');
    const bool minOk = parts[0].isEmpty() || port >= parts[0].toInt();
    const bool maxOk = (parts.size() < 2 || parts[1].isEmpty()) || port <= parts[1].toInt();
    return minOk && maxOk;
}

bool ProfilesFilterProxyModel::filterAcceptsRow(int sourceRow, const QModelIndex &) const {
    if (!hasActiveFilter()) return true;

    auto *model = profilesModel();
    if (!model) return true;

    // A profile that failed to load stays visible rather than becoming unreachable.
    const auto *key = model->filterKeyAt(sourceRow);
    if (!key) return true;

    if (m_search.startsWith(portPrefix)) return portMatches(key->port);
    return key->name.contains(m_search, Qt::CaseInsensitive) || key->address.contains(m_search, Qt::CaseInsensitive)
        || key->effectiveAddress.contains(m_search, Qt::CaseInsensitive) || key->type.contains(m_search, Qt::CaseInsensitive)
        || key->country.contains(m_search, Qt::CaseInsensitive);
}
