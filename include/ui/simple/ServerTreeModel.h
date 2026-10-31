#pragma once

#include <QAbstractItemModel>
#include <QList>
#include <QSet>
#include <QString>

// Subscriptions and their servers, as a two-level tree. The last top-level row is "add another subscription".
class ServerTreeModel : public QAbstractItemModel {
    Q_OBJECT

public:
    enum class Kind { Group, Auto, Server, AddRow };

    enum Role {
        KindRole = Qt::UserRole + 1,
        ProfileIdRole,
        GroupIdRole,
    };

    struct Server {
        int id = -1;
        QString name;
        QString country;
        int latency = 0;
    };

    struct Group {
        int id = -1;
        QString name;
        QString subtitle;
        QString quota;
        QString expiry;
        double usedShare = -1;
        bool autoAvailable = false;
        int autoId = -1;
        QList<Server> servers;
    };

    explicit ServerTreeModel(QObject *parent = nullptr);

    // Rereads the subscriptions from the repositories.
    void reload();

    // Only the given profiles' test results changed.
    void refreshProfiles(const QList<int> &ids);

    void setSelectedId(int id);
    int selectedId() const { return m_selectedId; }
    void setRunningId(int id);
    int runningId() const { return m_runningId; }

    bool hasGroups() const { return !m_groups.isEmpty(); }
    const QList<Group> &groups() const { return m_groups; }
    Kind kindOf(const QModelIndex &index) const;
    const Group &groupOf(const QModelIndex &index) const;
    const Server &serverOf(const QModelIndex &index) const;
    bool isLastInGroup(const QModelIndex &index) const;

    QModelIndex index(int row, int column, const QModelIndex &parent = {}) const override;
    QModelIndex parent(const QModelIndex &child) const override;
    int rowCount(const QModelIndex &parent = {}) const override;
    int columnCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role = Qt::DisplayRole) const override;
    Qt::ItemFlags flags(const QModelIndex &index) const override;

private:
    QModelIndex indexOfProfile(int id) const;

    QList<Group> m_groups;
    int m_selectedId = -1;
    int m_runningId = -1;
};
