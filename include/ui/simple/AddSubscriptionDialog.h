#pragma once

#include <QDialog>

class QLabel;
class QLineEdit;
class QPushButton;

// Adds a subscription and waits for its first download, so a dead link is reported here and leaves no empty group behind.
class AddSubscriptionDialog : public QDialog {
    Q_OBJECT

public:
    explicit AddSubscriptionDialog(const QString &link, QWidget *parent = nullptr);

    int groupId() const { return m_groupId; }

    void accept() override;

signals:
    void advancedModeRequested();

private:
    void syncLink();
    // Plain text unless asked otherwise: an error may quote the server.
    void setStatus(const QString &text, bool problem, Qt::TextFormat format = Qt::PlainText);
    // The check adds the group as it goes, so a cancel then would leave it behind.
    void reject() override;

    QLineEdit *m_link;
    QLineEdit *m_name;
    QLabel *m_status;
    QPushButton *m_add;
    QPushButton *m_cancel;
    int m_groupId = -1;
    bool m_busy = false;
};
