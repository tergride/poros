#include "include/ui/simple/SimpleSettings.h"
#include "NkrVersion.h"

#include <QApplication>
#include <QButtonGroup>
#include <QCheckBox>
#include <QClipboard>
#include <QComboBox>
#include <QDir>
#include <QFrame>
#include <QHBoxLayout>
#include <QJsonObject>
#include <QLabel>
#include <QLayoutItem>
#include <QMessageBox>
#include <QPainter>
#include <QPushButton>
#include <QRadioButton>
#include <QScrollArea>
#include <QStackedWidget>
#include <QToolButton>
#include <QVBoxLayout>

#include "include/configs/PresetCatalog.h"
#include "include/database/RoutesRepo.h"
#include "include/engine/EngineHost.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/engine/services/BackupArchive.h"
#include "include/engine/services/ReleaseUpdates.h"
#include "include/engine/services/SettingsRegistry.h"
#include "include/global/Configs.hpp"
#include "include/sys/AutoRun.hpp"
#include "include/sys/KillSwitch.hpp"
#include "include/sys/UrlScheme.hpp"
#include "include/ui/setting/PorosTheme.h"
#include "include/ui/shell/SegmentSwitch.h"
#include "include/ui/simple/SimpleSettingsNav.h"
#include "include/ui/utils/ConnectionModeController.h"
#include "include/ui/utils/NativeFileDialog.h"
#include "include/ui/utils/NoWheelComboBox.h"

namespace {
    auto &simpleSettingsRepo() { return Configs::dataManager->settingsRepo; }

    void simpleSettingsPut(const QJsonObject &patch) {
        const auto connection = Engine::host()->connection();
        Engine::SettingsRegistry::PutResult result;
        const auto &settings = simpleSettingsRepo();
        Engine::SettingsRegistry::Put(patch, {connection->runningSnapshot() != nullptr, settings->spmode_vpn, settings->spmode_system_proxy}, &result);
    }

    QLabel *simpleSettingsLabel(const QString &text, const char *role, QWidget *parent) {
        auto *label = new QLabel(text, parent);
        if (role != nullptr) label->setProperty("role", QString::fromLatin1(role));
        label->setWordWrap(true);
        return label;
    }

    QWidget *simpleSettingsPage(const QString &title, const QString &lead, QVBoxLayout **column, QWidget *parent) {
        auto *scroll = new QScrollArea(parent);
        scroll->setWidgetResizable(true);
        scroll->setFrameShape(QFrame::NoFrame);
        auto *page = new QWidget(scroll);
        *column = new QVBoxLayout(page);
        (*column)->setContentsMargins(28, 22, 28, 28);
        (*column)->setSpacing(12);
        (*column)->addWidget(simpleSettingsLabel(title, "sectionTitle", page));
        auto *leadLabel = simpleSettingsLabel(lead, "lead", page);
        leadLabel->setMaximumWidth(640);
        (*column)->addWidget(leadLabel);
        scroll->setWidget(page);
        return scroll;
    }

    QFrame *simpleSettingsCard(const QString &title, QVBoxLayout **column, QWidget *parent) {
        auto *card = new QFrame(parent);
        card->setProperty("role", QStringLiteral("card"));
        *column = new QVBoxLayout(card);
        (*column)->setContentsMargins(16, 10, 16, 10);
        (*column)->setSpacing(0);
        if (!title.isEmpty()) (*column)->addWidget(simpleSettingsLabel(title, "cardTitle", card));
        return card;
    }

    void simpleSettingsRow(QVBoxLayout *card, const QString &title, const QString &hint, QWidget *control) {
        auto *row = new QWidget(card->parentWidget());
        auto *rowLayout = new QHBoxLayout(row);
        rowLayout->setContentsMargins(0, 10, 0, 10);
        rowLayout->setSpacing(14);
        auto *text = new QVBoxLayout;
        text->setSpacing(1);
        text->addWidget(simpleSettingsLabel(title, nullptr, row));
        if (!hint.isEmpty()) text->addWidget(simpleSettingsLabel(hint, "hint", row));
        rowLayout->addLayout(text, 1);
        control->setParent(row);
        rowLayout->addWidget(control, 0, Qt::AlignVCenter);
        card->addWidget(row);
    }

    QCheckBox *simpleSettingsSwitch() {
        auto *box = new QCheckBox;
        box->setProperty("role", QStringLiteral("switch"));
        return box;
    }
} // namespace

SimpleSettings::SimpleSettings(const Hooks &hooks, QWidget *parent) : QWidget(parent), m_hooks(hooks) {
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(0, 0, 0, 0);
    row->setSpacing(0);

    auto *nav = new QWidget(this);
    nav->setObjectName(QStringLiteral("settingsNav"));
    nav->setAttribute(Qt::WA_StyledBackground);
    nav->setFixedWidth(232);
    auto *navColumn = new QVBoxLayout(nav);
    navColumn->setContentsMargins(12, 12, 12, 12);
    navColumn->setSpacing(2);
    auto *back = new SimpleSettingsNavButton(QStringLiteral("back"), tr("To the servers"), nav);
    back->setCheckable(false);
    connect(back, &QAbstractButton::clicked, this, [this] { m_hooks.back(); });
    navColumn->addWidget(back);
    auto *separator = new QFrame(nav);
    separator->setFrameShape(QFrame::HLine);
    navColumn->addWidget(separator);

    m_pages = new QStackedWidget(this);
    const struct {
        const char *icon;
        QString title;
        QWidget *page;
    } sections[] = {
        {"layers", tr("Presets"), buildPresets()},
        {"globe", tr("Connection"), buildConnection()},
        {"link", tr("Subscriptions"), buildSubscriptions()},
        {"power", tr("Launch"), buildStartup()},
        {"app", tr("Appearance"), buildAppearance()},
        {"download", tr("Backup"), buildBackup()},
        {"info", tr("Help"), buildHelp()},
    };
    for (const auto &section : sections) {
        const int index = m_pages->addWidget(section.page);
        auto *button = new SimpleSettingsNavButton(QString::fromLatin1(section.icon), section.title, nav);
        connect(button, &QAbstractButton::clicked, this, [this, index] { showSection(index); });
        navColumn->addWidget(button);
        m_nav << button;
    }
    navColumn->addStretch();
    row->addWidget(nav);
    row->addWidget(m_pages, 1);
    showSection(0);
}

void SimpleSettings::showSection(int index) {
    m_pages->setCurrentIndex(index);
    for (int i = 0; i < m_nav.size(); i++) m_nav[i]->setChecked(i == index);
}

void SimpleSettings::reload() {
    rebuildPresetRows();
    for (const auto &reload : m_reloaders) reload();
}

QWidget *SimpleSettings::buildPresets() {
    QVBoxLayout *column = nullptr;
    auto *page = simpleSettingsPage(tr("Presets"), tr("A preset decides which traffic goes through the server and which goes direct. "
                                                      "Built-in presets do not change: to add your own settings, make a copy."), &column, this);
    QVBoxLayout *builtin = nullptr;
    column->addWidget(simpleSettingsCard(tr("Built-in"), &builtin, page));
    m_builtinPresets = new QVBoxLayout;
    m_builtinPresets->setSpacing(0);
    builtin->addLayout(m_builtinPresets);
    QVBoxLayout *own = nullptr;
    column->addWidget(simpleSettingsCard(tr("My presets"), &own, page));
    m_ownPresets = new QVBoxLayout;
    m_ownPresets->setSpacing(0);
    own->addLayout(m_ownPresets);

    auto *actions = new QHBoxLayout;
    actions->setSpacing(8);
    auto *import = new QPushButton(PorosTheme::Icon(QStringLiteral("download")), tr("Import from clipboard"), page);
    connect(import, &QPushButton::clicked, this, &SimpleSettings::importPreset);
    actions->addWidget(import);
    auto *exportButton = new QPushButton(PorosTheme::Icon(QStringLiteral("upload")), tr("Copy current as a link"), page);
    connect(exportButton, &QPushButton::clicked, this, &SimpleSettings::exportPreset);
    actions->addWidget(exportButton);
    actions->addStretch();
    column->addLayout(actions);
    column->addStretch();
    return page;
}

void SimpleSettings::rebuildPresetRows() {
    const auto clear = [](QLayout *layout) {
        while (auto *item = layout->takeAt(0)) {
            delete item->widget();
            delete item;
        }
    };
    clear(m_builtinPresets);
    clear(m_ownPresets);

    auto routes = Configs::dataManager->routesRepo->GetAllRouteProfiles();
    const auto keys = Configs::PresetCatalog::Keys();
    std::stable_sort(routes.begin(), routes.end(), [&keys](const auto &a, const auto &b) { return keys.indexOf(a->builtinKey) < keys.indexOf(b->builtinKey); });
    const int current = simpleSettingsRepo()->current_route_id;
    for (const auto &route : routes) {
        const bool builtin = route->IsBuiltin();
        auto *parent = (builtin ? m_builtinPresets : m_ownPresets)->parentWidget();
        auto *row = new QWidget(parent);
        auto *rowLayout = new QHBoxLayout(row);
        rowLayout->setContentsMargins(0, 10, 0, 10);
        rowLayout->setSpacing(12);

        auto *radio = new QRadioButton(row);
        radio->setAutoExclusive(false);
        radio->setChecked(route->id == current);
        connect(radio, &QRadioButton::clicked, this, [this, id = route->id] {
            m_hooks.chooseRoute(id);
            rebuildPresetRows();
        });
        rowLayout->addWidget(radio);

        auto *text = new QVBoxLayout;
        text->setSpacing(1);
        text->addWidget(simpleSettingsLabel(route->name, nullptr, row));
        QString note = builtin ? Configs::PresetCatalog::Description(route->builtinKey) : QString();
        if (!builtin && !route->baseKey.isEmpty()) note = tr("Based on “%1”").arg(Configs::PresetCatalog::DisplayName(route->baseKey));
        if (!note.isEmpty()) text->addWidget(simpleSettingsLabel(note, "hint", row));
        rowLayout->addLayout(text, 1);

        if (builtin) {
            auto *copy = new QPushButton(PorosTheme::Icon(QStringLiteral("copy")), tr("Make a copy"), row);
            connect(copy, &QPushButton::clicked, this, [this, id = route->id] { copyPreset(id); });
            rowLayout->addWidget(copy);
        } else {
            auto *edit = new QPushButton(PorosTheme::Icon(QStringLiteral("edit")), tr("Edit"), row);
            connect(edit, &QPushButton::clicked, this, [this, id = route->id] { m_hooks.editPreset(id); });
            rowLayout->addWidget(edit);
            auto *remove = new QToolButton(row);
            remove->setIcon(PorosTheme::Icon(QStringLiteral("trash")));
            remove->setToolTip(tr("Delete"));
            connect(remove, &QToolButton::clicked, this, [this, id = route->id] { deletePreset(id); });
            rowLayout->addWidget(remove);
        }
        (builtin ? m_builtinPresets : m_ownPresets)->addWidget(row);
    }
    if (m_ownPresets->isEmpty()) m_ownPresets->addWidget(simpleSettingsLabel(tr("Make a copy of a built-in preset to add your own sites and programs."), "hint", m_ownPresets->parentWidget()));
}

void SimpleSettings::copyPreset(int routeId) {
    const auto source = Configs::dataManager->routesRepo->GetRouteProfile(routeId);
    if (source == nullptr) return;
    auto copy = std::make_shared<Configs::RouteProfile>(*source);
    copy->id = -1;
    copy->DetachFromBuiltin();
    copy->name = tr("%1 (copy)").arg(source->name);
    Configs::dataManager->routesRepo->AddRouteProfile(copy);
    rebuildPresetRows();
    m_hooks.editPreset(copy->id);
}

void SimpleSettings::deletePreset(int routeId) {
    const auto route = Configs::dataManager->routesRepo->GetRouteProfile(routeId);
    if (route == nullptr || QMessageBox::question(this, tr("Delete"), tr("Remove %1?").arg(route->name)) != QMessageBox::Yes) return;
    if (simpleSettingsRepo()->current_route_id == routeId) {
        for (const auto &other : Configs::dataManager->routesRepo->GetAllRouteProfiles()) {
            if (other->builtinKey == QLatin1String("all")) m_hooks.chooseRoute(other->id);
        }
    }
    Configs::dataManager->routesRepo->DeleteRouteProfile(routeId);
    rebuildPresetRows();
}

void SimpleSettings::importPreset() {
    QString error;
    QString warnings;
    const auto imported = Configs::RouteProfile::FromShareInput(QApplication::clipboard()->text(), &error, &warnings, nullptr);
    if (imported == nullptr) {
        QMessageBox::warning(this, tr("Import from clipboard"), tr("The clipboard holds no preset: %1").arg(error));
        return;
    }
    auto added = imported;
    Configs::dataManager->routesRepo->AddRouteProfile(added);
    rebuildPresetRows();
}

void SimpleSettings::exportPreset() {
    const auto route = Configs::dataManager->routesRepo->GetRouteProfile(simpleSettingsRepo()->current_route_id);
    if (route == nullptr) return;
    QApplication::clipboard()->setText(route->ToShareLink());
}

QWidget *SimpleSettings::buildConnection() {
    QVBoxLayout *column = nullptr;
    auto *page = simpleSettingsPage(tr("Connection"), tr("How Poros connects and what it does when the connection breaks."), &column, this);
    QVBoxLayout *card = nullptr;
    column->addWidget(simpleSettingsCard({}, &card, page));

    auto *killSwitch = simpleSettingsSwitch();
    connect(killSwitch, &QCheckBox::clicked, this, [](bool on) { simpleSettingsPut({{QStringLiteral("kill_switch"), on}}); });
    simpleSettingsRow(card, tr("Kill switch"), tr("Block the internet when the connection to the server breaks."), killSwitch);

    auto *lan = simpleSettingsSwitch();
    connect(lan, &QCheckBox::clicked, this, [this](bool on) { m_hooks.setAllowLan(on); });
    simpleSettingsRow(card, tr("Let other devices connect"), tr("A phone or a TV on this network can go out through Poros."), lan);

    auto *exitLookup = simpleSettingsSwitch();
    connect(exitLookup, &QCheckBox::clicked, this, [](bool on) {
        simpleSettingsRepo()->exit_country_lookup = on;
        simpleSettingsRepo()->Save();
    });
    simpleSettingsRow(card, tr("Show the exit IP and country"), tr("After connecting Poros asks an outside service for them."), exitLookup);

    column->addStretch();
    m_reloaders << [=] {
        killSwitch->setChecked(simpleSettingsRepo()->kill_switch);
        lan->setChecked(QStringList{"::", "0.0.0.0"}.contains(simpleSettingsRepo()->inbound_address));
        exitLookup->setChecked(simpleSettingsRepo()->exit_country_lookup);
    };
    return page;
}

QWidget *SimpleSettings::buildSubscriptions() {
    QVBoxLayout *column = nullptr;
    auto *page = simpleSettingsPage(tr("Subscriptions"), tr("How the server list stays up to date."), &column, this);
    QVBoxLayout *card = nullptr;
    column->addWidget(simpleSettingsCard({}, &card, page));

    auto *autoUpdate = simpleSettingsSwitch();
    simpleSettingsRow(card, tr("Update automatically"), {}, autoUpdate);
    auto *interval = new NoWheelComboBox;
    interval->setFixedWidth(220);
    interval->addItem(tr("As the provider advises"), 0);
    interval->addItem(tr("Every hour"), 60);
    interval->addItem(tr("Every 6 hours"), 360);
    interval->addItem(tr("Once a day"), 1440);
    simpleSettingsRow(card, tr("How often"), {}, interval);
    const auto applyUpdate = [autoUpdate, interval] {
        const int minutes = interval->currentData().toInt();
        const int current = qAbs(simpleSettingsRepo()->sub_auto_update);
        const int value = minutes > 0 ? minutes : qMax(current, 30);
        simpleSettingsPut({{QStringLiteral("sub_auto_update"), autoUpdate->isChecked() ? value : -value},
                           {QStringLiteral("sub_respect_server_interval"), minutes == 0}});
    };
    connect(autoUpdate, &QCheckBox::clicked, this, applyUpdate);
    connect(interval, &QComboBox::activated, this, applyUpdate);

    const auto flag = [&](const QString &title, const QString &hint, bool Configs::SettingsRepo::*member) {
        auto *box = simpleSettingsSwitch();
        connect(box, &QCheckBox::clicked, this, [member](bool on) {
            (*simpleSettingsRepo()).*member = on;
            simpleSettingsRepo()->Save();
        });
        simpleSettingsRow(card, title, hint, box);
        m_reloaders << [box, member] { box->setChecked((*simpleSettingsRepo()).*member); };
    };
    flag(tr("Update through the connection"), tr("Helps when the provider's site cannot be reached directly."), &Configs::SettingsRepo::net_use_proxy);
    flag(tr("Show changes"), tr("After a manual update Poros shows which servers came and went."), &Configs::SettingsRepo::sub_show_change_popup);
    flag(tr("Send device data"), tr("Needed by providers that limit the number of devices."), &Configs::SettingsRepo::sub_send_hwid);

    column->addStretch();
    m_reloaders << [=] {
        const auto &settings = simpleSettingsRepo();
        autoUpdate->setChecked(settings->sub_auto_update > 0);
        const int index = settings->sub_respect_server_interval ? 0 : interval->findData(qAbs(settings->sub_auto_update));
        interval->setCurrentIndex(qMax(index, 0));
    };
    return page;
}

QWidget *SimpleSettings::buildStartup() {
    QVBoxLayout *column = nullptr;
    auto *page = simpleSettingsPage(tr("Launch"), tr("The same as the second step of the quick setup."), &column, this);
    QVBoxLayout *card = nullptr;
    column->addWidget(simpleSettingsCard({}, &card, page));

    auto *autoRun = simpleSettingsSwitch();
    connect(autoRun, &QCheckBox::clicked, this, [this, autoRun](bool on) {
        QString error;
        if (!AutoRun_SetEnabled(on, &error)) QMessageBox::warning(this, tr("Start with Windows"), error);
        autoRun->setChecked(AutoRun_IsEnabled());
    });
    simpleSettingsRow(card, tr("Start with Windows"), {}, autoRun);

    auto *remember = simpleSettingsSwitch();
    connect(remember, &QCheckBox::clicked, this, [](bool on) { simpleSettingsPut({{QStringLiteral("remember_enable"), on}}); });
    simpleSettingsRow(card, tr("Connect to the last profile"), {}, remember);

    auto *links = simpleSettingsSwitch();
    connect(links, &QCheckBox::clicked, this, [](bool on) {
        simpleSettingsRepo()->url_scheme_auto_register = on;
        if (on) UrlScheme_Install(Association::Links);
        else UrlScheme_Uninstall(Association::Links);
        simpleSettingsRepo()->Save();
    });
    simpleSettingsRow(card, tr("Accept poros:// links"), {}, links);

    auto *minimal = simpleSettingsSwitch();
    connect(minimal, &QCheckBox::clicked, this, [](bool on) {
        simpleSettingsRepo()->start_minimal = on;
        simpleSettingsRepo()->Save();
    });
    simpleSettingsRow(card, tr("Start minimized"), tr("The window does not open and Poros stays in the tray."), minimal);

    auto *tray = simpleSettingsSwitch();
    connect(tray, &QCheckBox::clicked, this, [this](bool on) {
        simpleSettingsRepo()->disable_tray = !on;
        simpleSettingsRepo()->Save();
        m_hooks.trayChanged();
    });
    simpleSettingsRow(card, tr("Minimize to the tray on close"), tr("If off, the close button quits Poros."), tray);

    column->addStretch();
    m_reloaders << [=] {
        const auto &settings = simpleSettingsRepo();
        autoRun->setChecked(AutoRun_IsEnabled());
        remember->setChecked(settings->remember_enable);
        links->setChecked(settings->url_scheme_auto_register);
        minimal->setChecked(settings->start_minimal);
        tray->setChecked(!settings->disable_tray);
    };
    return page;
}

QWidget *SimpleSettings::buildAppearance() {
    QVBoxLayout *column = nullptr;
    auto *page = simpleSettingsPage(tr("Appearance"), tr("The language and the size of the interface."), &column, this);
    QVBoxLayout *card = nullptr;
    column->addWidget(simpleSettingsCard({}, &card, page));

    auto *language = new NoWheelComboBox;
    language->setFixedWidth(220);
    language->addItem(tr("As in Windows"), 0);
    language->addItem(QStringLiteral("Русский"), 4);
    language->addItem(QStringLiteral("English"), 1);
    connect(language, &QComboBox::activated, this, [this, language] {
        simpleSettingsRepo()->language = language->currentData().toInt();
        simpleSettingsRepo()->Save();
        if (QMessageBox::question(this, tr("Language"), tr("The language changes after a restart. Restart now?")) == QMessageBox::Yes) m_hooks.restartProgram();
    });
    simpleSettingsRow(card, tr("Language"), {}, language);

    auto *size = new NoWheelComboBox;
    size->setFixedWidth(220);
    size->addItem(tr("Regular"), 10);
    size->addItem(tr("Large"), 12);
    connect(size, &QComboBox::activated, this, [size] {
        simpleSettingsRepo()->font_size = size->currentData().toInt();
        simpleSettingsRepo()->Save();
        QFont font = qApp->font();
        font.setPointSize(size->currentData().toInt());
        qApp->setFont(font);
    });
    simpleSettingsRow(card, tr("Text size"), {}, size);

    column->addStretch();
    m_reloaders << [=] {
        language->setCurrentIndex(qMax(language->findData(simpleSettingsRepo()->language), 0));
        size->setCurrentIndex(simpleSettingsRepo()->font_size > 10 ? 1 : 0);
    };
    return page;
}

QWidget *SimpleSettings::buildBackup() {
    QVBoxLayout *column = nullptr;
    auto *page = simpleSettingsPage(tr("Backup"), tr("Subscriptions, presets and settings in one file."), &column, this);
    QVBoxLayout *card = nullptr;
    column->addWidget(simpleSettingsCard({}, &card, page));

    auto *create = new QPushButton(PorosTheme::Icon(QStringLiteral("upload")), tr("Create"));
    connect(create, &QPushButton::clicked, this, &SimpleSettings::createBackup);
    simpleSettingsRow(card, tr("Create a backup"), tr("The file can be moved to another computer."), create);

    auto *restore = new QPushButton(PorosTheme::Icon(QStringLiteral("download")), tr("Choose a file"));
    connect(restore, &QPushButton::clicked, this, &SimpleSettings::restoreBackup);
    simpleSettingsRow(card, tr("Restore from a backup"), tr("The current data will be replaced and Poros will restart."), restore);
    column->addStretch();
    return page;
}

void SimpleSettings::createBackup() {
    Configs::BackupParts parts;
    parts.profiles = parts.routes = parts.settings = parts.otp = true;
    Engine::BackupArchive::Prepare(parts);
    const QString path = NativeFileDialog::getSaveFileName(this, tr("Create a backup"), QDir::homePath() + "/Poros-backup.thrbackup", tr("Poros Backup (*.thrbackup)"));
    if (path.isEmpty()) return;
    if (const auto error = Engine::BackupArchive::Build(parts, path); !error.isEmpty()) QMessageBox::critical(this, tr("Create a backup"), error);
    else QMessageBox::information(this, tr("Create a backup"), tr("The backup is saved:\n%1").arg(path));
}

void SimpleSettings::restoreBackup() {
    const QString path = NativeFileDialog::getOpenFileName(this, tr("Restore from a backup"), QDir::homePath(), tr("Poros Backup (*.thrbackup)"));
    if (path.isEmpty()) return;
    Engine::BackupArchive::Info info;
    if (const auto error = Engine::BackupArchive::Inspect(path, &info); !error.isEmpty()) {
        QMessageBox::critical(this, tr("Restore from a backup"), error);
        return;
    }
    if (QMessageBox::question(this, tr("Restore from a backup"), tr("Replace the current data and restart Poros?")) != QMessageBox::Yes) return;
    Engine::BackupArchive::ApplyResult result;
    if (const auto error = Engine::BackupArchive::Apply(path, info.available, {}, &result); !error.isEmpty()) {
        QMessageBox::critical(this, tr("Restore from a backup"), error);
        return;
    }
    m_hooks.restartProgram();
}

QWidget *SimpleSettings::buildHelp() {
    QVBoxLayout *column = nullptr;
    auto *page = simpleSettingsPage(tr("Help"), tr("If something does not work."), &column, this);
    QVBoxLayout *card = nullptr;
    column->addWidget(simpleSettingsCard({}, &card, page));

    auto *mode = new SegmentSwitch({tr("Simple"), tr("Advanced")});
    mode->setFixedHeight(36);
    mode->setCurrent(0);
    connect(mode, &SegmentSwitch::activated, this, [this, mode](int index) {
        if (index == 1) m_hooks.setUiMode(QStringLiteral("advanced"));
        mode->setCurrent(0);
    });
    simpleSettingsRow(card, tr("Interface mode"), tr("The advanced mode opens everything Poros can do."), mode);

    const auto action = [&](const QString &icon, const QString &button, const QString &title, const QString &hint, const std::function<void()> &run) {
        auto *widget = new QPushButton(PorosTheme::Icon(icon), button);
        connect(widget, &QPushButton::clicked, this, run);
        simpleSettingsRow(card, title, hint, widget);
        return widget;
    };
    action(QStringLiteral("bolt"), tr("Run"), tr("Quick setup"), tr("Go through the first-launch steps again."), [this] { m_hooks.runWizard(); });
    action(QStringLiteral("log"), tr("Open"), tr("Log"), tr("Useful when you write to support."), [this] { m_hooks.showLog(); });
    action(QStringLiteral("refresh"), tr("Reset"), tr("Reset the connection"), tr("Restarts the core if TUN stopped working."), [this] { m_hooks.resetCore(); });
    const bool updates = Engine::ReleaseUpdates::Available();
    action(QStringLiteral("download"), tr("Check"), tr("Updates"), updates ? tr("Current version: %1").arg(QStringLiteral(NKR_VERSION)) : tr("Updates are off in this build."),
           [this] { m_hooks.checkForUpdates(); })->setEnabled(updates);
    auto *autoUpdate = simpleSettingsSwitch();
    autoUpdate->setEnabled(updates);
    connect(autoUpdate, &QCheckBox::clicked, this, [](bool on) {
        simpleSettingsRepo()->auto_update_app = on;
        simpleSettingsRepo()->Save();
    });
    simpleSettingsRow(card, tr("Update automatically"), tr("Looks for a new version at launch and every four hours, and downloads it. When off, Poros updates only when you press Check."), autoUpdate);
    column->addStretch();
    m_reloaders << [=] { autoUpdate->setChecked(simpleSettingsRepo()->auto_update_app); };
    return page;
}
