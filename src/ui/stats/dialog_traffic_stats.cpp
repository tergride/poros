#include "include/ui/stats/dialog_traffic_stats.h"

#include "include/database/DatabaseManager.h"
#include "include/database/ProfilesRepo.h"
#include "include/database/TrafficStatsRepo.h"
#include "include/database/entities/Profile.h"
#include "include/stats/traffic/TrafficStatsManager.hpp"
#include "include/global/Utils.hpp"
#include "include/ui/setting/PorosTheme.h"

#include <QComboBox>
#include <QDateTime>
#include <QHash>
#include <QHeaderView>
#include <QLabel>
#include <QPushButton>
#include <QTableWidget>

#include <algorithm>

#include "include/configs/generate.h"

namespace {
    // Named rows per breakdown table; the rest is folded into one "Other" row.
    constexpr int kMaxBreakdownRows = 9;

    // Sorts on the raw byte value; the default compares the formatted text, ranking "900 MiB" above "1 GiB".
    class TrafficStatsSizeItem : public QTableWidgetItem {
    public:
        TrafficStatsSizeItem(const QString& text, long long value) : QTableWidgetItem(text) {
            QTableWidgetItem::setData(Qt::UserRole, QVariant::fromValue<qlonglong>(value));
            setTextAlignment(Qt::AlignRight | Qt::AlignVCenter);
            setFont(PorosTheme::MonoFont(13));
        }
        bool operator<(const QTableWidgetItem& other) const override {
            return data(Qt::UserRole).toLongLong() < other.data(Qt::UserRole).toLongLong();
        }
    };
}

DialogTrafficStats::DialogTrafficStats(QWidget* parent) : QDialog(parent), ui(new Ui::DialogTrafficStats) {
    ui->setupUi(this);

    ui->refreshBtn->setIcon(PorosTheme::Icon(QStringLiteral("refresh")));

    const auto fitColumns = [](QTableWidget *table, int firstNumber) {
        table->verticalHeader()->setDefaultSectionSize(36);
        auto *header = table->horizontalHeader();
        header->setDefaultAlignment(Qt::AlignLeft | Qt::AlignVCenter);
        for (int c = 0; c < table->columnCount(); ++c) {
            header->setSectionResizeMode(c, c < firstNumber ? QHeaderView::Stretch : QHeaderView::ResizeToContents);
            if (c >= firstNumber) table->horizontalHeaderItem(c)->setTextAlignment(Qt::AlignRight | Qt::AlignVCenter);
        }
    };
    fitColumns(ui->profileTable, 2);
    fitColumns(ui->appTable, 1);
    // The table sorts by this column, which the mockup leaves out next to the applications' total.
    ui->profileTable->setColumnHidden(4, true);

    connect(ui->refreshBtn, &QPushButton::clicked, this, [this] { refresh(); });
    connect(ui->periodCombo, &QComboBox::currentIndexChanged, this, [this](int) { refresh(); });

    refresh();
}

DialogTrafficStats::~DialogTrafficStats() {
    delete ui;
}

long long DialogTrafficStats::selectedWindowSecs() const {
    switch (ui->periodCombo->currentIndex()) {
        case 1: return 7LL * 86400LL;
        case 2: return 30LL * 86400LL;
        case 3: return 90LL * 86400LL;
        case 0:
        default: return 24LL * 3600LL;
    }
}

void DialogTrafficStats::refresh() {
    if (!Configs::dataManager || !Configs::dataManager->trafficStatsRepo) return;

    // The in-progress minute lives in memory until flushed.
    Stats::trafficStatsManager->Flush();

    const long long now = QDateTime::currentSecsSinceEpoch();
    const long long from = now - selectedWindowSecs();
    populateProfileTable(from, now);
    populateAppTable(from, now);
}

void DialogTrafficStats::populateProfileTable(long long fromSecs, long long toSecs) {
    auto* repo = Configs::dataManager->trafficStatsRepo.get();
    auto usage = repo->QueryConfigUsage(fromSecs, toSecs);
    QHash<int, Configs::ConfigMetaRow> meta;
    for (const auto& m : repo->GetAllConfigMeta()) meta.insert(m.profile_id, m);

    // Sorted here, not by the table, so the top-N cut is by total even after the user re-sorts.
    std::sort(usage.begin(), usage.end(), [](const Configs::ConfigUsage& a, const Configs::ConfigUsage& b) {
        return (a.down + a.up) > (b.down + b.up);
    });
    const int count = static_cast<int>(usage.size());
    const int shown = qMin(count, kMaxBreakdownRows);
    const bool hasOther = count > kMaxBreakdownRows;
    long long otherDown = 0, otherUp = 0, totalDown = 0, totalUp = 0;
    for (int i = 0; i < count; ++i) {
        totalDown += usage[i].down;
        totalUp += usage[i].up;
        if (i >= shown) {
            otherDown += usage[i].down;
            otherUp += usage[i].up;
        }
    }
    ui->summaryLabel->setText(tr("Download: %1  ·  Upload: %2  ·  Total: %3")
                                  .arg(ReadableSize(totalDown), ReadableSize(totalUp), ReadableSize(totalDown + totalUp)));

    ui->profileTable->setSortingEnabled(false);
    ui->profileTable->setRowCount(shown + (hasOther ? 1 : 0));
    for (int i = 0; i < shown; ++i) {
        const auto& u = usage[i];
        QString name, group;
        if (const auto it = meta.constFind(u.profile_id); it != meta.constEnd()) {
            name = it->name;
            group = it->group_name;
        }
        if (name.isEmpty()) {
            if (u.profile_id == Stats::DIRECT_STAT_PROFILE_ID) {
                name = tr("Direct");
            } else if (const auto prof = Configs::dataManager->profilesRepo->GetProfile(u.profile_id)) {
                name = prof->name;
            } else if (u.profile_id == Configs::warpProfileID) {
                name = "built-in warp";
            } else {
                name = tr("Profile #%1 (deleted)").arg(u.profile_id);
            }
        }
        ui->profileTable->setItem(i, 0, new QTableWidgetItem(name));
        ui->profileTable->setItem(i, 1, new QTableWidgetItem(group));
        ui->profileTable->setItem(i, 2, new TrafficStatsSizeItem(ReadableSize(u.down), u.down));
        ui->profileTable->setItem(i, 3, new TrafficStatsSizeItem(ReadableSize(u.up), u.up));
        ui->profileTable->setItem(i, 4, new TrafficStatsSizeItem(ReadableSize(u.down + u.up), u.down + u.up));
    }
    if (hasOther) {
        ui->profileTable->setItem(shown, 0, new QTableWidgetItem(tr("Other")));
        ui->profileTable->setItem(shown, 1, new QTableWidgetItem(QString()));
        ui->profileTable->setItem(shown, 2, new TrafficStatsSizeItem(ReadableSize(otherDown), otherDown));
        ui->profileTable->setItem(shown, 3, new TrafficStatsSizeItem(ReadableSize(otherUp), otherUp));
        ui->profileTable->setItem(shown, 4, new TrafficStatsSizeItem(ReadableSize(otherDown + otherUp), otherDown + otherUp));
    }
    ui->profileTable->setSortingEnabled(true);
    ui->profileTable->sortItems(4, Qt::DescendingOrder);
}

void DialogTrafficStats::populateAppTable(long long fromSecs, long long toSecs) {
    auto* repo = Configs::dataManager->trafficStatsRepo.get();
    auto usage = repo->QueryAppUsage(fromSecs, toSecs);

    std::sort(usage.begin(), usage.end(), [](const Configs::AppUsage& a, const Configs::AppUsage& b) {
        return (a.down + a.up) > (b.down + b.up);
    });
    const int count = static_cast<int>(usage.size());
    const int shown = qMin(count, kMaxBreakdownRows);
    const bool hasOther = count > kMaxBreakdownRows;
    long long otherDown = 0, otherUp = 0;
    for (int i = shown; i < count; ++i) {
        otherDown += usage[i].down;
        otherUp += usage[i].up;
    }

    ui->appTable->setSortingEnabled(false);
    ui->appTable->setRowCount(shown + (hasOther ? 1 : 0));
    for (int i = 0; i < shown; ++i) {
        const auto& u = usage[i];
        QString name = u.process_name.isEmpty() ? tr("Unknown") : u.process_name;
        ui->appTable->setItem(i, 0, new QTableWidgetItem(name));
        ui->appTable->setItem(i, 1, new TrafficStatsSizeItem(ReadableSize(u.down), u.down));
        ui->appTable->setItem(i, 2, new TrafficStatsSizeItem(ReadableSize(u.up), u.up));
        ui->appTable->setItem(i, 3, new TrafficStatsSizeItem(ReadableSize(u.down + u.up), u.down + u.up));
    }
    if (hasOther) {
        ui->appTable->setItem(shown, 0, new QTableWidgetItem(tr("Other")));
        ui->appTable->setItem(shown, 1, new TrafficStatsSizeItem(ReadableSize(otherDown), otherDown));
        ui->appTable->setItem(shown, 2, new TrafficStatsSizeItem(ReadableSize(otherUp), otherUp));
        ui->appTable->setItem(shown, 3, new TrafficStatsSizeItem(ReadableSize(otherDown + otherUp), otherDown + otherUp));
    }
    ui->appTable->setSortingEnabled(true);
    ui->appTable->sortItems(3, Qt::DescendingOrder);
}
