// Built only with POROS_UI_TOUR. Opens every screen on fake data and saves it as a PNG, so a change can be checked
// from a build directory without touching a real desktop or a real data folder.
#include "include/ui/mainwindow.h"

#include <QApplication>
#include <QComboBox>
#include <QDateTime>
#include <cmath>
#include <QDir>
#include <QFile>
#include <QListWidget>
#include <QPushButton>
#include <QScopedValueRollback>
#include <QStackedWidget>
#include <QTableWidget>
#include <QTabWidget>
#include <QTimer>

#include "include/configs/PresetRules.h"
#include "include/database/GroupsRepo.h"
#include "include/database/OtpProfilesRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/database/RoutesRepo.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/engine/services/GroupService.h"
#include "include/engine/services/ImportService.h"
#include "include/ui/advanced/AdvancedPage.h"
#include "include/ui/advanced/EmbeddedDialog.h"
#include "include/ui/advanced/SettingsSection.h"
#include "include/ui/mainWindow/MainWindowInternal.h"
#include "include/ui/group/dialog_edit_group.h"
#include "include/ui/onboarding/OnboardingPage.h"
#include "include/ui/profile/dialog_profile_type.h"
#include "include/ui/profile/dialog_edit_profile.h"
#include "include/ui/shell/SegmentSwitch.h"
#include "include/ui/simple/PresetEditor.h"
#include "include/ui/simple/ServerTreeModel.h"
#include "include/ui/utils/ConnectionModeController.h"
#include "include/ui/utils/TableSearchEdit.h"
#include "include/ui/widget/AdminPromptDialog.h"
#include "include/ui/widget/ConnectionChoiceDialog.h"
#include "include/ui/widget/TrayOtpCodes.hpp"

namespace {
    struct TourStep {
        QString name;
        std::function<void()> open;
        int waitMs = 700;
    };

    const char *const tourLinks =
        "vless://11111111-1111-1111-1111-111111111111@de1.example.com:443?security=reality&sni=example.com&fp=chrome&pbk=SMXtxoJ8Zs4YYYh0lHPGM1mZ1P_2Sn2EuG3eCyLhxnU&type=tcp#Germany\n"
        "trojan://password@nl2.example.com:443?sni=example.com#Netherlands\n"
        "ss://YWVzLTI1Ni1nY206cGFzcw@tr1.example.com:8388#Turkey\n"
        "hysteria2://password@pl1.example.com:8443?sni=example.com#Poland\n"
        "tuic://11111111-1111-1111-1111-111111111111:password@jp1.example.com:443?sni=example.com#Japan\n";

    void tourWizardNext(QWidget *page) {
        for (auto *button : page->findChildren<QPushButton *>()) {
            if (button->isDefault()) button->click();
        }
    }

    void tourSaveTabs(QWidget *top, const QString &dir, const QString &name) {
        int tabIndex = 0;
        for (auto *tabs : top->findChildren<QTabWidget *>()) {
            if (tabs->count() < 2) continue;
            for (int i = 0; i < tabs->count(); i++) {
                tabs->setCurrentIndex(i);
                QApplication::processEvents();
                top->grab().save(QStringLiteral("%1/%2-tab%3-%4.png").arg(dir, name).arg(tabIndex).arg(i));
            }
            tabIndex++;
        }
    }
} // namespace

void MainWindow::runUiTour(const QString &dir) {
    QDir().mkpath(dir);
    // Off every monitor: QWidget::grab() renders from the widgets, and the dialogs open over this window.
    setGeometry(-20000, 100, 1180, 760);
    // A subscription group refuses imports, so the fake one becomes a subscription once it holds its servers.
    int subscription = -1;
    Engine::GroupService::Add({QStringLiteral("Main subscription"), {}, false}, &subscription);
    Engine::ImportService::Import({Engine::ImportService::Source::Text, QString::fromLatin1(tourLinks), subscription}, [this, subscription](const auto &) {
        const auto group = Configs::dataManager->groupsRepo->GetGroup(subscription);
        if (group == nullptr) return;
        group->url = QStringLiteral("https://sub.example.com/s/1");
        group->sub_info.valid = true;
        group->sub_info.has_quota = true;
        group->sub_info.download = qint64(38) * 1024 * 1024 * 1024;
        group->sub_info.total = qint64(100) * 1024 * 1024 * 1024;
        group->sub_info.expire = QDateTime::currentSecsSinceEpoch() + 40 * 24 * 3600;
        group->sub_last_update = QDateTime::currentSecsSinceEpoch() - 2 * 3600;
        Configs::dataManager->groupsRepo->Save(group);
        m_serverModel->reload();
    });

    const auto trigger = [](QAction *action) { return [action] { action->trigger(); }; };
    const auto tile = [this](const QString &label) {
        return [this, label] {
            ui->menu_add_from_input->trigger();
            auto *picker = findChild<DialogProfileType *>();
            picker->findChild<QPushButton *>(QStringLiteral("more_types"))->click();
            for (auto *button : picker->findChildren<QPushButton *>()) {
                if (button->text() == label) button->click();
            }
        };
    };
    const auto fakeConnections = [] {
        const auto connection = [](const QString &id, const QString &process, const QString &domain, const QString &protocol, const QString &outbound, long long bytes) {
            Stats::ConnectionMetadata meta;
            meta.id = id;
            meta.createdAtMs = 0;
            meta.upload = bytes / 10;
            meta.download = bytes;
            meta.uploadSpeed = bytes / 500;
            meta.downloadSpeed = bytes / 50;
            meta.outbound = outbound;
            meta.outboundName = outbound;
            meta.network = QStringLiteral("tcp");
            meta.protocol = protocol;
            meta.domain = domain;
            meta.dest = domain + QStringLiteral(":443");
            meta.process = process;
            return meta;
        };
        return QList<Stats::ConnectionMetadata>{connection(QStringLiteral("1"), QStringLiteral("chrome.exe"), QStringLiteral("www.example.com"), QStringLiteral("tls"), QStringLiteral("nl2"), 64 << 20),
                                                connection(QStringLiteral("2"), QStringLiteral("chrome.exe"), QStringLiteral("cdn.example.net"), QStringLiteral("quic"), QStringLiteral("nl2"), 23 << 20),
                                                connection(QStringLiteral("3"), QStringLiteral("telegram.exe"), QStringLiteral("api.example.org"), QStringLiteral("tls"), QStringLiteral("nl2"), 12 << 20),
                                                connection(QStringLiteral("4"), QStringLiteral("steam.exe"), QStringLiteral("cache1.example.com"), QStringLiteral("tls"), QStringLiteral("direct"), 1400 << 20)};
    };
    const auto routingTab = [this](int tab) {
        return [this, tab] { m_routingPage->dialog()->findChild<QTabWidget *>(QStringLiteral("tabWidget"))->setCurrentIndex(tab); };
    };
    const auto routeTab = [this](int tab) {
        return [this, tab] { m_routingPage->dialog()->findChild<QTabWidget *>(QStringLiteral("routeTabs"))->setCurrentIndex(tab); };
    };
    const auto routeProfile = [this](bool raw) {
        return [this, raw] {
            auto profile = Configs::dataManager->routesRepo->NewRouteProfile();
            profile->name = raw ? QStringLiteral("Raw") : QStringLiteral("Remote");
            profile->isRaw = raw;
            profile->isRemote = !raw;
            profile->remoteURL = QStringLiteral("https://rules.example.com/poros.json");
            Configs::dataManager->routesRepo->AddRouteProfile(profile);
            m_routingPage->rebuild();
            auto *list = m_routingPage->dialog()->findChild<QListWidget *>(QStringLiteral("route_profiles"));
            list->setCurrentRow(list->count() - 1);
        };
    };
    auto steps = std::make_shared<QList<TourStep>>(QList<TourStep>{
        {"profiles", [this] { setUiMode(QStringLiteral("advanced")); }},
        {"profiles-subscription", [this, subscription] {
            const QList<int> latencies{42, 96, 143, 320, -1};
            const QStringList countries{QStringLiteral("DE"), QStringLiteral("NL"), QStringLiteral("TR"), QStringLiteral("PL"), QString()};
            const auto ids = Configs::dataManager->groupsRepo->GetGroup(subscription)->Profiles();
            for (int i = 0; i < ids.size() && i < latencies.size(); ++i) {
                const auto profile = Configs::dataManager->profilesRepo->GetProfile(ids[i]);
                profile->latency = latencies[i];
                profile->test_country = countries[i];
                Configs::dataManager->profilesRepo->Save(profile);
            }
            ui->tabWidget->setCurrentIndex(groupId2TabIndex(subscription));
            selectProfileRows({1});
        }},
        {"profiles-search", [this] { m_profileSearch->setText(QStringLiteral("nl2")); }},
        {"profiles-nothing", [this] { m_profileSearch->setText(QStringLiteral("port=1")); }},
        {"profiles-menu", [this, dir] {
            m_profileSearch->clear();
            ui->menu_server->adjustSize();
            ui->menu_server->grab().save(dir + QStringLiteral("/profiles-context.png"));
        }},
        {"profiles-bulk", [this] { selectProfileRows({0, 2}); }},
        {"connections", [this] {
            ui->profilesTableView->clearSelection();
            m_advancedPage->showSection(AdvancedPage::Connections);
        }},
        {"connections-data", [this, fakeConnections] {
            UpdateConnectionList(fakeConnections());
            ui->connections->expandAll();
            const qint64 now = MonotonicMs();
            for (int second = 0; second < 110; ++second) {
                const double wave = 1.6 + std::sin(second / 7.0) + std::sin(second / 3.0) / 2;
                add_traffic_point(now + second * 1000, true, {wave * (3 << 20), wave * (200 << 10), wave * (1 << 20), wave * (60 << 10)}, 0, 0, 0, 0);
            }
        }},
        {"connections-list-empty", [this] {
            ui->connections_tab->findChild<SegmentSwitch *>()->setCurrent(1);
            setConnectionsFlat(true);
        }},
        {"connections-list", [this, fakeConnections] { UpdateConnectionList(fakeConnections()); }},
        {"routing-section", [this] {
            ui->connections_tab->findChild<SegmentSwitch *>()->setCurrent(0);
            setConnectionsFlat(false);
            m_advancedPage->showSection(AdvancedPage::Routing);
        }},
        {"logs", [this] { m_advancedPage->showSection(AdvancedPage::Logs); }},
        {"logs-levels", [this] {
            MW_show_log(QStringLiteral("+0300 2026-11-01 12:00:01 INFO [1 0ms] outbound/vless[proxy]: connected\n"
                                       "+0300 2026-11-01 12:00:02 WARN [2 3ms] dns: exchange timed out\n"
                                       "+0300 2026-11-01 12:00:03 ERROR [3 9ms] router: connection refused\n"));
            ui->logLevel->setCurrentIndex(1);
        }},
        {"statistics", [this] {
            ui->logLevel->setCurrentIndex(0);
            m_advancedPage->showSection(AdvancedPage::Statistics);
            ui->stats_widget->setCurrentWidget(ui->runtime_tab);
        }},
        {"statistics-auto", [this] { ui->actionAuto_Selector->trigger(); }},
        {"tools", [this] { m_advancedPage->showSection(AdvancedPage::Tools); }},
        {"settings-section", [this] { m_advancedPage->showSection(AdvancedPage::Settings); }},
        {"traffic", trigger(ui->actionTraffic_Stats)},
        {"routing", trigger(ui->menu_routing_settings)},
        {"routing-dns", routingTab(1)},
        {"routing-warp", routingTab(2)},
        {"routing-general", routingTab(3)},
        {"routing-clone", [this] {
            auto *page = m_routingPage->dialog();
            page->findChild<QTabWidget *>(QStringLiteral("tabWidget"))->setCurrentIndex(0);
            page->findChild<QPushButton *>(QStringLiteral("clone_route"))->click();
        }},
        {"routing-rule", [this] {
            auto *rules = m_routingPage->dialog()->findChild<QTableWidget *>(QStringLiteral("route_items"));
            rules->setCurrentCell(qMin(1, rules->rowCount() - 1), 1);
        }},
        {"routing-simple", routeTab(0)},
        {"routing-endpoints", [this] {
            auto *tabs = m_routingPage->dialog()->findChild<QTabWidget *>(QStringLiteral("routeTabs"));
            tabs->setTabVisible(2, true);
            tabs->setCurrentIndex(2);
        }},
        {"routing-profile-dns", routeTab(3)},
        {"routing-remote", routeProfile(false)},
        {"routing-source", routeTab(4)},
        {"routing-raw", routeProfile(true)},
        {"otp", trigger(ui->menu_otp_manager)},
        {"otp-codes", [this] {
            const QList<QPair<QString, QString>> accounts{{QStringLiteral("Панель провайдера"), QStringLiteral("user@example.com")},
                                                          {QStringLiteral("GitHub"), QStringLiteral("poros-dev")},
                                                          {{}, QStringLiteral("Почта")}};
            for (const auto &[issuer, name] : accounts) {
                auto profile = Configs::OtpProfilesRepo::NewOtpProfile();
                profile->issuer = issuer;
                profile->name = name;
                profile->secret = QStringLiteral("JBSWY3DPEHPK3PXP");
                Configs::dataManager->otpProfilesRepo->AddOtpProfile(profile);
            }
            static_cast<EmbeddedDialog *>(m_toolTabs->widget(0))->rebuild();
        }},
        {"otp-tray", [this, dir] {
            openTrayOtpCodes();
            trayOtpCodes->grab().save(dir + QStringLiteral("/otp-tray-popup.png"));
            trayOtpCodes->close();
        }},
        {"tools-other", [this] { m_toolTabs->setCurrentIndex(1); }},
        {"basic-settings", trigger(ui->menu_basic_settings)},
        {"settings-adv-inbound", [this] { m_settingsSection->showDialog(m_basicSettings, 1); }},
        {"settings-adv-tests", [this] { m_settingsSection->showDialog(m_basicSettings, 2); }},
        {"settings-adv-subscriptions", [this] { m_settingsSection->showDialog(m_basicSettings, 3); }},
        {"settings-adv-appearance", [this] { m_settingsSection->showDialog(m_basicSettings, 4); }},
        {"settings-adv-log", [this] { m_settingsSection->showDialog(m_basicSettings, 5); }},
        {"settings-adv-core", [this] { m_settingsSection->showDialog(m_basicSettings, 6); }},
        {"settings-adv-network", [this] { m_settingsSection->showDialog(m_basicSettings, 7); }},
        {"settings-adv-security", [this] { m_settingsSection->showDialog(m_basicSettings, 8); }},
        {"settings-adv-backups", [this] { m_settingsSection->showDialog(m_basicSettings, 9); }},
        {"settings-adv-diagnostics", [this] { m_settingsSection->showDialog(m_basicSettings, 10); }},
        {"vpn-settings", trigger(ui->menu_vpn_settings)},
        {"preset-settings", trigger(ui->menu_preset_settings)},
        {"integration", trigger(ui->menu_integration_settings)},
        {"groups", trigger(ui->menu_manage_groups)},
        {"add-group", trigger(ui->actionAdd_New_Group)},
        {"wizard-1", [this] { showOnboarding(); }},
        {"wizard-2", [this] { m_onboarding->findChild<QPushButton *>(QString(), Qt::FindChildrenRecursively); tourWizardNext(m_onboarding); }},
        {"wizard-3", [this] { tourWizardNext(m_onboarding); }},
        {"wizard-4", [this] { tourWizardNext(m_onboarding); }},
        {"simple", [this] { finishOnboarding(QStringLiteral("simple")); }},
        {"add-subscription", [this] { showAddSubscription({}); }},
        {"settings-presets", [this] { showSimpleSettings(0); }},
        {"settings-connection", [this] { showSimpleSettings(1); }},
        {"settings-subscriptions", [this] { showSimpleSettings(2); }},
        {"settings-startup", [this] { showSimpleSettings(3); }},
        {"settings-appearance", [this] { showSimpleSettings(4); }},
        {"settings-backup", [this] { showSimpleSettings(5); }},
        {"settings-help", [this] { showSimpleSettings(6); }},
        {"preset-editor", [this] {
            auto copy = std::make_shared<Configs::RouteProfile>(*Configs::dataManager->routesRepo->GetAllRouteProfiles().at(1));
            copy->id = -1;
            copy->DetachFromBuiltin();
            copy->name = QStringLiteral("Home");
            Configs::dataManager->routesRepo->AddRouteProfile(copy);
            Configs::PresetRules::Write(*copy, {{QStringLiteral("example.org"), Configs::PresetRules::Action::Block, false},
                                                {QStringLiteral("video.example.ru"), Configs::PresetRules::Action::Proxy, false},
                                                {QStringLiteral("qbittorrent.exe"), Configs::PresetRules::Action::Direct, true}});
            Configs::dataManager->routesRepo->Save(copy);
            m_presetEditor->edit(copy);
            m_simpleStack->setCurrentWidget(m_presetEditor);
        }},
        {"simple-back", [this] { m_simpleStack->setCurrentIndex(0); }},
        {"choice", [this] { ConnectionChoiceDialog(this).exec(); }},
        {"admin-tun", [this] { AdminPromptDialog(Engine::PrivilegeReason::Tun, this).exec(); }},
        {"banner", [this] { showRightsFailure(Engine::PrivilegeReason::Tun); }},
        {"tun-start", [this] {
            m_banner->dismiss();
            m_modes->choose(ConnectionModeController::Mode::Tun);
            simpleStart();
        }},
        {"after-cancel", [this] {
            QFile report(QStringLiteral("after-cancel.txt"));
            if (report.open(QIODevice::WriteOnly)) {
                const auto &settings = Configs::dataManager->settingsRepo;
                report.write(QStringLiteral("spmode_vpn=%1 running=%2\n").arg(settings->spmode_vpn).arg(runningSnapshot() != nullptr).toUtf8());
            }
            m_modes->choose(ConnectionModeController::Mode::Unset);
        }},
        {"ruleset-failure", [this] {
            for (const auto &route : Configs::dataManager->routesRepo->GetAllRouteProfiles()) {
                if (route->builtinKey == QLatin1String("blocked-only")) connection()->chooseRoute(route->id);
            }
            // Past the simple mode's gate: a chosen mode could touch this machine's proxy, and without one the start would stop at a dialog.
            const QScopedValueRollback mode(Configs::dataManager->settingsRepo->ui_mode, QStringLiteral("advanced"));
            profile_start(m_serverModel->groups().first().servers.first().id);
        }, 14000},
        {"ruleset-result", [] {}, 30000},
        {"add-profile", trigger(ui->menu_add_from_input)},
        {"add-profile-others", [this] {
            ui->menu_add_from_input->trigger();
            findChild<DialogProfileType *>()->findChild<QPushButton *>(QStringLiteral("more_types"))->click();
        }},
        {"new-amnezia", tile(QStringLiteral("AmneziaWG"))},
        {"new-realm", tile(QStringLiteral("Hysteria Realm"))},
        {"new-custom", tile(QStringLiteral("Custom JSON"))},
        {"edit-profile", [this, subscription] {
            const auto id = Configs::dataManager->groupsRepo->GetGroup(subscription)->Profiles().first();
            (new DialogEditProfile({}, id, this))->setAttribute(Qt::WA_DeleteOnClose);
        }},
        {"new-vmess", [this] {
            (new DialogEditProfile(QStringLiteral("vmess"), Configs::dataManager->settingsRepo->current_group, this))->setAttribute(Qt::WA_DeleteOnClose);
        }},
        {"new-xray", [this] {
            (new DialogEditProfile(QStringLiteral("xrayvless"), Configs::dataManager->settingsRepo->current_group, this))->setAttribute(Qt::WA_DeleteOnClose);
        }},
        {"new-types", [this] {
            auto *dialog = new DialogEditProfile(QStringLiteral("vmess"), Configs::dataManager->settingsRepo->current_group, this);
            dialog->setAttribute(Qt::WA_DeleteOnClose);
            auto *types = dialog->findChild<QComboBox *>(QStringLiteral("type"));
            for (int index = 0; index < types->count(); ++index) types->setCurrentIndex(index);
        }},
        {"edit-group", [this, subscription] {
            auto *dialog = new DialogEditGroup(Configs::dataManager->groupsRepo->GetGroup(subscription), this);
            dialog->setAttribute(Qt::WA_DeleteOnClose);
            dialog->open();
        }},
        {"edit-save", [this, subscription] {
            const auto id = Configs::dataManager->groupsRepo->GetGroup(subscription)->Profiles().first();
            auto *dialog = new DialogEditProfile({}, id, this);
            dialog->setAttribute(Qt::WA_DeleteOnClose);
            dialog->findChild<QLineEdit *>(QStringLiteral("name"))->setText(QStringLiteral("Saved from the tour"));
            QTimer::singleShot(100, dialog, [dialog] { dialog->accept(); });
        }},
        {"edit-saved", [this] {
            setUiMode(QStringLiteral("advanced"));
            m_advancedPage->showSection(AdvancedPage::Profiles);
        }},
    });
    for (const auto *type : {"socks", "http", "shadowsocks", "trojan", "vless", "hysteria", "tuic", "naive", "trusttunnel", "anytls", "shadowtls",
                             "wireguard", "masque", "openvpn", "openconnect", "ssh", "direct", "chain", "juicity", "mieru", "snell"}) {
        steps->append({QStringLiteral("type-%1").arg(QLatin1String(type)), [this, type] {
            (new DialogEditProfile(QString::fromLatin1(type), Configs::dataManager->settingsRepo->current_group, this))->setAttribute(Qt::WA_DeleteOnClose);
        }});
    }

    auto step = std::make_shared<std::function<void(int)>>();
    *step = [=, this](int index) {
        if (index >= steps->size()) {
            QApplication::exit(0);
            return;
        }
        QTimer::singleShot(steps->at(index).waitMs, this, [=, this] {
            QWidget *top = QApplication::activeModalWidget();
            if (top == nullptr) top = QApplication::activeWindow();
            if (top == nullptr) top = this;
            const QString &name = steps->at(index).name;
            top->grab().save(QStringLiteral("%1/%2.png").arg(dir, name));
            if (top != this) tourSaveTabs(top, dir, name);
            if (top != this) top->close();
            QTimer::singleShot(200, this, [=] { (*step)(index + 1); });
        });
        steps->at(index).open();
    };
    QTimer::singleShot(1500, this, [=] { (*step)(0); });
}
