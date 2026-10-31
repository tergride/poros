#include "include/ui/advanced/SettingsSection.h"

#include <QDialog>
#include <QHBoxLayout>
#include <QStackedWidget>
#include <QTabBar>
#include <QTabWidget>
#include <QVBoxLayout>

#include "include/ui/advanced/EmbeddedDialog.h"
#include "include/ui/simple/SimpleSettingsNav.h"

SettingsSection::SettingsSection(QWidget *parent) : QWidget(parent) {
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(0, 0, 0, 0);
    row->setSpacing(0);
    auto *nav = new QWidget(this);
    nav->setObjectName(QStringLiteral("settingsNav"));
    nav->setAttribute(Qt::WA_StyledBackground);
    nav->setFixedWidth(212);
    m_list = new QVBoxLayout(nav);
    m_list->setContentsMargins(10, 10, 10, 10);
    m_list->setSpacing(2);
    m_list->addStretch();
    row->addWidget(nav);
    m_pages = new QStackedWidget(this);
    row->addWidget(m_pages, 1);
}

void SettingsSection::addDialog(EmbeddedDialog *dialog, const QList<Entry> &entries) {
    m_pages->addWidget(dialog);
    const bool tabsAreEntries = entries.size() > 1;
    connect(dialog, &EmbeddedDialog::built, this, [this, dialog, tabsAreEntries](QDialog *built) {
        if (auto *tabs = built->findChild<QTabWidget *>(); tabs != nullptr && tabsAreEntries) {
            tabs->tabBar()->hide();
            // Search opens a setting by switching the tab itself.
            connect(tabs, &QTabWidget::currentChanged, this, [this, dialog](int tab) {
                for (int i = 0; i < m_targets.size(); i++) {
                    if (m_targets[i].dialog != dialog || m_targets[i].tab != tab) continue;
                    m_current = i;
                    for (int b = 0; b < m_buttons.size(); b++) m_buttons[b]->setChecked(b == i);
                }
            });
        }
        if (m_current >= 0 && m_targets[m_current].dialog == dialog) applyTab(m_targets[m_current]);
    });
    for (const auto &entry : entries) {
        const int index = m_buttons.size();
        auto *button = new SimpleSettingsNavButton(entry.icon, entry.title, m_list->parentWidget());
        connect(button, &QAbstractButton::clicked, this, [this, index] { show(index); });
        m_list->insertWidget(index, button);
        m_buttons << button;
        m_targets.append({dialog, entry.tab});
    }
    if (m_current < 0) show(0);
}

void SettingsSection::showDialog(EmbeddedDialog *dialog, int tab) {
    for (int i = 0; i < m_targets.size(); i++) {
        if (m_targets[i].dialog == dialog && (tab < 0 || m_targets[i].tab == tab || m_targets[i].tab < 0)) {
            show(i);
            return;
        }
    }
}

void SettingsSection::show(int index) {
    m_current = index;
    for (int i = 0; i < m_buttons.size(); i++) m_buttons[i]->setChecked(i == index);
    m_pages->setCurrentWidget(m_targets[index].dialog);
    applyTab(m_targets[index]);
}

void SettingsSection::applyTab(const Target &target) {
    if (target.tab < 0 || !target.dialog->isVisible()) return;
    if (auto *tabs = target.dialog->dialog()->findChild<QTabWidget *>()) tabs->setCurrentIndex(target.tab);
}
