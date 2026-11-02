#pragma once

#include "Const.hpp"
#include "Utils.hpp"
#include "include/database/DatabaseManager.h"

#include <array>
#include <optional>
#include <string_view>
#include <utility>

namespace API {
}

namespace Configs {
    void initDB(const std::string& dbPath);

    QString FindCoreRealPath();

    bool IsAdmin(bool forceRenew=false);

    // The cached IsAdmin answer without an RPC or side effects; empty while unknown. Main thread.
    std::optional<bool> KnownAdmin();

    // Set once at startup to the data folder: the program folder, or the one -appdata names. Before that, the program folder.
    void SetBasePath(const QString &path);

    QString GetBasePath();
} // namespace Configs

#define ROUTES_PREFIX_NAME QString("route_profiles")
#define ROUTES_PREFIX QString(ROUTES_PREFIX_NAME + "/")
