#include "include/ui/mainwindow.h"

#include <QApplication>
#include <QFrame>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QLabel>
#include <QMenu>
#include <QPushButton>
#include <QStackedWidget>
#include <QTabBar>
#include <QTabWidget>
#include <QToolButton>
#include <QVBoxLayout>
#include <QWidgetAction>

#include "include/database/GroupsRepo.h"
#include "include/database/RoutesRepo.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/ui/advanced/AdvancedPage.h"
#include "include/ui/advanced/ConnectionBar.h"
#include "include/ui/advanced/EmbeddedDialog.h"
#include "include/ui/advanced/GroupPane.h"
#include "include/ui/advanced/SettingsSection.h"
#include "include/ui/mainWindow/MainWindowInternal.h"
#include "include/ui/setting/dialog_basic_settings.h"
#include "include/ui/setting/dialog_integration.h"
#include "include/ui/setting/dialog_manage_routes.h"
#include "include/ui/setting/dialog_otp_manager.h"
#include "include/ui/setting/dialog_preset_settings.h"
#include "include/ui/setting/dialog_vpn_settings.h"
#include "include/ui/stats/dialog_auto_selector.h"
#include "include/ui/stats/dialog_traffic_stats.h"
#include "include/ui/onboarding/OnboardingPage.h"
#include "include/ui/setting/PorosTheme.h"
#include "include/ui/shell/NavRail.h"
#include "include/ui/shell/SegmentSwitch.h"
#include "include/ui/shell/TitleBar.h"
#include "include/ui/shell/WindowFrame.h"
#include "include/ui/simple/SimpleHome.h"
#include "include/ui/utils/CommandPalette.h"
#include "include/ui/utils/ConnectionModeController.h"
#include "include/ui/utils/ScreenQrScanner.h"
#include "include/ui/widget/AdminPromptDialog.h"

namespace {
    QFrame *shellSectionHeader(const QString &title, QHBoxLayout **actions, QWidget *parent) {
        auto *header = new QFrame(parent);
        header->setProperty("role", QStringLiteral("toolbar"));
        auto *row = new QHBoxLayout(header);
        row->setContentsMargins(14, 8, 14, 8);
        row->setSpacing(6);
        auto *label = new QLabel(title, header);
        label->setProperty("role", QStringLiteral("subheading"));
        row->addWidget(label);
        row->addStretch();
        *actions = row;
        return header;
    }

    QWidget *shellSection(QWidget *header, QWidget *content, QWidget *parent) {
        auto *section = new QWidget(parent);
        auto *column = new QVBoxLayout(section);
        column->setContentsMargins(0, 0, 0, 0);
        column->setSpacing(0);
        column->addWidget(header);
        column->addWidget(content, 1);
        // A page taken out of a tab widget stays explicitly hidden.
        content->show();
        return section;
    }

    QToolButton *shellMenuButton(const QString &icon, const QString &text, QMenu *menu, QWidget *parent) {
        auto *button = new QToolButton(parent);
        button->setIcon(PorosTheme::Icon(icon));
        button->setIconSize({15, 15});
        button->setText(text);
        button->setToolButtonStyle(Qt::ToolButtonTextBesideIcon);
        button->setPopupMode(QToolButton::InstantPopup);
        button->setMenu(menu);
        return button;
    }

    // A section still made of the dialogs it opens.
    QWidget *shellActionSection(const QString &title, const QList<QAction *> &actions, QWidget *parent) {
        auto *page = new QWidget(parent);
        auto *grid = new QGridLayout(page);
        grid->setContentsMargins(22, 20, 22, 20);
        grid->setHorizontalSpacing(8);
        grid->setVerticalSpacing(8);
        auto *label = new QLabel(title, page);
        label->setProperty("role", QStringLiteral("sectionTitle"));
        grid->addWidget(label, 0, 0, 1, 2);
        int row = 1;
        for (auto *action : actions) {
            if (action->isSeparator() || action->text().isEmpty()) continue;
            auto *button = new QPushButton(QString(action->text()).remove('&'), page);
            QObject::connect(button, &QPushButton::clicked, action, &QAction::trigger);
            grid->addWidget(button, row++, 0);
        }
        grid->setColumnStretch(1, 1);
        grid->setRowStretch(row, 1);
        return page;
    }
} // namespace

bool MainWindow::nativeEvent(const QByteArray &eventType, void *message, qintptr *result) {
    if (m_titleBar != nullptr && WindowFrame::HandleNativeEvent(this, m_titleBar, message, result)) return true;
    return QMainWindow::nativeEvent(eventType, message, result);
}

void MainWindow::setupShell() {
    WindowFrame::Prepare(this);

    // The form still creates the widgets, and the rest of the window keeps addressing them through ui; here they move to their pages.
    auto *formCentral = takeCentralWidget();

    auto *root = new QWidget(this);
    root->setObjectName(QStringLiteral("shellRoot"));
    auto *column = new QVBoxLayout(root);
    column->setContentsMargins(0, 0, 0, 0);
    column->setSpacing(0);

    m_titleBar = new TitleBar(root);
    connect(m_titleBar, &TitleBar::minimizeRequested, this, &QWidget::showMinimized);
    connect(m_titleBar, &TitleBar::maximizeToggled, this, [this] { isMaximized() ? showNormal() : showMaximized(); });
    connect(m_titleBar, &TitleBar::closeRequested, this, &QWidget::close);
    connect(m_titleBar, &TitleBar::menuRequested, this, &MainWindow::showAppMenu);
    column->addWidget(m_titleBar);

    m_banner = new FailureBanner(root);
    column->addWidget(m_banner);

    m_rootStack = new QStackedWidget(root);
    column->addWidget(m_rootStack, 1);

    m_advancedPage = new AdvancedPage(m_rootStack);
    m_rootStack->addWidget(m_advancedPage);

    ui->checkBox_VPN->setProperty("role", QStringLiteral("switch"));
    ui->checkBox_VPN->setText(QStringLiteral("TUN"));
    ui->checkBox_SystemProxy->setProperty("role", QStringLiteral("switch"));
    ui->checkBox_SystemProxy->setText(QStringLiteral("System"));
    m_connectionBar = new ConnectionBar({ui->toolButton_startstop, ui->label_running, ui->label_inbound, ui->data_view, ui->label_speed,
                                         ui->checkBox_VPN, ui->checkBox_SystemProxy}, m_advancedPage);
    m_startButtons << ui->toolButton_startstop;
    m_advancedPage->setConnectionBar(m_connectionBar);
    connect(m_connectionBar, &ConnectionBar::routeChosen, this, [this](int id) { connection()->chooseRoute(id); });
    connect(m_connectionBar, &ConnectionBar::manageRoutesRequested, ui->menu_routing_settings, &QAction::trigger);

    auto *profiles = new QWidget(m_advancedPage);
    auto *profilesRow = new QHBoxLayout(profiles);
    profilesRow->setContentsMargins(0, 0, 0, 0);
    profilesRow->setSpacing(0);
    m_groupPane = new GroupPane(profiles);
    profilesRow->addWidget(m_groupPane);
    connect(m_groupPane, &GroupPane::groupActivated, this, [this](int gid) { ui->tabWidget->setCurrentIndex(groupId2TabIndex(gid)); });
    connect(m_groupPane, &GroupPane::orderChanged, this, [this](const QList<int> &ids) {
        Configs::dataManager->groupsRepo->SetGroupsTabOrder(ids);
        refresh_groups();
    });
    connect(m_groupPane, &GroupPane::menuRequested, this, &MainWindow::show_group_menu);
    connect(m_groupPane, &GroupPane::addRequested, ui->actionAdd_New_Group, &QAction::trigger);

    auto *toolbar = new QFrame(profiles);
    toolbar->setProperty("role", QStringLiteral("toolbar"));
    m_profileActions = new QHBoxLayout(toolbar);
    m_profileActions->setContentsMargins(14, 8, 14, 8);
    m_profileActions->setSpacing(6);
    const auto toolbarMenu = [this, toolbar](const QString &icon, const QString &text, const QList<QAction *> &actions) {
        auto *menu = new QMenu(toolbar);
        menu->addActions(actions);
        m_profileActions->addWidget(shellMenuButton(icon, text, menu, toolbar));
        return menu;
    };
    toolbarMenu(QStringLiteral("plus"), tr("Add"), {ui->menu_add_from_input, ui->menu_add_from_clipboard, ui->actionAdd_profile_from_File, ui->menu_scan_qr});
    toolbarMenu(QStringLiteral("gauge"), tr("Tests"), {ui->actionUrl_Test_Group, ui->actionSpeedtest_Group, ui->actionResolve_Out_IP, ui->menu_resolve_domain,
                                                       ui->menu_clear_test_result, ui->menu_stop_testing});
    m_profileActions->addWidget(shellMenuButton(QStringLiteral("trash"), tr("Clean up"), ui->menu_clean_up, toolbar));
    auto *groupMenu = toolbarMenu(QStringLiteral("layers"), tr("Group"), {ui->menu_update_subscription, ui->actionUpdate_All_Subscriptions, ui->actionEdit_Group,
                                                                          ui->actionAdd_New_Group, ui->menu_manage_groups, ui->actionDelete_Group});
    connect(groupMenu, &QMenu::aboutToShow, this, [this] {
        ui->actionDelete_Group->setEnabled(Configs::dataManager->groupsRepo->GetGroupsTabOrder().size() > 1);
    });
    m_profileActions->addStretch();

    ui->tabWidget->tabBar()->hide();
    ui->tabWidget->setDocumentMode(true);
    profilesRow->addWidget(shellSection(toolbar, ui->tabWidget, profiles), 1);
    m_advancedPage->setSection(AdvancedPage::Profiles, profiles);

    ui->stats_widget->removeTab(ui->stats_widget->indexOf(ui->Logs));
    ui->stats_widget->removeTab(ui->stats_widget->indexOf(ui->connections_tab));
    auto *logsHeader = shellSectionHeader(tr("Logs"), &m_logActions, m_advancedPage);
    m_advancedPage->setSection(AdvancedPage::Logs, shellSection(logsHeader, ui->Logs, m_advancedPage));
    auto *connectionsHeader = shellSectionHeader(tr("Connections"), &m_connectionActions, m_advancedPage);
    m_advancedPage->setSection(AdvancedPage::Connections, shellSection(connectionsHeader, ui->connections_tab, m_advancedPage));
    m_advancedPage->setSection(AdvancedPage::Statistics, ui->stats_widget);

    m_trafficPage = new EmbeddedDialog([](QWidget *parent) -> QDialog * { return new DialogTrafficStats(parent); }, ui->stats_widget);
    ui->stats_widget->addTab(m_trafficPage, tr("Traffic"));
    ui->stats_widget->setTabVisible(ui->stats_widget->indexOf(m_trafficPage), !Configs::dataManager->settingsRepo->disable_traffic_aggregation);
    m_autoSelectorPage = new EmbeddedDialog([this](QWidget *parent) -> QDialog * {
        m_autoSelectorDialog = new DialogAutoSelector(parent);
        connect(m_autoSelectorDialog, &QObject::destroyed, this, [this] { m_autoSelectorDialog = nullptr; });
        m_autoSelectorDialog->refresh();
        return m_autoSelectorDialog;
    }, ui->stats_widget);
    ui->stats_widget->addTab(m_autoSelectorPage, tr("Auto selector"));

    m_routingPage = new EmbeddedDialog([](QWidget *parent) -> QDialog * { return new DialogManageRoutes(parent); }, m_advancedPage);
    m_advancedPage->setSection(AdvancedPage::Routing, m_routingPage);

    m_toolTabs = new QTabWidget(m_advancedPage);
    m_toolTabs->addTab(new EmbeddedDialog([](QWidget *parent) -> QDialog * { return new DialogOtpManager(parent); }, m_toolTabs), tr("OTP"));
    m_toolTabs->addTab(shellActionSection(tr("Other"), {ui->actionSpeedtest_Current, ui->actionUpdate_Rule_Sets, ui->menu_open_config_folder, ui->actionCheck_For_Update}, m_toolTabs), tr("Other"));
    m_advancedPage->setSection(AdvancedPage::Tools, m_toolTabs);

    m_settingsSection = new SettingsSection(m_advancedPage);
    m_basicSettings = new EmbeddedDialog([](QWidget *parent) -> QDialog * { return new DialogBasicSettings(parent); }, m_settingsSection);
    m_vpnSettings = new EmbeddedDialog([](QWidget *parent) -> QDialog * { return new DialogVPNSettings(parent); }, m_settingsSection);
    m_presetSettings = new EmbeddedDialog([](QWidget *parent) -> QDialog * { return new DialogPresetSettings(parent); }, m_settingsSection);
    m_hotkeySettings = new EmbeddedDialog([this](QWidget *parent) -> QDialog * { return new DialogIntegration(parent, getActionsForShortcut()); }, m_settingsSection);
    for (auto *page : {m_basicSettings, m_vpnSettings, m_presetSettings, m_hotkeySettings}) m_settingsSection->addDialog(page);
    m_settingsSection->addGroup(tr("General"), {
        {QStringLiteral("power"), tr("Startup"), m_basicSettings, 0},
        {QStringLiteral("server"), tr("Inbound"), m_basicSettings, 1},
        {QStringLiteral("gauge"), tr("Tests"), m_basicSettings, 2},
        {QStringLiteral("link"), tr("Subscriptions"), m_basicSettings, 3},
        {QStringLiteral("app"), tr("Appearance"), m_basicSettings, 4},
        {QStringLiteral("log"), tr("Log"), m_basicSettings, 5},
        {QStringLiteral("key"), tr("Hotkeys"), m_hotkeySettings, -1},
    });
    m_settingsSection->addGroup(tr("Connection"), {
        {QStringLiteral("conn"), QStringLiteral("TUN"), m_vpnSettings, -1},
        {QStringLiteral("layers"), tr("Protocols and bypass"), m_presetSettings, -1},
        {QStringLiteral("bolt"), tr("Core"), m_basicSettings, 6},
        {QStringLiteral("globe"), tr("Network"), m_basicSettings, 7},
        {QStringLiteral("shield"), tr("Security"), m_basicSettings, 8},
    });
    m_settingsSection->addGroup(tr("Data"), {
        {QStringLiteral("download"), tr("Backups"), m_basicSettings, 9},
        {QStringLiteral("tool"), tr("Diagnostics"), m_basicSettings, 10},
    });
    connect(m_settingsSection, &SettingsSection::searchRequested, this, [this](const QString &text) {
        showCommandPalette();
        commandPalette->setQuery(text);
    });
    m_advancedPage->setSection(AdvancedPage::Settings, m_settingsSection);

    connect(m_advancedPage, &AdvancedPage::sectionChanged, this, [this](AdvancedPage::Section section) {
        syncConnectionViewState();
        if (section == AdvancedPage::Profiles) scheduleProxyListRefresh();
        if (section == AdvancedPage::Routing) m_routingPage->rebuild();
        if (section == AdvancedPage::Settings) {
            for (auto *page : {m_basicSettings, m_vpnSettings, m_presetSettings}) page->rebuild();
        }
    });
    connect(m_advancedPage->rail(), &NavRail::searchRequested, this, &MainWindow::showCommandPalette);

    setupSimpleMode();
    m_rootStack->addWidget(m_simpleStack);

    setCentralWidget(root);
    delete formCentral;
    WindowFrame::Attach(this);
    m_advancedPage->showSection(AdvancedPage::Profiles);
}

void MainWindow::applyUiMode() {
    const bool simple = Configs::dataManager->settingsRepo->ui_mode != QLatin1String("advanced");
    m_rootStack->setCurrentWidget(simple ? static_cast<QWidget *>(m_simpleStack) : m_advancedPage);
    m_titleBar->setModeText(simple ? tr("Simple mode") : tr("Advanced mode"));
    if (simple) m_modes->adopt();
    syncConnectionViewState();
    refreshSimplePortal();
    refresh_startstop_button();
    if (!simple) scheduleProxyListRefresh();
}

void MainWindow::showAdvancedSection(int section) {
    setUiMode(QStringLiteral("advanced"));
    m_advancedPage->showSection(static_cast<AdvancedPage::Section>(section));
}

void MainWindow::showOnboarding() {
    if (m_onboarding == nullptr) {
        m_onboarding = new OnboardingPage(m_rootStack);
        m_rootStack->addWidget(m_onboarding);
        connect(m_onboarding, &OnboardingPage::finished, this, [this](bool, const QString &uiMode) { finishOnboarding(uiMode); });
        connect(m_onboarding, &OnboardingPage::qrScanRequested, this, [this] {
            bool captured = false;
            const auto texts = ScreenQr::ScanScreens(this, captured);
            if (!texts.isEmpty()) m_onboarding->setLink(texts.first());
            else MessageBoxInfo(software_name, captured ? tr("QR Code not found") : tr("Unable to capture screen"));
        });
    } else {
        m_onboarding->restart();
    }
    m_titleBar->setMenuVisible(false);
    m_rootStack->setCurrentWidget(m_onboarding);
}

void MainWindow::finishOnboarding(const QString &uiMode) {
    auto &settings = Configs::dataManager->settingsRepo;
    settings->onboarding_done = true;
    settings->ui_mode = uiMode;
    settings->Save();
    m_titleBar->setMenuVisible(true);
    applyUiMode();
    for (const auto &link : std::exchange(m_deferredDeeplinks, {})) handle_deeplink_impl(link);
}

void MainWindow::setUiMode(const QString &mode) {
    auto &settings = Configs::dataManager->settingsRepo;
    if (settings->ui_mode == mode) return;
    settings->ui_mode = mode;
    settings->Save();
    applyUiMode();
}

void MainWindow::showAppMenu(const QPoint &globalPos) {
    const bool simple = Configs::dataManager->settingsRepo->ui_mode != QLatin1String("advanced");
    QMenu menu(this);
    const auto add = [&menu](const QString &icon, QAction *action) {
        action->setIcon(PorosTheme::Icon(icon));
        menu.addAction(action);
    };
    const auto heading = [&menu](const QString &text) { menu.addAction(text)->setEnabled(false); };

    if (simple) {
        connect(menu.addAction(PorosTheme::Icon(QStringLiteral("plus")), tr("Add subscription…")), &QAction::triggered, this,
                [this] { showAddSubscription({}); });
        connect(menu.addAction(PorosTheme::Icon(QStringLiteral("gear")), tr("Settings")), &QAction::triggered, this, [this] { showSimpleSettings(0); });
        connect(menu.addAction(PorosTheme::Icon(QStringLiteral("log")), tr("Log")), &QAction::triggered, this, [this] { showSimpleLog(); });
        menu.addSeparator();
    } else {
        heading(tr("Add"));
        add(QStringLiteral("clip"), ui->menu_add_from_clipboard);
        add(QStringLiteral("file"), ui->actionAdd_profile_from_File);
        add(QStringLiteral("qr"), ui->menu_scan_qr);
        add(QStringLiteral("edit"), ui->menu_add_from_input);
        menu.addSeparator();
    }

    connect(menu.addAction(PorosTheme::Icon(QStringLiteral("bolt")), tr("Quick setup…")), &QAction::triggered, this, &MainWindow::showOnboarding);
    menu.addSeparator();
    heading(tr("Interface mode"));
    auto *modeSwitch = new SegmentSwitch({tr("Simple"), tr("Advanced")}, &menu);
    modeSwitch->setCurrent(simple ? 0 : 1);
    modeSwitch->setFixedHeight(36);
    connect(modeSwitch, &SegmentSwitch::activated, this, [this, &menu](int index) {
        menu.close();
        setUiMode(index == 0 ? QStringLiteral("simple") : QStringLiteral("advanced"));
    });
    auto *modeHolder = new QWidget(&menu);
    auto *modeRow = new QHBoxLayout(modeHolder);
    modeRow->setContentsMargins(6, 2, 6, 6);
    modeRow->addWidget(modeSwitch);
    auto *modeAction = new QWidgetAction(&menu);
    modeAction->setDefaultWidget(modeHolder);
    menu.addAction(modeAction);
    menu.addSeparator();

    if (!simple) {
        add(QStringLiteral("refresh"), ui->actionRestart_Proxy);
        add(QStringLiteral("power"), ui->actionRestart_Program);
        add(QStringLiteral("folderopen"), ui->menu_open_config_folder);
    }
    add(QStringLiteral("download"), ui->actionCheck_For_Update);
    menu.addSeparator();
    add(QStringLiteral("hide"), ui->actionHide_window);
    add(QStringLiteral("close"), ui->menu_exit);
    menu.exec(globalPos);
}

void MainWindow::refreshGroupPane() {
    QList<GroupPane::Entry> entries;
    for (const int gid : Configs::dataManager->groupsRepo->GetGroupsTabOrder()) {
        const auto group = Configs::dataManager->groupsRepo->GetGroup(gid);
        if (group == nullptr) continue;
        entries.append({gid, group->name, static_cast<int>(group->Profiles().size()), !group->url.isEmpty(),
                        ui->tabWidget->tabToolTip(groupId2TabIndex(gid))});
    }
    m_groupPane->setGroups(entries, Configs::dataManager->settingsRepo->current_group);
}

void MainWindow::refreshRouteChoice() {
    QList<QPair<int, QString>> routes;
    QString key = QString::number(Configs::dataManager->settingsRepo->current_route_id);
    for (const auto &route : Configs::dataManager->routesRepo->GetAllRouteProfiles()) {
        routes.append({route->id, route->name});
        key += QStringLiteral("|%1:%2").arg(route->id).arg(route->name);
    }
    if (key == m_routeChoiceKey) return;
    m_routeChoiceKey = key;
    m_connectionBar->setRoutes(routes, Configs::dataManager->settingsRepo->current_route_id);
}
