#pragma once

#include <QFrame>
#include <QList>
#include <QPair>

class QComboBox;

// The strip over every advanced section: Start, what runs, speed, routing profile, Tun and System.
class ConnectionBar : public QFrame {
    Q_OBJECT

public:
    struct Parts {
        QWidget *start;
        QWidget *status;
        QWidget *inbound;
        QWidget *panel;
        QWidget *speed;
        QWidget *tun;
        QWidget *systemProxy;
    };

    // Takes the window's existing widgets, so the code that updates them does not change.
    explicit ConnectionBar(const Parts &parts, QWidget *parent = nullptr);

    // Pairs of routing profile id and name.
    void setRoutes(const QList<QPair<int, QString>> &routes, int currentId);

signals:
    void routeChosen(int id);
    void manageRoutesRequested();

private:
    QComboBox *m_routes;
    int m_routeId = -1;
};
