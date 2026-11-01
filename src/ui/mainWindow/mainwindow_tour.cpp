// Built only with POROS_UI_TOUR. Opens every screen on fake data and saves it as a PNG, so a change can be checked
// from a build directory without touching a real desktop or a real data folder.
#include "include/ui/mainwindow.h"

#include <QApplication>
#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QPushButton>
#include <QStackedWidget>
#include <QTabWidget>
#include <QTimer>

#include "include/configs/PresetRules.h"
#include "include/database/GroupsRepo.h"
#include "include/database/RoutesRepo.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/engine/services/GroupService.h"
#include "include/engine/services/ImportService.h"
#include "include/ui/advanced/AdvancedPage.h"
#include "include/ui/advanced/SettingsSection.h"
#include "include/ui/onboarding/OnboardingPage.h"
#include "include/ui/simple/PresetEditor.h"
#include "include/ui/simple/ServerTreeModel.h"
#include "include/ui/utils/ConnectionModeController.h"
#include "include/ui/widget/AdminPromptDialog.h"
#include "include/ui/widget/ConnectionChoiceDialog.h"

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
        group->sub_info.has_quota = true;
        group->sub_info.download = qint64(38) * 1024 * 1024 * 1024;
        group->sub_info.total = qint64(100) * 1024 * 1024 * 1024;
        group->sub_info.expire = QDateTime::currentSecsSinceEpoch() + 40 * 24 * 3600;
        group->sub_last_update = QDateTime::currentSecsSinceEpoch() - 2 * 3600;
        Configs::dataManager->groupsRepo->Save(group);
        m_serverModel->reload();
    });

    const auto trigger = [](QAction *action) { return [action] { action->trigger(); }; };
    auto steps = std::make_shared<QList<TourStep>>(QList<TourStep>{
        {"profiles", [this] { setUiMode(QStringLiteral("advanced")); }},
        {"connections", [this] { m_advancedPage->showSection(AdvancedPage::Connections); }},
        {"routing-section", [this] { m_advancedPage->showSection(AdvancedPage::Routing); }},
        {"logs", [this] { m_advancedPage->showSection(AdvancedPage::Logs); }},
        {"statistics", [this] { m_advancedPage->showSection(AdvancedPage::Statistics); }},
        {"tools", [this] { m_advancedPage->showSection(AdvancedPage::Tools); }},
        {"settings-section", [this] { m_advancedPage->showSection(AdvancedPage::Settings); }},
        {"traffic", trigger(ui->actionTraffic_Stats)},
        {"routing", trigger(ui->menu_routing_settings)},
        {"otp", trigger(ui->menu_otp_manager)},
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
            profile_start(m_serverModel->groups().first().servers.first().id);
        }, 14000},
        {"ruleset-result", [] {}, 30000},
        {"add-profile", trigger(ui->menu_add_from_input)},
    });

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
