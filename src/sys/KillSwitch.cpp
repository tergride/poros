#include "include/sys/KillSwitch.hpp"

#include "include/global/Configs.hpp"
#include "include/global/Logger.hpp"
#include "include/sys/Process.hpp"

#include <QDeadlineTimer>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QTimer>

#include <memory>
#include <utility>

namespace Sys {
    namespace {
        constexpr int kReadyTimeoutMs = 15000;
        constexpr int kCloseGraceMs = 5000;
        constexpr int kRespawnIntervalMs = 30000;
        constexpr int kShutdownWaitMs = 3000;

        const QString kPrivilege = QStringLiteral("privilege");
        const QString kUnsupported = QStringLiteral("unsupported");
        const QString kConfig = QStringLiteral("config");
        const QString kTampered = QStringLiteral("tampered");
        const QString kInternal = QStringLiteral("internal");
        const QString kCrash = QStringLiteral("crash");
        const QString kTimeout = QStringLiteral("timeout");

        // Must match genTunName() in generate.cpp.
        QString tunName() {
            return QStringLiteral("poros-net");
        }

        void stopHard(QProcess *process) {
            process->kill();
        }

        const char *stateName(KillSwitch::State state) {
            switch (state) {
                case KillSwitch::State::Disabled: return "disabled";
                case KillSwitch::State::Arming: return "arming";
                case KillSwitch::State::Armed: return "armed";
                case KillSwitch::State::Failed: return "failed";
            }
            return "unknown";
        }
    } // namespace

    namespace {
        QByteArray buildConfig() {
            const auto *settings = Configs::dataManager->settingsRepo.get();
            const QJsonObject object{
                {"allow_lan", !settings->disable_private_range_bypass},
                {"tun_name", tunName()},
                {"tun_cidrs", QJsonArray{settings->vpn_tun_ipv4_cidr, settings->vpn_tun_ipv6_cidr}},
            };
            return QJsonDocument(object).toJson(QJsonDocument::Compact);
        }
    } // namespace

    class GuardProcess : public QProcess {
    public:
        explicit GuardProcess(QObject *parent) : QProcess(parent), readyTimer(new QTimer(this)) {
            readyTimer->setSingleShot(true);
        }

        QByteArray config;
        QByteArray stderrTail;
        QTimer *readyTimer;
        bool ready = false;
        // Once set, the guard's exit is expected and never read as a failure.
        bool closing = false;
    };

    // Never destroyed: ~QProcess would SIGKILL a guard that still has to remove its own rules.
    KillSwitch *KillSwitch::instance() {
        static auto *self = [] {
            auto *created = new KillSwitch;
            return created;
        }();
        return self;
    }

    bool KillSwitch::allowsStart() const {
        const auto current = state();
        return current == State::Disabled || current == State::Armed;
    }

    bool KillSwitch::failedForPrivileges() const {
        return state() == State::Failed && failureCode_ == kPrivilege;
    }

    bool KillSwitch::failedPersistently() const {
        if (state() != State::Failed) return false;
        return failureCode_ == kPrivilege || failureCode_ == kUnsupported || failureCode_ == kConfig;
    }

    QString KillSwitch::failureText() const {
        QString text;
        if (failureCode_ == kPrivilege) {
            text = tr("The kill switch needs Poros to run as administrator.");
        } else if (failureCode_ == kUnsupported) {
            return failureMessage_.isEmpty() ? tr("The kill switch is not supported on this system.") : failureMessage_;
        } else if (failureCode_ == kTampered) {
            text = tr("Another program removed the kill switch rules or disabled the system firewall.");
        } else if (failureCode_ == kConfig) {
            text = tr("The kill switch rejected its settings.");
        } else {
            text = tr("The kill switch process stopped unexpectedly.");
        }
        if (!failureMessage_.isEmpty()) text += "\n\n" + tr("Details: %1").arg(failureMessage_);
        return text;
    }

    void KillSwitch::apply() {
        if (shutdown_) return;
        if (!Configs::dataManager->settingsRepo->kill_switch) {
            const bool wasOn = state() != State::Disabled;
            closeGuard(std::exchange(pending_, nullptr));
            closeGuard(std::exchange(active_, nullptr));
            if (wasOn) MW_show_log(tr("[Kill switch] Off."));
            setState(State::Disabled);
            settle();
            return;
        }
        const auto config = buildConfig();
        if (pending_ != nullptr && pending_->config == config) return;
        if (active_ != nullptr && active_->config == config) {
            if (pending_ != nullptr) {
                closeGuard(std::exchange(pending_, nullptr));
                settle();
            }
            return;
        }
        spawn(config);
    }

    void KillSwitch::whenSettled(QObject *context, std::function<void(bool armed)> callback) {
        if (state() != State::Arming) {
            callback(state() == State::Armed);
            return;
        }
        waiters_.append({context, context != nullptr, std::move(callback)});
    }

    void KillSwitch::shutdown() {
        if (shutdown_) return;
        shutdown_ = true;
        waiters_.clear();
        active_ = nullptr;
        pending_ = nullptr;

        QList<GuardProcess *> guards;
        // GuardProcess has no metaobject of its own, and every QProcess child is one.
        for (auto *child : findChildren<QProcess *>(QString(), Qt::FindDirectChildrenOnly)) guards << static_cast<GuardProcess *>(child);
        for (auto *guard : guards) {
            guard->closing = true;
            guard->readyTimer->stop();
            if (guard->state() != QProcess::NotRunning) guard->closeWriteChannel();
        }
        const QDeadlineTimer deadline(kShutdownWaitMs);
        for (auto *guard : guards) {
            if (guard->state() != QProcess::NotRunning) guard->waitForFinished(int(qMax<qint64>(0, deadline.remainingTime())));
        }
        for (auto *guard : guards) {
            if (guard->state() == QProcess::NotRunning) continue;
            LOG_WARN("kill switch: a guard did not exit after its input closed, stopping it");
            stopHard(guard);
            guard->waitForFinished(1000);
        }
    }

    void KillSwitch::spawn(const QByteArray &config) {
        closeGuard(std::exchange(pending_, nullptr));
        auto *guard = new GuardProcess(this);
        guard->config = config;
        pending_ = guard;

        connect(guard->readyTimer, &QTimer::timeout, this, [this, guard] { onReadyTimeout(guard); });
        // The write channel stays open for the guard's whole life: EOF is its signal to remove the rules.
        connect(guard, &QProcess::started, this, [guard] {
            if (!guard->closing) guard->write(guard->config + '\n');
        });
        connect(guard, &QProcess::readyReadStandardOutput, this, [this, guard] { readOutput(guard); });
        connect(guard, &QProcess::readyReadStandardError, this, [this, guard] { readErrors(guard, false); });
        connect(guard, &QProcess::errorOccurred, this, [this, guard](QProcess::ProcessError error) {
            if (error == QProcess::FailedToStart) {
                onStartFailed(guard);
                return;
            }
            LOG_WARN(QString("kill switch: guard process error %1: %2").arg(int(error)).arg(guard->errorString()));
        });
        connect(guard, &QProcess::finished, this, [this, guard](int exitCode, QProcess::ExitStatus status) {
            onFinished(guard, exitCode, status == QProcess::CrashExit);
        });

        LOG_INFO("kill switch: starting a guard with " + QString::fromUtf8(guard->config));
        // Before start(): Windows reports a failed start synchronously, and that must move Arming on to Failed.
        if (active_ == nullptr) setState(State::Arming);
        guard->readyTimer->start(kReadyTimeoutMs);
        Configs_sys::IsolateChild(guard);
        guard->start(Configs::FindCoreRealPath(), {QStringLiteral("--guard")});
    }

    void KillSwitch::closeGuard(GuardProcess *guard) {
        if (guard == nullptr || guard->closing) return;
        guard->closing = true;
        guard->readyTimer->stop();
        if (guard->state() == QProcess::NotRunning) {
            guard->deleteLater();
            return;
        }
        guard->closeWriteChannel();
        QTimer::singleShot(kCloseGraceMs, guard, [guard] {
            if (guard->state() == QProcess::NotRunning) return;
            LOG_WARN("kill switch: a closed guard is still running, stopping it");
            stopHard(guard);
        });
    }

    void KillSwitch::readOutput(GuardProcess *guard) {
        while (guard->canReadLine()) {
            const auto line = QString::fromUtf8(guard->readLine()).trimmed();
            if (!line.isEmpty()) handleLine(guard, line);
        }
    }

    void KillSwitch::readErrors(GuardProcess *guard, bool flush) {
        guard->stderrTail += guard->readAllStandardError();
        for (auto end = guard->stderrTail.indexOf('\n'); end >= 0; end = guard->stderrTail.indexOf('\n')) {
            const auto line = QString::fromUtf8(guard->stderrTail.left(end)).trimmed();
            guard->stderrTail.remove(0, end + 1);
            if (!line.isEmpty()) MW_show_log("[Kill switch] " + line);
        }
        if (flush && !guard->stderrTail.trimmed().isEmpty()) {
            MW_show_log("[Kill switch] " + QString::fromUtf8(guard->stderrTail).trimmed());
            guard->stderrTail.clear();
        }
    }

    void KillSwitch::handleLine(GuardProcess *guard, const QString &line) {
        const auto verb = line.section(' ', 0, 0);
        const auto rest = line.section(' ', 1);
        if (verb == QLatin1String("WARN")) {
            LOG_WARN("kill switch: " + rest);
            MW_show_log(tr("[Kill switch] Warning: %1").arg(rest));
            return;
        }
        if (verb == QLatin1String("READY")) {
            onReady(guard);
            return;
        }
        if (verb != QLatin1String("ERROR") && verb != QLatin1String("FAIL")) {
            MW_show_log("[Kill switch] " + line);
            return;
        }
        if (guard->closing) return;
        const auto code = rest.section(' ', 0, 0);
        const auto message = rest.section(' ', 1);
        const bool wasActive = guard == active_;
        const bool wasPending = guard == pending_;
        if (wasActive) active_ = nullptr;
        if (wasPending) pending_ = nullptr;
        closeGuard(guard);
        if (wasActive) protectionLost(code, message);
        else if (wasPending) armFailed(code, message);
    }

    void KillSwitch::onReady(GuardProcess *guard) {
        if (guard != pending_ || guard->ready) return;
        guard->ready = true;
        guard->readyTimer->stop();
        auto *previous = std::exchange(active_, guard);
        pending_ = nullptr;
        closeGuard(previous);
        LOG_INFO("kill switch: armed with " + QString::fromUtf8(guard->config));
        MW_show_log(previous == nullptr ? tr("[Kill switch] On: only traffic through Poros can leave this device.")
                                        : tr("[Kill switch] New settings applied."));
        setState(State::Armed);
        settle();
    }

    void KillSwitch::onReadyTimeout(GuardProcess *guard) {
        if (guard != pending_ || guard->ready) return;
        pending_ = nullptr;
        closeGuard(guard);
        stopHard(guard);
        armFailed(kTimeout, tr("the kill switch did not respond within %1 seconds").arg(kReadyTimeoutMs / 1000));
    }

    void KillSwitch::onStartFailed(GuardProcess *guard) {
        guard->readyTimer->stop();
        const auto message = tr("could not start %1: %2").arg(guard->program(), guard->errorString());
        LOG_ERROR("kill switch: " + message);
        const bool wasPending = !guard->closing && guard == pending_;
        if (wasPending) pending_ = nullptr;
        guard->closing = true;
        guard->deleteLater();
        if (wasPending) armFailed(kInternal, message);
    }

    void KillSwitch::onFinished(GuardProcess *guard, int exitCode, bool crashed) {
        readOutput(guard);
        if (const auto last = QString::fromUtf8(guard->readAllStandardOutput()).trimmed(); !last.isEmpty()) {
            handleLine(guard, last);
        }
        readErrors(guard, true);
        guard->readyTimer->stop();

        Logging::Write(guard->closing ? Logging::Level::Info : Logging::Level::Error,
                       QString("kill switch: guard %1 (code %2)").arg(crashed ? "crashed" : "exited").arg(exitCode));
        const bool wasActive = !guard->closing && guard == active_;
        const bool wasPending = !guard->closing && guard == pending_;
        if (wasActive) active_ = nullptr;
        if (wasPending) pending_ = nullptr;
        guard->closing = true;
        guard->deleteLater();
        if (wasActive) {
            protectionLost(kCrash, crashed ? tr("the kill switch process crashed")
                                           : tr("the kill switch process exited with code %1").arg(exitCode));
        } else if (wasPending) {
            armFailed(kCrash, crashed ? tr("the kill switch process crashed before it was ready")
                                      : tr("the kill switch process exited with code %1 before it was ready").arg(exitCode));
        }
    }

    void KillSwitch::armFailed(const QString &code, const QString &message) {
        LOG_ERROR(QString("kill switch: arming failed (%1): %2").arg(code, message));
        if (active_ != nullptr) {
            MW_show_log(tr("[Kill switch] The new settings could not be applied, the previous ones stay in force: %1").arg(message));
            if (code != kPrivilege && code != kUnsupported && code != kConfig) {
                // apply() is a no-op once the kept guard's config matches again.
                QTimer::singleShot(kRespawnIntervalMs, this, [this] {
                    if (shutdown_ || pending_ != nullptr || active_ == nullptr) return;
                    if (Configs::dataManager->settingsRepo->prepare_exit) return;
                    apply();
                });
            }
            settle();
            return;
        }
        fail(code, message);
    }

    void KillSwitch::protectionLost(const QString &code, const QString &message) {
        LOG_ERROR(QString("kill switch: protection lost (%1): %2").arg(code, message));
        MW_show_log(tr("[Kill switch] Protection lost: %1").arg(message));
        if (shutdown_) return;
        if (!Configs::dataManager->settingsRepo->kill_switch) {
            apply();
            return;
        }
        if (pending_ != nullptr) {
            setState(State::Arming);
            settle();
            return;
        }
        const bool recoverable = code != kPrivilege && code != kUnsupported && code != kConfig;
        if (recoverable && (!lastRespawn_.isValid() || lastRespawn_.hasExpired(kRespawnIntervalMs))) {
            lastRespawn_.start();
            MW_show_log(tr("[Kill switch] Starting it again..."));
            spawn(buildConfig());
            return;
        }
        fail(code, message);
    }

    void KillSwitch::fail(const QString &code, const QString &message) {
        failureCode_ = code;
        failureMessage_ = message;
        MW_show_log(tr("[Kill switch] Not active: %1").arg(message.isEmpty() ? code : message));
        setState(State::Failed);
        settle();
    }

    void KillSwitch::setState(State state) {
        if (state != State::Failed) {
            failureCode_.clear();
            failureMessage_.clear();
        }
        if (state_.exchange(state) == state) return;
        LOG_INFO(QString("kill switch: %1").arg(stateName(state)));
        emit stateChanged();
    }

    void KillSwitch::settle() {
        const auto waiting = std::exchange(waiters_, {});
        for (const auto &waiter : waiting) {
            if (state() == State::Arming) {
                waiters_.append(waiter);
                continue;
            }
            if (waiter.bound && waiter.context.isNull()) continue;
            waiter.callback(state() == State::Armed);
        }
    }
} // namespace Sys
