#include "include/engine/services/AutoGroupService.h"

#include <QCoreApplication>

#include "include/configs/AutoSelectorPlan.h"
#include "include/configs/outbounds/autoselector.h"
#include "include/database/DatabaseManager.h"
#include "include/database/GroupsRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/engine/EngineEvents.h"
#include "include/engine/ThreadCheck.h"
#include "include/global/Configs.hpp"

namespace {
    int autoGroupHiddenId() {
        auto *settings = Configs::dataManager->settingsRepo.get();
        if (settings->auto_group_id >= 0 && Configs::dataManager->groupsRepo->GetGroup(settings->auto_group_id) != nullptr) return settings->auto_group_id;
        auto group = Configs::GroupsRepo::NewGroup();
        group->name = QStringLiteral("Auto");
        if (!Configs::dataManager->groupsRepo->AddHiddenGroup(group)) return -1;
        settings->auto_group_id = group->id;
        settings->Save();
        return group->id;
    }

    std::shared_ptr<Configs::Profile> autoGroupNewProfile(int groupId) {
        auto profile = Configs::ProfilesRepo::NewProfile(QStringLiteral("autoselector"));
        auto *selector = profile->AutoSelector();
        selector->gids = {groupId};
        selector->name = QCoreApplication::translate("AutoGroupService", "Auto");
        profile->name = selector->name;
        return profile;
    }
} // namespace

namespace Engine::AutoGroupService {
    int Find(int groupId) {
        for (const int id : Configs::dataManager->profilesRepo->GetProfileIdsByType(QStringLiteral("autoselector"))) {
            const auto profile = Configs::dataManager->profilesRepo->GetProfile(id);
            if (profile == nullptr || profile->gid != Configs::dataManager->settingsRepo->auto_group_id) continue;
            if (const auto *selector = profile->AutoSelector(); selector != nullptr && selector->gids == QList<int>{groupId}) return id;
        }
        return -1;
    }

    bool HasEligibleServers(int groupId) {
        return Configs::PlanAutoSelector(autoGroupNewProfile(groupId)).eligible > 0;
    }

    int Ensure(int groupId) {
        ENGINE_CHECK_MAIN_THREAD();
        if (const int existing = Find(groupId); existing >= 0) return existing;
        if (!HasEligibleServers(groupId)) return -1;
        const int hidden = autoGroupHiddenId();
        if (hidden < 0) return -1;
        auto profile = autoGroupNewProfile(groupId);
        if (!Configs::dataManager->profilesRepo->AddProfile(profile, hidden)) return -1;
        return profile->id;
    }

    void Remove(int groupId) {
        ENGINE_CHECK_MAIN_THREAD();
        QList<int> ids{Find(groupId)};
        if (ids.first() >= 0) Configs::dataManager->profilesRepo->BatchDeleteProfiles(ids);
    }
} // namespace Engine::AutoGroupService
