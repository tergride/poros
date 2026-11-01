// poros_unit: the logic that decides what a user ends up with. Built with -DPOROS_BUILD_TESTS=ON, from the same sources as the app.
#include <QApplication>
#include <QDir>
#include <QJsonObject>
#include <QTemporaryDir>
#include <QTest>

#include "include/configs/AutoSelectorPlan.h"
#include "include/configs/PresetCatalog.h"
#include "include/configs/PresetRules.h"
#include "include/configs/outbounds/autoselector.h"
#include "include/configs/outbounds/hysteria.h"
#include "include/configs/outbounds/vless.h"
#include "include/configs/sub/LinkKind.h"
#include "include/database/Database.h"
#include "include/database/GroupsRepo.h"
#include "include/database/OtpProfilesRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/database/RoutesRepo.h"
#include "include/database/TrafficStatsRepo.h"
#include "include/engine/app/StartupElevation.h"
#include "include/engine/services/AutoGroupService.h"
#include "include/engine/services/GroupService.h"
#include "include/engine/services/ReleaseUpdates.h"
#include "include/global/Configs.hpp"
#include "include/ui/utils/ConnectionModeController.h"

namespace {
    std::shared_ptr<Configs::RouteProfile> routeByKey(const QString &key) {
        for (const auto &route : Configs::dataManager->routesRepo->GetAllRouteProfiles()) {
            if (route->builtinKey == key) return route;
        }
        return nullptr;
    }

    QStringList ruleSets(const std::shared_ptr<Configs::RouteProfile> &route) {
        QStringList out;
        for (const auto &rule : route->Rules) out << rule->rule_set;
        return out;
    }
} // namespace

class PorosUnit : public QObject {
    Q_OBJECT

private:
    QTemporaryDir dir;

    void openDatabase() {
        delete Configs::dataManager;
        Configs::dataManager = nullptr;
        Configs::initDB(dir.filePath("poros.db").toStdString());
    }

private slots:
    void initTestCase() {
        QVERIFY(dir.isValid());
        openDatabase();
    }

    void cleanupTestCase() {
        delete Configs::dataManager;
        Configs::dataManager = nullptr;
    }

    void elevationAtStartup_data() {
        QTest::addColumn<bool>("lastRunElevated");
        QTest::addColumn<bool>("disabled");
        QTest::addColumn<bool>("rememberEnabled");
        QTest::addColumn<bool>("rememberTun");
        QTest::addColumn<bool>("killSwitch");
        QTest::addColumn<bool>("expected");
        QTest::newRow("system proxy, last run elevated") << true << false << false << false << false << false;
        QTest::newRow("tun without auto connect") << true << false << false << true << false << false;
        QTest::newRow("tun with auto connect") << true << false << true << true << false << true;
        QTest::newRow("kill switch") << true << false << false << false << true << true;
        QTest::newRow("never elevated") << false << false << true << true << true << false;
        QTest::newRow("elevation refused in settings") << true << true << true << true << true << false;
    }

    void elevationAtStartup() {
        QFETCH(bool, lastRunElevated);
        QFETCH(bool, disabled);
        QFETCH(bool, rememberEnabled);
        QFETCH(bool, rememberTun);
        QFETCH(bool, killSwitch);
        QFETCH(bool, expected);
        QCOMPARE(Engine::NeedsElevationAtStartup({lastRunElevated, disabled, rememberEnabled, rememberTun, killSwitch}), expected);
    }

    void presetsAreSeeded() {
        for (const auto &key : Configs::PresetCatalog::Keys()) {
            const auto route = routeByKey(key);
            QVERIFY2(route != nullptr, qPrintable(key));
            QVERIFY(route->IsBuiltin());
        }
        QCOMPARE(routeByKey("all")->id, Configs::dataManager->settingsRepo->current_route_id);
        QCOMPARE(routeByKey("all")->defaultOutboundID, int(Configs::proxyID));
        QCOMPARE(routeByKey("blocked-only")->defaultOutboundID, int(Configs::directID));
        QCOMPARE(ruleSets(routeByKey("except-ru")), QStringList({"geosite-ru", "geoip-ru"}));
        QCOMPARE(ruleSets(routeByKey("blocked-only")), QStringList({"geosite-ru-blocked", "geoip-ru-blocked"}));
        QVERIFY(routeByKey("hardened")->hardened);
        QVERIFY(!routeByKey("all")->hardened);
        QCOMPARE(Configs::dataManager->routesRepo->GetAllRouteProfileIds().size(), Configs::PresetCatalog::Keys().size());
    }

    void presetsSurviveAgainAndRepairThemselves() {
        Configs::PresetCatalog::Ensure();
        QCOMPARE(Configs::dataManager->routesRepo->GetAllRouteProfileIds().size(), Configs::PresetCatalog::Keys().size());

        auto route = routeByKey("except-ru");
        const int id = route->id;
        route->Rules.clear();
        route->defaultOutboundID = Configs::directID;
        Configs::dataManager->routesRepo->Save(route);

        Configs::PresetCatalog::Ensure();
        const auto repaired = routeByKey("except-ru");
        QCOMPARE(repaired->id, id);
        QCOMPARE(repaired->defaultOutboundID, int(Configs::proxyID));
        QCOMPARE(ruleSets(repaired), QStringList({"geosite-ru", "geoip-ru"}));
    }

    void changedDefaultStaysTheUsers() {
        QTemporaryDir other;
        QVERIFY(other.isValid());
        delete Configs::dataManager;
        Configs::dataManager = new Configs::DatabaseManager(other.filePath("poros.db").toStdString());
        auto mine = Configs::RouteProfile::GetDefaultChain();
        mine->name = "Mine";
        mine->defaultOutboundID = Configs::directID;
        Configs::dataManager->routesRepo->AddRouteProfile(mine);

        Configs::PresetCatalog::Ensure();

        const auto kept = Configs::dataManager->routesRepo->GetRouteProfile(mine->id);
        QCOMPARE(kept->name, QString("Mine"));
        QVERIFY(!kept->IsBuiltin());
        QVERIFY(routeByKey("all") != nullptr);
        QVERIFY(routeByKey("all")->id != mine->id);
        openDatabase();
    }

    void copyOfPresetIsTheUsers() {
        const auto original = routeByKey("hardened");
        const auto rules = original->Rules.size();

        auto copy = std::make_shared<Configs::RouteProfile>(*original);
        copy->id = -1;
        copy->DetachFromBuiltin();
        copy->Rules.clear();
        Configs::dataManager->routesRepo->AddRouteProfile(copy);

        QVERIFY(!copy->IsBuiltin());
        QCOMPARE(copy->baseKey, QString("hardened"));
        QVERIFY(copy->hardened);
        QCOMPARE(original->Rules.size(), rules);
        QVERIFY(routeByKey("hardened")->IsBuiltin());

        const auto stored = Configs::dataManager->routesRepo->GetRouteProfile(copy->id);
        QVERIFY(stored->hardened);
        QVERIFY(Configs::dataManager->routesRepo->IsHardened(copy->id));
        Configs::dataManager->routesRepo->DeleteRouteProfile(copy->id);
    }

    void hardenedTravelsWithTheShareObject() {
        const auto object = routeByKey("hardened")->ToShareObject();
        QVERIFY(object.value("hardened").toBool());
        QVERIFY(!routeByKey("all")->ToShareObject().contains("hardened"));
    }

    void connectionModeResolves_data() {
        QTest::addColumn<QString>("saved");
        QTest::addColumn<bool>("tun");
        QTest::addColumn<bool>("system");
        QTest::addColumn<int>("expected");
        using Mode = ConnectionModeController::Mode;
        QTest::newRow("nothing chosen, nothing on") << "" << false << false << int(Mode::Unset);
        QTest::newRow("nothing chosen, both on") << "" << true << true << int(Mode::Unset);
        QTest::newRow("only tun on") << "" << true << false << int(Mode::Tun);
        QTest::newRow("only system on") << "" << false << true << int(Mode::System);
        QTest::newRow("saved tun wins over the switches") << "tun" << false << true << int(Mode::Tun);
        QTest::newRow("saved system with both on") << "system" << true << true << int(Mode::System);
    }

    void connectionModeResolves() {
        QFETCH(QString, saved);
        QFETCH(bool, tun);
        QFETCH(bool, system);
        QFETCH(int, expected);
        QCOMPARE(int(ConnectionModeController::Resolve(saved, tun, system)), expected);
    }

    void linkKinds_data() {
        QTest::addColumn<QString>("link");
        QTest::addColumn<int>("expected");
        using Subscription::LinkKind;
        QTest::newRow("empty") << "   " << int(LinkKind::Empty);
        QTest::newRow("https") << " https://sub.example.com/s/1 " << int(LinkKind::Subscription);
        QTest::newRow("http") << "HTTP://sub.example.com/s/1" << int(LinkKind::Subscription);
        QTest::newRow("vless") << "vless://id@de1.example.com:443" << int(LinkKind::SingleServer);
        QTest::newRow("hysteria2") << "hysteria2://pass@pl1.example.com:8443" << int(LinkKind::SingleServer);
        QTest::newRow("words") << "just some words" << int(LinkKind::Unknown);
        QTest::newRow("url with a space") << "https://a b" << int(LinkKind::Unknown);
    }

    void linkKinds() {
        QFETCH(QString, link);
        QFETCH(int, expected);
        QCOMPARE(int(Subscription::ClassifyLink(link)), expected);
    }

    void autoProfileLivesAndDiesWithItsSubscription() {
        auto &groups = Configs::dataManager->groupsRepo;
        auto &profiles = Configs::dataManager->profilesRepo;
        const auto orderBefore = groups->GetGroupsTabOrder();

        int gid = -1;
        QVERIFY(!Engine::GroupService::Add({"Sub", "https://sub.example.com/s/2", false}, &gid).has_value());

        QCOMPARE(Engine::AutoGroupService::Ensure(gid), -1);

        auto server = Configs::ProfilesRepo::NewProfile("socks");
        server->outbound->server = "198.51.100.1";
        server->outbound->server_port = 1080;
        QVERIFY(profiles->AddProfile(server, gid));
        QVERIFY(Engine::AutoGroupService::HasEligibleServers(gid));

        const int autoId = Engine::AutoGroupService::Ensure(gid);
        QVERIFY(autoId >= 0);
        QCOMPARE(Engine::AutoGroupService::Ensure(gid), autoId);
        QCOMPARE(Engine::AutoGroupService::Find(gid), autoId);

        const int hidden = Configs::dataManager->settingsRepo->auto_group_id;
        QCOMPARE(profiles->GetProfile(autoId)->gid, hidden);
        QVERIFY(!groups->GetGroupsTabOrder().contains(hidden));
        QVERIFY(!groups->GetGroup(gid)->Profiles().contains(autoId));

        QVERIFY(!Engine::GroupService::Delete(gid).has_value());
        QCOMPARE(Engine::AutoGroupService::Find(gid), -1);
        QVERIFY(profiles->GetProfile(autoId) == nullptr);
        QCOMPARE(groups->GetGroupsTabOrder(), orderBefore);
    }

    void autoSkipsWhatItCannotUse() {
        int gid = -1;
        QVERIFY(!Engine::GroupService::Add({"Vpn only", "https://sub.example.com/s/3", false}, &gid).has_value());
        auto server = Configs::ProfilesRepo::NewProfile("openvpn");
        QVERIFY(Configs::dataManager->profilesRepo->AddProfile(server, gid));
        QVERIFY(!Engine::AutoGroupService::HasEligibleServers(gid));
        QCOMPARE(Engine::AutoGroupService::Ensure(gid), -1);
        QVERIFY(!Engine::GroupService::Delete(gid).has_value());
    }

    void autoSelectorStartsOnWorkingMembers() {
        int gid = -1;
        QVERIFY(!Engine::GroupService::Add({"Pool", "https://sub.example.com/s/4", false}, &gid).has_value());
        auto &profiles = Configs::dataManager->profilesRepo;
        QList<std::shared_ptr<Configs::Profile>> members;
        for (int i = 0; i < 4; ++i) {
            auto server = Configs::ProfilesRepo::NewProfile("socks");
            server->outbound->server = QStringLiteral("198.51.100.%1").arg(i + 1);
            server->outbound->server_port = 1080;
            QVERIFY(profiles->AddProfile(server, gid));
            members << server;
        }
        members[0]->SetLatency(300);
        members[1]->SetLatency(100);
        QVERIFY(profiles->Save(members[0]));
        QVERIFY(profiles->Save(members[1]));

        auto ent = Configs::ProfilesRepo::NewProfile("autoselector");
        auto *selector = ent->AutoSelector();
        selector->gids = {gid};
        selector->expected = 2;
        selector->activeSize = 2;
        selector->buildLimit = 2;
        QVERIFY(profiles->AddProfile(ent, gid));
        QCOMPARE(Configs::AutoSelectorStartQuorum(ent), 2);

        int working = 0;
        const auto needed = Configs::AutoSelectorUnmeasuredCandidates(ent, {members[0]->id}, &working);
        QCOMPARE(needed.size(), 3);
        QVERIFY(!needed.contains(members[1]->id));
        QCOMPARE(working, 1);

        QCOMPARE(Configs::RerankAutoSelectorPool(ent, {members[1]->id}).first(), members[0]->id);
        QCOMPARE(Configs::RerankAutoSelectorPool(ent).first(), members[1]->id);

        selector->lastBuilt = {members[1]->id, members[0]->id};
        QVERIFY(!Configs::AutoSelectorBuildChanged(ent));
        selector->lastBuilt = {members[1]->id, members[2]->id};
        QVERIFY(Configs::AutoSelectorBuildChanged(ent));

        QVERIFY(!Engine::GroupService::Delete(gid).has_value());
    }

    void hysteriaLinkWithUnitsOrNoSpeeds() {
        auto given = Configs::ProfilesRepo::NewProfile("hysteria");
        QVERIFY(given->outbound->ParseFromLink("hysteria://h.example.com:443?upmbps=55Mbps&downmbps=100"));
        const auto built = given->outbound->Build();
        QCOMPARE(built.object["up_mbps"].toInt(), 55);
        QCOMPARE(built.object["down_mbps"].toInt(), 100);

        auto missing = Configs::ProfilesRepo::NewProfile("hysteria");
        QVERIFY(missing->outbound->ParseFromLink("hysteria://h.example.com:443"));
        const auto defaults = missing->outbound->Build();
        QCOMPARE(defaults.object["up_mbps"].toInt(), Configs::kHysteriaV1DefaultUpMbps);
        QCOMPARE(defaults.object["down_mbps"].toInt(), Configs::kHysteriaV1DefaultDownMbps);
    }

    void vlessLinkKeepsOnlyAUsableFlow_data() {
        QTest::addColumn<QString>("query");
        QTest::addColumn<QString>("flow");
        QTest::newRow("unknown") << "security=tls&flow=bogus" << "";
        QTest::newRow("vision without tls") << "flow=xtls-rprx-vision" << "";
        QTest::newRow("vision over tls") << "security=tls&flow=xtls-rprx-vision" << "xtls-rprx-vision";
    }

    void vlessLinkKeepsOnlyAUsableFlow() {
        QFETCH(QString, query);
        QFETCH(QString, flow);
        auto profile = Configs::ProfilesRepo::NewProfile("vless");
        QVERIFY(profile->outbound->ParseFromLink("vless://11111111-2222-3333-4444-555555555555@v.example.com:443?" + query));
        QCOMPARE(dynamic_cast<Configs::vless *>(profile->outbound.get())->flow, flow);
    }

    void presetLinesFromWhatTheUserTypes_data() {
        QTest::addColumn<QString>("typed");
        QTest::addColumn<QString>("line");
        QTest::newRow("domain") << "Example.ORG" << "suffix:example.org";
        QTest::newRow("url") << "https://video.example.ru/watch?v=1" << "suffix:video.example.ru";
        QTest::newRow("path") << "example.org/page" << "suffix:example.org";
        QTest::newRow("wildcard") << "*.example.org" << "suffix:example.org";
        QTest::newRow("ip") << "203.0.113.7" << "ip:203.0.113.7";
        QTest::newRow("subnet") << "192.168.50.0/24" << "ip:192.168.50.0/24";
        QTest::newRow("empty") << "   " << "";
    }

    void presetLinesFromWhatTheUserTypes() {
        QFETCH(QString, typed);
        QFETCH(QString, line);
        QCOMPARE(Configs::PresetRules::SiteLine(typed), line);
    }

    void userRulesBeatTheirPreset() {
        using namespace Configs::PresetRules;
        auto copy = std::make_shared<Configs::RouteProfile>(*routeByKey("except-ru"));
        copy->id = -1;
        copy->DetachFromBuiltin();
        const auto locked = LockedRules(*copy);
        QCOMPARE(locked, QStringList({"Local networks", "Russian sites"}));

        const QList<UserRule> rules = {
            {"video.example.ru", Action::Proxy, false},
            {"203.0.113.0/24", Action::Block, false},
            {"qbittorrent.exe", Action::Direct, true},
        };
        QVERIFY(Write(*copy, rules).isEmpty());

        // The site is .ru: it reaches the server only because it is matched ahead of the preset's Russian sites.
        QCOMPARE(copy->Rules.first()->action, QString("hijack-dns"));
        int ownRule = -1;
        int russianSites = -1;
        for (int i = 0; i < copy->Rules.size(); i++) {
            if (copy->Rules[i]->domain_suffix.contains("video.example.ru")) ownRule = i;
            if (copy->Rules[i]->rule_set.contains("geosite-ru")) russianSites = i;
        }
        QVERIFY(ownRule >= 0);
        QVERIFY(russianSites > ownRule);

        const auto back = Read(*copy);
        QCOMPARE(back.size(), rules.size());
        for (const auto &rule : rules) QVERIFY2(back.contains(rule), qPrintable(rule.text));
        QCOMPARE(LockedRules(*copy), locked);

        QVERIFY(Write(*copy, {}).isEmpty());
        QVERIFY(Read(*copy).isEmpty());
        QCOMPARE(LockedRules(*copy), locked);
        QVERIFY(routeByKey("except-ru")->IsBuiltin());
        QCOMPARE(ruleSets(routeByKey("except-ru")), QStringList({"geosite-ru", "geoip-ru"}));
    }

    void newerVersionsCompareAsNumbers() {
        QVERIFY(Engine::IsNewerVersion("0.10.0", "0.9.0"));
        QVERIFY(Engine::IsNewerVersion("1.0.0", "0.99.9"));
        QVERIFY(Engine::IsNewerVersion("1.0.1", "1.0"));
        QVERIFY(!Engine::IsNewerVersion("0.1.0", "0.1.0"));
        QVERIFY(!Engine::IsNewerVersion("0.1.0", "0.2.0"));
    }

    void releaseFeedReadsAPublishedRelease() {
        const auto feed = Engine::ParseReleaseFeed(R"({"version": "0.2.0", "file": "Poros-0.2.0-windows-amd64.zip", "size": 42,
            "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "notes": {"ru": "Новое", "en": "New"}})");
        QVERIFY(feed.has_value());
        QCOMPARE(feed->version, QString("0.2.0"));
        QCOMPARE(feed->size, 42);
        QCOMPARE(feed->notesRu, QString("Новое"));
    }

    void releaseFeedRefusesWhatWouldReachAPath_data() {
        QTest::addColumn<QByteArray>("json");
        const QByteArray sha = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
        QTest::newRow("not json") << QByteArray("<html>");
        QTest::newRow("file leaves the folder") << QByteArray(R"({"version": "0.2.0", "file": "../../Poros.exe", "size": 1, "sha256": ")" + sha + R"("})");
        QTest::newRow("version shapes the url") << QByteArray(R"({"version": "0.2.0/../x", "file": "Poros-0.2.0-windows-amd64.zip", "size": 1, "sha256": ")" + sha + R"("})");
        QTest::newRow("no checksum") << QByteArray(R"({"version": "0.2.0", "file": "Poros-0.2.0-windows-amd64.zip", "size": 1})");
        QTest::newRow("no size") << QByteArray(R"({"version": "0.2.0", "file": "Poros-0.2.0-windows-amd64.zip", "sha256": ")" + sha + R"("})");
    }

    void releaseFeedRefusesWhatWouldReachAPath() {
        QFETCH(QByteArray, json);
        QVERIFY(!Engine::ParseReleaseFeed(json).has_value());
    }

    void cleanStartIsSimple() {
        QCOMPARE(Configs::dataManager->settingsRepo->ui_mode, QString("simple"));
        QVERIFY(!Configs::dataManager->settingsRepo->onboarding_done);
        QVERIFY(Configs::dataManager->settingsRepo->simple_conn_mode.isEmpty());
    }
};

QTEST_MAIN(PorosUnit)
#include "poros_unit.moc"
