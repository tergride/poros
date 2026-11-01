#include "include/ui/advanced/ConnectionBar.h"

#include <QComboBox>
#include <QHBoxLayout>
#include <QSignalBlocker>
#include <QVBoxLayout>

#include "include/ui/utils/NoWheelComboBox.h"

ConnectionBar::ConnectionBar(const Parts &parts, QWidget *parent) : QFrame(parent) {
    setFixedHeight(62);
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(14, 0, 14, 0);
    row->setSpacing(14);

    row->addWidget(parts.start);

    auto *who = new QVBoxLayout;
    who->setSpacing(0);
    who->addStretch();
    who->addWidget(parts.status);
    who->addWidget(parts.inbound);
    who->addStretch();
    row->addLayout(who);

    row->addWidget(parts.panel, 1);
    row->addWidget(parts.speed);

    m_routes = new NoWheelComboBox(this);
    m_routes->setFixedWidth(200);
    m_routes->setToolTip(tr("Routing profile"));
    row->addWidget(m_routes);
    row->addWidget(parts.tun);
    row->addWidget(parts.systemProxy);

    connect(m_routes, &QComboBox::activated, this, [this](int index) {
        const int id = m_routes->itemData(index).toInt();
        if (id >= 0) {
            m_routeId = id;
            emit routeChosen(id);
            return;
        }
        for (int i = 0; i < m_routes->count(); i++) {
            if (m_routes->itemData(i).toInt() == m_routeId) m_routes->setCurrentIndex(i);
        }
        emit manageRoutesRequested();
    });
}

void ConnectionBar::setRoutes(const QList<QPair<int, QString>> &routes, int currentId) {
    const QSignalBlocker blocker(m_routes);
    m_routes->clear();
    m_routeId = currentId;
    for (const auto &[id, name] : routes) {
        m_routes->addItem(name, id);
        if (id == currentId) m_routes->setCurrentIndex(m_routes->count() - 1);
    }
    m_routes->addItem(tr("Manage…"), -1);
}
