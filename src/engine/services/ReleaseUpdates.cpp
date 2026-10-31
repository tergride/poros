#include "include/engine/services/ReleaseUpdates.h"

#include <QCoreApplication>
#include <QCryptographicHash>
#include <QDir>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocale>
#include <QRegularExpression>

#include "NkrVersion.h"
#include "include/database/DatabaseManager.h"
#include "include/database/SettingsRepo.h"
#include "include/global/Configs.hpp"
#include "include/global/HTTPRequestHelper.hpp"

namespace Engine {
    namespace {
        // GitHub redirects this to the newest published release, and it has no API rate limit. 404 until the first release.
        constexpr auto kReleaseUpdatesFeedUrl = "https://github.com/tergride/poros/releases/latest/download/latest.json";
        constexpr auto kReleaseUpdatesArchiveUrl = "https://github.com/tergride/poros/releases/download/v%1/%2";
        constexpr auto kReleaseUpdatesFolder = "update";
        // Written by updater.exe next to the archive.
        constexpr auto kReleaseUpdatesErrorFile = "install-error.txt";
        constexpr qint64 kReleaseUpdatesFeedMaxBytes = 1024 * 1024;

        QString releaseUpdatesFolder() {
            return Configs::GetBasePath() + u'/' + QLatin1String(kReleaseUpdatesFolder);
        }

        QString releaseUpdatesArchivePath(const ReleaseFeed &feed) {
            return releaseUpdatesFolder() + u'/' + feed.file;
        }

        bool releaseUpdatesArchiveIsIntact(const ReleaseFeed &feed) {
            QFile file(releaseUpdatesArchivePath(feed));
            if (!file.open(QIODevice::ReadOnly) || file.size() != feed.size) return false;
            QCryptographicHash hash(QCryptographicHash::Sha256);
            return hash.addData(&file) && QString::fromLatin1(hash.result().toHex()) == feed.sha256;
        }

        void releaseUpdatesDropOtherArchives(const ReleaseFeed &feed) {
            QDir folder(releaseUpdatesFolder());
            for (const auto &name : folder.entryList({QStringLiteral("*.zip"), QStringLiteral("*.tmp")}, QDir::Files)) {
                if (name != feed.file) folder.remove(name);
            }
        }
    } // namespace

    std::optional<ReleaseFeed> ParseReleaseFeed(const QByteArray &json) {
        static const QRegularExpression version(QStringLiteral(R"(^\d+\.\d+\.\d+$)"));
        static const QRegularExpression file(QStringLiteral(R"(^Poros-\d+\.\d+\.\d+-windows-amd64\.zip$)"));
        static const QRegularExpression sha256(QStringLiteral("^[0-9a-f]{64}$"));

        const auto object = QJsonDocument::fromJson(json).object();
        ReleaseFeed feed;
        feed.version = object["version"].toString();
        feed.file = object["file"].toString();
        feed.size = object["size"].toInteger();
        feed.sha256 = object["sha256"].toString();
        const auto notes = object["notes"].toObject();
        feed.notesRu = notes["ru"].toString();
        feed.notesEn = notes["en"].toString();
        if (!version.match(feed.version).hasMatch() || !file.match(feed.file).hasMatch() || !sha256.match(feed.sha256).hasMatch() || feed.size <= 0) {
            return std::nullopt;
        }
        return feed;
    }

    bool IsNewerVersion(const QString &candidate, const QString &current) {
        const auto left = candidate.split(u'.');
        const auto right = current.split(u'.');
        for (qsizetype i = 0; i < qMax(left.size(), right.size()); ++i) {
            const int a = left.value(i).toInt();
            const int b = right.value(i).toInt();
            if (a != b) return a > b;
        }
        return false;
    }

    ReleaseUpdates *ReleaseUpdates::instance() {
        static auto *updates = new ReleaseUpdates;
        return updates;
    }

    bool ReleaseUpdates::Available() {
        return !QStringLiteral(NKR_VERSION).isEmpty() && !Configs::dataManager->settingsRepo->flag_use_appdata &&
               QFile::exists(QCoreApplication::applicationDirPath() + QStringLiteral("/updater.exe"));
    }

    void ReleaseUpdates::check(bool manual) {
        // Offering the install that just failed would only fail again, so the first automatic check after it stays quiet.
        if (!Available() || (!manual && m_installFailed.exchange(false)) || m_checking.exchange(true)) return;
        const QString error = run();
        m_checking = false;
        emit checked(manual, error);
    }

    void ReleaseUpdates::reportFailedInstall() {
        QFile failure(releaseUpdatesFolder() + u'/' + QLatin1String(kReleaseUpdatesErrorFile));
        if (!failure.open(QIODevice::ReadOnly)) return;
        const auto text = QString::fromUtf8(failure.readAll());
        failure.close();
        failure.remove();
        m_installFailed = true;
        emit installFailed(text);
    }

    QString ReleaseUpdates::run() {
        if (!readyVersion().isEmpty()) return {};

        HttpGetOptions options;
        options.strictTls = true;
        options.maxBytes = kReleaseUpdatesFeedMaxBytes;
        const auto response = NetworkRequestHelper::HttpGet(QString::fromLatin1(kReleaseUpdatesFeedUrl), options);
        if (response.status == 404) return {};
        if (!response.error.isEmpty()) return response.error;
        const auto feed = ParseReleaseFeed(response.data);
        if (!feed) return tr("The update information could not be read.");
        if (!IsNewerVersion(feed->version, QStringLiteral(NKR_VERSION))) return {};

        if (!QDir().mkpath(releaseUpdatesFolder())) return tr("Could not create the update folder.");
        releaseUpdatesDropOtherArchives(*feed);
        // An archive from an earlier run is reused, so a restart does not cost a second download.
        if (!releaseUpdatesArchiveIsIntact(*feed)) {
            const auto url = QString::fromLatin1(kReleaseUpdatesArchiveUrl).arg(feed->version, feed->file);
            const auto relative = QLatin1String(kReleaseUpdatesFolder) + u'/' + feed->file;
            if (const auto error = NetworkRequestHelper::DownloadAsset(url, relative, false, true); !error.isEmpty()) return error;
            if (!releaseUpdatesArchiveIsIntact(*feed)) {
                QFile::remove(releaseUpdatesArchivePath(*feed));
                return tr("The downloaded update is damaged.");
            }
        }
        QMutexLocker locker(&m_mutex);
        m_ready = feed;
        return {};
    }

    QString ReleaseUpdates::readyVersion() const {
        QMutexLocker locker(&m_mutex);
        return m_ready ? m_ready->version : QString();
    }

    QString ReleaseUpdates::readyNotes() const {
        QMutexLocker locker(&m_mutex);
        if (!m_ready) return {};
        const bool russian = QLocale().language() == QLocale::Russian;
        const auto &preferred = russian ? m_ready->notesRu : m_ready->notesEn;
        return preferred.isEmpty() ? (russian ? m_ready->notesEn : m_ready->notesRu) : preferred;
    }

    QStringList ReleaseUpdates::installArguments(const QStringList &programArguments) const {
        QMutexLocker locker(&m_mutex);
        if (!m_ready) return {};
        return QStringList{QStringLiteral("-pid"), QString::number(QCoreApplication::applicationPid()),
                           QStringLiteral("-archive"), QDir::toNativeSeparators(releaseUpdatesArchivePath(*m_ready)),
                           QStringLiteral("-dir"), QDir::toNativeSeparators(QCoreApplication::applicationDirPath()),
                           QStringLiteral("--")} + programArguments;
    }
} // namespace Engine
