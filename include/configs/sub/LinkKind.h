#pragma once

#include <QString>

namespace Subscription {
    enum class LinkKind {
        Empty,
        Subscription,
        SingleServer,
        Unknown,
    };

    // An http(s) address may be a subscription; any other scheme names one server, which only the advanced mode keeps.
    inline LinkKind ClassifyLink(const QString &text) {
        const QString link = text.trimmed();
        if (link.isEmpty()) return LinkKind::Empty;
        if (link.startsWith(QStringLiteral("http://"), Qt::CaseInsensitive) || link.startsWith(QStringLiteral("https://"), Qt::CaseInsensitive)) {
            return link.contains(u' ') ? LinkKind::Unknown : LinkKind::Subscription;
        }
        const auto scheme = link.indexOf(QStringLiteral("://"));
        return scheme > 0 && !link.left(scheme).contains(u' ') ? LinkKind::SingleServer : LinkKind::Unknown;
    }
} // namespace Subscription
