#pragma once

#include <QList>
#include <QWidget>

class QAbstractButton;
class QVBoxLayout;

// The advanced mode's section list.
class NavRail : public QWidget {
    Q_OBJECT

public:
    explicit NavRail(QWidget *parent = nullptr);

    int addSection(const QString &icon, const QString &text);
    void setCurrent(int index);
    int current() const { return m_current; }

signals:
    void currentChanged(int index);
    void searchRequested();

private:
    QVBoxLayout *m_column;
    QList<QAbstractButton *> m_buttons;
    int m_current = -1;
};
