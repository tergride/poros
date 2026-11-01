#include "include/ui/mainwindow.h"

#include <QApplication>
#include <QFrame>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QInputDialog>
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
#include "include/ui/utils/TableSearchEdit.h"
#include "include/ui/utils/ScreenQrScanner.h"
#include "include/ui/widget/AdminPromptDialog.h"
#include "include/ui/widget/SubscriptionInfoCard.hpp"

namespace {
    QWidget *shellSection(QWidget *header, QWidget *content, QWidget *parent) {
        auto *section = new QWidget(parent);
        auto *column = new QVBoxLayout(section);
        column->setContentsMargins(0, 0, 0, 0);
        column->setSpacing(0);
        column->addWidget(header);
        column->addWidget(content, 1);
        content->show();
        return section;
    }

    QPushButton *shellSmallButton(const QString &icon, const QString &text, QWidget *parent) {
        auto *button = new QPushButton(PorosTheme::Icon(icon), text, parent);
        button->setIconSize({14, 14});
        button->setProperty("role", QStringLiteral("small"));
        return button;
    }

    // A null action adds a separator.
    QMenu *shellMenu(const QList<QPair<QString, QAction *>> &actions, QWidget *parent) {
        auto *menu = new QMenu(parent);
        for (const auto &[icon, action] : actions) {
            if (action == nullptr) {
                menu->addSeparator();
                continue;
            }
            action->setIcon(PorosTheme::Icon(icon));
            menu->addAction(action);
        }
        return menu;
    }

    QPushButton *shellToolCard(QGridLayout *grid, int row, int column, const QString &title, const QString &hint, const QString &icon, const QString &text) {
        auto *card = new QFrame(grid->parentWidget());
        card->setProperty("role", QStringLiteral("card"));
        auto *content = new QVBoxLayout(card);
        content->setContentsMargins(16, 12, 16, 14);
        content->setSpacing(8);
        auto *heading = new QLabel(title, card);
        heading->setProperty("role", QStringLiteral("cardTitle"));
        content->addWidget(heading);
        auto *note = new QLabel(hint, card);
        note->setProperty("role", QStringLiteral("hint"));
        note->setWordWrap(true);
        content->addWidget(note);
        auto *button = new QPushButton(PorosTheme::Icon(icon), text, card);
        button->setIconSize({15, 15});
        content->addWidget(button, 0, Qt::AlignLeft);
        grid->addWidget(card, row, column);
        return button;
    }

    // A disabled action must not leave a button that silently does nothing.
    void shellFollowAction(QPushButton *button, QAction *action) {
        button->setEnabled(action->isEnabled());
        QObject::connect(action, &QAction::enabledChanged, button, &QPushButton::setEnabled);
        QObject::connect(button, &QPushButton::clicked, action, &QAction::trigger);
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

    for (const auto &[icon, action] : QList<QPair<QString, QAction *>>{
             {QStringLiteral("play"), ui->menu_start}, {QStringLiteral("stop"), ui->menu_stop}, {QStringLiteral("edit"), ui->menu_edit}, {QStringLiteral("copy"), ui->menu_clone},
             {QStringLiteral("qr"), ui->menu_qr}, {QStringLiteral("download"), ui->menu_export_config}, {QStringLiteral("link"), ui->menu_copy_links},
             {QStringLiteral("link"), ui->menu_copy_links_nkr}, {QStringLiteral("clip"), ui->actionCopy_Test_Result}, {QStringLiteral("gauge"), ui->actionUrl_Test_Selected},
             {QStringLiteral("bolt"), ui->actionSpeedtest_Selected}, {QStringLiteral("globe"), ui->actionResolve_Selected_Out_IP}, {QStringLiteral("search"), ui->menu_resolve_selected},
             {QStringLiteral("close"), ui->actionClear_Test_Result}, {QStringLiteral("refresh"), ui->menu_reset_traffic}, {QStringLiteral("trash"), ui->menu_delete}}) {
        action->setIcon(PorosTheme::Icon(icon));
    }

    m_groupInfo = new SubscriptionInfoCard(profiles);
    auto *shareLinks = new QAction(tr("Share links"), m_groupInfo);
    connect(shareLinks, &QAction::triggered, this, &MainWindow::copyGroupLinks);
    auto *groupMenu = shellMenu({{QStringLiteral("refresh"), ui->menu_update_subscription}, {QStringLiteral("edit"), ui->actionEdit_Group},
                                 {QStringLiteral("share"), shareLinks}, {QStringLiteral("radar"), ui->actionUpdate_All_Subscriptions},
                                 {QStringLiteral("layers"), ui->menu_manage_groups}, {{}, nullptr}, {QStringLiteral("trash"), ui->actionDelete_Group}}, m_groupInfo);
    connect(groupMenu, &QMenu::aboutToShow, this, [this] {
        ui->actionDelete_Group->setEnabled(Configs::dataManager->groupsRepo->GetGroupsTabOrder().size() > 1);
    });
    m_groupInfo->setActions(ui->menu_update_subscription, ui->actionEdit_Group, groupMenu);

    auto *toolbar = new QFrame(profiles);
    toolbar->setProperty("role", QStringLiteral("toolbar"));
    auto *toolbarRow = new QHBoxLayout(toolbar);
    toolbarRow->setContentsMargins(14, 10, 14, 10);
    toolbarRow->setSpacing(8);
    m_profileSearch = new TableSearchEdit(toolbar);
    m_profileSearch->setPlaceholderText(tr("Search profiles"));
    toolbarRow->addWidget(m_profileSearch, 1);
    auto *newProfile = shellSmallButton(QStringLiteral("plus"), tr("Profile"), toolbar);
    newProfile->setProperty("variant", QStringLiteral("primary"));
    connect(newProfile, &QPushButton::clicked, ui->menu_add_from_input, &QAction::trigger);
    toolbarRow->addWidget(newProfile);
    auto *enterLink = new QAction(tr("Enter a link…"), toolbar);
    connect(enterLink, &QAction::triggered, this, [this, enterLink] {
        bool ok = false;
        const QString text = QInputDialog::getMultiLineText(this, enterLink->text().remove(QChar(0x2026)), tr("Profile links or a subscription URL"), {}, &ok);
        if (ok && !text.trimmed().isEmpty()) import_or_handle_deeplink(text);
    });
    const auto dropButton = [toolbar, toolbarRow](const QString &icon, const QString &text, QMenu *menu) {
        auto *button = shellSmallButton(icon, text, toolbar);
        button->setProperty("menu", true);
        button->setMenu(menu);
        toolbarRow->addWidget(button);
    };
    dropButton(QStringLiteral("download"), tr("Import"),
               shellMenu({{QStringLiteral("clip"), ui->menu_add_from_clipboard}, {QStringLiteral("file"), ui->actionAdd_profile_from_File},
                          {QStringLiteral("qr"), ui->menu_scan_qr}, {QStringLiteral("edit"), enterLink}}, toolbar));
    dropButton(QStringLiteral("gauge"), tr("Tests"),
               shellMenu({{QStringLiteral("gauge"), ui->actionUrl_Test_Group}, {QStringLiteral("bolt"), ui->actionSpeedtest_Group},
                          {QStringLiteral("globe"), ui->actionResolve_Out_IP}, {QStringLiteral("search"), ui->menu_resolve_domain}, {{}, nullptr},
                          {QStringLiteral("close"), ui->menu_clear_test_result}, {QStringLiteral("stop"), ui->menu_stop_testing}}, toolbar));
    dropButton(QStringLiteral("trash"), tr("Clean up"),
               shellMenu({{QStringLiteral("trash"), ui->menu_remove_unavailable}, {QStringLiteral("copy"), ui->menu_delete_repeat},
                          {QStringLiteral("warn"), ui->menu_remove_insecure}, {QStringLiteral("close"), ui->menu_remove_invalid}}, toolbar));

    m_profilesEmpty = new QWidget(ui->profilesTableView->viewport());
    auto *emptyColumn = new QVBoxLayout(m_profilesEmpty);
    emptyColumn->setSpacing(10);
    m_profilesEmptyTitle = new QLabel(m_profilesEmpty);
    m_profilesEmptyTitle->setProperty("role", QStringLiteral("heading"));
    m_profilesEmptyTitle->setAlignment(Qt::AlignCenter);
    emptyColumn->addWidget(m_profilesEmptyTitle);
    m_profilesEmptyHint = new QLabel(m_profilesEmpty);
    m_profilesEmptyHint->setProperty("role", QStringLiteral("lead"));
    m_profilesEmptyHint->setAlignment(Qt::AlignCenter);
    emptyColumn->addWidget(m_profilesEmptyHint);
    m_profilesEmptyAdd = new QWidget(m_profilesEmpty);
    auto *emptyAddRow = new QHBoxLayout(m_profilesEmptyAdd);
    emptyAddRow->setContentsMargins(0, 4, 0, 0);
    emptyAddRow->setAlignment(Qt::AlignCenter);
    auto *emptyProfile = shellSmallButton(QStringLiteral("plus"), tr("Profile"), m_profilesEmptyAdd);
    emptyProfile->setProperty("variant", QStringLiteral("primary"));
    connect(emptyProfile, &QPushButton::clicked, ui->menu_add_from_input, &QAction::trigger);
    emptyAddRow->addWidget(emptyProfile);
    auto *emptyClipboard = shellSmallButton(QStringLiteral("clip"), tr("From clipboard"), m_profilesEmptyAdd);
    connect(emptyClipboard, &QPushButton::clicked, ui->menu_add_from_clipboard, &QAction::trigger);
    emptyAddRow->addWidget(emptyClipboard);
    emptyColumn->addWidget(m_profilesEmptyAdd);
    m_profilesEmptyUpdate = shellSmallButton(QStringLiteral("refresh"), tr("Update"), m_profilesEmpty);
    connect(m_profilesEmptyUpdate, &QPushButton::clicked, ui->menu_update_subscription, &QAction::trigger);
    emptyColumn->addWidget(m_profilesEmptyUpdate, 0, Qt::AlignCenter);
    auto *emptyPlace = new QVBoxLayout(ui->profilesTableView->viewport());
    emptyPlace->addWidget(m_profilesEmpty, 0, Qt::AlignCenter);
    m_profilesEmpty->hide();

    ui->tabWidget->tabBar()->hide();
    ui->tabWidget->setDocumentMode(true);
    auto *profilesHead = new QWidget(profiles);
    auto *profilesHeadColumn = new QVBoxLayout(profilesHead);
    profilesHeadColumn->setContentsMargins(0, 0, 0, 0);
    profilesHeadColumn->setSpacing(0);
    profilesHeadColumn->addWidget(m_groupInfo);
    profilesHeadColumn->addWidget(toolbar);
    profilesRow->addWidget(shellSection(profilesHead, ui->tabWidget, profiles), 1);
    m_advancedPage->setSection(AdvancedPage::Profiles, profiles);

    ui->stats_widget->removeTab(ui->stats_widget->indexOf(ui->Logs));
    ui->stats_widget->removeTab(ui->stats_widget->indexOf(ui->connections_tab));
    // A page taken out of a tab widget stays explicitly hidden.
    ui->Logs->show();
    m_advancedPage->setSection(AdvancedPage::Logs, ui->Logs);
    ui->connections_tab->show();
    m_advancedPage->setSection(AdvancedPage::Connections, ui->connections_tab);
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
    auto *otherTools = new QWidget(m_toolTabs);
    auto *toolGrid = new QGridLayout(otherTools);
    toolGrid->setContentsMargins(14, 14, 14, 14);
    toolGrid->setSpacing(12);
    toolGrid->setColumnStretch(0, 1);
    toolGrid->setColumnStretch(1, 1);
    toolGrid->setRowStretch(2, 1);
    auto *speedTest = shellToolCard(toolGrid, 0, 0, tr("Speed test of the current profile"), tr("The mode and addresses are set in the test settings."), QStringLiteral("bolt"), tr("Run"));
    speedTest->setToolTip(tr("Available while connected"));
    shellFollowAction(speedTest, ui->actionSpeedtest_Current);
    auto *ruleSets = shellToolCard(toolGrid, 0, 1, tr("Rule sets"), tr("Update geoip, geosite and remote sets."), QStringLiteral("refresh"), tr("Update"));
    connect(ruleSets, &QPushButton::clicked, this, &MainWindow::updateRuleSets);
    shellFollowAction(shellToolCard(toolGrid, 1, 0, tr("Poros updates"), tr("Check for a new version."), QStringLiteral("download"), tr("Check")), ui->actionCheck_For_Update);
    m_toolTabs->addTab(otherTools, tr("Other"));
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
