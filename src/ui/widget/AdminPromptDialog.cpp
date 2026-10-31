#include "include/ui/widget/AdminPromptDialog.h"

#include <QHBoxLayout>
#include <QLabel>
#include <QLayoutItem>
#include <QPushButton>
#include <QToolButton>
#include <QVBoxLayout>

#include "include/ui/setting/PorosTheme.h"

namespace {
    QString adminPromptTitle(Engine::PrivilegeReason reason) {
        switch (reason) {
            case Engine::PrivilegeReason::KillSwitch: return AdminPromptDialog::tr("Kill switch needs administrator rights");
            case Engine::PrivilegeReason::TlsSpoof: return AdminPromptDialog::tr("TLS spoofing needs administrator rights");
            default: return AdminPromptDialog::tr("TUN needs administrator rights");
        }
    }

    QString adminPromptText(Engine::PrivilegeReason reason) {
        switch (reason) {
            case Engine::PrivilegeReason::KillSwitch:
                return AdminPromptDialog::tr("The kill switch adds rules to the Windows firewall, so that traffic leaves only through Poros. Without rights the rules may not apply.");
            case Engine::PrivilegeReason::TlsSpoof:
                return AdminPromptDialog::tr("Spoofing works through the WinDivert driver, which Windows loads only for an administrator.");
            default:
                return AdminPromptDialog::tr("TUN creates a network adapter in Windows, which only an administrator may do. Poros will restart and connect to the server by itself.");
        }
    }
} // namespace

AdminPromptDialog::AdminPromptDialog(Engine::PrivilegeReason reason, QWidget *parent) : QDialog(parent) {
    setWindowTitle(adminPromptTitle(reason));
    setMinimumWidth(520);
    auto *column = new QVBoxLayout(this);
    column->setContentsMargins(20, 18, 20, 16);
    column->setSpacing(10);

    auto *title = new QLabel(adminPromptTitle(reason), this);
    title->setProperty("role", QStringLiteral("subheading"));
    title->setWordWrap(true);
    column->addWidget(title);

    auto *text = new QLabel(adminPromptText(reason), this);
    text->setWordWrap(true);
    column->addWidget(text);

    if (reason == Engine::PrivilegeReason::Tun) {
        auto *fallback = new QLabel(tr("System works without rights. <a href=\"system\">Switch to System</a>"), this);
        fallback->setProperty("role", QStringLiteral("hint"));
        fallback->setTextFormat(Qt::RichText);
        connect(fallback, &QLabel::linkActivated, this, [this] {
            m_choice = Choice::UseSystem;
            accept();
        });
        column->addWidget(fallback);
    }
    column->addSpacing(6);

    auto *buttons = new QHBoxLayout;
    buttons->setSpacing(8);
    auto *cancel = new QPushButton(tr("Cancel"), this);
    cancel->setProperty("variant", QStringLiteral("ghost"));
    cancel->setAutoDefault(false);
    connect(cancel, &QPushButton::clicked, this, &QDialog::reject);
    buttons->addWidget(cancel);
    buttons->addStretch();
    auto *without = new QPushButton(tr("Try without rights"), this);
    without->setAutoDefault(false);
    connect(without, &QPushButton::clicked, this, [this] {
        m_choice = Choice::TryWithout;
        accept();
    });
    buttons->addWidget(without);
    auto *restart = new QPushButton(PorosTheme::Icon(QStringLiteral("shield"), Qt::white), tr("Restart with rights"), this);
    restart->setProperty("variant", QStringLiteral("primary"));
    restart->setDefault(true);
    connect(restart, &QPushButton::clicked, this, [this] {
        m_choice = Choice::Restart;
        accept();
    });
    buttons->addWidget(restart);
    column->addLayout(buttons);
}

FailureBanner::FailureBanner(QWidget *parent) : QFrame(parent) {
    setObjectName(QStringLiteral("failureBanner"));
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(12, 8, 8, 8);
    row->setSpacing(10);

    auto *icon = new QLabel(this);
    icon->setPixmap(PorosTheme::Icon(QStringLiteral("warn"), QColor(PorosTheme::Warn)).pixmap(16, 16));
    row->addWidget(icon);

    m_text = new QLabel(this);
    m_text->setWordWrap(true);
    m_text->setTextFormat(Qt::PlainText);
    row->addWidget(m_text, 1);

    m_actions = new QHBoxLayout;
    m_actions->setSpacing(6);
    row->addLayout(m_actions);

    auto *close = new QToolButton(this);
    close->setIcon(PorosTheme::Icon(QStringLiteral("close")));
    close->setIconSize({14, 14});
    connect(close, &QToolButton::clicked, this, &FailureBanner::dismiss);
    row->addWidget(close);
    hide();
}

void FailureBanner::present(const QString &text, const QList<Action> &actions) {
    while (auto *item = m_actions->takeAt(0)) {
        delete item->widget();
        delete item;
    }
    m_text->setText(text);
    for (const auto &action : actions) {
        auto *button = new QPushButton(action.text, this);
        connect(button, &QPushButton::clicked, this, [this, run = action.run] {
            dismiss();
            run();
        });
        m_actions->addWidget(button);
    }
    show();
}

void FailureBanner::dismiss() {
    hide();
}
