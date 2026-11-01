#include "include/ui/onboarding/OnboardingPage.h"

#include <QApplication>
#include <QButtonGroup>
#include <QCheckBox>
#include <QClipboard>
#include <QHBoxLayout>
#include <QLabel>
#include <QLineEdit>
#include <QMouseEvent>
#include <QPainter>
#include <QPointer>
#include <QPushButton>
#include <QRadioButton>
#include <QStackedWidget>
#include <QTimer>
#include <QUrl>
#include <QVBoxLayout>

#include <algorithm>

#include "include/configs/PresetCatalog.h"
#include "include/configs/sub/LinkKind.h"
#include "include/database/GroupsRepo.h"
#include "include/database/RoutesRepo.h"
#include "include/engine/services/GroupService.h"
#include "include/global/Configs.hpp"
#include "include/sys/AutoRun.hpp"
#include "include/sys/UrlScheme.hpp"
#include "include/ui/setting/PorosTheme.h"
#include "include/ui/widget/ChoiceCard.h"

namespace {
    constexpr int onboardingArtWidth = 470;
    constexpr int onboardingLinkStep = 1;

    class OnboardingStepper : public QWidget {
    public:
        OnboardingStepper(int *step, QWidget *parent) : QWidget(parent), m_step(step) {
            setFixedHeight(26);
            m_names = {OnboardingPage::tr("Mode"), OnboardingPage::tr("Subscription"), OnboardingPage::tr("Launch"), OnboardingPage::tr("Routing")};
        }

    protected:
        void paintEvent(QPaintEvent *) override {
            using namespace PorosTheme;
            QPainter painter(this);
            painter.setRenderHint(QPainter::Antialiasing);
            int widths = 0;
            for (const auto &name : m_names) widths += 26 + 9 + fontMetrics().horizontalAdvance(name);
            const int gap = (width() - widths) / qMax<int>(1, m_names.size() - 1);
            int x = 0;
            for (int i = 0; i < m_names.size(); i++) {
                const bool done = i < *m_step;
                const bool current = i == *m_step;
                const QRectF circle(x + 0.5, 0.5, 25, 25);
                if (current) {
                    painter.setPen(Qt::NoPen);
                    painter.setBrush(PrimaryGradient(circle));
                } else {
                    painter.setPen(QPen(QColor(done ? Act : Line2), 1));
                    painter.setBrush(QColor(done ? Act : S3));
                }
                painter.drawEllipse(circle);
                if (done) {
                    PorosTheme::Icon(QStringLiteral("check"), Qt::white).paint(&painter, QRect(x + 6, 6, 14, 14));
                } else {
                    painter.setFont(MonoFont(12));
                    painter.setPen(current ? QColor(Qt::white) : QColor(T2));
                    painter.drawText(circle, Qt::AlignCenter, QString::number(i + 1));
                }
                painter.setFont(font());
                painter.setPen(QColor(current ? T1 : T3));
                const int textWidth = fontMetrics().horizontalAdvance(m_names[i]);
                painter.drawText(QRect(x + 35, 0, textWidth + 2, 26), Qt::AlignVCenter | Qt::AlignLeft, m_names[i]);
                x += 35 + textWidth;
                if (i + 1 < m_names.size()) {
                    const QRectF line(x + 10, 12, gap - 20, 2);
                    painter.setPen(Qt::NoPen);
                    if (done) painter.setBrush(RoadGradient(line));
                    else painter.setBrush(QColor(Line2));
                    painter.drawRoundedRect(line, 1, 1);
                    x += gap;
                }
            }
        }

    private:
        int *m_step;
        QStringList m_names;
    };

    // The whole row toggles the box. An exclusive radio button stays on, as it does when clicked.
    template <typename Toggle>
    class OnboardingRow : public QFrame {
    public:
        OnboardingRow(const QString &title, const QString &text, QWidget *parent) : QFrame(parent) {
            setProperty("role", QStringLiteral("checkRow"));
            setCursor(Qt::PointingHandCursor);
            auto *row = new QHBoxLayout(this);
            row->setContentsMargins(16, 14, 16, 14);
            row->setSpacing(14);
            box = new Toggle(this);
            row->addWidget(box, 0, Qt::AlignTop);
            auto *column = new QVBoxLayout;
            column->setSpacing(2);
            auto *titleLabel = new QLabel(title, this);
            titleLabel->setProperty("role", QStringLiteral("cardTitle"));
            column->addWidget(titleLabel);
            auto *textLabel = new QLabel(text, this);
            textLabel->setProperty("role", QStringLiteral("hint"));
            textLabel->setWordWrap(true);
            column->addWidget(textLabel);
            row->addLayout(column, 1);
        }

        Toggle *box;

    protected:
        void mouseReleaseEvent(QMouseEvent *event) override {
            if (event->button() == Qt::LeftButton && rect().contains(event->position().toPoint())) box->toggle();
        }
    };

    QWidget *onboardingStep(const QString &title, const QString &text, QVBoxLayout **body, QWidget *parent) {
        auto *page = new QWidget(parent);
        auto *column = new QVBoxLayout(page);
        column->setContentsMargins(0, 0, 0, 0);
        column->setSpacing(16);
        auto *heading = new QLabel(title, page);
        heading->setProperty("role", QStringLiteral("wizardTitle"));
        heading->setWordWrap(true);
        column->addWidget(heading);
        if (!text.isEmpty()) {
            auto *lead = new QLabel(text, page);
            lead->setWordWrap(true);
            lead->setProperty("role", QStringLiteral("lead"));
            column->addWidget(lead);
        }
        *body = column;
        return page;
    }
} // namespace

OnboardingPage::OnboardingPage(QWidget *parent)
    : QWidget(parent), m_art(QStringLiteral(":/poros/img/portal.jpg")), m_wordmark(QStringLiteral(":/poros/img/wordmark.png")) {
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(onboardingArtWidth, 0, 0, 0);
    row->setSpacing(0);

    auto *main = new QWidget(this);
    main->setObjectName(QStringLiteral("wizardMain"));
    main->setAttribute(Qt::WA_StyledBackground);
    auto *column = new QVBoxLayout(main);
    column->setContentsMargins(44, 28, 44, 16);
    column->setSpacing(0);
    row->addWidget(main, 1);

    m_stepper = new OnboardingStepper(&m_step, main);
    column->addWidget(m_stepper);
    column->addSpacing(40);

    m_steps = new QStackedWidget(main);
    column->addWidget(m_steps, 1);

    QVBoxLayout *body = nullptr;
    auto *modeStep = onboardingStep(tr("Choose the interface mode"), {}, &body, m_steps);
    auto *cards = new QHBoxLayout;
    cards->setSpacing(12);
    m_simple = new ChoiceCard(tr("Simple"), {}, {tr("The server list and a Start button"), tr("Subscriptions as groups"), tr("Ready presets and your own copies"), tr("Only the settings you need")}, modeStep);
    m_advanced = new ChoiceCard(tr("Advanced"), {}, {tr("Everything from the simple mode"), tr("Your own groups and manual profiles"), tr("Routing rules, DNS, WARP"), tr("Logs, connections, tools")}, modeStep);
    m_simple->setAutoExclusive(true);
    m_advanced->setAutoExclusive(true);
    cards->addWidget(m_simple, 1);
    cards->addWidget(m_advanced, 1);
    body->addLayout(cards);
    auto *modeHint = new QLabel(tr("You can change the mode at any time in the app menu."), modeStep);
    modeHint->setProperty("role", QStringLiteral("hint"));
    body->addWidget(modeHint);
    body->addStretch();
    m_steps->addWidget(modeStep);

    auto *linkStep = onboardingStep(tr("Add a subscription"),
                                    tr("Paste the link from your provider and Poros will load the server list. It usually comes in an email or a bot after payment."),
                                    &body, m_steps);
    auto *linkRow = new QHBoxLayout;
    linkRow->setSpacing(8);
    m_link = new QLineEdit(linkStep);
    m_link->setProperty("role", QStringLiteral("big"));
    m_link->setPlaceholderText(QStringLiteral("https://…"));
    linkRow->addWidget(m_link, 1);
    auto *paste = new QPushButton(PorosTheme::Icon(QStringLiteral("clip")), tr("Paste"), linkStep);
    paste->setProperty("role", QStringLiteral("big"));
    paste->setAutoDefault(false);
    connect(paste, &QPushButton::clicked, this, [this] { m_link->setText(QApplication::clipboard()->text().trimmed()); });
    linkRow->addWidget(paste);
    auto *qr = new QPushButton(PorosTheme::Icon(QStringLiteral("qr")), {}, linkStep);
    qr->setProperty("role", QStringLiteral("big"));
    qr->setToolTip(tr("Scan a QR code on the screen"));
    connect(qr, &QPushButton::clicked, this, &OnboardingPage::qrScanRequested);
    linkRow->addWidget(qr);
    body->addLayout(linkRow);

    m_linkLine = new QFrame(linkStep);
    m_linkLine->setProperty("role", QStringLiteral("statusLine"));
    auto *statusRow = new QHBoxLayout(m_linkLine);
    statusRow->setContentsMargins(14, 10, 14, 10);
    statusRow->setSpacing(10);
    m_linkIcon = new QLabel(m_linkLine);
    statusRow->addWidget(m_linkIcon);
    m_linkStatus = new QLabel(m_linkLine);
    m_linkStatus->setWordWrap(true);
    statusRow->addWidget(m_linkStatus, 1);
    m_linkLine->hide();
    body->addWidget(m_linkLine);
    body->addStretch();
    m_steps->addWidget(linkStep);

    auto *launchStep = onboardingStep(tr("How to start Poros"), {}, &body, m_steps);
    auto *autoRow = new OnboardingRow<QCheckBox>(tr("Start with Windows"), tr("Poros opens when you sign in."), launchStep);
    auto *rememberRow = new OnboardingRow<QCheckBox>(tr("Connect to the last profile"), tr("After starting, Poros connects to the server that worked last time."), launchStep);
    auto *linksRow = new OnboardingRow<QCheckBox>(tr("Accept poros:// links"), tr("A link from a site or a messenger opens right in Poros."), launchStep);
    m_autoStart = autoRow->box;
    m_remember = rememberRow->box;
    m_links = linksRow->box;
    body->setSpacing(10);
    body->addWidget(autoRow);
    body->addWidget(rememberRow);
    body->addWidget(linksRow);
    auto *launchHint = new QLabel(tr("Windows keeps the autostart and the links outside the Poros folder. You can turn them off in the settings."), launchStep);
    launchHint->setProperty("role", QStringLiteral("hint"));
    launchHint->setWordWrap(true);
    body->addWidget(launchHint);
    body->addStretch();
    m_steps->addWidget(launchStep);

    auto *routeStep = onboardingStep(tr("Which traffic goes through the server"), {}, &body, m_steps);
    body->setSpacing(10);
    m_routes = new QButtonGroup(this);
    const auto routes = Configs::dataManager->routesRepo->GetAllRouteProfiles();
    for (const auto &key : Configs::PresetCatalog::Keys()) {
        const auto route = std::ranges::find_if(routes, [&key](const auto &candidate) { return candidate->builtinKey == key; });
        if (route == routes.end()) continue;
        auto *routeRow = new OnboardingRow<QRadioButton>((*route)->name, Configs::PresetCatalog::Description(key), routeStep);
        m_routes->addButton(routeRow->box, (*route)->id);
        body->addWidget(routeRow);
    }
    auto *routeHint = new QLabel(tr("You can change the preset at any time on the main screen."), routeStep);
    routeHint->setProperty("role", QStringLiteral("hint"));
    body->addWidget(routeHint);
    body->addStretch();
    m_steps->addWidget(routeStep);

    auto *footer = new QHBoxLayout;
    footer->setSpacing(8);
    auto *skip = new QPushButton(tr("Skip"), main);
    skip->setProperty("variant", QStringLiteral("ghost"));
    skip->setAutoDefault(false);
    connect(skip, &QPushButton::clicked, this, [this] { emit finished(m_step > 0 ? chosenMode() : Configs::dataManager->settingsRepo->ui_mode, serverLink()); });
    footer->addWidget(skip);
    footer->addStretch();
    m_back = new QPushButton(PorosTheme::Icon(QStringLiteral("back")), tr("Back"), main);
    m_back->setProperty("variant", QStringLiteral("ghost"));
    m_back->setAutoDefault(false);
    connect(m_back, &QPushButton::clicked, this, [this] { showStep(m_step - 1); });
    footer->addWidget(m_back);
    m_next = new QPushButton(main);
    m_next->setProperty("variant", QStringLiteral("primary"));
    m_next->setDefault(true);
    connect(m_next, &QPushButton::clicked, this, [this] {
        if (m_step < m_steps->count() - 1) {
            showStep(m_step + 1);
            return;
        }
        applyChoices();
        emit finished(chosenMode(), serverLink());
    });
    footer->addWidget(m_next);
    column->addSpacing(16);
    column->addLayout(footer);

    m_linkTimer = new QTimer(this);
    m_linkTimer->setSingleShot(true);
    m_linkTimer->setInterval(500);
    connect(m_linkTimer, &QTimer::timeout, this, &OnboardingPage::checkLink);
    connect(m_link, &QLineEdit::textChanged, this, [this] {
        m_linkToken++;
        dropCheckedGroup();
        m_linkTimer->start();
    });
    restart();
}

void OnboardingPage::restart() {
    const auto &settings = Configs::dataManager->settingsRepo;
    m_autoStart->setChecked(AutoRun_IsEnabled());
    m_remember->setChecked(settings->onboarding_done ? settings->remember_enable : true);
    m_links->setChecked(settings->onboarding_done ? settings->url_scheme_auto_register : true);
    (settings->ui_mode == QLatin1String("advanced") ? m_advanced : m_simple)->setChecked(true);
    // A preset of the user's own has no row here, so no row stays chosen and finishing keeps it.
    m_routes->setExclusive(false);
    for (auto *button : m_routes->buttons()) button->setChecked(m_routes->id(button) == settings->current_route_id);
    m_routes->setExclusive(true);
    m_groupId = -1;
    m_linkToken++;
    m_link->clear();
    m_linkLine->hide();
    showStep(0);
}

void OnboardingPage::setLink(const QString &link) {
    m_link->setText(link);
}

QString OnboardingPage::chosenMode() const {
    return m_advanced->isChecked() ? QStringLiteral("advanced") : QStringLiteral("simple");
}

QString OnboardingPage::serverLink() const {
    const QString link = m_link->text().trimmed();
    return m_advanced->isChecked() && Subscription::ClassifyLink(link) == Subscription::LinkKind::SingleServer ? link : QString();
}

void OnboardingPage::showStep(int step) {
    m_step = step;
    // What a one-server link means depends on the mode, which the user may have just changed.
    if (step == onboardingLinkStep && Subscription::ClassifyLink(m_link->text().trimmed()) == Subscription::LinkKind::SingleServer) checkLink();
    m_steps->setCurrentIndex(step);
    m_back->setVisible(step > 0);
    const bool last = step == m_steps->count() - 1;
    m_next->setText(last ? tr("Done") : tr("Next"));
    m_next->setIcon(PorosTheme::Icon(last ? QStringLiteral("check") : QStringLiteral("right"), Qt::white));
    m_stepper->update();
}

void OnboardingPage::setLinkStatus(const QString &text, const QString &icon, bool problem) {
    m_linkLine->setVisible(!text.isEmpty());
    m_linkStatus->setText(text);
    if (!icon.isEmpty()) {
        const QColor color(problem ? PorosTheme::Warn : PorosTheme::Ok);
        m_linkIcon->setPixmap(PorosTheme::Icon(icon, color).pixmap(16, 16));
    }
}

void OnboardingPage::dropCheckedGroup() {
    if (m_groupId < 0) return;
    Engine::GroupService::Delete(std::exchange(m_groupId, -1));
}

void OnboardingPage::checkLink() {
    using Subscription::LinkKind;
    const QString link = m_link->text().trimmed();
    switch (Subscription::ClassifyLink(link)) {
        case LinkKind::Empty:
            setLinkStatus({}, {}, false);
            return;
        case LinkKind::SingleServer:
            if (m_advanced->isChecked()) {
                setLinkStatus(tr("This link is one server. Poros will add it as a profile."), QStringLiteral("check"), false);
            } else {
                setLinkStatus(tr("This link is one server, not a subscription. Manual servers are added in the advanced mode, which you can choose at step 1."),
                              QStringLiteral("info"), true);
            }
            return;
        case LinkKind::Unknown:
            setLinkStatus(tr("This does not look like a subscription link."), QStringLiteral("warn"), true);
            return;
        case LinkKind::Subscription:
            break;
    }

    setLinkStatus(tr("Checking the link…"), QStringLiteral("refresh"), false);
    int groupId = -1;
    if (const auto refusal = Engine::GroupService::Add({QUrl(link).host(), link}, &groupId)) {
        setLinkStatus(refusal->message, QStringLiteral("warn"), true);
        return;
    }
    const int token = m_linkToken;
    const QPointer<OnboardingPage> self(this);
    const auto refusal = Engine::GroupService::Update(groupId, [self, token, groupId](const Subscription::UpdateOutcome &outcome) {
        // Superseded by another link, or the wizard is gone: this group has no owner any more.
        if (self == nullptr || token != self->m_linkToken) {
            Engine::GroupService::Delete(groupId);
            return;
        }
        if (!outcome.ok()) {
            Engine::GroupService::Delete(groupId);
            self->setLinkStatus(outcome.error.isEmpty() ? tr("No servers were found at this link.") : outcome.error, QStringLiteral("warn"), true);
            return;
        }
        self->m_groupId = groupId;
        const auto group = Configs::dataManager->groupsRepo->GetGroup(groupId);
        const int count = group != nullptr ? static_cast<int>(group->Profiles().size()) : 0;
        self->setLinkStatus(tr("Subscription found: %n server(s)", nullptr, count), QStringLiteral("check"), false);
    });
    if (refusal) {
        Engine::GroupService::Delete(groupId);
        setLinkStatus(refusal->message, QStringLiteral("warn"), true);
    }
}

void OnboardingPage::applyChoices() {
    auto &settings = Configs::dataManager->settingsRepo;
    if (m_autoStart->isChecked() != AutoRun_IsEnabled()) AutoRun_SetEnabled(m_autoStart->isChecked());
    settings->remember_enable = m_remember->isChecked();
    settings->url_scheme_auto_register = m_links->isChecked();
    if (m_links->isChecked()) UrlScheme_Install();
    else UrlScheme_Uninstall();
    settings->Save();
    if (m_routes->checkedId() >= 0) emit routeChosen(m_routes->checkedId());
}

void OnboardingPage::paintEvent(QPaintEvent *event) {
    QWidget::paintEvent(event);
    using namespace PorosTheme;
    QPainter painter(this);
    painter.setRenderHint(QPainter::SmoothPixmapTransform);
    const QRect art(0, 0, onboardingArtWidth, height());
    const QSize scaled = m_art.size().scaled(art.size(), Qt::KeepAspectRatioByExpanding);
    painter.drawPixmap(QRect(art.center() - QPoint(scaled.width() / 2, scaled.height() / 2), scaled), m_art);
    QLinearGradient fade(0, 0, 0, height());
    QColor clear(S0);
    clear.setAlpha(0);
    QColor solid(S0);
    solid.setAlpha(235);
    fade.setColorAt(0.55, clear);
    fade.setColorAt(1, solid);
    painter.fillRect(art, fade);

    constexpr int wordmarkHeight = 18;
    const int wordmarkWidth = wordmarkHeight * m_wordmark.width() / qMax(1, m_wordmark.height());
    const int top = height() - 88;
    painter.drawPixmap(QRect(32, top, wordmarkWidth, wordmarkHeight), m_wordmark);
    painter.setPen(QColor(T2));
    painter.drawText(QRect(32, top + wordmarkHeight + 12, 320, 40), Qt::TextWordWrap, tr("Quick setup takes less than a minute."));
}
