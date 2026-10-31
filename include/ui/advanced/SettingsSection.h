#pragma once

#include <QList>
#include <QWidget>

class EmbeddedDialog;
class QAbstractButton;
class QDialog;
class QStackedWidget;
class QVBoxLayout;

// The advanced Settings section: one list on the left, the settings dialogs behind it, their tabs turned into list entries.
class SettingsSection : public QWidget {
    Q_OBJECT

public:
    explicit SettingsSection(QWidget *parent = nullptr);

    // A dialog with tabs: entries maps each tab shown to its list entry, in tab order.
    struct Entry {
        QString icon;
        QString title;
        int tab = -1;
    };
    void addDialog(EmbeddedDialog *dialog, const QList<Entry> &entries);

    void showDialog(EmbeddedDialog *dialog, int tab = 0);

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
