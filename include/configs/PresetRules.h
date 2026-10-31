#pragma once

#include <QList>
#include <QString>
#include <QStringList>

namespace Configs {
    class RouteProfile;
}

// A preset copy as the simple mode edits it: the user's own sites and programs over the rules the preset came with.
namespace Configs::PresetRules {
    enum class Action { Proxy, Direct, Block };

    struct UserRule {
        QString text;
        Action action = Action::Proxy;
        bool program = false;

        bool operator==(const UserRule &) const = default;
    };

    // The simple-rule line for what the user typed: a site, an IP or a subnet, or a program.
    QString SiteLine(const QString &text);
    QString ProgramLine(const QString &text);

    QList<UserRule> Read(RouteProfile &profile);

    // Replaces the user's rules and puts them ahead of the preset's own: the first match wins, and a copy exists to override
    // its preset. Returns what could not be stored, one reason per line.
    QString Write(RouteProfile &profile, const QList<UserRule> &rules);

    // The names of the rules a copy keeps from its preset, shown locked.
    QStringList LockedRules(const RouteProfile &profile);
} // namespace Configs::PresetRules
