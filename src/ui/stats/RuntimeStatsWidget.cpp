#include "include/ui/stats/RuntimeStatsWidget.h"

#include "include/ui/stats/dialog_endpoint_details.h"
#include "include/api/RPC.h"
#include "include/engine/EngineHost.h"
#include "include/engine/connection/ConnectionManager.h"
#include "include/engine/supervisor/CoreSupervisor.h"
#include "include/engine/vpn/VpnEndpoints.h"
#include "include/configs/sub/GroupUpdater.hpp"
#include "include/stats/traffic/TrafficLooper.hpp"
#include "include/database/DatabaseManager.h"
#include "include/database/SettingsRepo.h"
#include "include/global/Utils.hpp"
#include "include/global/ExitLookup.hpp"
#include "include/global/LocalNetwork.hpp"

#include <QBrush>
#include <QColor>
#include <QCoreApplication>
#include <QDateTime>
#include <QDir>
#include <QFileInfo>
#include <QHeaderView>
#include <QItemSelectionModel>
#include <QLocale>
#include <QPointer>
#include <QStringList>
#include <QPushButton>
#include <QTableWidget>
#include <QTimer>

#include <algorithm>
#include <limits>

namespace {
    std::shared_ptr<Configs::Profile> runningProfile() {
        auto *engine = Engine::host();
        const auto running = engine != nullptr ? engine->connection()->runningSnapshot() : nullptr;
        return running != nullptr && running->outbound != nullptr ? running : nullptr;
    }

    QString runningConfigName() {
        const auto running = runningProfile();
        return running != nullptr ? running->outbound->DisplayTypeAndName() : QString();
    }

    QString formatCpu(const Sys::ProcessMetrics::Sample& s) {
        return s.ok ? QLocale().toString(s.cpuPercent, 'f', 1) + QStringLiteral("%") : QStringLiteral("—");
    }

    QString formatRam(const Sys::ProcessMetrics::Sample& s) {
        return s.ok ? ReadableSize(s.rssBytes) : QStringLiteral("—");
    }

    // QLabel clips rather than elides, and these values are as long as the user's profile names.
    void setElided(QLabel* label, const QString& text) {
        label->setText(label->fontMetrics().elidedText(text, Qt::ElideRight, label->width()));
        label->setToolTip(text);
    }

    // The provider the exit lookup asks, or empty while the lookup is off.
    QString runtimeExitLookupProvider() {
        const auto* settings = Configs::dataManager->settingsRepo.get();
        if (!settings->exit_country_lookup) return {};
        return settings->exit_lookup_provider.isEmpty() ? Configs_network::ExitLookupAutoId : settings->exit_lookup_provider;
    }
}

RuntimeStatsWidget::RuntimeStatsWidget(QWidget* parent) : QWidget(parent), ui(new Ui::RuntimeStatsWidget) {
    ui->setupUi(this);

    // A lookup service's answer: as AutoText a tag in it would render as HTML.
    ui->vOutIp->setTextFormat(Qt::PlainText);
    ui->vCountry->setTextFormat(Qt::PlainText);

    ui->groupEndpoints->setVisible(false);
    auto* endpoints = ui->endpointsTable;
    endpoints->verticalHeader()->setVisible(false);
    endpoints->verticalHeader()->setDefaultSectionSize(36);
    endpoints->horizontalHeader()->setDefaultAlignment(Qt::AlignLeft | Qt::AlignVCenter);
    endpoints->setEditTriggers(QAbstractItemView::NoEditTriggers);
    endpoints->setSelectionBehavior(QAbstractItemView::SelectRows);
    endpoints->setSelectionMode(QAbstractItemView::SingleSelection);
    endpoints->horizontalHeader()->setStretchLastSection(false);
    endpoints->horizontalHeader()->setSectionResizeMode(0, QHeaderView::Stretch);
    endpoints->horizontalHeader()->setSectionResizeMode(1, QHeaderView::ResizeToContents);
    endpoints->horizontalHeader()->setSectionResizeMode(2, QHeaderView::ResizeToContents);
    connect(endpoints, &QTableWidget::itemDoubleClicked, this,
            [this](QTableWidgetItem*) { openEndpointDetails(selectedEndpointTag()); });

    timer_ = new QTimer(this);
    timer_->setInterval(1000);
    connect(timer_, &QTimer::timeout, this, [this]() { refreshLive(); });
}

RuntimeStatsWidget::~RuntimeStatsWidget() {
    delete ui;
}

void RuntimeStatsWidget::setSpeedChart(QWidget* chart) {
    ui->speedLayout->addWidget(chart);
}

// A splitter collapses a pane by moving it off-screen, never by hiding it, so isVisible() alone still reads true.
bool RuntimeStatsWidget::panelActive() const {
    return isVisible() && !window()->isMinimized() && !visibleRegion().isEmpty();
}

void RuntimeStatsWidget::showEvent(QShowEvent *event) {
    QWidget::showEvent(event);
    // A hidden gap would turn the next CPU delta into an average over that gap.
    metrics_.reset();
    // The panel outlives a visit now, so the egress snapshot has to go stale with it; lastProbeSecs_ still rate-limits.
    egressSnapshotDone_ = false;
    timer_->start();
    // After the layout: the elided values need their labels' real width.
    QTimer::singleShot(0, this, [this] { refreshLive(); });
}

void RuntimeStatsWidget::hideEvent(QHideEvent *event) {
    QWidget::hideEvent(event);
    timer_->stop();
    // Nothing refreshes it while the panel is away, and it would sit there showing a frozen endpoint.
    if (details_) details_->close();
}

void RuntimeStatsWidget::refreshLive() {
    if (!panelActive()) return;
    auto *engine = Engine::host();

    const auto selfSample = metrics_.sample(QCoreApplication::applicationPid());
    const qint64 corePid = engine != nullptr ? engine->supervisor()->corePid() : 0;
    const auto coreSample = corePid > 0 ? metrics_.sample(corePid) : Sys::ProcessMetrics::Sample{};

    const QString coreVersion = engine != nullptr ? engine->supervisor()->coreVersion() : QString();
    setElided(ui->vCore, coreVersion.isEmpty() ? QStringLiteral("—") : QStringLiteral("sing-box %1").arg(coreVersion));

    const qint64 ram = (selfSample.ok ? selfSample.rssBytes : 0) + (coreSample.ok ? coreSample.rssBytes : 0);
    ui->vMemory->setText(ReadableSize(ram));
    ui->vMemory->setToolTip(tr("Poros: %1\nCore: %2").arg(formatRam(selfSample), formatRam(coreSample)));
    const double cpu = (selfSample.ok ? selfSample.cpuPercent : 0) + (coreSample.ok ? coreSample.cpuPercent : 0);
    ui->vCpu->setText(QLocale().toString(cpu, 'f', 1) + QStringLiteral("%"));
    ui->vCpu->setToolTip(tr("Poros: %1\nCore: %2").arg(formatCpu(selfSample), formatCpu(coreSample)));

    const auto nextUpd = [](int interval, qint64 last) -> QString {
        if (interval < 30) return RuntimeStatsWidget::tr("Disabled");
        const qint64 remaining = last > 0
            ? last + static_cast<qint64>(interval) * 60 - QDateTime::currentSecsSinceEpoch()
            : 0;
        if (remaining <= 0) return RuntimeStatsWidget::tr("Due now");
        return RuntimeStatsWidget::tr("in %1").arg(Stats::HumanizeDuration(remaining));
    };
    auto* settings = Configs::dataManager->settingsRepo.get();
    const qint64 nextSub = Subscription::updater()->NextAutoUpdate();
    const qint64 now = QDateTime::currentSecsSinceEpoch();
    ui->vSubUpdate->setText(nextSub < 0      ? RuntimeStatsWidget::tr("Disabled")
                            : nextSub <= now ? RuntimeStatsWidget::tr("Due now")
                                             : RuntimeStatsWidget::tr("in %1").arg(Stats::HumanizeDuration(nextSub - now)));
    ui->vRouteUpdate->setText(nextUpd(settings->route_auto_update, settings->route_auto_update_last));

    qint64 dbBytes = 0;
    const QDir dir(QDir::currentPath());
    for (const QFileInfo& fi : dir.entryInfoList(QStringList{QStringLiteral("poros*.db*")}, QDir::Files))
        dbBytes += fi.size();
    ui->vDbSize->setText(ReadableSize(dbBytes));

    const qint64 up = appStartEpoch > 0 ? QDateTime::currentSecsSinceEpoch() - appStartEpoch : 0;
    ui->vUptime->setText(Stats::HumanizeDuration(up));

    if (const QString lookup = runtimeExitLookupProvider(); lookup != egressLookup_) {
        egressLookup_ = lookup;
        egressSnapshotDone_ = false;
        lastProbeSecs_ = 0;
        const QString shown = lookup.isEmpty() ? tr("Off") : QStringLiteral("—");
        ui->vOutIp->setText(shown);
        ui->vCountry->setText(shown);
    }

    const QString cfgName = runningConfigName();
    const qint64 nowSecs = QDateTime::currentSecsSinceEpoch();
    if (cfgName.isEmpty()) {
        ui->vCfgName->setText(QStringLiteral("—"));
        ui->vEndpoint->setText(QStringLiteral("—"));
        ui->vOutIp->setText(QStringLiteral("—"));
        ui->vCountry->setText(QStringLiteral("—"));
        ui->vPing->setText(QStringLiteral("—"));
        lastProbedConfig_.clear();
        egressSnapshotDone_ = false;
    } else {
        setElided(ui->vCfgName, cfgName);
        setElided(ui->vEndpoint, runningProfile()->outbound->DisplayAddress());
        if (cfgName != lastProbedConfig_) {
            lastProbedConfig_ = cfgName;
            egressSnapshotDone_ = false;
            lastProbeSecs_ = 0;
            // The probe runs for seconds; without this the old config's egress sits under the new config's name.
            ui->vOutIp->setText(QStringLiteral("—"));
            ui->vCountry->setText(QStringLiteral("—"));
            ui->vPing->setText(QStringLiteral("—"));
        }
        if (!egressSnapshotDone_ && (lastProbeSecs_ == 0 || nowSecs - lastProbeSecs_ >= 30)) {
            lastProbeSecs_ = nowSecs;
            probeEgress();
        }
    }

    if (!connBusy_.exchange(true)) {
        QPointer<RuntimeStatsWidget> self(this);
        runOnNewThread([self]() {
            const auto conns = API::defaultClient->QueryConnections();
            int proxied = 0, direct = 0, system = 0;
            for (const auto& c : conns.active) {
                const QString outbound = QString::fromStdString(c.outbound.value());
                if (outbound.isEmpty()) ++system;
                else if (outbound == QStringLiteral("direct")) ++direct;
                else ++proxied;
            }
            const QString networkKind = LocalNetwork::LanInterfaceKind();
            runOnUiThread([self, proxied, direct, system, networkKind]() {
                if (!self) return;
                self->ui->vState->setText(networkKind.isEmpty() ? RuntimeStatsWidget::tr("Offline") : RuntimeStatsWidget::tr("Online"));
                self->ui->vNetworkKind->setText(networkKind.isEmpty() ? QStringLiteral("—") : networkKind);
                self->ui->vViaProxy->setText(QString::number(proxied));
                self->ui->vDirect->setText(QString::number(direct));
                self->ui->vSystem->setText(QString::number(system));
                self->connBusy_.store(false);
            });
        });
    }

    if (!vpnBusy_.exchange(true)) {
        QPointer<RuntimeStatsWidget> self(this);
        runOnNewThread([self]() {
            bool ok = false;
            // Empty tag list = every live endpoint; 0 ms = current state, never wait.
            const auto status = API::defaultClient->QueryVPNStatus(&ok, {}, 0);
            QList<Stats::VpnEndpointView> views;
            if (ok) {
                views.reserve(static_cast<qsizetype>(status.results.size()));
                for (const auto& result : status.results) views << Stats::MakeVpnEndpointView(result);
            }
            runOnUiThread([self, views]() {
                if (!self) return;
                self->applyEndpoints(views);
                self->vpnBusy_.store(false);
            });
        });
    }
}

QString RuntimeStatsWidget::selectedEndpointTag() const {
    const auto* selection = ui->endpointsTable->selectionModel();
    const auto rows = selection != nullptr ? selection->selectedRows() : QModelIndexList{};
    if (rows.isEmpty()) return {};
    const auto* item = ui->endpointsTable->item(rows.first().row(), 0);
    return item == nullptr ? QString() : item->data(Qt::UserRole).toString();
}

// A table's size hint ignores its rows, and the header's geometry is meaningless before show.
void RuntimeStatsWidget::fitEndpointTable() {
    auto* table = ui->endpointsTable;
    // Rows keep the vertical header's default section size until asked.
    table->resizeRowsToContents();
    // A cell widget is invisible to resizeRowsToContents, so the button sets the floor.
    for (int row = 0; row < table->rowCount(); row++) {
        if (auto* cell = table->cellWidget(row, 2))
            table->setRowHeight(row, std::max(table->rowHeight(row), cell->sizeHint().height()));
    }
    const int shown = std::min(table->rowCount(), 4);
    int height = table->horizontalHeader()->sizeHint().height() + 2 * table->frameWidth();
    for (int row = 0; row < shown; row++) height += table->rowHeight(row);
    table->setFixedHeight(height);
}

void RuntimeStatsWidget::applyEndpoints(const QList<Stats::VpnEndpointView>& views) {
    endpointViews_ = views;

    if (views.isEmpty()) {
        ui->groupEndpoints->setVisible(false);
        ui->endpointsTable->setRowCount(0);
        endpointTags_.clear();
        if (details_) details_->markGone();
        return;
    }

    QStringList tags;
    tags.reserve(views.size());
    for (const auto& view : views) tags << view.tag;

    ui->groupEndpoints->setVisible(true);

    auto* table = ui->endpointsTable;
    const bool rebuilt = tags != endpointTags_;
    if (rebuilt) {
        const QString keep = selectedEndpointTag();
        endpointTags_ = tags;
        table->setRowCount(static_cast<int>(views.size()));
        for (int row = 0; row < views.size(); row++) {
            for (int column = 0; column < 3; column++) {
                if (table->item(row, column) == nullptr) table->setItem(row, column, new QTableWidgetItem());
            }
            table->item(row, 0)->setData(Qt::UserRole, views[row].tag);
            auto* details = new QPushButton(tr("Details"), table);
            const QString tag = views[row].tag;
            connect(details, &QPushButton::clicked, this, [this, tag]() { openEndpointDetails(tag); });
            table->setCellWidget(row, 2, details);
        }
        const auto keptRow = static_cast<int>(tags.indexOf(keep));
        if (!keep.isEmpty() && keptRow >= 0) table->selectRow(keptRow);
        else if (!keep.isEmpty()) table->clearSelection();
    }

    for (int row = 0; row < views.size(); row++) {
        const auto& view = views[row];
        table->item(row, 0)->setText(view.displayName);
        table->item(row, 1)->setText(Stats::VpnStateText(view.state));
        table->item(row, 1)->setForeground(QBrush(Stats::VpnStateColor(view.state)));
        const QString hint = view.error.isEmpty() ? view.tag : view.tag + QLatin1Char(0x0a) + view.error;
        table->item(row, 0)->setToolTip(hint);
        table->item(row, 1)->setToolTip(hint);
    }
    if (rebuilt) fitEndpointTable();

    if (details_) {
        const auto found = std::find_if(views.cbegin(), views.cend(),
                                        [this](const Stats::VpnEndpointView& view) {
                                            return view.tag == details_->tag();
                                        });
        if (found != views.cend()) details_->applyStatus(*found);
        else details_->markGone();
    }
}

void RuntimeStatsWidget::openEndpointDetails(const QString& tag) {
    if (tag.isEmpty()) return;
    const auto found = std::find_if(endpointViews_.cbegin(), endpointViews_.cend(),
                                    [&tag](const Stats::VpnEndpointView& view) { return view.tag == tag; });
    if (found == endpointViews_.cend()) return;

    if (details_ && details_->tag() == tag) {
        details_->raise();
        details_->activateWindow();
        return;
    }
    if (details_) details_->close();

    details_ = new DialogEndpointDetails(*found, window());
    details_->setAttribute(Qt::WA_DeleteOnClose);
    details_->show();
    details_->raise();
    details_->activateWindow();
}

void RuntimeStatsWidget::probeEgress() {
    if (probing_.exchange(true)) return;

    if (runningConfigName().isEmpty()) {
        probing_.store(false);
        return;
    }

    const QString provider = egressLookup_;
    const QString config = lastProbedConfig_;
    ui->vPing->setText(QStringLiteral("…"));
    for (auto* row : {ui->vOutIp, ui->vCountry}) {
        if (provider.isEmpty()) row->setText(tr("Off"));
        else if (row->text() == QStringLiteral("—")) row->setText(QStringLiteral("…"));
    }

    QPointer<RuntimeStatsWidget> self(this);
    runOnNewThread([self, provider, config]() {
        QString pingText;
        {
            libcore::TestReq req;
            req.test_current = true;
            req.url = Configs::dataManager->settingsRepo->test_latency_url.toStdString();
            bool ok = false;
            const auto res = API::defaultClient->Test(&ok, req);
            if (ok && !res.results.empty()) {
                const int lat = res.results[0].latency_ms.value();
                const auto vpnText = lat > 0 ? QString() : Stats::LiveVpnConnectOkText();
                pingText = lat > 0 ? QStringLiteral("%1 ms").arg(lat)
                                   : (vpnText.isEmpty() ? RuntimeStatsWidget::tr("Unavailable") : vpnText);
            } else {
                pingText = RuntimeStatsWidget::tr("N/A");
            }
        }

        // Off still counts as a finished snapshot, so the 30 s retry does not keep re-pinging.
        QString ipText = RuntimeStatsWidget::tr("Off");
        QString countryText = ipText;
        bool egressOk = true;
        if (!provider.isEmpty()) {
            const auto info = Configs_network::LookupExit(provider);
            ipText = info.ip.isEmpty() ? RuntimeStatsWidget::tr("N/A") : info.ip;
            countryText = info.hasCountry() ? Configs_network::ExitLocationText(info) : RuntimeStatsWidget::tr("N/A");
            egressOk = !info.ip.isEmpty() || info.hasCountry();
        }

        runOnUiThread([self, provider, config, pingText, ipText, countryText, egressOk]() {
            if (!self) return;
            self->probing_.store(false);
            // The lookup setting or the config changed mid-probe, and refreshLive's own probe bailed on probing_.
            if (provider != self->egressLookup_ || config != self->lastProbedConfig_) {
                self->lastProbeSecs_ = 0;
                return;
            }
            if (!self->egressSnapshotDone_) {
                self->ui->vPing->setText(pingText);
                self->ui->vOutIp->setText(ipText);
                self->ui->vCountry->setText(countryText);
                if (egressOk) self->egressSnapshotDone_ = true;
            }
        });
    });
}
