#pragma once

#include <QSortFilterProxyModel>
#include <QString>

class ProfilesTableModel;

class ProfilesFilterProxyModel : public QSortFilterProxyModel {
    Q_OBJECT
public:
    explicit ProfilesFilterProxyModel(QObject *parent = nullptr);

    // Matches the name, address, type or exit country; "port=N", "port=MIN:MAX", "port=MIN:", "port=:MAX" match the port.
    void setSearch(const QString &text);
    bool hasActiveFilter() const;

    ProfilesTableModel *profilesModel() const;

    int toSourceRow(int proxyRow) const;
    int toProxyRow(int sourceRow) const;

protected:
    bool filterAcceptsRow(int sourceRow, const QModelIndex &sourceParent) const override;

private:
    bool portMatches(int port) const;

    QString m_search;
};
