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
    // The rows the selection panel acts on, or 0 to hide it: a plain click selects a row without opening the panel.
    void bulkRowsChanged(int rows);

public slots:
    void reset() override;

protected:
    void dragEnterEvent(QDragEnterEvent *event) override;
    void dragMoveEvent(QDragMoveEvent *event) override;
    void dropEvent(QDropEvent *event) override;
    void keyPressEvent(QKeyEvent *event) override;
    void leaveEvent(QEvent *event) override;
    void mouseDoubleClickEvent(QMouseEvent *event) override;
    void mouseMoveEvent(QMouseEvent *event) override;
    void mousePressEvent(QMouseEvent *event) override;
    void mouseReleaseEvent(QMouseEvent *event) override;
    void selectionChanged(const QItemSelection &selected, const QItemSelection &deselected) override;

private:
    bool tickAt(const QMouseEvent *event);
    void notifyBulkRows();

    class ProfilesFilterProxyModel *m_filterProxy = nullptr;
    int m_hoveredRow = -1;
    bool m_ticked = false;
    bool m_tickPressed = false;
};
