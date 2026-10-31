#pragma once

#include <QFrame>
#include <QList>

class QToolButton;

// A row of mutually exclusive choices. It may show none: a choice the user has not made yet stays visibly open.
class SegmentSwitch : public QFrame {
    Q_OBJECT

public:
    explicit SegmentSwitch(const QStringList &items, QWidget *parent = nullptr);

    int current() const { return m_current; }

    // -1 for none. Silent: only a click reports a change.
    void setCurrent(int index);

signals:
    void activated(int index);

private:
    QList<QToolButton *> m_buttons;
    int m_current = -1;
};
