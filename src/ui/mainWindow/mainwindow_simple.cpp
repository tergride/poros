#include "include/ui/mainwindow.h"

#include <QApplication>
#include <QClipboard>
#include <QInputDialog>
#include <QMenu>
#include <QMessageBox>
#include <QPushButton>
#include <QStackedWidget>
#include <QTextBrowser>
#include <QTimer>
#include <QVBoxLayout>

#include "include/database/GroupsRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/database/RoutesRepo.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/engine/EngineHost.h"
#include "include/engine/services/AutoGroupService.h"
#include "include/engine/services/GroupService.h"
#include "include/engine/tests/TestRunner.h"
#include "include/ui/setting/PorosTheme.h"
#include "include/ui/shell/SegmentSwitch.h"
#include "include/ui/simple/AddSubscriptionDialog.h"
#include "include/ui/simple/PortalPanel.h"
#include "include/ui/simple/ServerListView.h"
#include "include/ui/simple/PresetEditor.h"
#include "include/ui/simple/ServerTreeModel.h"
#include "include/ui/simple/SimpleSettings.h"
#include "include/ui/simple/SimpleHome.h"
#include "include/ui/utils/ConnectionModeController.h"
#include "include/ui/widget/AdminPromptDialog.h"
#include "include/ui/widget/ConnectionChoiceDialog.h"
#include "include/ui/widget/StartStopButton.hpp"

void MainWindow::setupSimpleMode() {
    m_serverModel = new ServerTreeModel(this);
    m_simpleHome = new SimpleHome(m_serverModel, this);
    m_modes = new ConnectionModeController(connection(), this);

    m_simpleStack = new QStackedWidget(this);
    m_simpleStack->addWidget(m_simpleHome);

    SimpleSettings::Hooks hooks;
    hooks.back = [this] {
        m_simpleStack->setCurrentWidget(m_simpleHome);
        refreshSimplePortal();
    };
    hooks.chooseRoute = [this](int id) {
        connection()->chooseRoute(id);
        refreshRouteChoice();
        refreshSimplePortal();
    };
    hooks.routeEdited = [this] { refreshRouteChoice(); };
    hooks.editPreset = [this](int id) {
        const auto route = Configs::dataManager->routesRepo->GetRouteProfile(id);
        if (route == nullptr) return;
        m_presetEditor->edit(route);
        m_simpleStack->setCurrentWidget(m_presetEditor);
    };
    hooks.setAllowLan = [this](bool on) { setAllowLan(on); };
    hooks.trayChanged = [] { MW_dialog_message(MwMessage::UpdateSettings, {MwArg::DisableTray}); };
    hooks.restartProgram = [] { MW_dialog_message(MwMessage::RestartProgram, {}); };
    hooks.showLog = [this] { showSimpleLog(); };
    hooks.runWizard = [this] { showOnboarding(); };
    hooks.setUiMode = [this](const QString &mode) { setUiMode(mode); };
    hooks.resetCore = [this] { ui->actionRestart_Proxy->trigger(); };
    hooks.checkForUpdates = [this] { ui->actionCheck_For_Update->trigger(); };
    m_simpleSettings = new SimpleSettings(hooks, m_simpleStack);
    m_simpleStack->addWidget(m_simpleSettings);

    m_presetEditor = new PresetEditor(m_simpleStack);
    m_simpleStack->addWidget(m_presetEditor);
    connect(m_presetEditor, &PresetEditor::closed, this, [this] { showSimpleSettings(0); });
    connect(m_presetEditor, &PresetEditor::saved, this, [this](int id) {
        if (Configs::dataManager->settingsRepo->current_route_id == id) connection()->restartStartedProfile();
        else connection()->chooseRoute(id);
        refreshRouteChoice();
        showSimpleSettings(0);
        refreshSimplePortal();
    });

    auto *portal = m_simpleHome->portal();
    connect(portal, &PortalPanel::presetClicked, this, [this] { showSimpleSettings(0); });
    m_startButtons << portal->startButton();
    connect(portal->startButton(), &QAbstractButton::clicked, this, &MainWindow::simpleStart);
    connect(portal->modeSwitch(), &SegmentSwitch::activated, this, [this](int index) {
        m_modes->choose(index == 0 ? ConnectionModeController::Mode::Tun : ConnectionModeController::Mode::System);
    });
    connect(m_modes, &ConnectionModeController::modeChanged, this, &MainWindow::refreshSimplePortal);

    auto *list = m_simpleHome->list();
    connect(list, &ServerListView::serverPicked, this, &MainWindow::simplePick);
    connect(list, &ServerListView::autoPicked, this, [this](int groupId) {
        const int id = Engine::AutoGroupService::Ensure(groupId);
        if (id < 0) return;
        if (m_serverModel->groups().end() != std::find_if(m_serverModel->groups().begin(), m_serverModel->groups().end(),
                                                         [groupId](const auto &group) { return group.id == groupId && group.autoId < 0; })) {
            m_serverModel->reload();
        }
        simplePick(id);
    });
    connect(list, &ServerListView::refreshRequested, this, [this](int groupId) {
        Engine::GroupService::Update(groupId, [](const Subscription::UpdateOutcome &) {});
    });
    connect(list, &ServerListView::groupMenuRequested, this, &MainWindow::showSimpleGroupMenu);
    connect(m_simpleHome, &SimpleHome::addRequested, this, &MainWindow::showAddSubscription);
    connect(m_simpleHome, &SimpleHome::wizardRequested, this, &MainWindow::showOnboarding);
    connect(m_simpleHome, &SimpleHome::testRequested, this, [this] {
        QList<int> ids;
        for (const auto &group : m_serverModel->groups()) {
            for (const auto &server : group.servers) ids << server.id;
        }
        tests()->runUrlTests(ids);
    });

    auto *simpleClock = new QTimer(this);
    simpleClock->setInterval(1000);
    connect(simpleClock, &QTimer::timeout, this, [this] {
        if (m_simpleHome->isVisible() && runningSnapshot() != nullptr) refreshSimplePortal();
    });
    simpleClock->start();

    m_serverModel->setSelectedId(Configs::dataManager->settingsRepo->remember_id);
    m_serverModel->reload();
}

void MainWindow::showSimpleSettings(int section) {
    m_simpleSettings->reload();
    m_simpleSettings->showSection(section);
    m_simpleStack->setCurrentWidget(m_simpleSettings);
}

void MainWindow::showSimpleLog() {
    auto *dialog = new QDialog(this);
    dialog->setAttribute(Qt::WA_DeleteOnClose);
    dialog->setWindowTitle(tr("Log"));
    dialog->resize(860, 560);
    auto *column = new QVBoxLayout(dialog);
    auto *browser = new QTextBrowser(dialog);
    browser->setDocument(qvLogDocument);
    browser->setFont(ui->masterLogBrowser->font());
    column->addWidget(browser, 1);
    auto *buttons = new QHBoxLayout;
    buttons->addStretch();
    auto *copy = new QPushButton(PorosTheme::Icon(QStringLiteral("copy")), tr("Copy for support"), dialog);
    connect(copy, &QPushButton::clicked, this, [this] { QApplication::clipboard()->setText(qvLogDocument->toPlainText()); });
    buttons->addWidget(copy);
    column->addLayout(buttons);
    dialog->open();
}

void MainWindow::simplePick(int profileId) {
    m_serverModel->setSelectedId(profileId);
    auto &settings = Configs::dataManager->settingsRepo;
    settings->remember_id = profileId;
    settings->Save();
    if (runningSnapshot() != nullptr) profile_start(profileId);
    refreshSimplePortal();
    refresh_startstop_button();
}

bool MainWindow::simpleModeShown() const {
    return Configs::dataManager->settingsRepo->ui_mode != QLatin1String("advanced");
}

int MainWindow::simpleProfileToStart() const {
    if (const int id = m_serverModel->selectedId(); id >= 0 && Configs::dataManager->profilesRepo->GetProfile(id) != nullptr) return id;
    for (const auto &group : m_serverModel->groups()) {
        if (!group.servers.isEmpty()) return group.servers.first().id;
    }
    return -1;
}

void MainWindow::simpleStart() {
    if (runningSnapshot() != nullptr) {
        profile_stop(false, false, true);
        return;
    }
    const int id = simpleProfileToStart();
    if (id < 0) return;
    if (m_modes->mode() == ConnectionModeController::Mode::Unset) {
        ConnectionChoiceDialog dialog(this);
        if (dialog.exec() != QDialog::Accepted) return;
        m_modes->choose(dialog.choice());
    }
    m_banner->dismiss();
    m_pendingStart = id;
    // The rights prompt can switch to System, which then needs no rights at all.
    const bool applied = m_modes->apply() || (m_modes->mode() == ConnectionModeController::Mode::System && m_modes->apply());
    m_pendingStart = -1;
    if (!applied) return;
    simplePick(id);
    if (runningSnapshot() == nullptr) profile_start(id);
}

void MainWindow::showAddSubscription(const QString &link) {
    auto *dialog = new AddSubscriptionDialog(link, this);
    dialog->setAttribute(Qt::WA_DeleteOnClose);
    connect(dialog, &AddSubscriptionDialog::advancedModeRequested, this, [this] { setUiMode(QStringLiteral("advanced")); });
    dialog->open();
}

void MainWindow::showSimpleGroupMenu(int groupId, const QPoint &globalPos) {
    QMenu menu(this);
    const auto item = [&menu](const QString &icon, const QString &text) { return menu.addAction(PorosTheme::Icon(icon), text); };

    connect(item(QStringLiteral("refresh"), tr("Update")), &QAction::triggered, this, [groupId] {
        Engine::GroupService::Update(groupId, [](const Subscription::UpdateOutcome &) {});
    });
    connect(item(QStringLiteral("gauge"), tr("Test servers")), &QAction::triggered, this, [this, groupId] {
        if (const auto group = Configs::dataManager->groupsRepo->GetGroup(groupId)) tests()->runUrlTests(group->Profiles());
    });
    connect(item(QStringLiteral("copy"), tr("Copy link")), &QAction::triggered, this, [groupId] {
        if (const auto group = Configs::dataManager->groupsRepo->GetGroup(groupId)) QApplication::clipboard()->setText(group->url);
    });
    connect(item(QStringLiteral("edit"), tr("Rename")), &QAction::triggered, this, [this, groupId] {
        const auto group = Configs::dataManager->groupsRepo->GetGroup(groupId);
        if (group == nullptr) return;
        bool ok = false;
        const QString name = QInputDialog::getText(this, tr("Rename"), tr("Name"), QLineEdit::Normal, group->name, &ok).trimmed();
        if (!ok || name.isEmpty()) return;
        group->name = name;
        Configs::dataManager->groupsRepo->Save(group);
        MW_dialog_message(MwMessage::GroupsChanged, {});
    });
    menu.addSeparator();
    connect(item(QStringLiteral("trash"), tr("Delete subscription")), &QAction::triggered, this, [this, groupId] {
        const auto group = Configs::dataManager->groupsRepo->GetGroup(groupId);
        if (group == nullptr) return;
        if (QMessageBox::question(this, tr("Confirmation"), tr("Remove %1?").arg(group->name)) == QMessageBox::Yes) {
            delete_group(groupId, tr("Delete subscription"));
        }
    });
    menu.exec(globalPos);
}

void MainWindow::refreshSimplePortal() {
    if (m_simpleHome == nullptr) return;
    using Mode = ConnectionModeController::Mode;
    auto *portal = m_simpleHome->portal();
    const auto &settings = Configs::dataManager->settingsRepo;
    const auto running = runningSnapshot();
    const auto state = connection()->connectionState();

    if (running != nullptr && !m_simpleSince.isValid()) m_simpleSince.start();
    if (running == nullptr) m_simpleSince.invalidate();

    if (state == ConnectionState::Connecting) {
        portal->setState(PortalPanel::State::Busy, tr("Connecting…"));
    } else if (state == ConnectionState::Stopping) {
        portal->setState(PortalPanel::State::Busy, tr("Stopping…"));
    } else if (running != nullptr) {
        const qint64 seconds = m_simpleSince.isValid() ? m_simpleSince.elapsed() / 1000 : 0;
        portal->setState(PortalPanel::State::On, tr("Connected · %1").arg(QStringLiteral("%1:%2:%3")
                                                                              .arg(seconds / 3600, 2, 10, QChar('0'))
                                                                              .arg(seconds / 60 % 60, 2, 10, QChar('0'))
                                                                              .arg(seconds % 60, 2, 10, QChar('0'))));
    } else {
        portal->setState(PortalPanel::State::Off, tr("Not connected"));
    }

    const int shownId = running != nullptr ? running->id : simpleProfileToStart();
    if (running == nullptr) m_serverModel->setSelectedId(shownId);
    const auto shown = shownId >= 0 ? Configs::dataManager->profilesRepo->GetProfile(shownId) : nullptr;
    if (shown == nullptr) {
        portal->setServer(tr("No server selected"), tr("Add a subscription to begin"));
    } else {
        const auto group = Configs::dataManager->groupsRepo->GetGroup(shown->gid);
        QString meta = running != nullptr ? running->runningCountryInfo : (group != nullptr ? group->name : QString());
        if (shown->latency > 0) meta += (meta.isEmpty() ? QString() : QStringLiteral(" · ")) + tr("%1 ms").arg(shown->latency);
        portal->setServer(shown->outbound ? shown->outbound->DisplayName() : shown->name, meta);
    }

    if (const auto rates = running != nullptr ? Engine::host()->freshLiveRates(3000) : std::nullopt) {
        const auto perSecond = [](qint64 bytes) { return ReadableSize(bytes) + tr("/s"); };
        portal->setStats(perSecond(rates->proxyDown + rates->directDown), perSecond(rates->proxyUp + rates->directUp), running->test_country);
    } else {
        portal->setStats({}, {}, {});
    }

    const Mode mode = m_modes->mode();
    portal->modeSwitch()->setCurrent(mode == Mode::Tun ? 0 : mode == Mode::System ? 1 : -1);
    portal->setModeHint(mode == Mode::Tun ? tr("All the traffic of the computer goes through the server, games and programs included.")
                        : mode == Mode::System ? tr("Browsers and the programs that use the system proxy go through the server.")
                                               : tr("Choose how to connect, or Poros will ask at the first start."));

    const auto route = Configs::dataManager->routesRepo->GetRouteProfile(settings->current_route_id);
    portal->setPreset(route != nullptr ? route->name : QString());

    m_serverModel->setRunningId(running != nullptr ? running->id : -1);
    if (running != nullptr) m_serverModel->setSelectedId(running->id);
}
