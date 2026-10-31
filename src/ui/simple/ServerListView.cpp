#include "include/ui/simple/ServerListView.h"

#include <QMouseEvent>
#include <QPainter>
#include <QPainterPath>
#include <QSortFilterProxyModel>
#include <QStyledItemDelegate>

#include "include/ui/setting/PorosTheme.h"
#include "include/ui/simple/ServerTreeModel.h"

namespace {
    using Kind = ServerTreeModel::Kind;

    constexpr int serverListMargin = 16;
    constexpr int serverListRowHeight = 42;
    constexpr int serverListGroupHeight = 58;
    constexpr int serverListGroupGap = 12;
    constexpr int serverListButton = 30;

    QRect serverListCard(const QRect &row) {
        return row.adjusted(serverListMargin, 0, -serverListMargin, 0);
    }

    QRect serverListRefreshButton(const QRect &row) {
        const QRect card = serverListCard(row);
        return {card.right() - 2 * serverListButton - 8, row.top() + (serverListGroupHeight - serverListButton) / 2, serverListButton, serverListButton};
    }

    QRect serverListMoreButton(const QRect &row) {
        return serverListRefreshButton(row).translated(serverListButton + 2, 0);
    }

    QPainterPath serverListCardPath(const QRectF &rect, bool roundTop, bool roundBottom) {
        constexpr qreal radius = 12;
        QPainterPath path;
        path.moveTo(rect.left(), rect.top() + (roundTop ? radius : 0));
        if (roundTop) path.quadTo(rect.topLeft(), {rect.left() + radius, rect.top()});
        path.lineTo(rect.right() - (roundTop ? radius : 0), rect.top());
        if (roundTop) path.quadTo(rect.topRight(), {rect.right(), rect.top() + radius});
        path.lineTo(rect.right(), rect.bottom() - (roundBottom ? radius : 0));
        if (roundBottom) path.quadTo(rect.bottomRight(), {rect.right() - radius, rect.bottom()});
        path.lineTo(rect.left() + (roundBottom ? radius : 0), rect.bottom());
        if (roundBottom) path.quadTo(rect.bottomLeft(), {rect.left(), rect.bottom() - radius});
        path.closeSubpath();
        return path;
    }

    QColor serverListPingColor(int latency) {
        if (latency <= 0) return QColor(PorosTheme::T3);
        if (latency < 100) return QColor(PorosTheme::Ok);
        return QColor(latency < 300 ? PorosTheme::Warn : PorosTheme::Bad);
    }

    class ServerDelegate : public QStyledItemDelegate {
    public:
        ServerDelegate(const ServerTreeModel *model, const QSortFilterProxyModel *filter, QObject *parent)
            : QStyledItemDelegate(parent), m_model(model), m_filter(filter) {}

        QSize sizeHint(const QStyleOptionViewItem &, const QModelIndex &viewIndex) const override {
            const auto index = m_filter->mapToSource(viewIndex);
            switch (m_model->kindOf(index)) {
                case Kind::Group: return {200, serverListGroupHeight + (m_model->rowCount(index) == 0 ? serverListGroupGap : 0)};
                case Kind::AddRow: return {200, serverListRowHeight + serverListGroupGap};
                default: return {200, serverListRowHeight + (m_model->isLastInGroup(index) ? serverListGroupGap : 0)};
            }
        }

        void paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &viewIndex) const override {
            const auto index = m_filter->mapToSource(viewIndex);
            painter->save();
            painter->setRenderHint(QPainter::Antialiasing);
            painter->setClipRect(option.rect);
            switch (m_model->kindOf(index)) {
                case Kind::Group: paintGroup(painter, option, viewIndex, index); break;
                case Kind::AddRow: paintAddRow(painter, option); break;
                default: paintServer(painter, option, index); break;
            }
            painter->restore();
        }

    private:
        void paintGroup(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &viewIndex, const QModelIndex &index) const {
            using namespace PorosTheme;
            const auto &group = m_model->groupOf(index);
            const bool open = option.state & QStyle::State_Open;
            const bool closedLook = !open || !viewIndex.model()->hasChildren(viewIndex);
            QRect card = serverListCard(option.rect);
            card.setHeight(serverListGroupHeight);
            const auto path = serverListCardPath(QRectF(card).adjusted(0.5, 0.5, -0.5, closedLook ? -0.5 : 0.5), true, closedLook);
            painter->fillPath(path, QColor(S2));
            painter->strokePath(path, QPen(QColor(Line), 1));

            PorosTheme::Icon(open ? QStringLiteral("down") : QStringLiteral("right")).paint(painter, card.left() + 12, card.top() + 21, 16, 16);

            constexpr int quotaWidth = 280;
            const bool hasQuota = !group.quota.isEmpty() || !group.expiry.isEmpty();
            const int textRight = card.right() - 2 * serverListButton - 16 - (hasQuota ? quotaWidth + 12 : 0);
            const QRect title(card.left() + 38, card.top() + 9, textRight - card.left() - 38, 20);
            QFont bold = option.font;
            bold.setWeight(QFont::DemiBold);
            painter->setFont(bold);
            painter->setPen(QColor(T1));
            painter->drawText(title, Qt::AlignLeft | Qt::AlignVCenter, QFontMetrics(bold).elidedText(group.name, Qt::ElideRight, title.width()));

            QFont small = option.font;
            small.setPixelSize(12);
            painter->setFont(small);
            painter->setPen(QColor(T3));
            painter->drawText(title.translated(0, 20), Qt::AlignLeft | Qt::AlignVCenter, QFontMetrics(small).elidedText(group.subtitle, Qt::ElideRight, title.width()));

            if (hasQuota) {
                const QRect quota(textRight + 12, card.top() + 12, quotaWidth, 16);
                painter->setPen(QColor(T2));
                painter->drawText(quota, Qt::AlignLeft | Qt::AlignVCenter, group.quota);
                painter->setPen(QColor(T3));
                painter->drawText(quota, Qt::AlignRight | Qt::AlignVCenter, group.expiry);
                if (group.usedShare >= 0) {
                    const QRectF bar(quota.left(), quota.bottom() + 6, quota.width(), 5);
                    painter->setPen(Qt::NoPen);
                    painter->setBrush(QColor(S5));
                    painter->drawRoundedRect(bar, 2.5, 2.5);
                    const QRectF used(bar.topLeft(), QSizeF(bar.width() * qBound(0.0, group.usedShare, 1.0), bar.height()));
                    painter->setBrush(RoadGradient(bar));
                    painter->drawRoundedRect(used, 2.5, 2.5);
                }
            }
            const QPoint mouse = option.widget ? option.widget->mapFromGlobal(QCursor::pos()) : QPoint();
            paintHeaderButton(painter, serverListRefreshButton(option.rect), QStringLiteral("refresh"), mouse);
            paintHeaderButton(painter, serverListMoreButton(option.rect), QStringLiteral("more"), mouse);
        }

        static void paintHeaderButton(QPainter *painter, const QRect &rect, const QString &icon, const QPoint &) {
            PorosTheme::Icon(icon).paint(painter, rect.adjusted(7, 7, -7, -7));
        }

        void paintServer(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const {
            using namespace PorosTheme;
            const bool last = m_model->isLastInGroup(index);
            const bool isAuto = m_model->kindOf(index) == Kind::Auto;
            QRect card = serverListCard(option.rect);
            card.setHeight(serverListRowHeight);
            const int id = isAuto ? m_model->groupOf(index).autoId : m_model->serverOf(index).id;
            const bool selected = id >= 0 && id == m_model->selectedId();
            const bool hover = option.state & QStyle::State_MouseOver;

            const auto path = serverListCardPath(QRectF(card).adjusted(0.5, 0, -0.5, last ? -0.5 : 0), false, last);
            painter->fillPath(path, QColor(selected ? S4 : hover ? S3 : S2));
            painter->strokePath(path, QPen(QColor(Line), 1));

            const QRect badge(card.left() + 12, card.center().y() - 10, 30, 22);
            painter->setPen(Qt::NoPen);
            if (isAuto) painter->setBrush(PrimaryGradient(badge));
            else painter->setBrush(QColor(S4));
            painter->drawRoundedRect(badge, 5, 5);

            const QRect radio(card.right() - 28, card.center().y() - 8, 16, 16);
            if (selected) {
                painter->setPen(QPen(QColor(Blue), 5));
                painter->setBrush(Qt::NoBrush);
                painter->drawEllipse(QRectF(radio).adjusted(2.5, 2.5, -2.5, -2.5));
            } else {
                painter->setPen(QPen(QColor(Line2), 1.5));
                painter->setBrush(Qt::NoBrush);
                painter->drawEllipse(QRectF(radio).adjusted(1, 1, -1, -1));
            }

            int textRight = radio.left() - 10;
            if (isAuto) {
                PorosTheme::Icon(QStringLiteral("bolt"), Qt::white).paint(painter, badge.adjusted(8, 4, -8, -4));
                painter->setFont(option.font);
                painter->setPen(QColor(T1));
                const QString title = ServerTreeModel::tr("Auto");
                const int titleWidth = option.fontMetrics.horizontalAdvance(title);
                const QRect name(badge.right() + 11, card.top(), titleWidth, card.height());
                painter->drawText(name, Qt::AlignVCenter | Qt::AlignLeft, title);
                QFont small = option.font;
                small.setPixelSize(12);
                painter->setFont(small);
                painter->setPen(QColor(T3));
                const QRect note(name.right() + 8, card.top(), textRight - name.right() - 8, card.height());
                painter->drawText(note, Qt::AlignVCenter | Qt::AlignLeft,
                                  QFontMetrics(small).elidedText(ServerTreeModel::tr("the fastest, switches by itself on failure"), Qt::ElideRight, note.width()));
                return;
            }

            const auto &server = m_model->serverOf(index);
            if (server.country.isEmpty()) {
                PorosTheme::Icon(QStringLiteral("globe"), QColor(T3)).paint(painter, badge.adjusted(8, 4, -8, -4));
            } else {
                painter->setFont(MonoFont(11));
                painter->setPen(QColor(T1));
                painter->drawText(badge, Qt::AlignCenter, server.country.left(2).toUpper());
            }

            if (server.latency != 0) {
                const QString ping = server.latency > 0 ? ServerTreeModel::tr("%1 ms").arg(server.latency) : ServerTreeModel::tr("no response");
                painter->setFont(MonoFont(12));
                painter->setPen(serverListPingColor(server.latency));
                const int pingWidth = painter->fontMetrics().horizontalAdvance(ping);
                painter->drawText(QRect(textRight - pingWidth, card.top(), pingWidth, card.height()), Qt::AlignVCenter | Qt::AlignRight, ping);
                textRight -= pingWidth + 10;
            }
            const QRect name(badge.right() + 11, card.top(), textRight - badge.right() - 11, card.height());
            painter->setFont(option.font);
            painter->setPen(QColor(T1));
            painter->drawText(name, Qt::AlignVCenter | Qt::AlignLeft, option.fontMetrics.elidedText(server.name, Qt::ElideRight, name.width()));
        }

        static void paintAddRow(QPainter *painter, const QStyleOptionViewItem &option) {
            using namespace PorosTheme;
            QRect card = serverListCard(option.rect);
            card.setHeight(serverListRowHeight);
            const bool hover = option.state & QStyle::State_MouseOver;
            QPen pen(QColor(hover ? Blue : Line2), 1, Qt::DashLine);
            painter->setPen(pen);
            painter->setBrush(Qt::NoBrush);
            painter->drawRoundedRect(QRectF(card).adjusted(0.5, 0.5, -0.5, -0.5), 12, 12);
            const QString text = ServerTreeModel::tr("Add another subscription");
            painter->setFont(option.font);
            const int width = option.fontMetrics.horizontalAdvance(text) + 24;
            const int left = card.center().x() - width / 2;
            PorosTheme::Icon(QStringLiteral("plus")).paint(painter, left, card.center().y() - 8, 16, 16);
            painter->setPen(QColor(hover ? T1 : T2));
            painter->drawText(QRect(left + 24, card.top(), width, card.height()), Qt::AlignVCenter | Qt::AlignLeft, text);
        }

        const ServerTreeModel *m_model;
        const QSortFilterProxyModel *m_filter;
    };
} // namespace

ServerListView::ServerListView(ServerTreeModel *model, QWidget *parent) : QTreeView(parent), m_servers(model) {
    m_filter = new QSortFilterProxyModel(this);
    m_filter->setSourceModel(model);
    m_filter->setRecursiveFilteringEnabled(true);
    m_filter->setFilterCaseSensitivity(Qt::CaseInsensitive);
    setModel(m_filter);
    setItemDelegate(new ServerDelegate(model, m_filter, this));

    setObjectName(QStringLiteral("serverList"));
    setHeaderHidden(true);
    setRootIsDecorated(false);
    setIndentation(0);
    setItemsExpandable(false);
    setExpandsOnDoubleClick(false);
    setSelectionMode(QAbstractItemView::NoSelection);
    setFocusPolicy(Qt::NoFocus);
    setMouseTracking(true);
    setFrameShape(QFrame::NoFrame);
    setVerticalScrollMode(QAbstractItemView::ScrollPerPixel);
    verticalScrollBar();

    connect(model, &QAbstractItemModel::modelReset, this, &ServerListView::restoreExpansion);
}

void ServerListView::setFilterText(const QString &text) {
    m_filter->setFilterFixedString(text.trimmed());
    if (text.trimmed().isEmpty()) restoreExpansion();
    else expandAll();
}

void ServerListView::restoreExpansion() {
    for (int row = 0; row < m_filter->rowCount(); row++) {
        const auto viewIndex = m_filter->index(row, 0);
        setExpanded(viewIndex, !m_closedGroups.contains(viewIndex.data(ServerTreeModel::GroupIdRole).toInt()));
    }
}

void ServerListView::mouseReleaseEvent(QMouseEvent *event) {
    QTreeView::mouseReleaseEvent(event);
    if (event->button() != Qt::LeftButton) return;
    const auto viewIndex = indexAt(event->position().toPoint());
    if (!viewIndex.isValid()) return;
    const auto index = m_filter->mapToSource(viewIndex);
    const QRect row = visualRect(viewIndex);
    const QPoint pos = event->position().toPoint();
    switch (m_servers->kindOf(index)) {
        case Kind::Group: {
            const int gid = viewIndex.data(ServerTreeModel::GroupIdRole).toInt();
            if (serverListRefreshButton(row).contains(pos)) {
                emit refreshRequested(gid);
            } else if (serverListMoreButton(row).contains(pos)) {
                emit groupMenuRequested(gid, viewport()->mapToGlobal(pos));
            } else {
                const bool open = !isExpanded(viewIndex);
                setExpanded(viewIndex, open);
                if (open) m_closedGroups.remove(gid);
                else m_closedGroups.insert(gid);
            }
            break;
        }
        case Kind::Auto: emit autoPicked(viewIndex.data(ServerTreeModel::GroupIdRole).toInt()); break;
        case Kind::Server: emit serverPicked(viewIndex.data(ServerTreeModel::ProfileIdRole).toInt()); break;
        case Kind::AddRow: emit addRequested(); break;
    }
}
