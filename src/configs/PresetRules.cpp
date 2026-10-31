#include "include/configs/PresetRules.h"

#include <QHostAddress>
#include <QUrl>

#include "include/database/entities/RouteProfile.h"

namespace {
    using Configs::PresetRules::Action;

    Configs::simpleAction presetRulesSimpleAction(Action action) {
        switch (action) {
            case Action::Direct: return Configs::bypass;
            case Action::Block: return Configs::block;
            default: return Configs::proxy;
        }
    }

    bool presetRulesIsUserRule(const Configs::RouteRule &rule) {
        switch (rule.type) {
            case Configs::simpleAddressProxy:
            case Configs::simpleAddressBypass:
            case Configs::simpleAddressBlock:
            case Configs::simpleProcessNameProxy:
            case Configs::simpleProcessNameBypass:
            case Configs::simpleProcessNameBlock:
            case Configs::simpleProcessPathProxy:
            case Configs::simpleProcessPathBypass:
            case Configs::simpleProcessPathBlock:
                return true;
            default:
                return false;
        }
    }

    // DNS and sniffing come first whatever the rules below them say.
    bool presetRulesIsPlumbing(const Configs::RouteRule &rule) {
        return rule.action != QLatin1String("route") && rule.action != QLatin1String("reject") && rule.action != QLatin1String("bypass");
    }
} // namespace

namespace Configs::PresetRules {
    QString SiteLine(const QString &text) {
        QString value = text.trimmed();
        if (value.isEmpty()) return {};
        if (value.contains(u'/') && QHostAddress::parseSubnet(value).second >= 0) return QStringLiteral("ip:") + value;
        if (!QHostAddress(value).isNull()) return QStringLiteral("ip:") + value;
        if (value.contains(QLatin1String("://"))) value = QUrl(value).host();
        else if (const auto slash = value.indexOf(u'/'); slash > 0) value = value.left(slash);
        if (value.startsWith(QLatin1String("*."))) value = value.mid(2);
        return value.isEmpty() ? QString() : QStringLiteral("suffix:") + value.toLower();
    }

    QString ProgramLine(const QString &text) {
        const QString value = text.trimmed();
        if (value.isEmpty()) return {};
        return (value.contains(u'\\') || value.contains(u'/') ? QStringLiteral("processPath:") : QStringLiteral("processName:")) + value;
    }

    QList<UserRule> Read(RouteProfile &profile) {
        QList<UserRule> rules;
        for (const Action action : {Action::Proxy, Action::Direct, Action::Block}) {
            const auto lines = profile.GetSimpleRules(presetRulesSimpleAction(action)).split(u'\n', Qt::SkipEmptyParts);
            for (const auto &line : lines) {
                const auto colon = line.indexOf(u':');
                const QString kind = line.left(colon);
                rules.append({line.mid(colon + 1), action, kind.startsWith(QLatin1String("process"))});
            }
        }
        return rules;
    }

    QString Write(RouteProfile &profile, const QList<UserRule> &rules) {
        QString problems;
        for (const Action action : {Action::Proxy, Action::Direct, Action::Block}) {
            QStringList lines;
            for (const auto &rule : rules) {
                if (rule.action != action) continue;
                const QString line = rule.program ? ProgramLine(rule.text) : SiteLine(rule.text);
                if (!line.isEmpty()) lines << line;
            }
            problems += profile.UpdateSimpleRules(lines.join(u'\n'), presetRulesSimpleAction(action));
        }

        QList<std::shared_ptr<RouteRule>> plumbing;
        QList<std::shared_ptr<RouteRule>> own;
        QList<std::shared_ptr<RouteRule>> base;
        for (const auto &rule : profile.Rules) {
            if (presetRulesIsUserRule(*rule)) own << rule;
            else if (presetRulesIsPlumbing(*rule) && base.isEmpty()) plumbing << rule;
            else base << rule;
        }
        profile.Rules = plumbing + own + base;
        return problems;
    }

    QStringList LockedRules(const RouteProfile &profile) {
        QStringList names;
        for (const auto &rule : profile.Rules) {
            if (!presetRulesIsUserRule(*rule) && !presetRulesIsPlumbing(*rule)) names << rule->name;
        }
        return names;
    }
} // namespace Configs::PresetRules
