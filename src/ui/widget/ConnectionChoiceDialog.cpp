#include "include/ui/widget/ConnectionChoiceDialog.h"

#include <QHBoxLayout>
#include <QLabel>
#include <QPushButton>
#include <QVBoxLayout>

#include "include/ui/widget/ChoiceCard.h"

ConnectionChoiceDialog::ConnectionChoiceDialog(QWidget *parent) : QDialog(parent) {
    setWindowTitle(tr("How should Poros connect?"));
    setMinimumWidth(560);
    auto *column = new QVBoxLayout(this);
    column->setContentsMargins(20, 18, 20, 16);
    column->setSpacing(12);

    auto *title = new QLabel(tr("How should Poros connect?"), this);
    title->setProperty("role", QStringLiteral("subheading"));
    column->addWidget(title);

    auto *text = new QLabel(tr("Poros remembers the answer, and you can change it next to the Start button."), this);
    text->setProperty("role", QStringLiteral("hint"));
    text->setWordWrap(true);
    column->addWidget(text);

    auto *cards = new QHBoxLayout;
    cards->setSpacing(12);
    auto *tun = new ChoiceCard(QStringLiteral("TUN"), tr("All the traffic of the computer, games and programs included."),
                               {tr("Needs administrator rights")}, this);
    auto *system = new ChoiceCard(QStringLiteral("System"), tr("Browsers and the programs that use the system proxy."),
                                  {tr("Works without administrator rights")}, this);
    cards->addWidget(tun, 1);
    cards->addWidget(system, 1);
    column->addLayout(cards);
    connect(tun, &QAbstractButton::clicked, this, [this] {
        m_choice = ConnectionModeController::Mode::Tun;
        accept();
    });
    connect(system, &QAbstractButton::clicked, this, [this] {
        m_choice = ConnectionModeController::Mode::System;
        accept();
    });

    auto *buttons = new QHBoxLayout;
    buttons->addStretch();
    auto *cancel = new QPushButton(tr("Cancel"), this);
    cancel->setProperty("variant", QStringLiteral("ghost"));
    connect(cancel, &QPushButton::clicked, this, &QDialog::reject);
    buttons->addWidget(cancel);
    column->addLayout(buttons);
}
