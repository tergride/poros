#include "include/engine/services/SettingsRegistry.h"

#include <QJsonValue>
#include <QList>
#include <QSet>

#include <cmath>
#include <functional>
#include <tuple>
#include <type_traits>
#include <utility>

#include "include/configs/generate.h"
#include "include/engine/EngineEvents.h"
#include "include/engine/ThreadCheck.h"
#include "include/global/Configs.hpp"
#include "include/global/Logger.hpp"
#include "include/global/PeriodicRunner.hpp"
#include "include/global/Validation.hpp"
#include "include/sys/KillSwitch.hpp"

namespace Engine::SettingsRegistryImpl {
    using Settings = Configs::SettingsRepo;

    constexpr int kMinAutoUpdate = 30;
    constexpr int kMaxAutoUpdate = 525600;

    enum class Restart { None, Profile, Tun };

    enum Effect {
        NoEffect = 0,
        ApplyKillSwitch = 1,
        CheckPeriodic = 2,
    };

    template <typename T>
    struct Write {
        QString key;
        T Settings::*member;
        T value;
    };

    // The member writes of one Put, each under the key that made it; value() reads them over the stored settings.
    class Patch {
    public:
        explicit Patch(const Settings &settings) : settings_(settings) {}

        template <typename T>
        void set(const QString &key, T Settings::*member, std::type_identity_t<T> value) {
            auto &writes = std::get<QList<Write<T>>>(writes_);
            writes.removeIf([member](const Write<T> &write) { return write.member == member; });
            writes.append({key, member, std::move(value)});
            if (!keys_.contains(key)) keys_ << key;
        }

        template <typename T>
        T value(T Settings::*member) const {
            for (const auto &write : std::get<QList<Write<T>>>(writes_)) {
                if (write.member == member) return write.value;
            }
            return settings_.*member;
        }

        template <typename T>
        bool changes(T Settings::*member) const {
            return value(member) != settings_.*member;
        }

        bool touches(const QString &key) const {
            return keys_.contains(key);
        }

        // Writes the members that differ and returns their keys, in the order the keys were first set.
        QStringList commit(Settings &settings) const {
            QSet<QString> changed;
            const auto writeChanged = [&settings, &changed](const auto &writes) {
                for (const auto &write : writes) {
                    if (settings.*write.member == write.value) continue;
                    settings.*write.member = write.value;
                    changed.insert(write.key);
                }
            };
            std::apply([&writeChanged](const auto &...writes) { (writeChanged(writes), ...); }, writes_);
            QStringList keys;
            for (const auto &key : keys_) {
                if (changed.contains(key)) keys << key;
            }
            return keys;
        }

    private:
        const Settings &settings_;
        std::tuple<QList<Write<bool>>, QList<Write<int>>, QList<Write<QString>>> writes_;
        QStringList keys_;
    };

    struct Key {
        QString name;
        Restart restart = Restart::None;
        int effects = NoEffect;
        std::function<QJsonValue(const Settings &)> get;
        // check and set are called only for a value that differs from get(); check returns the problem naming the key, or empty.
        std::function<QString(const QJsonValue &)> check;
        std::function<void(const QJsonValue &, Patch *)> set;
    };

    ServiceError badRequest(const QString &message) {
        return {400, QStringLiteral("bad_request"), message};
    }

    // Like the Router's readInteger: a JSON number without a fraction that a double holds exactly.
    bool wholeNumber(const QJsonValue &value, qint64 *out) {
        constexpr double kExactLimit = 9007199254740992.0;
        const double number = value.toDouble();
        if (!value.isDouble() || std::trunc(number) != number || std::abs(number) > kExactLimit) return false;
        *out = static_cast<qint64>(number);
        return true;
    }

    QString integerProblem(const QString &name, const QJsonValue &value, qint64 min, qint64 max) {
        qint64 number = 0;
        if (!wholeNumber(value, &number)) return QStringLiteral("\"%1\" must be an integer").arg(name);
        if (number < min || number > max) {
            return QStringLiteral("\"%1\" must be from %2 to %3").arg(name, QString::number(min), QString::number(max));
        }
        return {};
    }

    Key boolKey(const QString &name, bool Settings::*member, Restart restart, int effects = NoEffect) {
        return {name, restart, effects,
                [member](const Settings &settings) { return QJsonValue(settings.*member); },
                [name](const QJsonValue &value) {
                    return value.isBool() ? QString() : QStringLiteral("\"%1\" must be true or false").arg(name);
                },
                [name, member](const QJsonValue &value, Patch *patch) { patch->set(name, member, value.toBool()); }};
    }

    Key intKey(const QString &name, int Settings::*member, int min, int max, Restart restart, int effects = NoEffect) {
        return {name, restart, effects,
                [member](const Settings &settings) { return QJsonValue(settings.*member); },
                [name, min, max](const QJsonValue &value) { return integerProblem(name, value, min, max); },
                [name, member](const QJsonValue &value, Patch *patch) {
                    patch->set(name, member, static_cast<int>(value.toDouble()));
                }};
    }

    // rule completes "\"<name>\" ...". Stored trimmed, as the dialogs store them.
    Key stringKey(const QString &name, QString Settings::*member, Restart restart, std::function<bool(const QString &)> valid,
                  const QString &rule) {
        return {name, restart, NoEffect,
                [member](const Settings &settings) { return QJsonValue(settings.*member); },
                [name, valid = std::move(valid), rule](const QJsonValue &value) {
                    if (!value.isString()) return QStringLiteral("\"%1\" must be a string").arg(name);
                    return valid(value.toString().trimmed()) ? QString() : QStringLiteral("\"%1\" %2").arg(name, rule);
                },
                [name, member](const QJsonValue &value, Patch *patch) { patch->set(name, member, value.toString().trimmed()); }};
    }

    Key enumKey(const QString &name, QString Settings::*member, Restart restart, const QStringList &allowed) {
        return stringKey(name, member, restart, [allowed](const QString &text) { return allowed.contains(text); },
                         QStringLiteral("must be one of: %1").arg(allowed.join(QStringLiteral(", "))));
    }

    // Stored negative = off, keeping the minutes for when it is turned back on; a magnitude below 30 counts as off.
    int autoUpdateOff(int stored, int fallbackMinutes) {
        if (stored >= kMinAutoUpdate) return -stored;
        return stored <= -kMinAutoUpdate ? stored : -fallbackMinutes;
    }

    // Minutes: 0 = off, else 30..525600; 1..29 would silently read as off, so they are refused.
    Key autoUpdateKey(const QString &name, int Settings::*member, int fallbackMinutes) {
        return {name, Restart::None, CheckPeriodic,
                [member](const Settings &settings) {
                    return QJsonValue(settings.*member >= kMinAutoUpdate ? settings.*member : 0);
                },
                [name](const QJsonValue &value) {
                    qint64 minutes = 0;
                    if (!wholeNumber(value, &minutes)) return QStringLiteral("\"%1\" must be an integer").arg(name);
                    if (minutes != 0 && (minutes < kMinAutoUpdate || minutes > kMaxAutoUpdate)) {
                        return QStringLiteral("\"%1\" must be 0 (off) or from %2 to %3 minutes")
                            .arg(name, QString::number(kMinAutoUpdate), QString::number(kMaxAutoUpdate));
                    }
                    return QString();
                },
                [name, member, fallbackMinutes](const QJsonValue &value, Patch *patch) {
                    const int minutes = static_cast<int>(value.toDouble());
                    patch->set(name, member, minutes != 0 ? minutes : autoUpdateOff(patch->value(member), fallbackMinutes));
                }};
    }

    const QList<Key> &keys() {
        static const QList<Key> table = [] {
            const QStringList http = {QStringLiteral("http"), QStringLiteral("https")};
            QList<Key> list;
            list << boolKey(QStringLiteral("remember_enable"), &Settings::remember_enable, Restart::None);
            list << enumKey(QStringLiteral("log_level"), &Settings::log_level, Restart::Profile,
                            QStringLiteral("trace debug info warn error fatal panic").split(QLatin1Char(' ')));
            list << enumKey(QStringLiteral("xray_log_level"), &Settings::xray_log_level, Restart::Profile, Configs::Xray::XrayLogLevels);
            list << stringKey(QStringLiteral("inbound_address"), &Settings::inbound_address, Restart::Profile,
                              [](const QString &text) { return Validation::Ip(text).isEmpty(); },
                              QStringLiteral("must be an IPv4 or IPv6 address, such as 127.0.0.1 or ::"));

            auto inboundPort = intKey(QStringLiteral("inbound_port"), &Settings::inbound_socks_port, 1, 65535, Restart::Profile);
            // A random port is re-picked at every launch, which would discard the one set here.
            inboundPort.set = [setPort = inboundPort.set](const QJsonValue &value, Patch *patch) {
                setPort(value, patch);
                patch->set(QStringLiteral("inbound_port"), &Settings::random_inbound_port, false);
            };
            list << inboundPort;

            list << stringKey(QStringLiteral("test_url"), &Settings::test_latency_url, Restart::None,
                              [http](const QString &text) { return Validation::Url(text, http, Validation::Empty::Required).isEmpty(); },
                              QStringLiteral("must be an http:// or https:// URL with a host"));
            list << intKey(QStringLiteral("test_concurrency"), &Settings::test_concurrent, 1, 499, Restart::None);
            list << intKey(QStringLiteral("test_timeout_ms"), &Settings::url_test_timeout_ms, 0, 60000, Restart::None);
            list << autoUpdateKey(QStringLiteral("sub_auto_update"), &Settings::sub_auto_update, 30);
            list << boolKey(QStringLiteral("sub_respect_server_interval"), &Settings::sub_respect_server_interval, Restart::None, CheckPeriodic);
            list << autoUpdateKey(QStringLiteral("route_auto_update"), &Settings::route_auto_update, 1440);
            list << boolKey(QStringLiteral("kill_switch"), &Settings::kill_switch, Restart::None, ApplyKillSwitch);
            list << boolKey(QStringLiteral("tun_strict_route"), &Settings::vpn_strict_route, Restart::Tun, ApplyKillSwitch);
            list << intKey(QStringLiteral("tun_mtu"), &Settings::vpn_mtu, 1000, 10000, Restart::Tun, ApplyKillSwitch);
            list << boolKey(QStringLiteral("tun_ipv6"), &Settings::vpn_ipv6, Restart::Tun, ApplyKillSwitch);
            auto tunIpv4Cidr = stringKey(QStringLiteral("tun_ipv4_cidr"), &Settings::vpn_tun_ipv4_cidr, Restart::Tun,
                                         [](const QString &text) { return Validation::Cidr(text, Validation::Family::V4).isEmpty(); },
                                         QStringLiteral("must be an IPv4 address with a prefix length, such as 172.19.1.1/24"));
            tunIpv4Cidr.effects = ApplyKillSwitch;
            list << tunIpv4Cidr;
            auto tunIpv6Cidr = stringKey(QStringLiteral("tun_ipv6_cidr"), &Settings::vpn_tun_ipv6_cidr, Restart::Tun,
                                         [](const QString &text) { return Validation::Cidr(text, Validation::Family::V6).isEmpty(); },
                                         QStringLiteral("must be an IPv6 address with a prefix length, such as fdfe:dcba:9877::1/96"));
            tunIpv6Cidr.effects = ApplyKillSwitch;
            list << tunIpv6Cidr;

            list << intKey(QStringLiteral("dns_in_port"), &Settings::core_dns_in_port, 1, 65535, Restart::Profile);

            const auto dnsAddress = [](const QString &text) { return Configs::ParseDnsAddress(text, nullptr); };
            const auto dnsRule = QStringLiteral("must be a DNS address: local, dhcp://auto, dhcp://<interface>, udp|tcp|tls|quic://host[:port], "
                                                "https|h3://host[:port][/path] or host[:port]");
            list << stringKey(QStringLiteral("remote_dns"), &Settings::remote_dns, Restart::Profile, dnsAddress, dnsRule);
            list << stringKey(QStringLiteral("direct_dns"), &Settings::direct_dns, Restart::Profile, dnsAddress, dnsRule);

            list << boolKey(QStringLiteral("ntp_enabled"), &Settings::enable_ntp, Restart::Profile);
            list << stringKey(QStringLiteral("ntp_server"), &Settings::ntp_server_address, Restart::Profile,
                              [](const QString &text) { return Validation::Host(text, Validation::Empty::Allowed).isEmpty(); },
                              QStringLiteral("must be empty or a host name or IP address, without a scheme, port or spaces"));
            list << stringKey(QStringLiteral("ntp_interval"), &Settings::ntp_interval, Restart::Profile,
                              [](const QString &text) { return Validation::Duration(text, {Validation::Empty::Required}).isEmpty(); },
                              QStringLiteral("must be a duration such as 30m or 1h (units ns, us, ms, s, m, h, d)"));
            return list;
        }();
        return table;
    }

    const Key *findKey(const QString &name) {
        for (const auto &key : keys()) {
            if (key.name == name) return &key;
        }
        return nullptr;
    }

    struct PortUser {
        QString key;
        int Settings::*member;
        QString label;
    };

    // The settings dialog's port rule over the pending values, for each port this Put changes or brings back into use.
    QString portConflict(const Patch &pending) {
        const QList<PortUser> users = {
            {QStringLiteral("inbound_port"), &Settings::inbound_socks_port, QStringLiteral("the mixed inbound (inbound_port)")},
            {QStringLiteral("dns_in_port"), &Settings::core_dns_in_port, QStringLiteral("the core's DNS server (dns_in_port)")},
        };
        const bool fixedPort = !pending.value(&Settings::random_inbound_port);
        const auto inUse = [&pending, fixedPort](const PortUser &user) {
            return pending.value(user.member) > 0 && (user.member != &Settings::inbound_socks_port || fixedPort);
        };
        for (const auto &user : users) {
            // While the inbound port was random, the DNS server could take its stored port unchecked.
            const bool reenabled = user.member == &Settings::inbound_socks_port && fixedPort && pending.changes(&Settings::random_inbound_port);
            if (!pending.changes(user.member) && !reenabled) continue;
            const int port = pending.value(user.member);
            if (port <= 0) continue;
            for (const auto &other : users) {
                if (other.member != user.member && inUse(other) && pending.value(other.member) == port) {
                    return QStringLiteral("\"%1\" %2 is already used by %3; choose a different port")
                        .arg(user.key, QString::number(port), other.label);
                }
            }
        }
        return {};
    }
} // namespace Engine::SettingsRegistryImpl

namespace Engine::SettingsRegistry {
    std::optional<ServiceError> Put(const QJsonObject &patch, const Context &context, PutResult *result) {
        using Settings = SettingsRegistryImpl::Settings;
        ENGINE_CHECK_MAIN_THREAD();
        if (result != nullptr) *result = {};
        auto &settings = *Configs::dataManager->settingsRepo;
        if (settings.prepare_exit || settings.noSave) {
            return ServiceError{503, QStringLiteral("exiting"), QStringLiteral("Poros is shutting down")};
        }
        SettingsRegistryImpl::Patch pending(settings);
        for (auto it = patch.constBegin(); it != patch.constEnd(); ++it) {
            const auto *key = SettingsRegistryImpl::findKey(it.key());
            if (key == nullptr) return SettingsRegistryImpl::badRequest(QStringLiteral("Unknown setting \"%1\"").arg(it.key()));
            const QJsonValue value = it.value();
            // A stored value may be invalid, so one sent back unchanged passes and writes nothing.
            if (value.isNull() || value == key->get(settings)) continue;
            if (const auto problem = key->check(value); !problem.isEmpty()) return SettingsRegistryImpl::badRequest(problem);
            key->set(value, &pending);
        }

        if (const auto problem = SettingsRegistryImpl::portConflict(pending); !problem.isEmpty()) {
            return SettingsRegistryImpl::badRequest(problem);
        }
        // The core refuses an empty NTP interval, which would fail every start.
        if ((pending.touches(QStringLiteral("ntp_enabled")) || pending.touches(QStringLiteral("ntp_interval"))) &&
            pending.value(&Settings::enable_ntp) && pending.value(&Settings::ntp_interval).isEmpty()) {
            pending.set(QStringLiteral("ntp_interval"), &Settings::ntp_interval, QStringLiteral("30m"));
        }

        auto changed = pending.commit(settings);
        if (changed.isEmpty()) return std::nullopt;
        changed.sort();

        bool restartNeeded = false;
        int effects = SettingsRegistryImpl::NoEffect;
        for (const auto &name : changed) {
            const auto *key = SettingsRegistryImpl::findKey(name);
            if (key == nullptr) continue;
            effects |= key->effects;
            if (key->restart == SettingsRegistryImpl::Restart::Profile) restartNeeded |= context.profileRunning;
            if (key->restart == SettingsRegistryImpl::Restart::Tun) restartNeeded |= context.profileRunning && context.tunOn;
        }
        if (result != nullptr) {
            result->changed = changed;
            result->restartNeeded = restartNeeded;
        }

        settings.Save();
        LOG_INFO(QStringLiteral("Settings changed: %1").arg(changed.join(QStringLiteral(", "))));
        if (effects & SettingsRegistryImpl::ApplyKillSwitch) Sys::KillSwitch::instance()->apply();
        if (effects & SettingsRegistryImpl::CheckPeriodic) Poros::PeriodicRunner::instance()->CheckNow();
        EngineEvents::SettingsChanged(changed);
        return std::nullopt;
    }
} // namespace Engine::SettingsRegistry
