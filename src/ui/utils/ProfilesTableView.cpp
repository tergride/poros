#include "include/ui/utils/ProfilesTableView.h"
#include "include/ui/setting/PorosTheme.h"
#include "include/ui/utils/ProfilesFilterProxyModel.h"
#include "include/ui/utils/ProfilesTableModel.h"
#include <QDragMoveEvent>
#include <QHeaderView>
#include <QKeyEvent>
#include <QMimeData>
#include <QPainter>
#include <QPainterPath>
#include <QStyledItemDelegate>

namespace {
    constexpr int profilesRowHeight = 36;
    constexpr int profilesCellPadding = 10;

    void paintTick(QPainter *painter, const QRect &cell, bool ticked) {
        using namespace PorosTheme;
        constexpr int size = 16;
        const QRectF box(cell.center().x() - size / 2 + 0.5, cell.center().y() - size / 2 + 0.5, size - 1, size - 1);
        painter->setRenderHint(QPainter::Antialiasing);
        painter->setPen(QPen(QColor(ticked ? Act : Line2), 1.5));
        painter->setBrush(QColor(ticked ? Act : S0));
        painter->drawRoundedRect(box, 5, 5);
        if (!ticked) return;
        QPainterPath mark(box.topLeft() + QPointF(4, 7.5));
        mark.lineTo(box.topLeft() + QPointF(6.5, 10));
        mark.lineTo(box.topLeft() + QPointF(11, 5));
        painter->setPen(QPen(Qt::white, 2, Qt::SolidLine, Qt::RoundCap, Qt::RoundJoin));
        painter->setBrush(Qt::NoBrush);
        painter->drawPath(mark);
    }

    // Hover, selection and the running mark belong to the whole row, which the stylesheet's per-cell states cannot draw.
    class ProfilesRowDelegate : public QStyledItemDelegate {
    public:
        using QStyledItemDelegate::QStyledItemDelegate;

        void paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const override {
            using namespace PorosTheme;
            painter->save();
            const bool running = index.data(ProfilesTableModel::RunningRole).toBool();
            const auto *view = qobject_cast<const ProfilesTableView *>(option.widget);
            if (running) painter->fillRect(option.rect, QColor(S4));
            else if (option.state & QStyle::State_Selected) painter->fillRect(option.rect, QColor(S3));
            else if (view != nullptr && view->hoveredRow() == index.row()) painter->fillRect(option.rect, QColor(S2));
            painter->setPen(QColor(Line));
            painter->drawLine(option.rect.bottomLeft(), option.rect.bottomRight());

            if (index.column() == ProfilesTableModel::ColSelect) {
                paintTick(painter, option.rect, option.state & QStyle::State_Selected);
                painter->restore();
                return;
            }

            QRect text = option.rect.adjusted(profilesCellPadding, 0, -profilesCellPadding, 0);
            if (index.column() == ProfilesTableModel::ColName) {
                if (running) {
                    constexpr int playSize = 12;
                    PorosTheme::Icon(QStringLiteral("play")).paint(painter, QRect(text.left(), text.center().y() - playSize / 2, playSize, playSize));
                    text.setLeft(text.left() + playSize + 6);
                }
                if (const QString country = index.data(ProfilesTableModel::CountryRole).toString(); !country.isEmpty()) {
                    const QRect chip(text.left(), text.center().y() - 9, 26, 18);
                    painter->setRenderHint(QPainter::Antialiasing);
                    painter->setPen(Qt::NoPen);
                    painter->setBrush(QColor(S4));
                    painter->drawRoundedRect(chip, 5, 5);
                    painter->setFont(MonoFont(11));
                    painter->setPen(QColor(T1));
                    painter->drawText(chip, Qt::AlignCenter, country.left(2).toUpper());
                    text.setLeft(chip.right() + 7);
                }
            }

            const auto alignment = Qt::Alignment(index.data(Qt::TextAlignmentRole).toInt());
            const QVariant font = index.data(Qt::FontRole);
            painter->setFont(font.isValid() ? font.value<QFont>() : option.font);
            const QString display = index.data(Qt::DisplayRole).toString();
            if (display.isEmpty()) {
                if (index.column() != ProfilesTableModel::ColName) {
                    painter->setPen(QColor(T3));
                    painter->drawText(text, alignment, QStringLiteral("—"));
                }
            } else {
                const QVariant color = index.data(Qt::ForegroundRole);
                painter->setPen(color.isValid() ? color.value<QColor>() : QColor(T1));
                const QString shown = painter->fontMetrics().elidedText(display, Qt::ElideRight, text.width());
                const QString security = index.column() == ProfilesTableModel::ColType ? index.data(ProfilesTableModel::SecurityRole).toString() : QString();
                if (!security.isEmpty() && shown == display) {
                    const QString type = display.left(display.size() - security.size());
                    painter->drawText(text, alignment, type);
                    painter->setPen(QColor(T3));
                    painter->drawText(text.adjusted(painter->fontMetrics().horizontalAdvance(type), 0, 0, 0), alignment, security);
                } else {
                    painter->drawText(text, alignment, shown);
                }
            }
            painter->restore();
        }

        QSize sizeHint(const QStyleOptionViewItem &option, const QModelIndex &index) const override {
            QSize size = QStyledItemDelegate::sizeHint(option, index);
            size.rwidth() += 2 * profilesCellPadding;
            return size;
        }
    };
} // namespace

ProfilesTableView::ProfilesTableView(QWidget *parent)
    : QTableView(parent) {
    setDragDropMode(InternalMove);
    setDropIndicatorShown(true);
    setSelectionBehavior(SelectRows);
    setDragEnabled(true);
    setAcceptDrops(true);
    setDefaultDropAction(Qt::MoveAction);

    setItemDelegate(new ProfilesRowDelegate(this));
    setShowGrid(false);
    setMouseTracking(true);
    connect(this, &QAbstractItemView::entered, this, [this](const QModelIndex &index) {
        if (m_hoveredRow == index.row()) return;
        m_hoveredRow = index.row();
        viewport()->update();
    });

    verticalHeader()->hide();
    verticalHeader()->setDefaultSectionSize(profilesRowHeight);
    verticalHeader()->setSectionResizeMode(QHeaderView::Fixed);
    horizontalHeader()->setSectionsClickable(true);
    horizontalHeader()->setSortIndicatorShown(true);
    horizontalHeader()->setDefaultAlignment(Qt::AlignLeft | Qt::AlignVCenter);
}

void ProfilesTableView::setModel(QAbstractItemModel *model) {
    QTableView::setModel(model);
    m_filterProxy = qobject_cast<ProfilesFilterProxyModel*>(model);
}

void ProfilesTableView::leaveEvent(QEvent *event) {
    QTableView::leaveEvent(event);
    m_hoveredRow = -1;
    viewport()->update();
}

void ProfilesTableView::reset() {
    QTableView::reset();
    notifyBulkRows();
}

void ProfilesTableView::notifyBulkRows() {
    if (selectionModel() == nullptr) return;
    const auto rows = selectionModel()->selectedRows().size();
    emit bulkRowsChanged(rows >= 2 || (rows == 1 && m_ticked) ? int(rows) : 0);
}

void ProfilesTableView::selectionChanged(const QItemSelection &selected, const QItemSelection &deselected) {
    QTableView::selectionChanged(selected, deselected);
    notifyBulkRows();
}

// A tick adds to the selection or takes from it, whatever the other rows hold.
bool ProfilesTableView::tickAt(const QMouseEvent *event) {
    if (event->button() != Qt::LeftButton) return false;
    const QModelIndex index = indexAt(event->position().toPoint());
    if (!index.isValid() || index.column() != ProfilesTableModel::ColSelect) return false;
    m_ticked = true;
    m_tickPressed = true;
    selectionModel()->setCurrentIndex(index, QItemSelectionModel::NoUpdate);
    selectionModel()->select(index, QItemSelectionModel::Toggle | QItemSelectionModel::Rows);
    return true;
}

void ProfilesTableView::mousePressEvent(QMouseEvent *event) {
    if (tickAt(event)) return;
    if (event->button() == Qt::LeftButton && event->modifiers() == Qt::NoModifier) m_ticked = false;
    QTableView::mousePressEvent(event);
    notifyBulkRows();
}

void ProfilesTableView::mouseDoubleClickEvent(QMouseEvent *event) {
    if (!tickAt(event)) QTableView::mouseDoubleClickEvent(event);
}

void ProfilesTableView::mouseMoveEvent(QMouseEvent *event) {
    if (!m_tickPressed) QTableView::mouseMoveEvent(event);
}

void ProfilesTableView::mouseReleaseEvent(QMouseEvent *event) {
    if (m_tickPressed) {
        m_tickPressed = false;
        return;
    }
    QTableView::mouseReleaseEvent(event);
}

int ProfilesTableView::firstVisibleRow() {
    QRect rect = this->viewport()->rect();

    QModelIndex topIndex = indexAt(rect.topLeft());

    if (!topIndex.isValid()) return 0;

    int startRow = topIndex.row();
    return startRow;
}


void ProfilesTableView::keyPressEvent(QKeyEvent *event) {
    if (event->key() == Qt::Key_Escape && selectionModel() && selectionModel()->hasSelection()) {
        m_ticked = false;
        clearSelection();
        selectionModel()->clearCurrentIndex();
        event->accept();
        return;
    }
    if (event->key() == Qt::Key_Up && currentIndex().isValid() && currentIndex().row() == 0) {
        emit focusSearchRequested();
        event->accept();
        return;
    }
    QTableView::keyPressEvent(event);
}

void ProfilesTableView::dragEnterEvent(QDragEnterEvent *event) {
    if (event->mimeData()->hasFormat("application/profile-row-number")) {

        event->accept();
        QTableView::dragEnterEvent(event);
    } else {
        event->ignore();
    }
}

void ProfilesTableView::dragMoveEvent(QDragMoveEvent *event) {
    if (event->mimeData()->hasFormat("application/profile-row-number")) {
        QPoint pos = event->position().toPoint();
        QModelIndex targetIndex = indexAt(pos);
        if (!targetIndex.isValid()) {
            QModelIndex lastRowIndex = model()->index(model()->rowCount() - 1, 0);
            QRect rect = visualRect(lastRowIndex);
            if (event->pos().y() > rect.bottom()) {
                QPoint fakePos(rect.center().x(), rect.bottom() - 5);

                QDragMoveEvent fakeEvent(
                    fakePos,
                    event->possibleActions(),
                    event->mimeData(),
                    event->mouseButtons(),
                    event->keyboardModifiers()
                );
                QTableView::dragMoveEvent(&fakeEvent);
                event->accept();
                return;
            }
        }
        event->accept();
        QTableView::dragMoveEvent(event);
    } else {
        event->ignore();
    }
}

void ProfilesTableView::dropEvent(QDropEvent *event) {
    if (event->source() == this && event->mimeData()->hasFormat("application/profile-row-number")) {
        QByteArray encodedData = event->mimeData()->data("application/profile-row-number");
        QDataStream stream(&encodedData, QIODevice::ReadOnly);
        // The proxy maps drag indices to the source, so this is a source row.
        int rowNum;
        stream >> rowNum;

        QPoint pos = event->position().toPoint();
        QModelIndex targetIndex = indexAt(pos);

        int newRow;
        if (!targetIndex.isValid()) {
            newRow = model()->rowCount() - 1;
        } else {
            DropIndicatorPosition indicatorPos = dropIndicatorPosition();
            newRow = targetIndex.row();
            if (indicatorPos == AboveItem) {
                newRow--;
            }
        }
        if (m_filterProxy && newRow >= 0) {
            newRow = m_filterProxy->toSourceRow(newRow);
            if (newRow < 0) return;
        }
        rowsSwapped(rowNum, newRow);
        event->accept();
    } else {
        QTableView::dropEvent(event);
    }
}