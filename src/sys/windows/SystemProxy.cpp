#include "include/sys/SystemProxy.hpp"

#include <QDir>
#include <QFile>
#include <QHostAddress>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMutex>
#include <QRegularExpression>
#include <QSaveFile>
#include <QStringList>
#include <QTcpSocket>

#include <map>
#include <string>
#include <vector>

#include <windows.h>
#include <wininet.h>
#include <ras.h>
#include <raserror.h>

namespace {
    QMutex proxyMutex;

    // What a connection had before the first Apply.
    struct SavedConnection {
        DWORD flags = PROXY_TYPE_DIRECT;
        std::wstring server;
    };

    // Connections switched to our proxy ("" is the LAN settings).
    std::map<std::wstring, SavedConnection> proxiedConnections;
    std::wstring appliedProxyServer;

    QString proxyStateFile() {
        return QDir::current().absoluteFilePath(QStringLiteral("system-proxy.json"));
    }

    // On disk as well, so a start after a crash can still give the connections back.
    void saveProxyState() {
        if (proxiedConnections.empty()) {
            QFile::remove(proxyStateFile());
            return;
        }
        QJsonArray connections;
        for (const auto &[name, saved] : proxiedConnections) {
            connections << QJsonObject{{"name", QString::fromStdWString(name)},
                                       {"flags", static_cast<qint64>(saved.flags)},
                                       {"server", QString::fromStdWString(saved.server)}};
        }
        QSaveFile file(proxyStateFile());
        if (!file.open(QIODevice::WriteOnly)) return;
        file.write(QJsonDocument(QJsonObject{{"applied", QString::fromStdWString(appliedProxyServer)}, {"connections", connections}}).toJson());
        file.commit();
    }

    // WinINet keeps separate settings for the LAN and for every dial-up/PPPoE/VPN connectoid, and uses the active one's.
    std::vector<std::wstring> proxyConnectionNames(QStringList &errors) {
        std::vector<std::wstring> names{std::wstring()};
        DWORD bytes = 0;
        DWORD count = 0;
        std::vector<RASENTRYNAMEW> entries;
        DWORD ret = RasEnumEntriesW(nullptr, nullptr, nullptr, &bytes, &count);
        if (ret == ERROR_BUFFER_TOO_SMALL) {
            entries.resize(bytes / sizeof(RASENTRYNAMEW) + 1);
            entries[0].dwSize = sizeof(RASENTRYNAMEW);
            bytes = static_cast<DWORD>(entries.size() * sizeof(RASENTRYNAMEW));
            ret = RasEnumEntriesW(nullptr, nullptr, entries.data(), &bytes, &count);
        }
        if (ret != ERROR_SUCCESS) {
            errors << QString("RasEnumEntries failed (%1), only the LAN settings were changed").arg(ret);
            return names;
        }
        for (DWORD i = 0; i < count; ++i) names.emplace_back(entries[i].szEntryName);
        return names;
    }

    INTERNET_PER_CONN_OPTION_LISTW connectionOptionList(std::wstring &connection, INTERNET_PER_CONN_OPTIONW *options, DWORD count) {
        INTERNET_PER_CONN_OPTION_LISTW list{};
        list.dwSize = sizeof(list);
        list.pszConnection = connection.empty() ? nullptr : connection.data();
        list.dwOptionCount = count;
        list.pOptions = options;
        return list;
    }

    bool setConnectionOptions(std::wstring connection, INTERNET_PER_CONN_OPTIONW *options, DWORD count) {
        auto list = connectionOptionList(connection, options, count);
        return InternetSetOptionW(nullptr, INTERNET_OPTION_PER_CONNECTION_OPTION, &list, sizeof(list));
    }

    // Microsoft's rule: read FLAGS_UI (IE8+) and fall back to FLAGS, but always write FLAGS.
    DWORD connectionFlags(std::wstring connection) {
        for (const DWORD which : {DWORD(INTERNET_PER_CONN_FLAGS_UI), DWORD(INTERNET_PER_CONN_FLAGS)}) {
            INTERNET_PER_CONN_OPTIONW option{};
            option.dwOption = which;
            auto list = connectionOptionList(connection, &option, 1);
            DWORD size = sizeof(list);
            if (InternetQueryOptionW(nullptr, INTERNET_OPTION_PER_CONNECTION_OPTION, &list, &size)) return option.Value.dwValue;
        }
        return PROXY_TYPE_DIRECT;
    }

    // What the connection is set to now; empty when it cannot be read.
    std::wstring currentProxyServer(std::wstring connection) {
        INTERNET_PER_CONN_OPTIONW option{};
        option.dwOption = INTERNET_PER_CONN_PROXY_SERVER;
        auto list = connectionOptionList(connection, &option, 1);
        DWORD size = sizeof(list);
        if (!InternetQueryOptionW(nullptr, INTERNET_OPTION_PER_CONNECTION_OPTION, &list, &size) || option.Value.pszValue == nullptr) return {};
        std::wstring server = option.Value.pszValue;
        GlobalFree(option.Value.pszValue);
        return server;
    }

    void announceProxyChange() {
        InternetSetOptionW(nullptr, INTERNET_OPTION_SETTINGS_CHANGED, nullptr, 0);
        InternetSetOptionW(nullptr, INTERNET_OPTION_PROXY_SETTINGS_CHANGED, nullptr, 0);
        InternetSetOptionW(nullptr, INTERNET_OPTION_REFRESH, nullptr, 0);
    }

    QString connectionLabel(const std::wstring &connection) {
        return connection.empty() ? QStringLiteral("LAN") : QString::fromStdWString(connection);
    }

    std::wstring proxyServerString(const QString &host, int port) {
        const QString ip = host.contains(':') ? QString("[%1]").arg(host) : host;
        return QString("%1:%2").arg(ip).arg(port).toStdWString();
    }

    // A proxy on this computer that has gone away would leave the browsers without a connection. The list may name one
    // proxy per scheme ("http=host:port;https=host:port").
    bool proxyStillServes(const std::wstring &server) {
        static const QRegularExpression separators(QStringLiteral("[;\\s]+"));
        const auto entries = QString::fromStdWString(server).split(separators, Qt::SkipEmptyParts);
        for (auto entry : entries) {
            if (const auto equals = entry.indexOf('='); equals >= 0) entry = entry.mid(equals + 1);
            if (const auto scheme = entry.indexOf(QLatin1String("://")); scheme >= 0) entry = entry.mid(scheme + 3);
            const auto colon = entry.lastIndexOf(':');
            QString host = colon > 0 ? entry.left(colon) : entry;
            host.remove('[').remove(']');
            const QHostAddress address = host.compare(QLatin1String("localhost"), Qt::CaseInsensitive) == 0 ? QHostAddress(QHostAddress::LocalHost) : QHostAddress(host);
            if (!address.isLoopback()) return true;
            QTcpSocket probe;
            probe.connectToHost(address, static_cast<quint16>(colon > 0 ? entry.mid(colon + 1).toUInt() : 80));
            if (probe.waitForConnected(300)) return true;
        }
        return entries.isEmpty();
    }

    QString clearLocked() {
        if (proxiedConnections.empty()) return {};
        QStringList errors;
        for (auto it = proxiedConnections.begin(); it != proxiedConnections.end();) {
            // Another client or the user changed it since Apply: what is there now is theirs.
            const auto current = currentProxyServer(it->first);
            if ((!current.empty() && current != appliedProxyServer) || !(connectionFlags(it->first) & PROXY_TYPE_PROXY)) {
                it = proxiedConnections.erase(it);
                continue;
            }
            auto &saved = it->second;
            DWORD flags = saved.flags | PROXY_TYPE_DIRECT;
            if ((flags & PROXY_TYPE_PROXY) && !proxyStillServes(saved.server)) flags &= ~DWORD(PROXY_TYPE_PROXY);
            INTERNET_PER_CONN_OPTIONW options[2]{};
            options[0].dwOption = INTERNET_PER_CONN_FLAGS;
            options[0].Value.dwValue = flags;
            options[1].dwOption = INTERNET_PER_CONN_PROXY_SERVER;
            options[1].Value.pszValue = saved.server.empty() ? nullptr : saved.server.data();
            if (setConnectionOptions(it->first, options, 2)) {
                it = proxiedConnections.erase(it);
            } else {
                const auto code = GetLastError();
                errors << QString("%1: InternetSetOption failed (%2)").arg(connectionLabel(it->first)).arg(code);
                ++it;
            }
        }
        announceProxyChange();
        saveProxyState();
        return errors.join("; ");
    }
}

QString SystemProxy_Apply(const QString &host, int port) {
    QMutexLocker lock(&proxyMutex);
    QStringList errors;
    auto server = proxyServerString(host, port);
    appliedProxyServer = server;
    for (const auto &connection : proxyConnectionNames(errors)) {
        // Only the first Apply still sees the user's settings; a later one would record our own.
        if (!proxiedConnections.contains(connection)) proxiedConnections[connection] = {connectionFlags(connection), currentProxyServer(connection)};

        INTERNET_PER_CONN_OPTIONW options[2]{};
        options[0].dwOption = INTERNET_PER_CONN_FLAGS;
        options[0].Value.dwValue = PROXY_TYPE_DIRECT | PROXY_TYPE_PROXY;
        options[1].dwOption = INTERNET_PER_CONN_PROXY_SERVER;
        options[1].Value.pszValue = server.data();
        if (!setConnectionOptions(connection, options, 2)) {
            const auto code = GetLastError();
            errors << QString("%1: InternetSetOption failed (%2)").arg(connectionLabel(connection)).arg(code);
        }
    }
    saveProxyState();
    announceProxyChange();
    return errors.join("; ");
}

QString SystemProxy_Clear() {
    QMutexLocker lock(&proxyMutex);
    return clearLocked();
}

QString SystemProxy_Recover() {
    QMutexLocker lock(&proxyMutex);
    if (!proxiedConnections.empty()) return {};
    QFile file(proxyStateFile());
    if (!file.open(QIODevice::ReadOnly)) return {};
    const auto root = QJsonDocument::fromJson(file.readAll()).object();
    file.close();
    appliedProxyServer = root["applied"].toString().toStdWString();
    for (const auto &value : root["connections"].toArray()) {
        const auto saved = value.toObject();
        proxiedConnections[saved["name"].toString().toStdWString()] = {static_cast<DWORD>(saved["flags"].toInteger()), saved["server"].toString().toStdWString()};
    }
    // A damaged file names nothing to give back; it goes either way.
    if (proxiedConnections.empty() || appliedProxyServer.empty()) {
        proxiedConnections.clear();
        QFile::remove(proxyStateFile());
        return {};
    }
    return clearLocked();
}
