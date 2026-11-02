#include "include/engine/services/ProfileService.h"

#include <QSet>

#include "include/database/DatabaseManager.h"
#include "include/database/GroupsRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/engine/EngineEvents.h"
#include "include/engine/ThreadCheck.h"
#include "include/global/Utils.hpp"

namespace Engine::ProfileService {
    int RemoveUnavailable(QList<int> profileIds) {
        ENGINE_CHECK_MAIN_THREAD();
        const auto group = Configs::dataManager->groupsRepo->CurrentGroup();
        if (!group) return 0;

        if (profileIds.isEmpty()) profileIds = group->Profiles();

        QList<int> del_ids;
        for (const auto &profile : Configs::dataManager->profilesRepo->GetProfileBatch(profileIds)) {
            if (profile->IsUnavailable()) del_ids += profile->id;
        }
        if (del_ids.isEmpty()) return 0;

        // Strips started_id from del_ids, so the running profile survives.
        if (!Configs::dataManager->profilesRepo->BatchDeleteProfiles(del_ids)) return 0;
        if (del_ids.isEmpty()) return 0;
        EngineEvents::ProfilesRemoved(del_ids);
        return static_cast<int>(del_ids.size());
    }

    int ClearTestResults(const QList<int> &profileIds, QList<int> *groupIds) {
        ENGINE_CHECK_MAIN_THREAD();
        if (groupIds != nullptr) groupIds->clear();
        const auto ents = Configs::dataManager->profilesRepo->GetProfileBatch(profileIds);
        if (ents.empty()) return 0;
        QSet<int> gids;
        for (const auto &ent: ents) {
            ent->ClearTestResults();
            gids.insert(ent->gid);
        }
        Configs::dataManager->profilesRepo->SaveBatch(ents);
        if (groupIds != nullptr) *groupIds = gids.values();
        return static_cast<int>(ents.size());
    }
} // namespace Engine::ProfileService
