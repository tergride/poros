#include "include/ui/simple/AddSubscriptionDialog.h"

#include <QApplication>
#include <QClipboard>
#include <QHBoxLayout>
#include <QLabel>
#include <QLineEdit>
#include <QPointer>
#include <QPushButton>
#include <QUrl>
#include <QVBoxLayout>

#include "include/configs/sub/LinkKind.h"
#include "include/engine/services/GroupService.h"
#include "include/ui/setting/PorosTheme.h"

AddSubscriptionDialog::AddSubscriptionDialog(const QString &link, QWidget *parent) : QDialog(parent) {
    setWindowTitle(tr("Add subscription"));
    setMinimumWidth(520);
    auto *column = new QVBoxLayout(this);
    column->setContentsMargins(20, 18, 20, 16);
    column->setSpacing(10);

    auto *title = new QLabel(tr("Add subscription"), this);
    title->setProperty("role", QStringLiteral("subheading"));
    column->addWidget(title);

    column->addWidget(new QLabel(tr("Link"), this));
    auto *row = new QHBoxLayout;
    row->setSpacing(8);
    m_link = new QLineEdit(link, this);
    m_link->setPlaceholderText(QStringLiteral("https://…"));
    row->addWidget(m_link, 1);
    auto *paste = new QPushButton(PorosTheme::Icon(QStringLiteral("clip")), tr("Paste"), this);
    paste->setAutoDefault(false);
    connect(paste, &QPushButton::clicked, this, [this] { m_link->setText(QApplication::clipboard()->text().trimmed()); });
    row->addWidget(paste);
    column->addLayout(row);

    m_status = new QLabel(this);
    m_status->setWordWrap(true);
    m_status->setTextFormat(Qt::RichText);
    m_status->hide();
    connect(m_status, &QLabel::linkActivated, this, [this] {
        emit advancedModeRequested();
        reject();
    });
    column->addWidget(m_status);

    column->addWidget(new QLabel(tr("Name"), this));
    m_name = new QLineEdit(this);
    m_name->setPlaceholderText(tr("Taken from the subscription"));
    column->addWidget(m_name);

    auto *hint = new QLabel(tr("The subscription appears on the main screen as a separate group."), this);
    hint->setProperty("role", QStringLiteral("hint"));
    hint->setWordWrap(true);
    column->addWidget(hint);
    column->addSpacing(6);

    auto *buttons = new QHBoxLayout;
    m_cancel = new QPushButton(tr("Cancel"), this);
    m_cancel->setProperty("variant", QStringLiteral("ghost"));
    m_cancel->setAutoDefault(false);
    connect(m_cancel, &QPushButton::clicked, this, &QDialog::reject);
    buttons->addWidget(m_cancel);
    buttons->addStretch();
    m_add = new QPushButton(tr("Add"), this);
    m_add->setProperty("variant", QStringLiteral("primary"));
    m_add->setDefault(true);
    connect(m_add, &QPushButton::clicked, this, &QDialog::accept);
    buttons->addWidget(m_add);
    column->addLayout(buttons);

    connect(m_link, &QLineEdit::textChanged, this, &AddSubscriptionDialog::syncLink);
    syncLink();
}

void AddSubscriptionDialog::setStatus(const QString &text, bool problem) {
    m_status->setVisible(!text.isEmpty());
    m_status->setText(text);
    m_status->setProperty("colorRole", problem ? QStringLiteral("danger") : QStringLiteral("muted"));
    m_status->setStyleSheet({});
    m_status->style()->unpolish(m_status);
    m_status->style()->polish(m_status);
}

void AddSubscriptionDialog::syncLink() {
    if (m_busy) return;
    using Subscription::LinkKind;
    const auto kind = Subscription::ClassifyLink(m_link->text());
    m_add->setEnabled(kind == LinkKind::Subscription);
    if (kind == LinkKind::SingleServer) {
        setStatus(tr("This link is one server, not a subscription. <a href=\"advanced\">You can add it in the advanced mode.</a>"), false);
    } else if (kind == LinkKind::Unknown) {
        setStatus(tr("This does not look like a subscription link."), true);
    } else {
        setStatus({}, false);
    }
}

void AddSubscriptionDialog::accept() {
    if (m_busy) return;
    const QString url = m_link->text().trimmed();
    QString name = m_name->text().trimmed();
    if (name.isEmpty()) name = QUrl(url).host();

    if (const auto refusal = Engine::GroupService::Add({name, url}, &m_groupId)) {
        setStatus(refusal->message, true);
        return;
    }
    m_busy = true;
    m_add->setEnabled(false);
    m_link->setEnabled(false);
    m_name->setEnabled(false);
    setStatus(tr("Checking the link…"), false);

    const QPointer<AddSubscriptionDialog> self(this);
    const int groupId = m_groupId;
    const auto refusal = Engine::GroupService::Update(groupId, [self, groupId](const Subscription::UpdateOutcome &outcome) {
        if (outcome.ok()) {
            if (self) self->QDialog::accept();
            return;
        }
        // Nothing to show for it yet, so the group goes with the failed link.
        Engine::GroupService::Delete(groupId);
        if (!self) return;
        self->m_groupId = -1;
        self->m_busy = false;
        self->m_link->setEnabled(true);
        self->m_name->setEnabled(true);
        self->m_add->setEnabled(true);
        self->setStatus(outcome.error.isEmpty() ? tr("No servers were found at this link.") : outcome.error, true);
    });
    if (refusal) {
        Engine::GroupService::Delete(groupId);
        m_groupId = -1;
        m_busy = false;
        m_link->setEnabled(true);
        m_name->setEnabled(true);
        m_add->setEnabled(true);
        setStatus(refusal->message, true);
    }
}
