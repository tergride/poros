#include "include/engine/app/Bootstrap.h"

#include <cstdio>
#include <cstring>
#include <utility>

#include <QCoreApplication>
#include <QCryptographicHash>
#include <QDateTime>
#include <QDeadlineTimer>
#include <QDirIterator>
#include <QFile>
#include <QFileInfo>
#include <QStandardPaths>
#include <QThread>

#include "include/engine/app/DataDirLock.h"
#include "include/engine/app/InstanceServer.h"
#include "include/global/Configs.hpp"
#include "include/global/DeviceDetailsHelper.hpp"
#include "include/global/Logger.hpp"
#include "include/stats/traffic/TrafficStatsManager.hpp"

#include "include/sys/windows/MiniDump.h"

#include <cstdlib>
#include <string>

#include <windows.h>
// <windows.h> defines SetPort, which under unity builds clobbers Configs::outbound::SetPort.
#undef SetPort

#define LOCAL_SERVER_PREFIX "poros-"

namespace Engine::Bootstrap {
    namespace {

        // QFileInfo::isWritable reports the read-only attribute, not what a UAC-filtered token can actually do.
        bool DirIsWritable(const QDir &dir) {
            if (!dir.exists() && !QDir().mkpath(dir.absolutePath())) return false;
            QFile probe(dir.absoluteFilePath(".poros-write-test"));
            if (!probe.open(QIODevice::WriteOnly)) return false;
            probe.close();
            probe.remove();
            return true;
        }

        bool ConfigDirIsUsable(const QDir &configDir) {
            if (!DirIsWritable(configDir)) return false;
            const QString db = configDir.absoluteFilePath("poros.db");
            if (!QFile::exists(db)) return true;
            QFile file(db);
            return file.open(QIODevice::ReadWrite);
        }

        // The core inherits these, so its temp files and those of the programs it starts stay under <config> as well.
        void bootstrapKeepTempInConfigDir(const QString &configDir) {
            const QString temp = QDir(configDir).absoluteFilePath(QStringLiteral("temp"));
            QDir().mkpath(temp);
            const std::wstring native = QDir::toNativeSeparators(temp).toStdWString();
            _wputenv_s(L"TMP", native.c_str());
            _wputenv_s(L"TEMP", native.c_str());
        }

    } // namespace

    Launch PreScan(int argc, char **argv) {
        Launch launch;
        for (int i = 1; i < argc; i++) {
            if (argv[i] != nullptr && std::strcmp(argv[i], "-dump-configs") == 0) launch.mode = Mode::DumpConfigs;
        }
        return launch;
    }

    void SetSoftwareNames() {
        software_name = "Poros";
        software_core_name = "sing-box";
    }

    DataDir ResolveDataDir(const QStringList &arguments, const QDir &relativeBase) {
        DataDir res;
        res.wd = QDir(QCoreApplication::applicationDirPath());
        if (arguments.contains("-appdata")) {
            res.useAppdata = true;
            const auto appdataIndex = arguments.indexOf("-appdata");
            if (arguments.size() > appdataIndex + 1 && !arguments.at(appdataIndex + 1).startsWith("-")) {
                res.appdataDir = arguments.at(appdataIndex + 1);
            }
        }
        // Portable: the program folder, unless the caller names another one.
        if (res.appdataDir.isEmpty()) res.useAppdata = false;
        QCoreApplication::setApplicationName("Poros");
        if (res.useAppdata) {
            if (!res.appdataDir.isEmpty()) {
                res.wd.setPath(relativeBase.absoluteFilePath(res.appdataDir));
            } else {
                res.wd.setPath(QStandardPaths::writableLocation(QStandardPaths::AppConfigLocation));
            }
        }
        if (!res.wd.exists()) res.wd.mkpath(res.wd.absolutePath());
        if (!res.wd.exists("config")) res.wd.mkdir("config");
        res.configDir = res.wd.absoluteFilePath("config");
        return res;
    }

    bool DataDirIsUsable(const DataDir &dataDir) {
        return ConfigDirIsUsable(QDir(dataDir.configDir));
    }

    QString InstanceServerName(const QDir &wd) {
        QByteArray hashBytes = QCryptographicHash::hash(wd.absolutePath().toUtf8(), QCryptographicHash::Md5).toBase64(QByteArray::OmitTrailingEquals);
        hashBytes.replace('+', '0').replace('/', '1');
        return LOCAL_SERVER_PREFIX + QString::fromUtf8(hashBytes);
    }

    RelaunchParent::RelaunchParent(QStringList &arguments) {
        const QString prefix = "-wait_pid=";
        for (qsizetype i = arguments.size() - 1; i >= 1; i--) {
            if (arguments.at(i).startsWith(prefix)) pid = arguments.takeAt(i).mid(prefix.size()).toLongLong();
        }
        if (pid > 0) {
            handle = OpenProcess(SYNCHRONIZE, FALSE, static_cast<DWORD>(pid));
            if (handle != nullptr) WaitForSingleObject(handle, 30000);
        }
    }

    RelaunchParent::~RelaunchParent() {
        if (handle != nullptr) CloseHandle(handle);
    }

    bool RelaunchParent::gone() const {
        if (pid <= 0) return true;
        return handle == nullptr || WaitForSingleObject(handle, 0) == WAIT_OBJECT_0;
    }

    void WarnDroppedPayload(const QStringList &payload) {
        if (!payload.isEmpty()) LOG_WARN("dropped the launch payload: " + payload.join(' '));
    }

    Claim ClaimDataDir(DataDirLock &lock, const DataDir &dataDir, const QString &serverName, const QStringList &payload,
                       const RelaunchParent &relaunchParent) {
        const QString &configDir = dataDir.configDir;
        // For up to 60 s a restarted instance never hands off: the owner is its exiting parent, which still answers on the socket.
        const QDeadlineTimer relaunchDeadline(relaunchParent.relaunched() ? 60000 : 0);
        QDeadlineTimer handOffDeadline(QDeadlineTimer::Forever);
        while (true) {
            const auto lockResult = lock.tryAcquire(configDir, QStringLiteral("gui"));
            if (lockResult == DataDirLock::Result::Failed) {
                LOG_WARN("running without the data dir lock: " + lock.errorString());
                break;
            }
            if (lockResult == DataDirLock::Result::Acquired) {
                if (!lock.errorString().isEmpty()) LOG_WARN(lock.errorString());
                break;
            }
            if (!relaunchDeadline.hasExpired() && !relaunchParent.gone()) {
                QThread::msleep(100);
                continue;
            }
            if (handOffDeadline.isForever()) {
                if (relaunchParent.relaunched() && !relaunchParent.gone()) {
                    LOG_WARN(QString("the data dir is still held after waiting for the restarting PID %1").arg(relaunchParent.pid));
                }
                handOffDeadline.setRemainingTime(10000);
            }
            // The owner may still be starting up, or exiting and about to release the lock.
            if (InstanceServer::HandOff(serverName, payload)) return {ClaimOutcome::HandedOff};
            if (handOffDeadline.hasExpired()) {
                const auto owner = DataDirLock::ReadOwner(configDir);
                const auto pid = owner.value("pid").toInteger();
                LOG_WARN(QString("the data dir is held by PID %1, which does not answer on %2").arg(QString::number(pid), serverName));
                WarnDroppedPayload(payload);
                return {ClaimOutcome::HeldByOther, pid};
            }
            QThread::msleep(100);
        }
        // An instance without the lock still answers on its socket; a restarted one would only find its exiting parent there.
        if (relaunchParent.gone() && InstanceServer::HandOff(serverName, payload)) {
            return {ClaimOutcome::HandedOff};
        }
        return {};
    }

    void OpenDataDir(const DataDir &dataDir, const QStringList &arguments) {
        const bool useAppdata = dataDir.useAppdata;
        const QString &appdataDir = dataDir.appdataDir;
        const QString &configDir = dataDir.configDir;
        SetDeviceIdStore(configDir);

        QDir::setCurrent(configDir);
        QDir("temp").removeRecursively();
        bootstrapKeepTempInConfigDir(configDir);

        appStartEpoch = QDateTime::currentSecsSinceEpoch();

        Configs::initDB(QString(QDir::currentPath() + QDir::separator() + "poros.db").toStdString());

        Logging::SetLevel(Logging::LevelFromString(Configs::dataManager->settingsRepo->log_file_level));

        Stats::trafficStatsManager->Init();

        Configs::dataManager->settingsRepo->argv = arguments;
        if (Configs::dataManager->settingsRepo->argv.contains("-many")) Configs::dataManager->settingsRepo->flag_many = true;
        if (Configs::dataManager->settingsRepo->argv.contains("-tray")) Configs::dataManager->settingsRepo->flag_tray = true;
        if (Configs::dataManager->settingsRepo->argv.contains("-debug")) Configs::dataManager->settingsRepo->flag_debug = true;
        if (Configs::dataManager->settingsRepo->argv.contains("-flag_restart_tun_on")) Configs::dataManager->settingsRepo->flag_restart_tun_on = true;
        for (const auto &argument : Configs::dataManager->settingsRepo->argv) {
            if (argument.startsWith(QLatin1String("-flag_restart_profile="))) Configs::dataManager->settingsRepo->flag_restart_profile = argument.section('=', 1).toInt();
        }
        Configs::dataManager->settingsRepo->flag_use_appdata = useAppdata;
        if(useAppdata && !appdataDir.isEmpty()) Configs::dataManager->settingsRepo->appdataDir = appdataDir;
#ifdef NKR_CPP_DEBUG
        Configs::dataManager->settingsRepo->flag_debug = true;
#endif
    }

    void PrepareRuntime() {
        DS_cores = new QThread;
        DS_cores->start();
    }

    void StartLogging(const DataDir &dataDir) {
        Logging::Init(dataDir.configDir);
        LOG_INFO(QString("appdata mode: %1").arg(dataDir.useAppdata ? "yes" : "no"));
        Windows_SetCrashDumpPath();
    }

    bool ExitInProgress() {
        return Configs::dataManager != nullptr && Configs::dataManager->settingsRepo->prepare_exit;
    }

} // namespace Engine::Bootstrap
