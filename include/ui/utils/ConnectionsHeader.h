#pragma once

#include <QAbstractItemView>
#include <QHash>
#include <QHeaderView>

#include "include/ui/utils/ConnectionsTreeModel.h"

class ConnectionsHeader : public QHeaderView {
    Q_OBJECT
public:
    explicit ConnectionsHeader(QWidget *parent = nullptr) : QHeaderView(Qt::Horizontal, parent) {
        setSectionsClickable(true);
        setDefaultAlignment(Qt::AlignLeft | Qt::AlignVCenter);
    }

    // The sort runs in the lister, not in the view, so the indicator only reports it. -1 shows none.
    void setSortSection(int section, bool descending) {
        const bool moved = section != m_sortSection;
        m_sortSection = section;
        setSortIndicatorShown(section >= 0);
        if (section >= 0) setSortIndicator(section, descending ? Qt::DescendingOrder : Qt::AscendingOrder);
        if (moved) {
            resizeSections();
            fitOutboundSection();
        }
    }

    // Only ever widens: an exact fit would resize the section every poll as live digits change.
    void growSection(int section) {
        const auto *view = qobject_cast<QAbstractItemView *>(parent());
        const int contentWidth = view != nullptr ? view->sizeHintForColumn(section) : -1;
        if (contentWidth <= m_grownWidths.value(section)) return;
        m_grownWidths.insert(section, contentWidth * 115 / 100);
        resizeSections();
    }

protected:
    QSize sectionSizeFromContents(int logicalIndex) const override {
        QSize s = QHeaderView::sectionSizeFromContents(logicalIndex);
        s.setWidth(qMax(s.width(), m_grownWidths.value(logicalIndex)));
        return s;
    }

    // Runs after every relayout, expand, viewport resize and (via the view's column timer) section resize.
    void updateGeometries() override {
        QHeaderView::updateGeometries();
        fitOutboundSection();
    }

private:
    // Outbound fits its content into what the stretched Target leaves past the Target's own content, so a long address wins.
    void fitOutboundSection() {
        using Model = ConnectionsTreeModel;
        const auto *view = qobject_cast<QAbstractItemView *>(parent());
        if (view == nullptr || count() < Model::ColumnCount) return;

        const int floor = sectionSizeHint(Model::ColOutbound);
        const int wanted = qMax(floor, view->sizeHintForColumn(Model::ColOutbound));
        int room = viewport()->width() - qMax(sectionSizeHint(Model::ColTarget), view->sizeHintForColumn(Model::ColTarget));
        for (int section = 0; section < count(); ++section) {
            if (section != Model::ColTarget && section != Model::ColOutbound && !isSectionHidden(section))
                room -= sectionSize(section);
        }
        const int width = qBound(floor, room, wanted);
        if (width != sectionSize(Model::ColOutbound)) resizeSection(Model::ColOutbound, width);
    }

    int m_sortSection = -1;
    QHash<int, int> m_grownWidths;
};
