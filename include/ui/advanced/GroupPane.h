#pragma once

#include <QList>
#include <QStyledItemDelegate>
#include <QWidget>

class QListWidget;

// A row of the side panes: icon, name and, when CountRole holds one, a number at the right edge.
class PaneRowDelegate : public QStyledItemDelegate {
public:
    static constexpr int CountRole = Qt::UserRole + 1;

    using QStyledItemDelegate::QStyledItemDelegate;

    QSize sizeHint(const QStyleOptionViewItem &option, const QModelIndex &index) const override;
    void paint(QPainter *painter, const QStyleOptionViewItem &option, const QModelIndex &index) const override;
};

// The list of subscriptions and groups beside the profile table.
class GroupPane : public QWidget {
    Q_OBJECT

public:
    struct Entry {
        int id = -1;
        QString name;
        int profiles = 0;
        bool subscription = false;
        QString toolTip;
    };

    explicit GroupPane(QWidget *parent = nullptr);

    // Keeps the scroll position and selects currentId; sends no signal.
    void setGroups(const QList<Entry> &groups, int currentId);
    void setCurrent(int id);

signals:
    void groupActivated(int id);
    void orderChanged(const QList<int> &ids);
    void menuRequested(int id, const QPoint &globalPos);
    void addRequested();

private:
    QListWidget *m_list;
    bool m_updating = false;
};
