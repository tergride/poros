#include "include/ui/simple/ServerTreeModel.h"

#include <QDateTime>

#include "include/database/GroupsRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/engine/services/AutoGroupService.h"
#include "include/engine/vpn/VpnEndpoints.h"
#include "include/global/Configs.hpp"
#include "include/ui/widget/SubscriptionInfoCard.hpp"

namespace {
    QString serverTreeUpdatedText(qint64 lastUpdate) {
        if (lastUpdate <= 0) return ServerTreeModel::tr("not updated yet");
        const qint64 age = QDateTime::currentSecsSinceEpoch() - lastUpdate;
        if (age < 60) return ServerTreeModel::tr("updated just now");
        return ServerTreeModel::tr("updated %1 ago").arg(Stats::HumanizeDuration(age));
    }

    ServerTreeModel::Server serverTreeEntry(const std::shared_ptr<Configs::Profile> &profile) {
        return {profile->id, profile->outbound ? profile->outbound->DisplayName() : profile->name, profile->test_country, profile->latency};
    }
} // namespace

ServerTreeModel::ServerTreeModel(QObject *parent) : QAbstractItemModel(parent) {}

void ServerTreeModel::reload() {
    beginResetModel();
    m_groups.clear();
    for (const int gid : Configs::dataManager->groupsRepo->GetGroupsTabOrder()) {
        const auto source = Configs::dataManager->groupsRepo->GetGroup(gid);
        if (source == nullptr || source->url.isEmpty()) continue;
        Group group;
        group.id = gid;
        group.name = source->name;
        const auto ids = source->Profiles();
        group.subtitle = tr("%n server(s)", nullptr, static_cast<int>(ids.size())) + QStringLiteral(" · ") + serverTreeUpdatedText(source->sub_last_update);
        const auto &info = source->sub_info;
        if (info.has_quota) {
            group.quota = info.total > 0 ? tr("%1 of %2").arg(ReadableSize(info.used()), ReadableSize(info.total)) : ReadableSize(info.used());
            if (info.total > 0) group.usedShare = static_cast<double>(info.used()) / static_cast<double>(info.total);
        }
        if (info.expire > 0) group.expiry = SubscriptionInfoCard::expiryText(info.expire);
        group.autoId = Engine::AutoGroupService::Find(gid);
        group.autoAvailable = group.autoId >= 0 || Engine::AutoGroupService::HasEligibleServers(gid);
        for (const auto &profile : Configs::dataManager->profilesRepo->GetProfileBatch(ids)) {
            if (profile != nullptr) group.servers << serverTreeEntry(profile);
        }
        m_groups << group;
    }
    endResetModel();
}

QModelIndex ServerTreeModel::indexOfProfile(int id) const {
    for (int g = 0; g < m_groups.size(); g++) {
        const auto parentIndex = index(g, 0);
        const int offset = m_groups[g].autoAvailable ? 1 : 0;
        if (offset == 1 && m_groups[g].autoId == id) return index(0, 0, parentIndex);
        for (int s = 0; s < m_groups[g].servers.size(); s++) {
            if (m_groups[g].servers[s].id == id) return index(s + offset, 0, parentIndex);
        }
    }
    return {};
}

void ServerTreeModel::refreshProfiles(const QList<int> &ids) {
    for (const int id : ids) {
        const auto modelIndex = indexOfProfile(id);
        if (!modelIndex.isValid() || kindOf(modelIndex) != Kind::Server) continue;
        const auto profile = Configs::dataManager->profilesRepo->GetProfile(id);
        if (profile == nullptr) continue;
        const int offset = m_groups[modelIndex.parent().row()].autoAvailable ? 1 : 0;
        m_groups[modelIndex.parent().row()].servers[modelIndex.row() - offset] = serverTreeEntry(profile);
        emit dataChanged(modelIndex, modelIndex);
    }
}

void ServerTreeModel::setSelectedId(int id) {
    if (id == m_selectedId) return;
    const auto before = indexOfProfile(m_selectedId);
    m_selectedId = id;
    if (before.isValid()) emit dataChanged(before, before);
    if (const auto after = indexOfProfile(id); after.isValid()) emit dataChanged(after, after);
}

void ServerTreeModel::setRunningId(int id) {
    if (id == m_runningId) return;
    const auto before = indexOfProfile(m_runningId);
    m_runningId = id;
    if (before.isValid()) emit dataChanged(before, before);
    if (const auto after = indexOfProfile(id); after.isValid()) emit dataChanged(after, after);
}

ServerTreeModel::Kind ServerTreeModel::kindOf(const QModelIndex &modelIndex) const {
    if (!modelIndex.parent().isValid()) return modelIndex.row() < m_groups.size() ? Kind::Group : Kind::AddRow;
    return m_groups[modelIndex.parent().row()].autoAvailable && modelIndex.row() == 0 ? Kind::Auto : Kind::Server;
}

const ServerTreeModel::Group &ServerTreeModel::groupOf(const QModelIndex &modelIndex) const {
    return m_groups[modelIndex.parent().isValid() ? modelIndex.parent().row() : modelIndex.row()];
}

const ServerTreeModel::Server &ServerTreeModel::serverOf(const QModelIndex &modelIndex) const {
    const auto &group = groupOf(modelIndex);
    return group.servers[modelIndex.row() - (group.autoAvailable ? 1 : 0)];
}

bool ServerTreeModel::isLastInGroup(const QModelIndex &modelIndex) const {
    return modelIndex.row() == rowCount(modelIndex.parent()) - 1;
}

QModelIndex ServerTreeModel::index(int row, int column, const QModelIndex &parentIndex) const {
    if (!hasIndex(row, column, parentIndex)) return {};
    return createIndex(row, column, parentIndex.isValid() ? quintptr(parentIndex.row() + 1) : quintptr(0));
}

QModelIndex ServerTreeModel::parent(const QModelIndex &child) const {
    if (!child.isValid() || child.internalId() == 0) return {};
    return createIndex(static_cast<int>(child.internalId()) - 1, 0, quintptr(0));
}

int ServerTreeModel::rowCount(const QModelIndex &parentIndex) const {
    if (!parentIndex.isValid()) return m_groups.isEmpty() ? 0 : m_groups.size() + 1;
    if (parentIndex.internalId() != 0 || parentIndex.row() >= m_groups.size()) return 0;
    const auto &group = m_groups[parentIndex.row()];
    return group.servers.size() + (group.autoAvailable ? 1 : 0);
}

int ServerTreeModel::columnCount(const QModelIndex &) const {
    return 1;
}

QVariant ServerTreeModel::data(const QModelIndex &modelIndex, int role) const {
    if (!modelIndex.isValid()) return {};
    const Kind kind = kindOf(modelIndex);
    switch (role) {
        case KindRole: return static_cast<int>(kind);
        case GroupIdRole: return kind == Kind::AddRow ? -1 : groupOf(modelIndex).id;
        case ProfileIdRole:
            if (kind == Kind::Server) return serverOf(modelIndex).id;
            return kind == Kind::Auto ? groupOf(modelIndex).autoId : -1;
        case Qt::DisplayRole:
            if (kind == Kind::Server) return serverOf(modelIndex).name;
            if (kind == Kind::Group) return groupOf(modelIndex).name;
            return {};
        default: return {};
    }
}

Qt::ItemFlags ServerTreeModel::flags(const QModelIndex &modelIndex) const {
    return modelIndex.isValid() ? Qt::ItemIsEnabled : Qt::NoItemFlags;
}
