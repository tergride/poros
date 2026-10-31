#pragma once

#include <QObject>
#include <QString>

namespace Engine {
    class ConnectionManager;
}

// The simple mode's one choice of TUN or System, over the engine's two independent switches.
class ConnectionModeController : public QObject {
    Q_OBJECT

public:
    enum class Mode { Unset, Tun, System };

    // What the simple mode shows: the saved choice, else the only switch that is on.
    static Mode Resolve(const QString &saved, bool tunOn, bool systemOn);

    explicit ConnectionModeController(Engine::ConnectionManager *connection, QObject *parent = nullptr);

    Mode mode() const;

    // Saves it, and applies it at once only while connected: choosing is no reason to ask for rights.
    void choose(Mode mode);

    // Brings the engine in line before a start. False when it could not, as when the user refused the rights TUN needs.
    bool apply();

    // Coming from the advanced mode, where both switches may be on or off.
    void adopt();

signals:
    void modeChanged();

private:
    void save(Mode mode);

    Engine::ConnectionManager *m_connection;
};
