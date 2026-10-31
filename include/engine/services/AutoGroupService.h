#pragma once

// One Auto profile per subscription, kept in a hidden group: a subscription update would delete it from its own.
namespace Engine::AutoGroupService {
    // The subscription's Auto profile, or -1 while it has none.
    int Find(int groupId);

    // Whether Auto has anything to pick from there.
    bool HasEligibleServers(int groupId);

    // Find, creating the profile on first use. -1 when the subscription has no eligible server.
    int Ensure(int groupId);

    void Remove(int groupId);
} // namespace Engine::AutoGroupService
