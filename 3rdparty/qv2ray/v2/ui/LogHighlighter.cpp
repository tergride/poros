#include "LogHighlighter.hpp"

#include "include/ui/setting/PorosTheme.h"

#define TO_EOL "(([\\s\\S]*)|([\\d\\D]*)|([\\w\\W]*))$"
#define REGEX_IPV6_ADDR \
    R"(\[\s*((([0-9A-Fa-f]{1,4}:){7}([0-9A-Fa-f]{1,4}|:))|(([0-9A-Fa-f]{1,4}:){6}(:[0-9A-Fa-f]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){5}(((:[0-9A-Fa-f]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){4}(((:[0-9A-Fa-f]{1,4}){1,3})|((:[0-9A-Fa-f]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){3}(((:[0-9A-Fa-f]{1,4}){1,4})|((:[0-9A-Fa-f]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){2}(((:[0-9A-Fa-f]{1,4}){1,5})|((:[0-9A-Fa-f]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){1}(((:[0-9A-Fa-f]{1,4}){1,6})|((:[0-9A-Fa-f]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9A-Fa-f]{1,4}){1,7})|((:[0-9A-Fa-f]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))(%.+)?\s*\])"
#define REGEX_IPV4_ADDR \
    R"((\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5]))"
#define REGEX_PORT_NUMBER R"(([0-9]|[1-9]\d{1,3}|[1-5]\d{4}|6[0-5]{2}[0-3][0-5])*)"

namespace Qv2ray::ui {
    SyntaxHighlighter::SyntaxHighlighter(bool darkMode, QTextDocument *parent) : QSyntaxHighlighter(parent) {
        HighlightingRule rule;

        if (darkMode) {
            tcpudpFormat.setForeground(QColor(PorosTheme::T1));
            ipHostFormat.setForeground(QColor(PorosTheme::T1));
            warningFormat.setForeground(QColor(PorosTheme::Warn));
            warningFormat2.setForeground(QColor(PorosTheme::Warn));
        } else {
            ipHostFormat.setForeground(QColor(0, 52, 130));
            tcpudpFormat.setForeground(QColor(0, 52, 130));
            warningFormat.setBackground(QColor(255, 160, 15));
            warningFormat.setForeground(Qt::white);
            warningFormat2.setForeground(Qt::darkCyan);
        }
        const static QColor darkGreenColor(10, 180, 0);

        acceptedFormat.setForeground(darkGreenColor);
        acceptedFormat.setFontItalic(true);
        acceptedFormat.setFontWeight(QFont::Bold);
        rule.pattern = QRegularExpression("\\saccepted\\s");
        rule.format = acceptedFormat;
        highlightingRules.append(rule);
        //
        rejectedFormat.setForeground(QColor(PorosTheme::Bad));
        rule.pattern = QRegularExpression("\\srejected\\s" TO_EOL);
        rule.format = rejectedFormat;
        highlightingRules.append(rule);
        //
        dateFormat.setForeground(darkMode ? QColor(PorosTheme::T3) : Qt::darkCyan);
        rule.pattern = QRegularExpression("\\d\\d\\d\\d/\\d\\d/\\d\\d");
        rule.format = dateFormat;
        highlightingRules.append(rule);
        //
        timeFormat.setForeground(darkMode ? QColor(PorosTheme::T3) : Qt::darkCyan);
        rule.pattern = QRegularExpression("\\d\\d:\\d\\d:\\d\\d");
        rule.format = timeFormat;
        highlightingRules.append(rule);
        //
        debugFormat.setForeground(Qt::darkGray);
        rule.pattern = QRegularExpression(R"(\bD[Ee][Bb][Uu][Gg]\b)");
        rule.format = debugFormat;
        highlightingRules.append(rule);
        //
        infoFormat.setForeground(darkMode ? QColor(0x6F, 0xB6, 0xFF) : Qt::darkCyan);
        rule.pattern = QRegularExpression(R"(\bI[Nn][Ff][Oo]\b)");
        rule.format = infoFormat;
        highlightingRules.append(rule);
        //
        rule.pattern = QRegularExpression(R"(\bW[Aa][Rr][Nn]([Ii][Nn][Gg])?\b)");
        rule.format = warningFormat2;
        highlightingRules.append(rule);
        //
        rule.pattern = QRegularExpression(R"(\b[Ee][Rr][Rr][Oo][Rr]\b)");
        rule.format = rejectedFormat;
        highlightingRules.append(rule);

        //
        v2rayComponentFormat.setForeground(darkMode ? QColor(PorosTheme::T2) : Qt::darkYellow);
        rule.pattern = QRegularExpression(R"( (\w+\/)+\w+: )");
        rule.format = v2rayComponentFormat;
        highlightingRules.append(rule);
        //
        rule.pattern = QRegularExpression(">>>>+");
        rule.format = warningFormat;
        highlightingRules.append(rule);
        //
        rule.pattern = QRegularExpression("<<<<+");
        rule.format = warningFormat;
        highlightingRules.append(rule);

        {
            // IP IPv6 Host;
            rule.pattern = QRegularExpression(REGEX_IPV4_ADDR ":" REGEX_PORT_NUMBER);
            rule.pattern.setPatternOptions(QRegularExpression::ExtendedPatternSyntaxOption);
            rule.format = ipHostFormat;
            highlightingRules.append(rule);
            //
            rule.pattern = QRegularExpression(REGEX_IPV6_ADDR ":" REGEX_PORT_NUMBER);
            rule.pattern.setPatternOptions(QRegularExpression::ExtendedPatternSyntaxOption);
            rule.format = ipHostFormat;
            highlightingRules.append(rule);
            //
            rule.pattern = QRegularExpression("([a-zA-Z0-9]([a-zA-Z0-9\\-]{0,61}[a-zA-Z0-9])?\\.)+[a-zA-Z]{2,6}(/|):" REGEX_PORT_NUMBER);
            rule.pattern.setPatternOptions(QRegularExpression::PatternOption::ExtendedPatternSyntaxOption);
            rule.format = ipHostFormat;
            highlightingRules.append(rule);
        }

        for (const auto &pattern: {"tcp:", "udp:"}) {
            rule.pattern = QRegularExpression(pattern);
            rule.format = tcpudpFormat;
            highlightingRules.append(rule);
        }

        searchFormat.setBackground(darkMode ? QColor(0x3B, 0x2F, 0x7A) : QColor(255, 225, 90));
        searchFormat.setForeground(darkMode ? Qt::white : Qt::black);
    }

    void SyntaxHighlighter::setSearchPattern(const QRegularExpression &pattern) {
        searchPattern = pattern;
    }

    void SyntaxHighlighter::highlightBlock(const QString &text) {
        for (const HighlightingRule &rule: highlightingRules) {
            QRegularExpressionMatchIterator matchIterator = rule.pattern.globalMatch(text);

            while (matchIterator.hasNext()) {
                QRegularExpressionMatch match = matchIterator.next();
                setFormat(match.capturedStart(), match.capturedLength(), rule.format);
            }
        }

        // Last, so search hits paint over the syntax colors.
        if (!searchPattern.pattern().isEmpty()) {
            QRegularExpressionMatchIterator matchIterator = searchPattern.globalMatch(text);
            while (matchIterator.hasNext()) {
                QRegularExpressionMatch match = matchIterator.next();
                if (match.capturedLength() > 0) setFormat(match.capturedStart(), match.capturedLength(), searchFormat);
            }
        }

        setCurrentBlockState(0);
    }
} // namespace Qv2ray::ui
