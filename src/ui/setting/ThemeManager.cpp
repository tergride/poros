#include <QStyle>
#include <QApplication>
#include <QFont>
#include <QPalette>
#include <QColor>
#include <QRegularExpression>
#include <QStyleFactory>
#include <QStyleHints>

#include "include/ui/setting/PorosTheme.h"
#include "include/ui/setting/ThemeManager.hpp"

#include <QGlobalStatic>

Q_GLOBAL_STATIC(ThemeManager, themeManagerInstance)

ThemeManager *themeManager() {
    return themeManagerInstance();
}

extern QString ReadFileText(const QString &path);

static QPalette buildThemePalette() {
    using namespace PorosTheme;
    QPalette p;

    const auto setAll = [&](QPalette::ColorRole role, QRgb rgb) {
        const QColor color(rgb);
        p.setColor(QPalette::Active, role, color);
        p.setColor(QPalette::Inactive, role, color);
        p.setColor(QPalette::Disabled, role, color);
    };

    setAll(QPalette::Window,          S1);
    setAll(QPalette::WindowText,      T1);
    setAll(QPalette::Base,            S0);
    setAll(QPalette::AlternateBase,   S2);
    setAll(QPalette::Text,            T1);
    setAll(QPalette::Button,          S3);
    setAll(QPalette::ButtonText,      T1);
    setAll(QPalette::BrightText,      qRgb(255, 255, 255));
    setAll(QPalette::ToolTipBase,     S3);
    setAll(QPalette::ToolTipText,     T1);
    setAll(QPalette::Highlight,       Act);
    setAll(QPalette::HighlightedText, qRgb(255, 255, 255));
    setAll(QPalette::Link,            Blue);
    setAll(QPalette::LinkVisited,     Blue);
    setAll(QPalette::PlaceholderText, T3);
    setAll(QPalette::Accent,          Blue);

    // Frames and bevels the stylesheet doesn't cover fall back to Qt's light defaults otherwise.
    setAll(QPalette::Light,    S5);
    setAll(QPalette::Midlight, S4);
    setAll(QPalette::Mid,      Line2);
    setAll(QPalette::Dark,     Line);
    setAll(QPalette::Shadow,   S0);

    // Must follow setAll(), which wrote the Disabled group too.
    const QColor disabled(TextDisabled);
    p.setColor(QPalette::Disabled, QPalette::WindowText,      disabled);
    p.setColor(QPalette::Disabled, QPalette::Text,            disabled);
    p.setColor(QPalette::Disabled, QPalette::ButtonText,      disabled);
    p.setColor(QPalette::Disabled, QPalette::HighlightedText, disabled);
    p.setColor(QPalette::Disabled, QPalette::Link,            disabled);

    return p;
}

static ThemeTokens themeTokens() {
    using namespace PorosTheme;
    ThemeTokens t;
    t.surface   = QColor(S1);
    t.onSurface = QColor(T1);
    t.accent    = QColor(Blue);
    t.muted     = QColor(T3);
    t.tag       = QColor(Pink);
    t.danger    = QColor(Bad);
    t.success   = QColor(Ok);
    t.info      = QColor(Blue);
    t.chartSeries = {QColor(Blue), QColor(Orchid), QColor(Ok), QColor(Pink), QColor(Violet)};
    t.chartContext = QColor(T3);
    return t;
}

// Literal hex only, so no rule here can resolve against the wrong palette or be served stale
// from QStyleSheetStyle's render-rule cache.
static QString overlayStyleSheet(const ThemeTokens &t) {
    const auto hex = [](const QColor &c) { return c.name(QColor::HexRgb); };
    return QStringLiteral(
        "*[colorRole=\"muted\"] { color: %1; }\n"
        "*[colorRole=\"tag\"] { color: %2; }\n"
        "*[colorRole=\"danger\"] { color: %3; }\n"
        "*[colorRole=\"success\"] { color: %4; }\n"
        "QLineEdit[invalid=\"true\"], QLineEdit[invalid=\"true\"]:focus, QLineEdit[invalid=\"true\"]:hover,\n"
        "QPlainTextEdit[invalid=\"true\"], QTextEdit[invalid=\"true\"],\n"
        "QAbstractSpinBox[invalid=\"true\"], QAbstractSpinBox[invalid=\"true\"]:focus, QAbstractSpinBox[invalid=\"true\"]:hover { color: %3; }\n"
        // A menu-style popup paints its items under the combo's rule, as :active and never :editable or :read-only.
        "QComboBox[invalid=\"true\"]:editable, QComboBox[invalid=\"true\"]:editable:focus, QComboBox[invalid=\"true\"]:editable:hover,\n"
        "QComboBox[invalid=\"true\"]:read-only, QComboBox[invalid=\"true\"]:read-only:focus, QComboBox[invalid=\"true\"]:read-only:hover,\n"
        // Qt polishes a non-editable combo's inactive palette without :read-only.
        "QComboBox[invalid=\"true\"]:!editable:!active { color: %3; }\n"
    ).arg(hex(t.muted), hex(t.tag), hex(t.danger), hex(t.success));
}

static QString themeStyleSheet() {
    QString sheet = ReadFileText(QStringLiteral(":/poros/poros.qss"));
    const auto tokens = PorosTheme::StyleTokens();
    static const QRegularExpression tokenPattern(QStringLiteral("@([a-z0-9]+)"));
    QString resolved;
    resolved.reserve(sheet.size());
    qsizetype position = 0;
    for (auto it = tokenPattern.globalMatch(sheet); it.hasNext();) {
        const auto match = it.next();
        resolved += QStringView(sheet).mid(position, match.capturedStart() - position);
        resolved += tokens.value(match.captured(1), match.captured(0));
        position = match.capturedEnd();
    }
    resolved += QStringView(sheet).mid(position);
    return resolved;
}

void ThemeManager::ApplyTheme() {
    // Asks Windows for dark title bars on the dialogs that keep theirs; must precede the palette, which it resets.
    qApp->styleHints()->setColorScheme(Qt::ColorScheme::Dark);
    qApp->setStyle(QStyleFactory::create(QStringLiteral("Fusion")));
    qApp->setPalette(buildThemePalette());

    tokens = themeTokens();
    const QString sheet = themeStyleSheet() + overlayStyleSheet(tokens);
    qApp->setStyleSheet(sheet);

    // Every setStyle() above - setStyleSheet() runs one itself whenever it installs or drops the
    // proxy - refills Qt's per-class platform font table (QMenu/QAbstractItemView/QMessageBox...),
    // which outranks the app font. Re-asserting the font drops the table; the second sheet call is
    // a plain repolish that re-resolves the widgets the table already stamped (#1829).
    qApp->setFont(qApp->font());
    qApp->setStyleSheet(sheet);

    standardIcons.clear();
    // Loaded with the restyle, so the next dialog's validation marks don't pay for them.
    for (const auto pixmap : {QStyle::SP_MessageBoxCritical, QStyle::SP_MessageBoxWarning}) standardIcon(pixmap);

    emit themeChanged();
}

QIcon ThemeManager::standardIcon(QStyle::StandardPixmap pixmap) {
    auto it = standardIcons.constFind(pixmap);
    if (it == standardIcons.constEnd()) it = standardIcons.insert(pixmap, QApplication::style()->standardIcon(pixmap));
    return *it;
}
