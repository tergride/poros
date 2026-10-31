#include "include/engine/app/DataDirLock.h"

#include <QCoreApplication>
#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QJsonDocument>
#include <QStringList>

#include <windows.h>
// <windows.h> defines SetPort, which under unity builds clobbers Configs::outbound::SetPort.
#undef SetPort

namespace Engine {
    namespace {
        constexpr qint64 MAX_OWNER_RECORD = 4096;

        QByteArray OwnerRecord(const QString &mode) {
            return QJsonDocument(QJsonObject{
                {"mode", mode},
                {"pid", QCoreApplication::applicationPid()},
                {"started", QDateTime::currentSecsSinceEpoch()},
            }).toJson(QJsonDocument::Compact);
        }

        QJsonObject ParseOwnerRecord(const QByteArray &data) {
            const auto doc = QJsonDocument::fromJson(data.trimmed());
            return doc.isObject() ? doc.object() : QJsonObject{};
        }
    } // namespace

    QString DataDirLock::LockPath(const QString &configDir) {
        return QDir(configDir).absoluteFilePath(".lock");
    }

    DataDirLock::~DataDirLock() {
        release();
    }

    bool DataDirLock::isHeld() const {
        return handle != nullptr;
    }

    DataDirLock::Result DataDirLock::tryAcquire(const QString &configDir, const QString &mode) {
        release();
        error.clear();
        const QString path = LockPath(configDir);
        const QByteArray record = OwnerRecord(mode);
        const QString nativePath = QDir::toNativeSeparators(path);
        // Share-read only: a second owner gets ERROR_SHARING_VIOLATION while ReadOwner can still open it.
        const HANDLE h = CreateFileW(reinterpret_cast<LPCWSTR>(nativePath.utf16()), GENERIC_READ | GENERIC_WRITE,
                                     FILE_SHARE_READ, nullptr, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
        if (h == INVALID_HANDLE_VALUE) {
            const DWORD code = GetLastError();
            if (code == ERROR_SHARING_VIOLATION) return Result::Busy;
            error = QString("could not open %1: Windows error %2").arg(nativePath).arg(code);
            return Result::Failed;
        }
        handle = h;
        DWORD written = 0;
        if (!SetEndOfFile(h) || !WriteFile(h, record.constData(), static_cast<DWORD>(record.size()), &written, nullptr)) {
            error = QString("could not write the owner record to %1: Windows error %2").arg(nativePath).arg(GetLastError());
        }
        return Result::Acquired;
    }

    void DataDirLock::release() {
        if (handle == nullptr) return;
        CloseHandle(static_cast<HANDLE>(handle));
        handle = nullptr;
    }

    QJsonObject DataDirLock::ReadOwner(const QString &configDir) {
        const QString path = LockPath(configDir);
        const QString nativePath = QDir::toNativeSeparators(path);
        // FILE_SHARE_WRITE is required: the owner's handle already holds write access.
        const HANDLE h = CreateFileW(reinterpret_cast<LPCWSTR>(nativePath.utf16()), GENERIC_READ,
                                     FILE_SHARE_READ | FILE_SHARE_WRITE, nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
        if (h == INVALID_HANDLE_VALUE) return {};
        QByteArray data(MAX_OWNER_RECORD, Qt::Uninitialized);
        DWORD read = 0;
        const bool ok = ReadFile(h, data.data(), static_cast<DWORD>(data.size()), &read, nullptr);
        CloseHandle(h);
        if (!ok) return {};
        data.truncate(static_cast<qsizetype>(read));
        return ParseOwnerRecord(data);
    }

    QString DataDirLock::DescribeOwner(const QJsonObject &owner) {
        const QString mode = owner.value("mode").toString();
        const qint64 pid = owner.value("pid").toInteger();
        const qint64 started = owner.value("started").toInteger();
        QStringList details;
        if (pid > 0) details << QString("PID %1").arg(pid);
        if (started > 0) details << "since " + QDateTime::fromSecsSinceEpoch(started).toString("yyyy-MM-dd HH:mm:ss");
        QString text = mode.isEmpty() ? QStringLiteral("another Poros") : mode;
        if (!details.isEmpty()) text += " (" + details.join(", ") + ")";
        return text;
    }
} // namespace Engine
