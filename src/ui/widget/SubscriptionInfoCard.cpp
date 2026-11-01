#include "include/ui/widget/SubscriptionInfoCard.hpp"

#include <QAction>
#include <QDateTime>
#include <QDesktopServices>
#include <QHBoxLayout>
#include <QLabel>
#include <QMenu>
#include <QProgressBar>
#include <QPushButton>
#include <QResizeEvent>
#include <QStyle>
#include <QTextDocument>
#include <QTimer>
#include <QToolButton>
#include <QVBoxLayout>
#include <algorithm>

#include "include/configs/sub/GroupUpdater.hpp"
#include "include/database/entities/Group.h"
#include "include/global/Utils.hpp"
#include "include/ui/setting/PorosTheme.h"
#include "include/ui/stats/dialog_endpoint_details.h"

namespace {
    constexpr int subscriptionUrgentDays = 3;

    QUrl linkUrl(const QString &raw, bool allowTg) {
        const QUrl url(raw.trimmed());
        const QString scheme = url.scheme().toLower();
        const bool allowed = scheme == QLatin1String("http") || scheme == QLatin1String("https") || (allowTg && scheme == QLatin1String("tg"));
        return url.isValid() && allowed ? url : QUrl();
    }

    // Attribute selectors are only re-evaluated on polish.
    void setLevel(QWidget *widget, bool danger) {
        const QString level = danger ? QStringLiteral("danger") : QStringLiteral("ok");
        if (widget->property("level").toString() == level) return;
        widget->setProperty("level", level);
        widget->style()->unpolish(widget);
        widget->style()->polish(widget);
    }

    QToolButton *iconButton(const QString &icon, QWidget *parent) {
        auto *button = new QToolButton(parent);
        button->setIcon(PorosTheme::Icon(icon));
        button->setIconSize({15, 15});
        button->setPopupMode(QToolButton::InstantPopup);
        return button;
    }
} // namespace

SubscriptionInfoCard::SubscriptionInfoCard(QWidget *parent) : QFrame(parent) {
    setProperty("role", QStringLiteral("toolbar"));

    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(14, 10, 14, 10);
    row->setSpacing(16);

    auto *text = new QVBoxLayout;
    text->setSpacing(1);
    m_titleLabel = new QLabel(this);
    m_titleLabel->setProperty("role", QStringLiteral("subheading"));
    m_titleLabel->setTextFormat(Qt::PlainText);
    m_titleLabel->setSizePolicy(QSizePolicy::Ignored, QSizePolicy::Preferred);
    text->addWidget(m_titleLabel);
    m_detailLabel = new QLabel(this);
    m_detailLabel->setProperty("role", QStringLiteral("hint"));
    m_detailLabel->setTextFormat(Qt::PlainText);
    m_detailLabel->setSizePolicy(QSizePolicy::Ignored, QSizePolicy::Preferred);
    text->addWidget(m_detailLabel);
    row->addLayout(text, 1);

    m_quota = new QWidget(this);
    m_quota->setFixedWidth(280);
    auto *quotaColumn = new QVBoxLayout(m_quota);
    quotaColumn->setContentsMargins(0, 0, 0, 0);
    quotaColumn->setSpacing(4);
    auto *quotaRow = new QHBoxLayout;
    quotaRow->setSpacing(8);
    m_quotaLabel = new QLabel(m_quota);
    m_quotaLabel->setProperty("role", QStringLiteral("quota"));
    m_quotaLabel->setTextFormat(Qt::PlainText);
    quotaRow->addWidget(m_quotaLabel);
    quotaRow->addStretch();
    m_expiryLabel = new QLabel(m_quota);
    m_expiryLabel->setProperty("role", QStringLiteral("hint"));
    m_expiryLabel->setTextFormat(Qt::PlainText);
    quotaRow->addWidget(m_expiryLabel);
    quotaColumn->addLayout(quotaRow);
    m_progressBar = new QProgressBar(m_quota);
    m_progressBar->setTextVisible(false);
    m_progressBar->setRange(0, 100);
    m_progressBar->setFixedHeight(5);
    quotaColumn->addWidget(m_progressBar);
    row->addWidget(m_quota);

    auto *buttons = new QHBoxLayout;
    buttons->setSpacing(4);
    m_announceButton = iconButton(QStringLiteral("info"), this);
    connect(m_announceButton, &QToolButton::clicked, this, [this] {
        if (m_group != nullptr) MessageBoxScrollable(tr("Announcement - %1").arg(m_group->name), m_group->sub_info.announce.trimmed());
    });
    buttons->addWidget(m_announceButton);
    m_portalButton = iconButton(QStringLiteral("globe"), this);
    connect(m_portalButton, &QToolButton::clicked, this, [this] { QDesktopServices::openUrl(m_webUrl); });
    buttons->addWidget(m_portalButton);
    m_supportButton = iconButton(QStringLiteral("external"), this);
    connect(m_supportButton, &QToolButton::clicked, this, [this] { QDesktopServices::openUrl(m_supportUrl); });
    buttons->addWidget(m_supportButton);
    m_updateButton = new QPushButton(PorosTheme::Icon(QStringLiteral("refresh")), tr("Update"), this);
    m_updateButton->setIconSize({14, 14});
    m_updateButton->setProperty("role", QStringLiteral("small"));
    buttons->addWidget(m_updateButton);
    m_editButton = iconButton(QStringLiteral("edit"), this);
    m_editButton->setToolTip(tr("Edit group"));
    buttons->addWidget(m_editButton);
    m_moreButton = iconButton(QStringLiteral("more"), this);
    m_moreButton->setToolTip(tr("Group actions"));
    buttons->addWidget(m_moreButton);
    row->addLayout(buttons);

    connect(Subscription::updater(), &Subscription::GroupUpdater::asyncUpdateCallback, this, [this](int gid) {
        if (m_group && (gid < 0 || gid == m_group->id)) updateData(); });

    auto *countdownTimer = new QTimer(this);
    countdownTimer->setInterval(60000);
    connect(countdownTimer, &QTimer::timeout, this, [this] {
        if (isVisible()) updateData(); });
    countdownTimer->start();
}

void SubscriptionInfoCard::setActions(QAction *update, QAction *edit, QMenu *more) {
    connect(m_updateButton, &QPushButton::clicked, update, &QAction::trigger);
    connect(m_editButton, &QToolButton::clicked, edit, &QAction::trigger);
    m_moreButton->setMenu(more);
}

QString SubscriptionInfoCard::expiryText(qint64 expire) {
    if (expire <= 0)
        return {};
    const qint64 left = expire - QDateTime::currentSecsSinceEpoch();
    if (left < 0)
        return tr("Expired");
    if (left >= 86400)
        return tr("%1d %2h left").arg(left / 86400).arg(left % 86400 / 3600);
    if (left >= 3600)
        return tr("%1h %2m left").arg(left / 3600).arg(left % 3600 / 60);
    return tr("%1m left").arg(std::max<qint64>(1, left / 60));
}

void SubscriptionInfoCard::setGroup(const std::shared_ptr<Configs::Group> &group) {
    m_group = group;
    updateData();
}

void SubscriptionInfoCard::resizeEvent(QResizeEvent *event) {
    QFrame::resizeEvent(event);
    showTitle();
}

void SubscriptionInfoCard::showTitle() {
    if (m_group == nullptr) return;
    m_titleLabel->setText(m_titleLabel->fontMetrics().elidedText(m_group->name, Qt::ElideRight, m_titleLabel->width()));
}

void SubscriptionInfoCard::updateData() {
    if (m_group == nullptr) return;
    showTitle();

    const bool subscription = !m_group->url.isEmpty();
    const auto &sub = m_group->sub_info;
    m_updateButton->setVisible(subscription);

    QStringList tip{m_group->name};
    if (subscription) {
        QStringList detail;
        if (const QString host = QUrl(m_group->url).host(); !host.isEmpty()) detail << host;
        if (m_group->sub_last_update > 0) {
            detail << tr("updated %1 ago").arg(Stats::HumanizeDuration(QDateTime::currentSecsSinceEpoch() - m_group->sub_last_update));
            tip << tr("Last updated: %1").arg(DisplayTime(m_group->sub_last_update, QLocale::ShortFormat));
        }
        m_detailLabel->setText(detail.join(QStringLiteral(" · ")));

        if (!sub.title.isEmpty() && sub.title != m_group->name) tip << tr("Provider: %1").arg(sub.title);
        const auto plan = Subscription::ResolveAutoUpdate(*m_group);
        if (plan.interval > 0) {
            tip << tr("Auto-update: every %1").arg(Stats::HumanizeDuration(plan.interval));
            switch (plan.source) {
            case Subscription::AutoUpdatePlan::Source::server:
                tip << tr("Interval provided by the server");
                break;
            case Subscription::AutoUpdatePlan::Source::group:
                tip << tr("Interval set for this group");
                break;
            default:
                tip << tr("Global interval from Settings");
                break;
            }
            if (plan.source != Subscription::AutoUpdatePlan::Source::server && sub.server_interval > 0)
                tip << tr("Server suggests every %n hour(s)", nullptr, sub.server_interval);
        }
    } else {
        m_detailLabel->setText(tr("Your own group, profiles added by hand"));
    }
    m_titleLabel->setToolTip(Qt::convertFromPlainText(tip.join(QLatin1Char('\n'))));
    m_detailLabel->setToolTip(m_titleLabel->toolTip());

    const bool known = subscription && sub.valid;
    const bool hasQuota = known && sub.has_quota;
    const bool hasExpiry = known && sub.expire > 0;
    m_quota->setVisible(hasQuota || hasExpiry);
    m_quotaLabel->setVisible(hasQuota);
    m_progressBar->setVisible(hasQuota && sub.total > 0);
    if (hasQuota) {
        const int pct = static_cast<int>(sub.percentUsed());
        m_progressBar->setValue(pct);
        setLevel(m_progressBar, sub.isExpired() || pct >= 90);
        const QString used = ReadableSize(sub.used());
        m_quotaLabel->setText(sub.total > 0 ? tr("%1 of %2").arg(used, ReadableSize(sub.total)) : used);
    }
    m_expiryLabel->setVisible(hasExpiry);
    if (hasExpiry) {
        m_expiryLabel->setText(expiryText(sub.expire));
        m_expiryLabel->setToolTip(Qt::convertFromPlainText(tr("Expires: %1").arg(DisplayTime(sub.expire, QLocale::ShortFormat))));
        setLevel(m_expiryLabel, (sub.expire - QDateTime::currentSecsSinceEpoch()) / 86400 <= subscriptionUrgentDays);
    }

    m_webUrl = known ? linkUrl(sub.web_url, false) : QUrl();
    m_supportUrl = known ? linkUrl(sub.support_url, true) : QUrl();
    m_portalButton->setVisible(!m_webUrl.isEmpty());
    m_portalButton->setToolTip(Qt::convertFromPlainText(tr("Website / Portal: %1").arg(sub.web_url.trimmed())));
    m_supportButton->setVisible(!m_supportUrl.isEmpty());
    m_supportButton->setToolTip(Qt::convertFromPlainText(tr("Technical Support: %1").arg(sub.support_url.trimmed())));

    const QString announce = sub.announce.trimmed();
    const bool hasAnnounce = known && !announce.isEmpty() && announce.compare(QLatin1String("base64:"), Qt::CaseInsensitive) != 0;
    m_announceButton->setVisible(hasAnnounce);
    if (hasAnnounce) m_announceButton->setToolTip(Qt::convertFromPlainText(announce, Qt::WhiteSpaceNormal));
}
