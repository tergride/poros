#pragma once

#include <QList>
#include <QObject>

class QGridLayout;
class QVBoxLayout;
class QWidget;

// Turns a form of label and field rows into two columns with each label above its field, as in the mockups.
// It stays with the form: a row hidden later closes up, whatever hid it.
class FormColumns : public QObject {
    Q_OBJECT

public:
    // Every grid under root. The grid owns its FormColumns.
    static void adoptAll(QWidget *root);

    explicit FormColumns(QGridLayout *grid);

protected:
    bool eventFilter(QObject *watched, QEvent *event) override;

private:
    struct Cell {
        QVBoxLayout *layout;
        bool wide;
    };

    void reflow();

    QGridLayout *m_grid;
    QList<Cell> m_cells;
    bool m_reflowPending = false;
};
