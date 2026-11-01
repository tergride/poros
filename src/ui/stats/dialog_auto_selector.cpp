#include "include/ui/stats/dialog_auto_selector.h"

#include "include/ui/setting/PorosTheme.h"
#include "include/ui/utils/GuiUtils.hpp"

#include <QCheckBox>
#include <QDateTime>
#include <QFrame>
#include <QHBoxLayout>
#include <QHeaderView>
#include <QItemSelectionModel>
#include <QLabel>
#include <QPushButton>
#include <QTableWidget>
#include <QVBoxLayout>
#include <algorithm>

namespace
{
    enum Column {
        ColName = 0,
        ColState,
        ColLatency,
        ColLastOK,
        ColNote,
        ColCount
    };

    constexpr int kLatencyFontPx = 13;

    QString agoText(qint64 ms)
    {
        if (ms <= 0) return QObject::tr("never");
        const auto secs = (QDateTime::currentMSecsSinceEpoch() - ms) / 1000;
        if (secs < 5) return QObject::tr("just now");
        if (secs < 60) return QObject::tr("%1s ago").arg(secs);
        if (secs < 3600) return QObject::tr("%1m ago").arg(secs / 60);
        return QObject::tr("%1h ago").arg(secs / 3600);
    }

    QString stateText(const Stats::AutoSelectorMemberView &member)
    {
        if (member.state == "ok") return QObject::tr("Working");
        if (member.state == "degraded") return QObject::tr("Unstable");
        if (member.state == "untested") return QObject::tr("Not checked");
        if (member.state == "dead") return QObject::tr("Failing");
        if (member.state == "cooldown") return QObject::tr("Paused");
        return member.state;
    }

    int stateOrder(const Stats::AutoSelectorMemberView &member)
    {
        if (member.state == "ok") return 0;
        if (member.state == "degraded") return 1;
        if (member.state == "untested") return 2;
        if (member.state == "cooldown") return 3;
        return 4; // dead
    }

    struct Key {
        bool known = false;
        double value = 0;
    };

    Key latencyKey(const Stats::AutoSelectorMemberView &m) { return {m.samples > 0 && m.averageMs > 0, static_cast<double>(m.averageMs)}; }
    Key lastOKKey(const Stats::AutoSelectorMemberView &m) { return {m.lastOKms > 0, static_cast<double>(m.lastOKms)}; }

    QPushButton *smallButton(const QString &text, QWidget *parent)
    {
        auto *button = new QPushButton(text, parent);
        button->setProperty("role", QStringLiteral("small"));
        return button;
    }

    QString noteText(const Stats::AutoSelectorMemberView &member)
    {
        if (member.pinned) {
            if (member.selected) return QObject::tr("your choice, carrying traffic now");
            return QObject::tr("your choice, but not usable right now");
        }
        if (member.selected) return QObject::tr("carrying traffic now");
        if (member.state == "cooldown") {
            const auto secs = (member.cooldownUntilMs - QDateTime::currentMSecsSinceEpoch()) / 1000;
            if (secs > 0) return QObject::tr("failed to connect, retrying in %1s").arg(secs);
            return QObject::tr("failed to connect");
        }
        if (member.state == "dead") {
            return member.lastError.isEmpty() ? QObject::tr("every check failed") : member.lastError;
        }
        if (member.qualified) return QObject::tr("ready to take over");
        if (member.state == "untested") {
            return member.active ? QObject::tr("check in progress") : QObject::tr("queued for checking");
        }
        if (member.failures > 0) return QObject::tr("%1 of %2 checks failed").arg(member.failures).arg(member.samples);
        return {};
    }
}

DialogAutoSelector::DialogAutoSelector(QWidget *parent) : QDialog(parent)
{
    setWindowTitle(tr("Auto Selector Stats"));

    auto *layout = new QVBoxLayout(this);
    layout->setContentsMargins(0, 0, 0, 0);
    layout->setSpacing(0);

    auto *toolbar = new QFrame(this);
    toolbar->setProperty("role", QStringLiteral("toolbar"));
    auto *controls = new QHBoxLayout(toolbar);
    controls->setContentsMargins(14, 10, 14, 10);
    controls->setSpacing(8);

    m_headline = new QLabel(toolbar);
    m_headline->setTextFormat(Qt::PlainText);
    m_headline->setSizePolicy(QSizePolicy::Ignored, QSizePolicy::Preferred);
    controls->addWidget(m_headline, 1);

    m_count = new QLabel(toolbar);
    m_count->setProperty("role", QStringLiteral("tag"));
    m_count->setProperty("level", QStringLiteral("ok"));
    controls->addWidget(m_count);

    m_onlyProblems = new QCheckBox(tr("Only show profiles with problems"), toolbar);
    connect(m_onlyProblems, &QCheckBox::toggled, this, [this] { refresh(); });
    controls->addWidget(m_onlyProblems);

    m_pin = smallButton(tr("Use this profile"), toolbar);
    m_pin->setToolTip(tr("Keep the selected profile in use instead of letting the ranking choose. "
                         "Useful when several profiles measure much the same and you prefer one of "
                         "them.\n\nIt stays a preference, not a lock: if that profile stops working "
                         "the selector still moves on, and comes back to your choice once it "
                         "recovers."));
    connect(m_pin, &QPushButton::clicked, this, [this] { applySelection(highlightedTag()); });
    controls->addWidget(m_pin);

    m_release = smallButton(tr("Back to automatic"), toolbar);
    m_release->setToolTip(tr("Stop preferring a particular profile and let the ranking decide again."));
    connect(m_release, &QPushButton::clicked, this, [this] { applySelection({}); });
    controls->addWidget(m_release);

    m_recheck = smallButton(tr("Check all now"), toolbar);
    m_recheck->setIcon(PorosTheme::Icon(QStringLiteral("refresh")));
    m_recheck->setIconSize({14, 14});
    m_recheck->setToolTip(tr("Re-measure every running profile immediately instead of waiting for the "
                             "next scheduled check."));
    connect(m_recheck, &QPushButton::clicked, this, [this] {
        Stats::autoSelectorMonitor->RequestRecheck();
        m_footer->setText(tr("Re-checking every profile..."));
        m_footer->show();
    });
    controls->addWidget(m_recheck);
    layout->addWidget(toolbar);

    m_table = new QTableWidget(this);
    m_table->setProperty("role", QStringLiteral("table"));
    m_table->setProperty("flush", true);
    m_table->setColumnCount(ColCount);
    m_table->setHorizontalHeaderLabels({tr("Profile"), tr("Status"), tr("Latency"), tr("Last OK"), tr("Notes")});
    m_table->verticalHeader()->setVisible(false);
    m_table->verticalHeader()->setDefaultSectionSize(36);
    m_table->setEditTriggers(QAbstractItemView::NoEditTriggers);
    m_table->setSelectionBehavior(QAbstractItemView::SelectRows);
    m_table->setShowGrid(false);

    auto *header = m_table->horizontalHeader();
    header->setDefaultAlignment(Qt::AlignLeft | Qt::AlignVCenter);
    header->setSectionsClickable(true);
    connect(header, &QHeaderView::sectionClicked, this, &DialogAutoSelector::onHeaderClicked);
    header->setSectionResizeMode(QHeaderView::ResizeToContents);
    header->setSectionResizeMode(ColName, QHeaderView::Stretch);
    header->setSectionResizeMode(ColNote, QHeaderView::Stretch);
    connect(m_table, &QTableWidget::itemSelectionChanged, this, [this] {
        m_pin->setEnabled(!highlightedTag().isEmpty() && highlightedTag() != m_pinnedTag);
    });
    connect(m_table, &QTableWidget::itemDoubleClicked, this, [this](QTableWidgetItem *) {
        applySelection(highlightedTag());
    });
    layout->addWidget(m_table, 1);

    m_footer = new QLabel(this);
    m_footer->setProperty("role", QStringLiteral("hint"));
    m_footer->setWordWrap(true);
    m_footer->setContentsMargins(14, 8, 14, 8);
    layout->addWidget(m_footer);

    refresh();
}

QString DialogAutoSelector::highlightedTag() const
{
    const auto rows = m_table->selectionModel() != nullptr ? m_table->selectionModel()->selectedRows()
                                                           : QModelIndexList{};
    if (rows.isEmpty()) return {};
    // The tag rides on the name cell: sorting and filtering reorder rows under the selection.
    const auto *item = m_table->item(rows.first().row(), ColName);
    return item == nullptr ? QString() : item->data(Qt::UserRole).toString();
}

void DialogAutoSelector::applySelection(const QString &tag)
{
    if (tag.isEmpty() && m_pinnedTag.isEmpty()) return;
    const auto error = Stats::autoSelectorMonitor->RequestSelect(tag);
    m_footer->show();
    if (!error.isEmpty()) {
        m_footer->setText(tr("Could not change the profile: %1").arg(error));
        return;
    }
    // The core applies this at once, but the table only catches up on the next poll.
    m_pinnedTag = tag;
    m_footer->setText(tag.isEmpty() ? tr("Back to automatic — the selector will choose again.")
                                    : tr("Now using your chosen profile."));
    m_pin->setEnabled(false);
    m_release->setEnabled(!tag.isEmpty());
}

void DialogAutoSelector::refresh()
{
    const auto view = Stats::autoSelectorMonitor->Snapshot();
    if (!view.valid) {
        m_headline->setText(tr("No auto selector is running."));
        m_count->hide();
        m_footer->hide();
        m_table->setRowCount(0);
        m_recheck->setEnabled(false);
        m_pin->setEnabled(false);
        m_release->setEnabled(false);
        return;
    }
    m_recheck->setEnabled(true);
    m_pinnedTag = view.pinnedTag;
    m_pin->setEnabled(!highlightedTag().isEmpty() && highlightedTag() != m_pinnedTag);
    m_release->setEnabled(!m_pinnedTag.isEmpty());
    m_headline->setText(view.summary());
    m_headline->setToolTip(view.summary());
    m_count->setText(tr("Profiles: %1").arg(view.membersTotal));
    m_count->setToolTip(view.detail());
    m_count->show();

    QStringList footer;
    if (view.suspended) {
        footer << tr("Checks are paused because this machine has no network connection. "
                     "No profile is being blamed for it, and the ranking is frozen until the "
                     "connection returns.");
    } else {
        if (view.roundsCompleted > 0) footer << tr("Last check round %1.").arg(agoText(view.lastRoundMs));
        if (view.nextRoundMs > 0) {
            const auto secs = (view.nextRoundMs - QDateTime::currentMSecsSinceEpoch()) / 1000;
            if (secs > 0) footer << tr("Next in %1s.").arg(secs);
        }
    }
    if (!view.pinnedTag.isEmpty()) {
        const auto pinnedName = view.pinnedName.isEmpty() ? view.pinnedTag : view.pinnedName;
        footer << (view.pinnedTag == view.selectedTag
                       ? tr("Using %1 because you chose it.").arg(pinnedName)
                       : tr("You chose %1, but it is not working right now, so the selector picked "
                            "another. It will go back to yours once it recovers.")
                             .arg(pinnedName));
    }
    if (!view.lastSwitchReason.isEmpty() && view.lastSwitchMs > 0) {
        footer << tr("Last switch %1 (%2).").arg(agoText(view.lastSwitchMs), view.lastSwitchReason);
    }
    if (view.exhaustedSinceMs > 0) {
        footer << tr("Nothing is working right now — if this holds, the selector will rebuild from the "
                     "next best profiles.");
    }
    m_footer->setText(footer.join(" "));
    m_footer->setVisible(!footer.isEmpty());

    buildRows(view);
}

void DialogAutoSelector::sortRows(QList<Stats::AutoSelectorMemberView> &rows) const
{
    const bool ascending = m_sortOrder == Qt::AscendingOrder;
    const int column = m_sortColumn;

    const auto compareKeys = [ascending](const Key &left, const Key &right, bool &decided) {
        decided = true;
        if (left.known != right.known) return left.known; // unknowns always last
        if (!left.known) { decided = false; return false; }
        if (left.value == right.value) { decided = false; return false; }
        return ascending ? left.value < right.value : left.value > right.value;
    };

    std::stable_sort(rows.begin(), rows.end(),
                     [&](const Stats::AutoSelectorMemberView &a, const Stats::AutoSelectorMemberView &b) {
        bool decided = false;
        bool result = false;
        switch (column) {
            case ColName: {
                const int cmp = QString::compare(a.name.isEmpty() ? a.tag : a.name,
                                                 b.name.isEmpty() ? b.tag : b.name, Qt::CaseInsensitive);
                if (cmp != 0) return ascending ? cmp < 0 : cmp > 0;
                break;
            }
            case ColState:
                if (stateOrder(a) != stateOrder(b))
                    return ascending ? stateOrder(a) < stateOrder(b) : stateOrder(a) > stateOrder(b);
                break;
            case ColLatency: result = compareKeys(latencyKey(a), latencyKey(b), decided); break;
            case ColLastOK: result = compareKeys(lastOKKey(a), lastOKKey(b), decided); break;
            case ColNote: {
                const int cmp = QString::compare(noteText(a), noteText(b), Qt::CaseInsensitive);
                if (cmp != 0) return ascending ? cmp < 0 : cmp > 0;
                break;
            }
            default:
                break;
        }
        if (decided) return result;
        // Rank is the tiebreak everywhere, so equal rows don't shuffle between polls.
        return a.rank < b.rank;
    });
}

void DialogAutoSelector::onHeaderClicked(int column)
{
    if (column == m_sortColumn) {
        m_sortOrder = m_sortOrder == Qt::AscendingOrder ? Qt::DescendingOrder : Qt::AscendingOrder;
    } else {
        m_sortColumn = column;
        m_sortOrder = column == ColLastOK ? Qt::DescendingOrder : Qt::AscendingOrder;
    }
    m_table->horizontalHeader()->setSortIndicatorShown(true);
    m_table->horizontalHeader()->setSortIndicator(m_sortColumn, m_sortOrder);
    refresh();
}

void DialogAutoSelector::buildRows(const Stats::AutoSelectorView &view)
{
    const bool filtered = m_onlyProblems->isChecked();
    QList<Stats::AutoSelectorMemberView> rows;
    for (const auto &member : view.members) {
        if (filtered && !member.hasProblem()) continue;
        rows << member;
    }
    sortRows(rows);

    // A full reset on every poll would fight the user's scroll position.
    if (m_table->rowCount() != rows.size()) m_table->setRowCount(static_cast<int>(rows.size()));

    const auto setCell = [this](int row, int column, const QString &text, const QString &tip = {}) {
        auto *item = m_table->item(row, column);
        if (item == nullptr) {
            item = new QTableWidgetItem();
            m_table->setItem(row, column, item);
        }
        if (item->text() != text) item->setText(text);
        if (!tip.isEmpty()) item->setToolTip(tip);
    };

    for (int row = 0; row < rows.size(); row++) {
        const auto &member = rows[row];
        setCell(row, ColName, member.name.isEmpty() ? member.tag : member.name, member.tag);
        QStringList stateTip;
        if (member.samples > 0) stateTip << tr("Checks: %1 of %2 ok").arg(member.samples - member.failures).arg(member.samples);
        if (member.dialTotal > 0) stateTip << tr("Connections from apps: %1 of %2 ok").arg(member.dialTotal - member.dialFail).arg(member.dialTotal);
        setCell(row, ColState, stateText(member), stateTip.join(QLatin1Char('\n')));
        setCell(row, ColLatency, member.averageMs > 0 ? tr("%1 ms").arg(member.averageMs) : tr("no response"),
                member.samples > 0 ? tr("Best %1 ms, worst %2 ms, jitter %3 ms").arg(member.minMs).arg(member.maxMs).arg(member.deviationMs) : QString());
        m_table->item(row, ColLatency)->setFont(PorosTheme::MonoFont(kLatencyFontPx));
        setCell(row, ColLastOK, agoText(member.lastOKms));
        setCell(row, ColNote, noteText(member));

        auto *nameItem = m_table->item(row, ColName);
        nameItem->setData(Qt::UserRole, member.tag);
        auto font = nameItem->font();
        if (font.bold() != member.selected || font.italic() != member.pinned) {
            font.setBold(member.selected);
            font.setItalic(member.pinned);
            nameItem->setFont(font);
        }
    }
}
