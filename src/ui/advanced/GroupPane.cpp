#include "include/ui/advanced/GroupPane.h"

#include <QAbstractItemModel>
#include <QHBoxLayout>
#include <QLabel>
#include <QListWidget>
#include <QPainter>
#include <QScrollBar>
#include <QToolButton>
#include <QVBoxLayout>

#include "include/ui/setting/PorosTheme.h"

namespace {
    constexpr int paneRowHeight = 38;
    constexpr int groupPaneIdRole = Qt::UserRole + 2;
} // namespace

QSize PaneRowDelegate::sizeHint(const QStyleOptionViewItem &, const QModelIndex &) const {
    return {180, paneRowHeight};
}

void PaneRowDelegate::paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const {
    using namespace PorosTheme;
    painter->save();
    painter->setRenderHint(QPainter::Antialiasing);
    const bool selected = option.state & QStyle::State_Selected;
    const bool hover = option.state & QStyle::State_MouseOver;
    const QRect row = option.rect.adjusted(0, 1, 0, -1);
    if (selected || hover) {
        painter->setPen(Qt::NoPen);
        painter->setBrush(QColor(selected ? S4 : S3));
        painter->drawRoundedRect(row, 8, 8);
    }
    const auto icon = index.data(Qt::DecorationRole).value<QIcon>();
    constexpr int iconSize = 15;
    icon.paint(painter, QRect(row.left() + 9, row.center().y() - iconSize / 2, iconSize, iconSize), Qt::AlignCenter,
               selected ? QIcon::Selected : QIcon::Normal);

    const QVariant countValue = index.data(CountRole);
    int countWidth = 0;
    if (countValue.isValid()) {
        const QString count = QString::number(countValue.toInt());
        painter->setFont(MonoFont(12));
        countWidth = painter->fontMetrics().horizontalAdvance(count);
        painter->setPen(QColor(T3));
        painter->drawText(QRect(row.right() - 9 - countWidth, row.top(), countWidth, row.height()), Qt::AlignVCenter | Qt::AlignRight, count);
    }

    painter->setFont(option.font);
    painter->setPen(QColor(selected ? qRgb(255, 255, 255) : T2));
    const QRect text(row.left() + 33, row.top(), row.width() - 33 - 9 - countWidth - 6, row.height());
    painter->drawText(text, Qt::AlignVCenter | Qt::AlignLeft,
                      option.fontMetrics.elidedText(index.data(Qt::DisplayRole).toString(), Qt::ElideRight, text.width()));
    painter->restore();
}

GroupPane::GroupPane(QWidget *parent) : QWidget(parent) {
    setAttribute(Qt::WA_StyledBackground);
    setFixedWidth(226);
    auto *column = new QVBoxLayout(this);
    column->setContentsMargins(0, 0, 0, 0);
    column->setSpacing(0);

    auto *header = new QHBoxLayout;
    header->setContentsMargins(12, 8, 8, 4);
    auto *title = new QLabel(tr("Groups"), this);
    title->setProperty("role", QStringLiteral("hint"));
    header->addWidget(title);
    header->addStretch();
    auto *add = new QToolButton(this);
    add->setIcon(PorosTheme::Icon(QStringLiteral("plus")));
    add->setIconSize({14, 14});
    add->setToolTip(tr("Add new Group"));
    connect(add, &QToolButton::clicked, this, &GroupPane::addRequested);
    header->addWidget(add);
    column->addLayout(header);

    m_list = new QListWidget(this);
    m_list->setObjectName(QStringLiteral("groupList"));
    m_list->setItemDelegate(new PaneRowDelegate(m_list));
    m_list->setFrameShape(QFrame::NoFrame);
    m_list->setMouseTracking(true);
    m_list->setDragDropMode(QAbstractItemView::InternalMove);
    m_list->setDefaultDropAction(Qt::MoveAction);
    m_list->setContextMenuPolicy(Qt::CustomContextMenu);
    m_list->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    column->addWidget(m_list, 1);

    connect(m_list, &QListWidget::currentItemChanged, this, [this](QListWidgetItem *item) {
        if (!m_updating && item != nullptr) emit groupActivated(item->data(groupPaneIdRole).toInt());
    });
    connect(m_list, &QWidget::customContextMenuRequested, this, [this](const QPoint &pos) {
        const auto *item = m_list->itemAt(pos);
        emit menuRequested(item != nullptr ? item->data(groupPaneIdRole).toInt() : -1, m_list->viewport()->mapToGlobal(pos));
    });
    connect(m_list->model(), &QAbstractItemModel::rowsMoved, this, [this] {
        QList<int> ids;
        for (int i = 0; i < m_list->count(); i++) ids << m_list->item(i)->data(groupPaneIdRole).toInt();
        emit orderChanged(ids);
    });
}

void GroupPane::setGroups(const QList<Entry> &groups, int currentId) {
    m_updating = true;
    const int scroll = m_list->verticalScrollBar()->value();
    m_list->clear();
    for (const auto &group : groups) {
        auto *item = new QListWidgetItem(PorosTheme::Icon(group.subscription ? QStringLiteral("link") : QStringLiteral("folder")), group.name, m_list);
        item->setData(groupPaneIdRole, group.id);
        item->setData(PaneRowDelegate::CountRole, group.profiles);
        item->setToolTip(group.toolTip);
        if (group.id == currentId) m_list->setCurrentItem(item);
    }
    m_list->verticalScrollBar()->setValue(scroll);
    m_updating = false;
}

void GroupPane::setCurrent(int id) {
    m_updating = true;
    for (int i = 0; i < m_list->count(); i++) {
        if (m_list->item(i)->data(groupPaneIdRole).toInt() == id) m_list->setCurrentRow(i);
    }
    m_updating = false;
}
