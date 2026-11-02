#pragma once

#include <QDialog>
#include <QFrame>

#include "include/engine/EngineTypes.h"

class QHBoxLayout;
class QLabel;

// Asks for administrator rights at the moment an operation needs them, and says why.
class AdminPromptDialog : public QDialog {
    Q_OBJECT

public:
    enum class Choice { Cancel, TryWithout, Restart, UseSystem };

    explicit AdminPromptDialog(Engine::PrivilegeReason reason, QWidget *parent = nullptr);

    Choice choice() const { return m_choice; }

private:
    Choice m_choice = Choice::Cancel;
};

// A problem that needs a decision but no modal box: the window stays usable behind it.
class FailureBanner : public QFrame {
    Q_OBJECT

public:
    explicit FailureBanner(QWidget *parent = nullptr);

    struct Action {
        QString text;
        std::function<void()> run;
    };

    // A standing message (an update to install) comes back once a later one goes, until it is dismissed itself.
    void present(const QString &text, const QList<Action> &actions, bool standing = false);
    void dismiss();
    [[nodiscard]] QString text() const;
    // Whether the message on show is the standing one.
    [[nodiscard]] bool standing() const;

private:
    void display(const QString &text, const QList<Action> &actions);

    QLabel *m_text;
    QHBoxLayout *m_actions;
    QString m_standingText;
    QList<Action> m_standingActions;
};
