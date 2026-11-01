#include "include/engine/services/BackupArchive.h"

#include <cstring>
#include <exception>
#include <string>

#include <QDataStream>
#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMap>
#include <QSaveFile>
#include <QScopeGuard>
#include <QSysInfo>
#include <QUuid>

#include "NkrVersion.h"
#include "include/configs/sub/GroupUpdater.hpp"
#include "include/engine/ThreadCheck.h"
#include "include/global/Configs.hpp"

// Error texts are English and equal to the settings dialog's tr() sources, which translates them for display.
namespace Engine::BackupArchive {
    namespace {
        constexpr quint32 BACKUP_FORMAT_VERSION = 2;
        constexpr int BACKUP_CONTENT_VERSION = 2;

        Configs::BackupParts BackupPartsFromMeta(const QJsonObject &meta, const QMap<QString, QByteArray> &files) {
            Configs::BackupParts p;
            const QJsonObject po = meta["parts"].toObject();
            p.profiles = po["profiles"].toBool() && files.contains("database");
            p.routes = po["routes"].toBool() && files.contains("database");
            p.settings = po["settings"].toBool() && files.contains("database");
            p.otp = po["otp"].toBool() && files.contains("database");
            return p;
        }

        QString ReadArchive(const QString &path, QJsonObject *meta, QMap<QString, QByteArray> *files) {
            QFile inFile(path);
            if (!inFile.open(QIODevice::ReadOnly)) return QStringLiteral("Cannot open backup file: %1").arg(path);

            QDataStream stream(&inFile);
            stream.setByteOrder(QDataStream::LittleEndian);
            stream.setVersion(QDataStream::Qt_6_0);

            char magic[4];
            if (stream.readRawData(magic, 4) != 4 || strncmp(magic, "THRN", 4) != 0) return QStringLiteral("Not a valid Poros backup file.");

            quint32 formatVersion = 0;
            stream >> formatVersion;
            if (stream.status() != QDataStream::Ok) return QStringLiteral("Not a valid Poros backup file.");
            if (formatVersion != BACKUP_FORMAT_VERSION) {
                return QStringLiteral("Unsupported backup format version: %1.\nThis backup may have been created with a newer version of the application.")
                    .arg(formatVersion);
            }

            QString metaJson;
            stream >> metaJson;
            stream >> *files;
            // A truncated file would otherwise read as a partial or empty map.
            if (stream.status() != QDataStream::Ok) return QStringLiteral("The backup file is damaged or incomplete.");

            const auto doc = QJsonDocument::fromJson(metaJson.toUtf8());
            if (!metaJson.isEmpty() && !doc.isObject()) return QStringLiteral("The backup file is damaged or incomplete.");
            *meta = doc.object();
            return {};
        }

        // SQLite may leave its journal files next to a temp database it had open.
        void RemoveTempDatabase(const QString &path) {
            for (const auto *suffix : {"", "-wal", "-shm", "-journal"}) QFile::remove(path + suffix);
        }

        // The blob is untrusted: SQLite must read it as a sound database before the live one is touched.
        QString CheckDatabase(const QString &path) {
            try {
                SQLite::Database check(path.toStdString(), SQLite::OPEN_READWRITE);
                SQLite::Statement query(check, "PRAGMA quick_check");
                QStringList problems;
                while (problems.size() < 3 && query.executeStep()) problems << QString::fromUtf8(query.getColumn(0).getText());
                if (problems == QStringList{QStringLiteral("ok")}) return {};
                return problems.isEmpty() ? QStringLiteral("the integrity check returned nothing") : problems.join("; ");
            } catch (std::exception &e) {
                return QString::fromUtf8(e.what());
            }
        }

        QString BackupDeviceName(const QString &configured) {
            const QString name = configured.trimmed();
            return name.isEmpty() ? QSysInfo::machineHostName() : name;
        }

    } // namespace

    Device ThisDevice() {
        ENGINE_CHECK_MAIN_THREAD();
        auto &settings = *Configs::dataManager->settingsRepo;
        if (settings.device_id.isEmpty()) {
            settings.device_id = QUuid::createUuid().toString(QUuid::WithoutBraces);
            settings.Save();
        }
        return {settings.device_id, BackupDeviceName(settings.device_name)};
    }

    const QStringList &DeviceLocalSettingKeys() {
        static const QStringList keys = {
            QStringLiteral("device_id"), QStringLiteral("device_name"),
        };
        return keys;
    }

    void Prepare(const Configs::BackupParts &parts) {
        ENGINE_CHECK_MAIN_THREAD();
        ThisDevice();
        if (parts.settings) Configs::dataManager->settingsRepo->Save();
    }

    QString Build(const Configs::BackupParts &parts, const QString &path) {
        if (!parts.any()) return QStringLiteral("Select at least one part to include in the backup.");

        QMap<QString, QByteArray> files;

        {
            const QString tempDbPath = TempPath(QStringLiteral("backup-"));
            const auto removeTemp = qScopeGuard([&tempDbPath] { RemoveTempDatabase(tempDbPath); });

            try {
                Configs::dataManager->getDatabase().backupSelective(tempDbPath.toStdString(), parts, DeviceLocalSettingKeys());
            } catch (std::exception &e) {
                return QStringLiteral("Failed to create database snapshot: %1").arg(e.what());
            }

            QFile tempDbFile(tempDbPath);
            if (!tempDbFile.open(QIODevice::ReadOnly)) return QStringLiteral("Failed to read database snapshot.");
            files["database"] = tempDbFile.readAll();
            if (tempDbFile.error() != QFileDevice::NoError) return QStringLiteral("Failed to read database snapshot.");
        }

        // QSaveFile leaves an existing file at path untouched unless the whole archive was written.
        QSaveFile outFile(path);
        if (!outFile.open(QIODevice::WriteOnly)) return QStringLiteral("Cannot write to: %1").arg(path);

        QDataStream stream(&outFile);
        stream.setByteOrder(QDataStream::LittleEndian);
        stream.setVersion(QDataStream::Qt_6_0);

        if (stream.writeRawData("THRN", 4) != 4) return QStringLiteral("Cannot write to: %1").arg(path);
        stream << BACKUP_FORMAT_VERSION;

        QJsonObject partsObj;
        partsObj["profiles"] = parts.profiles;
        partsObj["routes"] = parts.routes;
        partsObj["settings"] = parts.settings;
        partsObj["otp"] = parts.otp;

        QJsonObject meta;
        meta["backup_version"] = BACKUP_CONTENT_VERSION;
        meta["created_at"] = QDateTime::currentDateTime().toString(Qt::TextDate);
        meta["platform"] = QSysInfo::kernelType();
        meta["parts"] = partsObj;
        meta["app_version"] = QStringLiteral(NKR_VERSION);
        {
            const auto &settings = *Configs::dataManager->settingsRepo;
            QJsonObject device;
            device["id"] = settings.device_id;
            device["name"] = BackupDeviceName(settings.device_name);
            meta["device"] = device;
        }
        stream << QString::fromUtf8(QJsonDocument(meta).toJson(QJsonDocument::Compact));

        stream << files;
        if (stream.status() != QDataStream::Ok || !outFile.commit()) return QStringLiteral("Cannot write to: %1").arg(path);
        return {};
    }

    QString Inspect(const QString &path, Info *info) {
        QJsonObject meta;
        QMap<QString, QByteArray> files;
        if (auto error = ReadArchive(path, &meta, &files); !error.isEmpty()) return error;

        const Configs::BackupParts available = BackupPartsFromMeta(meta, files);
        if (!available.any()) return QStringLiteral("This backup file does not contain any restorable data.");

        if (info != nullptr) {
            info->createdAt = meta["created_at"].toString();
            info->available = available;
            info->deviceName = meta["device"].toObject()["name"].toString();
        }
        return {};
    }

    QString Apply(const QString &path, const Configs::BackupParts &parts, ApplyResult *result) {
        ENGINE_CHECK_MAIN_THREAD();
        if (result != nullptr) *result = {};
        // An update writes through repo caches that the restore leaves stale.
        if (!Subscription::updater()->IsIdle()) return QStringLiteral("A subscription update is running; try again when it finishes.");
        QJsonObject meta;
        QMap<QString, QByteArray> files;
        if (auto error = ReadArchive(path, &meta, &files); !error.isEmpty()) return error;

        const Configs::BackupParts available = BackupPartsFromMeta(meta, files);
        Configs::BackupParts lacking;
        lacking.profiles = parts.profiles && !available.profiles;
        lacking.routes = parts.routes && !available.routes;
        lacking.settings = parts.settings && !available.settings;
        lacking.otp = parts.otp && !available.otp;
        if (lacking.any()) return QStringLiteral("The backup does not contain: %1").arg(PartNames(lacking).join(", "));
        if (!parts.any()) return QStringLiteral("Select at least one part to restore.");

        int skippedRules = 0;
        {
            const QString tempDbPath = TempPath(QStringLiteral("restore-"));
            const auto removeTemp = qScopeGuard([&tempDbPath] { RemoveTempDatabase(tempDbPath); });
            {
                const QByteArray database = files.take("database");
                QFile tempDbFile(tempDbPath);
                if (!tempDbFile.open(QIODevice::WriteOnly) || tempDbFile.write(database) != database.size() || !tempDbFile.flush()) {
                    return QStringLiteral("Failed to create temporary file for restore.");
                }
            }
            if (const auto problem = CheckDatabase(tempDbPath); !problem.isEmpty()) {
                return QStringLiteral("The backup's database is damaged: %1").arg(problem);
            }

            auto &settings = *Configs::dataManager->settingsRepo;
            // Kept keys keep the database's values, so the live ones must reach it first.
            settings.Save();
            auto &db = Configs::dataManager->getDatabase();
            try {
                skippedRules = db.restoreSelective(tempDbPath.toStdString(), parts, DeviceLocalSettingKeys());
            } catch (std::exception &e) {
                return QStringLiteral("Failed to restore database: %1").arg(e.what());
            }
            // The exit path's settingsRepo->Save() would write the stale in-memory values back over the restore.
            settings.noSave = true;
        }

        MW_show_log(QStringLiteral("[Backup] Restored %1 from %2").arg(PartNames(parts).join(", "), QFileInfo(path).fileName()));
        if (result != nullptr) result->skippedRules = skippedRules;
        return {};
    }

    QStringList PartNames(const Configs::BackupParts &parts) {
        QStringList names;
        if (parts.profiles) names << QStringLiteral("profiles");
        if (parts.routes) names << QStringLiteral("routes");
        if (parts.settings) names << QStringLiteral("settings");
        if (parts.otp) names << QStringLiteral("otp");
        return names;
    }

    QString TempPath(const QString &prefix) {
        // main.cpp wipes <configDir>/temp (the working directory's temp) at every start.
        const QString dir = QDir::current().absoluteFilePath(QStringLiteral("temp"));
        QDir().mkpath(dir);
        return QDir(dir).filePath(prefix + QUuid::createUuid().toString(QUuid::WithoutBraces));
    }
} // namespace Engine::BackupArchive
