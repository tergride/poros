#pragma once

#include <QString>
#include <memory>

#include "include/database/entities/RouteProfile.h"

namespace RouteUpdate {
    // A routing profile, even a large one, is a fraction of this.
    constexpr qint64 kMaxBytes = 16LL * 1024 * 1024;

    // Blocks on the network (worker thread only); returns an error string, empty on success.
    QString UpdateProfile(const std::shared_ptr<Configs::RouteProfile>& profile, QString* warnings = nullptr);
}

// Runs the whole batch on a worker thread and saves each result; the active route is not reloaded.
void UI_update_all_remote_routes(bool onlyAllowed = false);
