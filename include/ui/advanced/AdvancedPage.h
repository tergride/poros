#pragma once

#include <QWidget>

class NavRail;
class QStackedWidget;
class QVBoxLayout;

// The advanced mode: section rail on the left, the connection bar over the current section.
class AdvancedPage : public QWidget {
    Q_OBJECT

public:
    enum Section { Profiles, Connections, Routing, Logs, Statistics, Tools, Settings, SectionCount };

    explicit AdvancedPage(QWidget *parent = nullptr);

    NavRail *rail() const { return m_rail; }

    // Above the sections; the page owns it from here on.
    void setConnectionBar(QWidget *bar);
    void setSection(Section section, QWidget *widget);

    void showSection(Section section);
    Section currentSection() const;

signals:
    void sectionChanged(Section section);

private:
    NavRail *m_rail;
    QVBoxLayout *m_main;
    QStackedWidget *m_sections;
};
