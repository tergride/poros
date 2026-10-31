#include "include/configs/PresetCatalog.h"

#include <QCoreApplication>
#include <QJsonDocument>

#include "include/database/RoutesRepo.h"
#include "include/global/Configs.hpp"

extern QString ReadFileText(const QString &path);

namespace {
    using Configs::RouteProfile;

    std::shared_ptr<RouteProfile> presetCatalogLoad(const QString &key) {
        QString error;
        QString warnings;
        const QString text = ReadFileText(QStringLiteral(":/poros/presets/%1.json").arg(key));
        auto preset = RouteProfile::FromShareInput(text, &error, &warnings, nullptr, false);
        if (preset == nullptr) MW_show_log(QStringLiteral("Could not read the built-in preset %1: %2").arg(key, error));
        else if (!warnings.isEmpty()) MW_show_log(QStringLiteral("Built-in preset %1: %2").arg(key, warnings.trimmed()));
        return preset;
    }

    QByteArray presetCatalogContent(RouteProfile &profile) {
        return QJsonDocument(profile.ToShareObject()).toJson(QJsonDocument::Compact);
    }

    // The Default profile as it was first written: anything else is the user's and stays theirs.
    bool presetCatalogIsPristineDefault(const RouteProfile &profile) {
        if (profile.isRaw || profile.isRemote || profile.customDns || !profile.endpointProfileIDs.isEmpty() || profile.defaultOutboundID != Configs::proxyID) return false;
        if (profile.Rules.size() != 1) return false;
        const auto &rule = profile.Rules.first();
        return rule->action == QLatin1String("hijack-dns") && rule->protocol == QLatin1String("dns");
    }
} // namespace

namespace Configs::PresetCatalog {
    QStringList Keys() {
        return {QStringLiteral("all"), QStringLiteral("except-ru"), QStringLiteral("blocked-only"), QStringLiteral("hardened")};
    }

    QString DisplayName(const QString &key) {
        if (key == QLatin1String("all")) return QCoreApplication::translate("PresetCatalog", "All traffic");
        if (key == QLatin1String("except-ru")) return QCoreApplication::translate("PresetCatalog", "Except Russian sites");
        if (key == QLatin1String("blocked-only")) return QCoreApplication::translate("PresetCatalog", "Blocked sites only");
        if (key == QLatin1String("hardened")) return QCoreApplication::translate("PresetCatalog", "Hardened bypass");
        return key;
    }

    QString Description(const QString &key) {
        if (key == QLatin1String("all")) return QCoreApplication::translate("PresetCatalog", "Everything goes through the server, local networks stay direct.");
        if (key == QLatin1String("except-ru")) return QCoreApplication::translate("PresetCatalog", "Russian sites open directly, everything else goes through the server.");
        if (key == QLatin1String("blocked-only")) return QCoreApplication::translate("PresetCatalog", "Only blocked sites go through the server, everything else stays direct.");
        if (key == QLatin1String("hardened")) return QCoreApplication::translate("PresetCatalog", "Like All traffic, with TLS fragmentation. Try it when the connection keeps breaking.");
        return {};
    }

    void Ensure() {
        const auto &routes = dataManager->routesRepo;
        const auto existing = routes->GetAllRouteProfiles();
        for (const auto &key : Keys()) {
            const auto preset = presetCatalogLoad(key);
            if (preset == nullptr) continue;
            preset->builtinKey = key;
            preset->name = DisplayName(key);

            std::shared_ptr<RouteProfile> current;
            for (const auto &profile : existing) {
                if (profile->builtinKey == key) current = profile;
            }
            if (current == nullptr && key == QLatin1String("all")) {
                for (const auto &profile : existing) {
                    if (presetCatalogIsPristineDefault(*profile)) {
                        current = profile;
                        break;
                    }
                }
            }
            if (current == nullptr) {
                auto added = preset;
                routes->AddRouteProfile(added);
                continue;
            }
            if (current->builtinKey == key && presetCatalogContent(*current) == presetCatalogContent(*preset)) continue;
            current->builtinKey = key;
            current->name = preset->name;
            current->Rules = preset->Rules;
            current->defaultOutboundID = preset->defaultOutboundID;
            current->hardened = preset->hardened;
            routes->Save(current);
        }
    }
} // namespace Configs::PresetCatalog
