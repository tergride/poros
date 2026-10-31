#include "include/ui/simple/PresetEditor.h"

#include <QComboBox>
#include <QFrame>
#include <QHBoxLayout>
#include <QLabel>
#include <QLayoutItem>
#include <QLineEdit>
#include <QPushButton>
#include <QScrollArea>
#include <QToolButton>
#include <QVBoxLayout>

#include "include/configs/PresetCatalog.h"
#include "include/database/RoutesRepo.h"
#include "include/global/Configs.hpp"
#include "include/ui/setting/PorosTheme.h"
#include "include/ui/utils/NoWheelComboBox.h"

namespace {
    using Configs::PresetRules::Action;

    struct PresetDns {
        const char *label;
        const char *address;
    };

    constexpr PresetDns presetEditorDns[] = {
        {QT_TRANSLATE_NOOP("PresetEditor", "As in the preset"), ""},
        {"Cloudflare", "https://1.1.1.1/dns-query"},
        {"Google", "https://8.8.8.8/dns-query"},
        {QT_TRANSLATE_NOOP("PresetEditor", "Yandex"), "https://77.88.8.8/dns-query"},
        {QT_TRANSLATE_NOOP("PresetEditor", "Custom…"), ""},
    };
    constexpr int presetEditorCustomDns = 4;

    QFrame *presetEditorCard(const QString &title, QVBoxLayout **column, QWidget *parent) {
        auto *card = new QFrame(parent);
        card->setProperty("role", QStringLiteral("card"));
        *column = new QVBoxLayout(card);
        (*column)->setContentsMargins(16, 12, 16, 12);
        (*column)->setSpacing(6);
        auto *label = new QLabel(title, card);
        label->setProperty("role", QStringLiteral("cardTitle"));
        (*column)->addWidget(label);
        return card;
    }

    void presetEditorClear(QLayout *layout) {
        while (auto *item = layout->takeAt(0)) {
            delete item->widget();
            delete item;
        }
    }

    QComboBox *presetEditorActionBox(Action action, QWidget *parent) {
        auto *box = new NoWheelComboBox(parent);
        box->addItem(PresetEditor::tr("Through the server"), int(Action::Proxy));
        box->addItem(PresetEditor::tr("Direct"), int(Action::Direct));
        box->addItem(PresetEditor::tr("Block"), int(Action::Block));
        box->setCurrentIndex(box->findData(int(action)));
        box->setFixedWidth(170);
        return box;
    }
} // namespace

PresetEditor::PresetEditor(QWidget *parent) : QWidget(parent) {
    auto *outer = new QVBoxLayout(this);
    outer->setContentsMargins(0, 0, 0, 0);
    outer->setSpacing(0);

    auto *scroll = new QScrollArea(this);
    scroll->setWidgetResizable(true);
    scroll->setFrameShape(QFrame::NoFrame);
    auto *page = new QWidget(scroll);
    auto *column = new QVBoxLayout(page);
    column->setContentsMargins(28, 22, 28, 22);
    column->setSpacing(12);
    scroll->setWidget(page);
    outer->addWidget(scroll, 1);

    auto *top = new QHBoxLayout;
    auto *back = new QPushButton(PorosTheme::Icon(QStringLiteral("back")), tr("Presets"), page);
    back->setProperty("variant", QStringLiteral("ghost"));
    connect(back, &QPushButton::clicked, this, &PresetEditor::closed);
    top->addWidget(back);
    m_base = new QLabel(page);
    m_base->setProperty("role", QStringLiteral("chip"));
    m_base->setFixedHeight(26);
    top->addWidget(m_base);
    top->addStretch();
    column->addLayout(top);

    column->addWidget(new QLabel(tr("Name"), page));
    m_name = new QLineEdit(page);
    m_name->setMaximumWidth(420);
    column->addWidget(m_name);

    auto *cards = new QHBoxLayout;
    cards->setSpacing(12);
    auto *left = new QVBoxLayout;
    auto *right = new QVBoxLayout;
    left->setSpacing(12);
    right->setSpacing(12);

    QVBoxLayout *sitesCard = nullptr;
    left->addWidget(presetEditorCard(tr("Sites and IPs"), &sitesCard, page));
    auto *siteRow = new QHBoxLayout;
    m_siteInput = new QLineEdit(page);
    m_siteInput->setPlaceholderText(tr("Site, IP or subnet"));
    siteRow->addWidget(m_siteInput, 1);
    auto *addSite = new QPushButton(PorosTheme::Icon(QStringLiteral("plus")), tr("Add"), page);
    connect(addSite, &QPushButton::clicked, this, [this] { addRule(false, m_siteInput); });
    connect(m_siteInput, &QLineEdit::returnPressed, this, [this] { addRule(false, m_siteInput); });
    siteRow->addWidget(addSite);
    sitesCard->addLayout(siteRow);
    m_sites = new QVBoxLayout;
    m_sites->setSpacing(4);
    sitesCard->addLayout(m_sites);
    m_locked = new QVBoxLayout;
    m_locked->setSpacing(4);
    sitesCard->addLayout(m_locked);
    left->addStretch();

    QVBoxLayout *programsCard = nullptr;
    right->addWidget(presetEditorCard(tr("Programs"), &programsCard, page));
    auto *programRow = new QHBoxLayout;
    m_programInput = new QLineEdit(page);
    m_programInput->setPlaceholderText(tr("File name, for example steam.exe"));
    programRow->addWidget(m_programInput, 1);
    auto *addProgram = new QPushButton(PorosTheme::Icon(QStringLiteral("plus")), tr("Add"), page);
    connect(addProgram, &QPushButton::clicked, this, [this] { addRule(true, m_programInput); });
    connect(m_programInput, &QLineEdit::returnPressed, this, [this] { addRule(true, m_programInput); });
    programRow->addWidget(addProgram);
    programsCard->addLayout(programRow);
    m_programs = new QVBoxLayout;
    m_programs->setSpacing(4);
    programsCard->addLayout(m_programs);

    QVBoxLayout *dnsCard = nullptr;
    right->addWidget(presetEditorCard(tr("DNS"), &dnsCard, page));
    m_dns = new NoWheelComboBox(page);
    for (const auto &dns : presetEditorDns) m_dns->addItem(tr(dns.label));
    dnsCard->addWidget(m_dns);
    m_dnsCustom = new QLineEdit(page);
    m_dnsCustom->setPlaceholderText(QStringLiteral("https://1.1.1.1/dns-query"));
    m_dnsCustom->hide();
    dnsCard->addWidget(m_dnsCustom);
    connect(m_dns, &QComboBox::currentIndexChanged, this, [this](int index) { m_dnsCustom->setVisible(index == presetEditorCustomDns); });
    right->addStretch();

    cards->addLayout(left, 1);
    cards->addLayout(right, 1);
    column->addLayout(cards);
    column->addStretch();

    auto *footer = new QFrame(this);
    footer->setProperty("role", QStringLiteral("footer"));
    auto *footerRow = new QHBoxLayout(footer);
    footerRow->setContentsMargins(20, 12, 20, 12);
    auto *cancel = new QPushButton(tr("Cancel"), footer);
    cancel->setProperty("variant", QStringLiteral("ghost"));
    connect(cancel, &QPushButton::clicked, this, &PresetEditor::closed);
    footerRow->addWidget(cancel);
    m_problem = new QLabel(footer);
    m_problem->setProperty("colorRole", QStringLiteral("danger"));
    m_problem->setWordWrap(true);
    footerRow->addWidget(m_problem, 1);
    auto *save = new QPushButton(tr("Save and apply"), footer);
    save->setProperty("variant", QStringLiteral("primary"));
    connect(save, &QPushButton::clicked, this, &PresetEditor::save);
    footerRow->addWidget(save);
    outer->addWidget(footer);
}

void PresetEditor::edit(const std::shared_ptr<Configs::RouteProfile> &preset) {
    m_preset = preset;
    m_rules = Configs::PresetRules::Read(*preset);
    m_name->setText(preset->name);
    m_problem->clear();
    const bool hasBase = !preset->baseKey.isEmpty();
    m_base->setVisible(hasBase);
    m_base->setText(tr("Copy of the preset “%1”").arg(Configs::PresetCatalog::DisplayName(preset->baseKey)));

    int dns = 0;
    m_dnsCustom->clear();
    if (const auto active = preset->ActiveDns()) {
        dns = presetEditorCustomDns;
        m_dnsCustom->setText(active->remote_dns);
        for (int i = 1; i < presetEditorCustomDns; i++) {
            if (active->remote_dns == QLatin1String(presetEditorDns[i].address)) dns = i;
        }
    }
    m_dns->setCurrentIndex(dns);
    m_dnsCustom->setVisible(dns == presetEditorCustomDns);

    presetEditorClear(m_locked);
    for (const auto &name : Configs::PresetRules::LockedRules(*preset)) {
        auto *row = new QWidget(this);
        auto *rowLayout = new QHBoxLayout(row);
        rowLayout->setContentsMargins(0, 2, 0, 2);
        auto *icon = new QLabel(row);
        icon->setPixmap(PorosTheme::Icon(QStringLiteral("lock"), QColor(PorosTheme::T3)).pixmap(14, 14));
        rowLayout->addWidget(icon);
        auto *label = new QLabel(name, row);
        label->setProperty("role", QStringLiteral("hint"));
        rowLayout->addWidget(label, 1);
        m_locked->addWidget(row);
    }
    rebuildRules();
}

void PresetEditor::rebuildRules() {
    presetEditorClear(m_sites);
    presetEditorClear(m_programs);
    for (int i = 0; i < m_rules.size(); i++) {
        const auto &rule = m_rules[i];
        auto *target = rule.program ? m_programs : m_sites;
        auto *row = new QWidget(this);
        auto *rowLayout = new QHBoxLayout(row);
        rowLayout->setContentsMargins(0, 0, 0, 0);
        rowLayout->setSpacing(8);
        auto *label = new QLabel(rule.text, row);
        label->setProperty("role", QStringLiteral("mono"));
        label->setTextFormat(Qt::PlainText);
        rowLayout->addWidget(label, 1);
        auto *action = presetEditorActionBox(rule.action, row);
        connect(action, &QComboBox::activated, this, [this, i, action] { m_rules[i].action = static_cast<Action>(action->currentData().toInt()); });
        rowLayout->addWidget(action);
        auto *remove = new QToolButton(row);
        remove->setIcon(PorosTheme::Icon(QStringLiteral("close")));
        remove->setIconSize({14, 14});
        connect(remove, &QToolButton::clicked, this, [this, i] {
            m_rules.removeAt(i);
            rebuildRules();
        });
        rowLayout->addWidget(remove);
        target->addWidget(row);
    }
}

void PresetEditor::addRule(bool program, QLineEdit *input) {
    const QString text = input->text().trimmed();
    if (text.isEmpty()) return;
    const QString line = program ? Configs::PresetRules::ProgramLine(text) : Configs::PresetRules::SiteLine(text);
    if (const auto issue = Configs::RouteProfile::CheckSimpleRule(line); !issue.message.isEmpty()) {
        m_problem->setText(issue.message);
        return;
    }
    m_problem->clear();
    m_rules.append({line.mid(line.indexOf(u':') + 1), Configs::PresetRules::Action::Proxy, program});
    input->clear();
    rebuildRules();
}

void PresetEditor::save() {
    if (m_name->text().trimmed().isEmpty()) {
        m_problem->setText(tr("Give the preset a name."));
        return;
    }
    auto copy = std::make_shared<Configs::RouteProfile>(*m_preset);
    copy->name = m_name->text().trimmed();
    if (const auto problems = Configs::PresetRules::Write(*copy, m_rules); !problems.isEmpty()) {
        m_problem->setText(problems.trimmed());
        return;
    }

    const int dns = m_dns->currentIndex();
    copy->customDns = dns != 0;
    if (dns != 0) {
        auto settings = copy->dns.value_or(Configs::RouteDns::Global());
        settings.remote_dns = dns == presetEditorCustomDns ? m_dnsCustom->text().trimmed() : QString::fromLatin1(presetEditorDns[dns].address);
        if (const auto problem = settings.Problem(); !problem.isEmpty()) {
            m_problem->setText(problem);
            return;
        }
        copy->dns = settings;
    }

    Configs::dataManager->routesRepo->Save(copy);
    emit saved(copy->id);
}
