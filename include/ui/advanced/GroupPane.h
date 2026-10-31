#pragma once

#include <QList>
#include <QWidget>

class QListWidget;

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
