#pragma once

#include <QList>

namespace Engine::ProfileService {
    // Main thread. Deletes the unavailable ones among profileIds (the current group's when empty), never the running
    // profile, and emits ProfilesRemoved for them; no SubscriptionGroupChanged. Returns how many it deleted.
    int RemoveUnavailable(QList<int> profileIds);

    // Main thread. Clears the test results of the profiles; *groupIds gets their groups. Returns how many were cleared.
    int ClearTestResults(const QList<int> &profileIds, QList<int> *groupIds = nullptr);
} // namespace Engine::ProfileService
