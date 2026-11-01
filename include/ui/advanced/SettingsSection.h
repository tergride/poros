#pragma once

#include <QList>
#include <QWidget>

class EmbeddedDialog;
class QAbstractButton;
class QDialog;
class QStackedWidget;
class QVBoxLayout;

// The advanced Settings section: a grouped list on the left, the settings dialogs behind it, their tabs turned into list entries.
class SettingsSection : public QWidget {
    Q_OBJECT

public:
    explicit SettingsSection(QWidget *parent = nullptr);

    void addDialog(EmbeddedDialog *dialog);

    // tab is the dialog's tab behind the entry, -1 for a dialog without tabs.
    struct Entry {
        QString icon;
        QString title;
        EmbeddedDialog *dialog = nullptr;
        int tab = -1;
    };
    void addGroup(const QString &title, const QList<Entry> &entries);

    void showDialog(EmbeddedDialog *dialog, int tab = 0);

signals:
    void searchRequested(const QString &text);

private:
    struct Target {
        EmbeddedDialog *dialog;
        int tab;
    };

    void show(int index);
    void applyTab(const Target &target);

    QVBoxLayout *m_list;
    QStackedWidget *m_pages;
    QList<QAbstractButton *> m_buttons;
    QList<Target> m_targets;
    int m_current = -1;
};
