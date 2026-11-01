#include "include/global/Configs.hpp"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QStandardPaths>
#include <utility>
#include <include/api/RPC.h>

#include "include/database/GroupsRepo.h"
#include "include/database/RoutesRepo.h"

#include "include/sys/windows/guihelper.h"

#include "include/configs/PresetCatalog.h"

namespace Configs {
    void initDB(const std::string& dbPath) {
        dataManager = new DatabaseManager(dbPath);

        if (dataManager->groupsRepo->GetAllGroupIds().empty()) {
            auto defaultGroup = GroupsRepo::NewGroup();
            defaultGroup->name = QObject::tr("Default");
            dataManager->groupsRepo->AddGroup(defaultGroup);
        }
        // The window repairs this when it lists the groups; until then imports would target no group.
        if (dataManager->groupsRepo->CurrentGroup() == nullptr) {
            if (const auto order = dataManager->groupsRepo->GetGroupsTabOrder(); !order.isEmpty()) {
                dataManager->settingsRepo->current_group = order.first();
                dataManager->settingsRepo->Save();
            }
        }
        if (dataManager->routesRepo->GetAllRouteProfileIds().empty()) {
            auto defaultRoute = RouteProfile::GetDefaultChain();
            dataManager->routesRepo->AddRouteProfile(defaultRoute);
        }
        PresetCatalog::Ensure();
    }

    QString FindCoreRealPath() {
        auto fn = QCoreApplication::applicationDirPath() + "/PorosCore";
        fn += ".exe";
        auto fi = QFileInfo(fn);
        QString path;
        if (fi.isSymLink()) path =  fi.symLinkTarget();
        path = fn;
        path.replace("/", "\\");
        return path;
    }

    short isAdminCache = -1;

    // IsAdmin 主要判断：有无权限启动 Tun
    bool IsAdmin(bool forceRenew) {
        if (isAdminCache >= 0 && !forceRenew) return isAdminCache;

        bool admin = false;
        admin = Windows_IsInAdmin();
        Configs::dataManager->settingsRepo->windows_set_admin = admin;
        isAdminCache = admin;
        return admin;
    }

    std::optional<bool> KnownAdmin() {
        if (isAdminCache >= 0) return isAdminCache != 0;
        // Local and cheap; unlike IsAdmin it leaves windows_set_admin alone.
        return Windows_IsInAdmin();
    }

    namespace {
        QString basePathOverride;
    } // namespace

    void SetBasePath(const QString &path) {
        basePathOverride = path;
    }

    QString GetBasePath() {
        if (!basePathOverride.isEmpty()) return basePathOverride;
        if (Configs::dataManager->settingsRepo->flag_use_appdata) return QStandardPaths::writableLocation(
              QStandardPaths::AppConfigLocation);
        return QCoreApplication::applicationDirPath();
    }
} // namespace Configs
