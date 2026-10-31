#pragma once

#include <QString>
#include <QStringList>

// The built-in presets: routing profiles the interface only reads and copies.
namespace Configs::PresetCatalog {
    // In the order the interface lists them.
    QStringList Keys();

    // In the current language.
    QString DisplayName(const QString &key);
    QString Description(const QString &key);

    // Writes the presets over whatever the database holds under their keys, so an old backup or version cannot leave
    // a stale one behind. Idempotent; runs again once the translator is loaded, which is when the names can follow it.
    void Ensure();
} // namespace Configs::PresetCatalog
