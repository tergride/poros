#include "include/sys/ServiceManager.hpp"

#include "include/global/Logger.hpp"

#include <QByteArray>
#include <QStringList>

namespace Sys::ServiceManager {
    namespace {
        constexpr const char *kServiceManagerManagerVars[] = {
            "NOTIFY_SOCKET", "WATCHDOG_USEC", "WATCHDOG_PID", "LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES", "CREDENTIALS_DIRECTORY",
        };

        // The only THRONE_* variables the core reads (core/main.go, core/internal/guard).
        constexpr const char *kServiceManagerCoreVars[] = {"THRONE_CORE_SOCKET", "THRONE_CORE_DEBUG", "THRONE_GUARD"};

        constexpr Qt::CaseSensitivity kServiceManagerEnvCase = Qt::CaseInsensitive;

        bool serviceManagerCoreVar(const QString &name) {
            for (const char *kept : kServiceManagerCoreVars) {
                if (name.compare(QLatin1String(kept), kServiceManagerEnvCase) == 0) return true;
            }
            return false;
        }
    } // namespace

    QProcessEnvironment ChildEnvironment(QProcessEnvironment env) {
        for (const char *name : kServiceManagerManagerVars) env.remove(QString::fromLatin1(name));
        for (const auto &name : env.keys()) {
            if (name.startsWith(QLatin1String("THRONE_"), kServiceManagerEnvCase) && !serviceManagerCoreVar(name)) env.remove(name);
        }
        return env;
    }

} // namespace Sys::ServiceManager
