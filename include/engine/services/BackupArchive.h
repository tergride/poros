#pragma once

#include <QList>
#include <QString>
#include <QStringList>

#include "include/database/Database.h"

namespace Engine::BackupArchive {
    struct Info {
        // As the archive wrote it: local time, Qt::TextDate.
        QString createdAt;
        Configs::BackupParts available;
        QString deviceName;
    };

    struct Device {
        QString id;
        QString name;
    };

    // Main thread. This device's identity in backups; the id is generated and saved on first use.
    Device ThisDevice();

    // Settings (DB key names) that belong to this device: Build never ships them and Apply always keeps them.
    const QStringList &DeviceLocalSettingKeys();

    // Main thread, before Build: makes sure the device has an id and flushes the settings when they are part of the backup.
    void Prepare(const Configs::BackupParts &parts);

    // Any thread. Writes a .thrbackup archive of the parts to path. Returns an error text, empty on success.
    QString Build(const Configs::BackupParts &parts, const QString &path);

    // Any thread. Reads and checks the archive at path. Returns an error text, empty on success.
    QString Inspect(const QString &path, Info *info);

    struct ApplyResult {
        int skippedRules = 0;
    };

    // Main thread, with no profile running. Replaces the chosen parts with the archive's; a settings restore keeps
    // DeviceLocalSettingKeys(). Ids that no longer resolve are reset. The program must restart afterwards: settings are no
    // longer saved. Returns an error text, empty on success.
    QString Apply(const QString &path, const Configs::BackupParts &parts, ApplyResult *result = nullptr);

    QStringList PartNames(const Configs::BackupParts &parts);

    // A unique path under <configDir>/temp, which Throne wipes at start.
    QString TempPath(const QString &prefix);
} // namespace Engine::BackupArchive
