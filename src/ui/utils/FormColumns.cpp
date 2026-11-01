#include "include/ui/utils/FormColumns.h"

#include <QEvent>
#include <QGridLayout>
#include <QGroupBox>
#include <QLabel>
#include <QStyle>
#include <QVBoxLayout>

#include <algorithm>

namespace {
    constexpr int kColumnGap = 16;
    constexpr int kRowGap = 12;
    constexpr int kLabelGap = 2;

    struct Entry {
        int row;
        int column;
        int columnSpan;
        QLayoutItem *item;
    };

    QList<Entry> takeEntries(QGridLayout *grid) {
        QList<Entry> entries;
        while (grid->count() > 0) {
            int row, column, rowSpan, columnSpan;
            grid->getItemPosition(0, &row, &column, &rowSpan, &columnSpan);
            entries.append({row, column, columnSpan, grid->takeAt(0)});
        }
        std::sort(entries.begin(), entries.end(), [](const Entry &a, const Entry &b) {
            return a.row != b.row ? a.row < b.row : a.column < b.column;
        });
        return entries;
    }

    void addToCell(QVBoxLayout *cell, QLayoutItem *item) {
        if (auto *layout = item->layout()) cell->addLayout(layout);
        else cell->addItem(item);
    }

    bool isLabel(const Entry &entry) {
        return qobject_cast<QLabel *>(entry.item->widget()) != nullptr;
    }
}

void FormColumns::adoptAll(QWidget *root) {
    for (auto *grid : root->findChildren<QGridLayout *>()) new FormColumns(grid);
}

FormColumns::FormColumns(QGridLayout *grid) : QObject(grid), m_grid(grid) {
    const auto entries = takeEntries(grid);
    for (qsizetype i = 0; i < entries.size(); ++i) {
        const auto &entry = entries[i];
        if (entry.item->spacerItem() != nullptr) {
            delete entry.item;
            continue;
        }

        auto *cell = new QVBoxLayout;
        cell->setSpacing(kLabelGap);
        bool wide = entry.columnSpan >= 2;

        const bool hasField = isLabel(entry) && !wide && i + 1 < entries.size() && entries[i + 1].row == entry.row
                              && !isLabel(entries[i + 1]) && entries[i + 1].item->spacerItem() == nullptr;
        if (hasField) {
            auto *label = static_cast<QLabel *>(entry.item->widget());
            label->setIndent(0);
            if (!label->property("role").isValid()) {
                label->setProperty("role", QStringLiteral("field"));
                // A scroll area has already asked its contents for their size, which polishes them.
                label->style()->unpolish(label);
                label->style()->polish(label);
            }
            addToCell(cell, entry.item);
            const auto *field = entries[++i].item;
            wide = field->expandingDirections().testFlag(Qt::Vertical);
            addToCell(cell, entries[i].item);
        } else {
            // A checkbox on its own sits level with the fields beside it.
            cell->addStretch();
            addToCell(cell, entry.item);
        }

        for (int index = 0; index < cell->count(); ++index) {
            if (auto *widget = cell->itemAt(index)->widget()) widget->installEventFilter(this);
        }
        m_cells.append({cell, wide});
    }

    for (int column = 0; column < grid->columnCount(); ++column) grid->setColumnStretch(column, column < 2 ? 1 : 0);
    for (int row = 0; row < grid->rowCount(); ++row) grid->setRowStretch(row, 0);
    grid->setHorizontalSpacing(kColumnGap);
    grid->setVerticalSpacing(kRowGap);
    if (qobject_cast<QGroupBox *>(grid->parentWidget()) != nullptr) grid->setContentsMargins(16, 4, 16, 14);
    else grid->setContentsMargins(0, 0, 0, 0);
    reflow();
}

bool FormColumns::eventFilter(QObject *watched, QEvent *event) {
    if ((event->type() == QEvent::ShowToParent || event->type() == QEvent::HideToParent) && !m_reflowPending) {
        m_reflowPending = true;
        QMetaObject::invokeMethod(this, [this] { reflow(); }, Qt::QueuedConnection);
    }
    return QObject::eventFilter(watched, event);
}

void FormColumns::reflow() {
    m_reflowPending = false;
    for (const auto &cell : m_cells) m_grid->removeItem(cell.layout);

    int row = 0;
    int column = 0;
    // Rows of hidden cells collapse, so they can wait at the end.
    QList<QVBoxLayout *> hidden;
    for (const auto &cell : m_cells) {
        if (cell.layout->isEmpty()) {
            hidden.append(cell.layout);
            continue;
        }
        if (cell.wide && column != 0) {
            ++row;
            column = 0;
        }
        m_grid->addItem(cell.layout, row, column, 1, cell.wide ? 2 : 1);
        cell.layout->setParent(m_grid);
        if (cell.wide || column == 1) {
            ++row;
            column = 0;
        } else {
            column = 1;
        }
    }
    if (column != 0) ++row;
    for (auto *cell : hidden) {
        m_grid->addItem(cell, row++, 0);
        cell->setParent(m_grid);
    }
}
