#include "include/ui/advanced/AdvancedPage.h"

#include <QHBoxLayout>
#include <QStackedWidget>
#include <QVBoxLayout>

#include "include/ui/shell/NavRail.h"

AdvancedPage::AdvancedPage(QWidget *parent) : QWidget(parent) {
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(0, 0, 0, 0);
    row->setSpacing(0);

    m_rail = new NavRail(this);
    m_rail->addSection(QStringLiteral("server"), tr("Profiles"));
    m_rail->addSection(QStringLiteral("conn"), tr("Connections"));
    m_rail->addSection(QStringLiteral("route"), tr("Routing"));
    m_rail->addSection(QStringLiteral("log"), tr("Logs"));
    m_rail->addSection(QStringLiteral("chart"), tr("Statistics"));
    m_rail->addSection(QStringLiteral("tool"), tr("Tools"));
    m_rail->addSection(QStringLiteral("gear"), tr("Settings"));
    row->addWidget(m_rail);

    m_main = new QVBoxLayout;
    m_main->setContentsMargins(0, 0, 0, 0);
    m_main->setSpacing(0);
    m_sections = new QStackedWidget(this);
    for (int i = 0; i < SectionCount; i++) m_sections->addWidget(new QWidget(m_sections));
    m_main->addWidget(m_sections, 1);
    row->addLayout(m_main, 1);

    connect(m_rail, &NavRail::currentChanged, this, [this](int index) { showSection(static_cast<Section>(index)); });
    m_rail->setCurrent(Profiles);
}

void AdvancedPage::setConnectionBar(QWidget *bar) {
    bar->setParent(this);
    m_main->insertWidget(0, bar);
}

void AdvancedPage::setSection(Section section, QWidget *widget) {
    auto *placeholder = m_sections->widget(section);
    m_sections->insertWidget(section, widget);
    m_sections->removeWidget(placeholder);
    delete placeholder;
}

void AdvancedPage::showSection(Section section) {
    m_rail->setCurrent(section);
    m_sections->setCurrentIndex(section);
    emit sectionChanged(section);
}

AdvancedPage::Section AdvancedPage::currentSection() const {
    return static_cast<Section>(m_sections->currentIndex());
}
