#include "include/ui/group/dialog_edit_group.h"

#include <QClipboard>
#include <QHash>
#include <QHostAddress>
#include <QRegularExpression>
#include <QSslSocket>
#include <QStandardItemModel>
#include <QStringListModel>
#include <QCompleter>
#include <QStyle>
#include <QTimer>
#include <QAbstractItemView>
#include <QUrl>

#include "include/configs/common/utils.h"
#include "include/configs/generate.h"
#include "include/configs/sub/GroupUpdater.hpp"
#include "include/database/GroupsRepo.h"
#include "include/database/ProfilesRepo.h"
#include "include/database/SettingsRepo.h"
#include "include/global/RunningProfiles.hpp"
#include "include/global/Validation.hpp"
#include "include/ui/utils/FormColumns.h"
#include "include/ui/utils/GuiUtils.hpp"
#include "include/ui/utils/ValidationForm.h"

namespace {
    // Index 0 of an override combo is Keep Default; the enum values follow.
    template <typename E>
    int overrideIndex(const std::optional<E> &value) {
        return value ? static_cast<int>(*value) + 1 : 0;
    }

    template <typename E>
    std::optional<E> overrideAt(int index) {
        if (index <= 0) return std::nullopt;
        return static_cast<E>(index - 1);
    }

    // Empty unless the text is a bare IP address or host name: no scheme, port or path.
    QString groupEndpointHost(QString text) {
        text = text.trimmed();
        if (text.startsWith('[') && text.endsWith(']')) text = text.mid(1, text.size() - 2);
        if (IsIpAddressV6(text)) return QHostAddress(text).toString();
        // Qt reads "1.2.3" as IPv4 and "999.1.1.1" would pass as a name; both are typos, so neither is accepted.
        if (IsIpAddressV4(text)) return QHostAddress(text).toString() == text ? text : QString();
        static const QRegularExpression hostname(QStringLiteral(
            R"(^(?:(?!-)[A-Za-z0-9_-]{1,63}(?<!-)\.)*(?![0-9]+$)(?!-)[A-Za-z0-9_-]{1,63}(?<!-)$)"));
        const auto ace = Configs::toAceHost(text);
        return ace.size() <= 253 && hostname.match(ace).hasMatch() ? ace : QString();
    }

    // GroupUpdater sends only values below 1000 characters without line breaks, and silently falls back otherwise.
    QString groupHeaderValueError(const QString &text) {
        if (text.size() < 1000 && !text.contains(QLatin1Char('\n')) && !text.contains(QLatin1Char('\r'))) return {};
        return DialogEditGroup::tr("Must be shorter than 1000 characters, on a single line.");
    }

    bool editGroupRunningProfileInherits(const QList<int> &profileIds) {
        const int startedId = Configs::dataManager->settingsRepo->started_id;
        if (startedId < 0) return false;
        QList<int> running;
        for (const int id : profileIds) {
            if (id == startedId || Configs::RunningUsesProfile(id)) running << id;
        }
        for (const auto &profile : Configs::dataManager->profilesRepo->GetProfileBatch(running)) {
            if (profile != nullptr && profile->endpoint.mode == Configs::EndpointSource::Mode::Inherit) return true;
        }
        return false;
    }
}

DialogEditGroup::DialogEditGroup(const std::shared_ptr<Configs::Group> &ent, QWidget *parent) : QDialog(parent), ui(new Ui::DialogEditGroup) {
    ui->setupUi(this);
    FormColumns::adoptAll(this);
    this->ent = ent;
    if (ent->id >= 0) editLock.emplace(Engine::EditTarget::Group, ent->id);
    form = new Validation::Form(this);
    form->attachStatus(ui->buttonBox);
    ui->buttonBox->button(QDialogButtonBox::Ok)->setText(tr("Save"));
    ui->buttonBox->button(QDialogButtonBox::Cancel)->setProperty("variant", QStringLiteral("ghost"));
    ui->dialog_title->setText(ent->id >= 0 ? tr("Group") : tr("New group"));
    setWindowTitle(ui->dialog_title->text());

    connect(ui->type, &QComboBox::currentIndexChanged, this, [this] { syncType(); });
    form->bind(ui->url, [](const QString &text) {
        // The fetcher's QNetworkAccessManager reads file:// URLs too, and those carry no host.
        if (QUrl(text.trimmed()).isLocalFile()) return QString();
        return Validation::Url(text, {QStringLiteral("http"), QStringLiteral("https"), QStringLiteral("file")}, Validation::Empty::Required);
    }).when([this] { return ui->type->currentIndex() == 1; });

    ui->name->setText(ent->name);
    ui->auto_clear_unavailable->setChecked(ent->auto_clear_unavailable);
    ui->skip_auto_update->setChecked(ent->skip_auto_update);
    loadEndpoint();
    loadOptions();
    bindOptions();
    ui->url->setText(ent->url);
    ui->type->setCurrentIndex(ent->url.isEmpty() ? 0 : 1);
    syncType();
    ui->cat_share->setVisible(false);
    if (Configs::dataManager->profilesRepo->GetProfile(ent->front_proxy_id) == nullptr) {
        ent->front_proxy_id = -1;
        Configs::dataManager->groupsRepo->Save(ent);
    }
    if (Configs::dataManager->profilesRepo->GetProfile(ent->landing_proxy_id) == nullptr) {
        ent->landing_proxy_id = -1;
        Configs::dataManager->groupsRepo->Save(ent);
    }
    CACHE.front_proxy = ent->front_proxy_id;
    LANDING.landing_proxy = ent->landing_proxy_id;

    if (ent->id >= 0) { // already a group
        ui->type->setDisabled(true);
        if (!ent->Profiles().isEmpty()) {
            ui->cat_share->setVisible(true);
        }
    }

    auto proxyListRaw = Configs::dataManager->profilesRepo->GetAllProfileIDNameMapped();
    QHash<int, QString> idToName;
    idToName.reserve(proxyListRaw.size());
    for (const auto& [id, name] : proxyListRaw) idToName.insert(id, name);
    QList<std::pair<int, QString>> proxyList;
    // An auto selector moves server on its own, so it cannot hold a fixed front/landing slot.
    const auto selectorIDsRaw = Configs::dataManager->profilesRepo->GetProfileIdsByType("autoselector");
    const QSet<int> selectorIDs(selectorIDsRaw.begin(), selectorIDsRaw.end());
    auto groupIDs = Configs::dataManager->groupsRepo->GetGroupsTabOrder();
    for (auto groupID: groupIDs) {
        auto group = Configs::dataManager->groupsRepo->GetGroup(groupID);
        if (!group) continue;
        const QString prefix = "[" + group->name + "] ";
        for (int profileID : group->profiles) {
            if (selectorIDs.contains(profileID)) continue;
            auto it = idToName.constFind(profileID);
            if (it == idToName.constEnd()) continue;
            proxyList << std::make_pair(profileID, prefix + it.value());
        }
    }
    QStringList proxyNameList;
    proxyNameToId.clear();
    proxyNameToId.insert(tr("None"), -1);
    ui->front_proxy->setMaxCount(200);
    ui->landing_proxy->setMaxCount(200);
    ui->front_proxy->blockSignals(true);
    ui->landing_proxy->blockSignals(true);
    ui->front_proxy->setUpdatesEnabled(false);
    ui->landing_proxy->setUpdatesEnabled(false);
    ui->front_proxy->addItem(tr("None"), QVariant(-1));
    ui->landing_proxy->addItem(tr("None"), QVariant(-1));
    const int comboCap = ui->front_proxy->maxCount();
    int comboCount = 1; // "None" already added
    for (const auto&[id, name] : proxyList) {
        proxyNameList.append(name);
        if (!proxyNameToId.contains(name)) {
            proxyNameToId.insert(name, id);
        }
        if (comboCount < comboCap) {
            ui->front_proxy->addItem(name, QVariant(id));
            ui->landing_proxy->addItem(name, QVariant(id));
            ++comboCount;
        }
    }
    ui->front_proxy->blockSignals(false);
    ui->landing_proxy->blockSignals(false);
    ui->front_proxy->setUpdatesEnabled(true);
    ui->landing_proxy->setUpdatesEnabled(true);

    auto proxyCompleterModel = new QStringListModel(proxyNameList, this);

    auto attachDebouncedCompleter = [this](QComboBox* combo, QCompleter* completer) {
        auto* lineEdit = combo->lineEdit();
        completer->setWidget(lineEdit);
        auto* debounce = new QTimer(this);
        debounce->setSingleShot(true);
        debounce->setInterval(300);
        connect(debounce, &QTimer::timeout, completer, [completer, lineEdit] {
            const QString text = lineEdit->text();
            if (text.isEmpty()) return;
            completer->setCompletionPrefix(text);
            completer->complete();
        });
        connect(lineEdit, &QLineEdit::textEdited, debounce, [debounce, completer](const QString& text) {
            if (text.isEmpty()) {
                debounce->stop();
                completer->popup()->hide();
                return;
            }
            debounce->start();
        });
        connect(completer, qOverload<const QString&>(&QCompleter::activated),
                         lineEdit, [combo, lineEdit](const QString& text) {
            lineEdit->setText(text);
            const int index = combo->findText(text, Qt::MatchExactly);
            if (index >= 0) {
                combo->setCurrentIndex(index);
            }
        });
    };

    ui->front_proxy->setEditable(true);
    ui->front_proxy->setCurrentText(get_proxy_name(CACHE.front_proxy));
    ui->front_proxy->setInsertPolicy(QComboBox::NoInsert);
    auto frontCompleter = new QCompleter(this);
    frontCompleter->setModel(proxyCompleterModel);
    frontCompleter->setCompletionMode(QCompleter::PopupCompletion);
    frontCompleter->setCaseSensitivity(Qt::CaseInsensitive);
    frontCompleter->setFilterMode(Qt::MatchContains);
    ui->front_proxy->setCompleter(nullptr);
    attachDebouncedCompleter(ui->front_proxy, frontCompleter);
    connect(ui->front_proxy, &QComboBox::currentIndexChanged, this, [=,this](int index){
        CACHE.front_proxy = ui->front_proxy->itemData(index).value<int>();
    });

    ui->landing_proxy->setEditable(true);
    ui->landing_proxy->setCurrentText(get_proxy_name(LANDING.landing_proxy));
    ui->landing_proxy->setInsertPolicy(QComboBox::NoInsert);
    auto landingCompleter = new QCompleter(this);
    landingCompleter->setModel(proxyCompleterModel);
    landingCompleter->setCompletionMode(QCompleter::PopupCompletion);
    landingCompleter->setCaseSensitivity(Qt::CaseInsensitive);
    landingCompleter->setFilterMode(Qt::MatchContains);
    ui->landing_proxy->setCompleter(nullptr);
    attachDebouncedCompleter(ui->landing_proxy, landingCompleter);
    connect(ui->landing_proxy, &QComboBox::currentIndexChanged, this, [=,this](int index){
        LANDING.landing_proxy = ui->landing_proxy->itemData(index).value<int>();
    });

    const auto proxyRule = [this](QComboBox *combo, const QString &loaded) {
        return [this, combo, loaded](const QString &text) {
            const QString trimmed = text.trimmed();
            // An untouched name keeps the stored selection, which may sit outside the listed groups.
            if (trimmed.isEmpty() || trimmed == tr("None") || text == loaded) return QString();
            for (const auto &name : {text, trimmed}) {
                if (combo->findText(name, Qt::MatchExactly) >= 0 || proxyNameToId.contains(name)) return QString();
            }
            return tr("No profile named \"%1\". Pick one from the list.").arg(trimmed);
        };
    };
    form->bind(ui->front_proxy, proxyRule(ui->front_proxy, ui->front_proxy->currentText())).raw();
    form->bind(ui->landing_proxy, proxyRule(ui->landing_proxy, ui->landing_proxy->currentText())).raw();

    connect(ui->copy_links, &QPushButton::clicked, this, [=] {
        QStringList links;
        auto profiles = Configs::dataManager->profilesRepo->GetProfileBatch(ent->Profiles());
        for (const auto &profile: profiles) {
            auto link = profile->outbound->ExportToLink();
            if (link.isEmpty()) link = profile->outbound->ExportJsonLink();
            links += link;
        }
        QApplication::clipboard()->setText(links.join("\n"));
        MessageBoxInfo(software_name, tr("Copied"));
    });
    connect(ui->copy_links_nkr, &QPushButton::clicked, this, [=] {
        QStringList links;
        auto profiles = Configs::dataManager->profilesRepo->GetProfileBatch(ent->Profiles());
        for (const auto &profile: profiles) {
            links += profile->outbound->ExportJsonLink();
        }
        QApplication::clipboard()->setText(links.join("\n"));
        MessageBoxInfo(software_name, tr("Copied"));
    });

    ADD_ASTERISK(this)
    ui->name->setFocus();
    form->markAll();
}

DialogEditGroup::~DialogEditGroup() {
    delete ui;
}

void DialogEditGroup::syncType() {
    const bool subscription = ui->type->currentIndex() == 1;
    ui->label_4->setVisible(subscription);
    ui->url->setVisible(subscription);
    ui->skip_auto_update->setVisible(subscription);
    ui->request_box->setVisible(subscription);
    ui->tabs->setTabVisible(ui->tabs->indexOf(ui->tab_update), subscription);
}

void DialogEditGroup::loadEndpoint() {
    using Mode = Configs::EndpointSource::Mode;
    ui->endpoint_mode->addItem(tr("Profiles' own addresses"), static_cast<int>(Mode::Inherit));
    ui->endpoint_mode->addItem(tr("Fixed address"), static_cast<int>(Mode::Address));

    if (ent->endpoint.mode == Mode::Address) ui->endpoint_address->setText(ent->endpoint.address);
    ui->endpoint_mode->setCurrentIndex(qMax(0, ui->endpoint_mode->findData(static_cast<int>(ent->endpoint.mode))));

    const auto sync = [this] { ui->endpoint_address->setVisible(endpointFromUi().mode == Mode::Address); };
    connect(ui->endpoint_mode, &QComboBox::currentIndexChanged, this, sync);
    sync();
}

void DialogEditGroup::loadOptions() {
    const auto &options = ent->sub_options;
    const int serverIntervalHours = ent->sub_info.server_interval;
    const auto &settings = Configs::dataManager->settingsRepo;
    ui->update_interval->setSuffix(tr(" min"));
    ui->update_interval->setSpecialValueText(
        tr("Keep Default (%1)").arg(settings->sub_auto_update >= 30 ? tr("%1 min").arg(settings->sub_auto_update) : tr("Off")));
    ui->update_interval->setValue(options.update_interval);
    ui->server_interval->setItemText(0, tr("Keep Default (%1)").arg(ui->server_interval->itemText(settings->sub_respect_server_interval ? 1 : 2)));
    ui->server_interval->setCurrentIndex(options.respect_server_interval ? (*options.respect_server_interval ? 1 : 2) : 0);
    if (serverIntervalHours > 0) {
        const auto tip = ui->server_interval_l->toolTip() + "\n\n" + tr("This server last sent %1 h.").arg(serverIntervalHours);
        ui->server_interval_l->setToolTip(tip);
        ui->server_interval->setToolTip(tip);
    }

    const auto defaults = Subscription::ResolveIdentity(nullptr);
    globalSendHwid = defaults.sendHwid;
    ui->send_hwid->setItemText(static_cast<int>(Configs::sendHwid::keepDefault),
                               tr("Keep Default (%1)").arg(globalSendHwid ? tr("On") : tr("Off")));
    ui->tls_version->setItemText(0, tr("Keep Default (%1)").arg(ui->tls_version->itemText(overrideIndex(std::optional(defaults.tlsVersion)))));
    ui->http_version->setItemText(0, tr("Keep Default (%1)").arg(ui->http_version->itemText(overrideIndex(std::optional(defaults.httpVersion)))));
    if (auto *model = qobject_cast<QStandardItemModel *>(ui->tls_version->model()); model && !QSslSocket::isProtocolSupported(QSsl::TlsV1_3)) {
        model->item(overrideIndex(std::optional(Configs::subTlsVersion::tls13)))->setEnabled(false);
    }
    ui->user_agent->setPlaceholderText(defaults.userAgent);
    ui->hwid->setPlaceholderText(defaults.device.hwid);
    ui->hwid_os->setPlaceholderText(defaults.device.os);
    ui->hwid_os_version->setPlaceholderText(defaults.device.osVersion);
    ui->hwid_model->setPlaceholderText(defaults.device.model);

    ui->user_agent->setText(options.user_agent);
    ui->tls_version->setCurrentIndex(overrideIndex(options.tls_version));
    ui->http_version->setCurrentIndex(overrideIndex(options.http_version));
    ui->send_hwid->setCurrentIndex(static_cast<int>(options.send_hwid));
    ui->hwid->setText(options.hwid);
    ui->hwid_os->setText(options.hwid_os);
    ui->hwid_os_version->setText(options.hwid_os_version);
    ui->hwid_model->setText(options.hwid_model);
    ui->keep_working->setChecked(options.keep_working);
    ui->remove_duplicates->setChecked(options.remove_duplicates);
    ui->remove_insecure->setChecked(options.remove_insecure);
    ui->remove_invalid->setChecked(options.remove_invalid);
    ui->url_test->setChecked(options.url_test);
    ui->remove_unavailable->setChecked(options.remove_unavailable);
    ui->sort_by_latency->setChecked(options.sort_by_latency);

    connect(ui->send_hwid, &QComboBox::currentIndexChanged, this, [this] { syncHwidFields(); });
    syncHwidFields();

    const auto *checkStyle = ui->url_test->style();
    ui->url_test_follow_ups->setContentsMargins(checkStyle->pixelMetric(QStyle::PM_IndicatorWidth, nullptr, ui->url_test)
                                                    + checkStyle->pixelMetric(QStyle::PM_CheckBoxLabelSpacing, nullptr, ui->url_test),
                                                0, 0, 0);
    connect(ui->url_test, &QCheckBox::toggled, this, [this] { syncUrlTestFollowUps(); });
    syncUrlTestFollowUps();
}

void DialogEditGroup::bindOptions() {
    form->bind(ui->endpoint_address, [](const QString &text) {
        if (auto problem = Validation::Host(text); !problem.isEmpty()) return problem;
        return groupEndpointHost(text).isEmpty() ? tr("Enter a host name or an IP address, without a port.") : QString();
    }).label(tr("Fixed address"));
    for (QLineEdit *edit : {static_cast<QLineEdit *>(ui->user_agent), static_cast<QLineEdit *>(ui->hwid), static_cast<QLineEdit *>(ui->hwid_os),
                            static_cast<QLineEdit *>(ui->hwid_os_version), static_cast<QLineEdit *>(ui->hwid_model)})
        form->bind(edit, [](const QString &text) { return groupHeaderValueError(text); });
    form->addRule({ui->update_interval}, [this] {
        const int minutes = ui->update_interval->value();
        if (minutes == 0 || minutes >= 30) return QString();
        return tr("Must be at least 30 minutes, or 0 to keep the default.");
    });
}

Configs::EndpointSource DialogEditGroup::endpointFromUi() const {
    using Mode = Configs::EndpointSource::Mode;
    Configs::EndpointSource source;
    source.mode = static_cast<Mode>(ui->endpoint_mode->currentData().toInt());
    if (source.mode == Mode::Address) source.address = groupEndpointHost(ui->endpoint_address->text());
    return source;
}

Configs::SubscriptionOptions DialogEditGroup::optionsFromUi() const {
    auto options = ent->sub_options;
    options.user_agent = ui->user_agent->text().trimmed();
    options.tls_version = overrideAt<Configs::subTlsVersion>(ui->tls_version->currentIndex());
    options.http_version = overrideAt<Configs::subHttpVersion>(ui->http_version->currentIndex());
    options.send_hwid = static_cast<Configs::sendHwid>(ui->send_hwid->currentIndex());
    options.hwid = ui->hwid->text().trimmed();
    options.hwid_os = ui->hwid_os->text().trimmed();
    options.hwid_os_version = ui->hwid_os_version->text().trimmed();
    options.hwid_model = ui->hwid_model->text().trimmed();
    options.update_interval = ui->update_interval->value();
    const int serverInterval = ui->server_interval->currentIndex();
    options.respect_server_interval = serverInterval > 0 ? std::optional<bool>(serverInterval == 1) : std::nullopt;
    options.keep_working = ui->keep_working->isChecked();
    options.remove_duplicates = ui->remove_duplicates->isChecked();
    options.remove_insecure = ui->remove_insecure->isChecked();
    options.remove_invalid = ui->remove_invalid->isChecked();
    options.url_test = ui->url_test->isChecked();
    options.remove_unavailable = ui->remove_unavailable->isChecked();
    options.sort_by_latency = ui->sort_by_latency->isChecked();
    return options;
}

void DialogEditGroup::syncHwidFields() {
    const auto mode = static_cast<Configs::sendHwid>(ui->send_hwid->currentIndex());
    const bool sent = mode == Configs::sendHwid::on || (mode == Configs::sendHwid::keepDefault && globalSendHwid);
    const QList<QWidget *> fields{ui->hwid_l, ui->hwid, ui->hwid_os_l, ui->hwid_os,
                                  ui->hwid_os_version_l, ui->hwid_os_version, ui->hwid_model_l, ui->hwid_model};
    for (auto *field : fields) field->setEnabled(sent);
}

void DialogEditGroup::syncUrlTestFollowUps() {
    const bool tested = ui->url_test->isChecked();
    ui->remove_unavailable->setEnabled(tested);
    ui->sort_by_latency->setEnabled(tested);
}

int DialogEditGroup::resolve_proxy_selection(QComboBox *combo, int fallback) const {
    const QString text = combo->currentText().trimmed();
    if (text.isEmpty() || text == tr("None")) {
        return -1;
    }

    const int index = combo->findText(text, Qt::MatchExactly);
    if (index >= 0) {
        return combo->itemData(index).value<int>();
    }

    auto it = proxyNameToId.constFind(text);
    if (it != proxyNameToId.constEnd()) {
        return it.value();
    }

    return fallback;
}

void DialogEditGroup::accept() {
    if (!form->validateAll()) return;
    // A basic group keeps no URL, even one typed before switching the type back.
    const QString newUrl = ui->type->currentIndex() == 1 ? ui->url->text().trimmed() : QString();
    if (ent->url != newUrl) {
        ent->sub_info = Configs::SubUserInfo{};
        ent->sub_last_update = 0;
        ent->info.clear();
    }
    ent->name = ui->name->text().trimmed();
    ent->auto_clear_unavailable = ui->auto_clear_unavailable->isChecked();
    ent->url = newUrl;
    ent->skip_auto_update = ui->skip_auto_update->isChecked();
    ent->sub_options = optionsFromUi();
    const auto endpoint = endpointFromUi();
    restartNeeded = endpoint != ent->endpoint && editGroupRunningProfileInherits(ent->Profiles());
    ent->endpoint = endpoint;
    ent->front_proxy_id = resolve_proxy_selection(ui->front_proxy, CACHE.front_proxy);
    ent->landing_proxy_id = resolve_proxy_selection(ui->landing_proxy, LANDING.landing_proxy);
    QDialog::accept();
}

QString DialogEditGroup::get_proxy_name(int id) {
    if (id == -1) return tr("None");
    if (auto profile = Configs::dataManager->profilesRepo->GetProfile(id)) {
        auto group = Configs::dataManager->groupsRepo->GetGroup(profile->gid);
        if (group == nullptr) return "INVALID";
        return QString("[" + group->name + "] ") + profile->name;
    }
    return "INVALID";
}
