#pragma once

#include <QTableView>
#include <functional>

class ProfilesTableView : public QTableView {
    Q_OBJECT
public:
    explicit ProfilesTableView(QWidget *parent = nullptr);

    // Drop-to-reorder: (from, to) as source-model rows, not view rows.
    std::function<void(int row1, int row2)> rowsSwapped;

    void setModel(QAbstractItemModel *model) override;

    int firstVisibleRow();

    [[nodiscard]] int hoveredRow() const { return m_hoveredRow; }

signals:
    void focusSearchRequested();

protected:
    void dragEnterEvent(QDragEnterEvent *event) override;
    void dragMoveEvent(QDragMoveEvent *event) override;
    void dropEvent(QDropEvent *event) override;
    void keyPressEvent(QKeyEvent *event) override;
    void leaveEvent(QEvent *event) override;

private:
    class ProfilesFilterProxyModel *m_filterProxy = nullptr;
    int m_hoveredRow = -1;
};
