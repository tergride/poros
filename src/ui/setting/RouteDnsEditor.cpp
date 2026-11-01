#include "include/ui/setting/RouteDnsEditor.h"

#include "include/configs/generate.h"
#include "include/global/Configs.hpp"
#include "include/ui/utils/ValidationForm.h"

#include <QCheckBox>
#include <QComboBox>
#include <QGridLayout>
#include <QLabel>
#include <QVBoxLayout>

#include <algorithm>

namespace DnsServerHistory {
    constexpr int maxEntries = 8;

    const QStringList& RemotePresets() {
        static const QStringList presets = {"https://8.8.8.8/dns-query", "tls://8.8.8.8", "tls://1.1.1.1", "8.8.8.8", "1.1.1.1"};
        return presets;
    }

    const QStringList& DirectPresets() {
        static const QStringList presets = {"localhost", "223.5.5.5", "119.29.29.29", "178.22.122.100", "77.88.8.8"};
        return presets;
    }

    void Fill(QComboBox* combo, const QStringList& history, const QStringList& presets, const QString& current) {
        QStringList items = history;
        for (const auto& preset : presets) {
            if (!items.contains(preset)) items << preset;
        }
        combo->clear();
        combo->addItems(items);
        combo->setCurrentText(current);
    }

    bool Remember(QStringList& history, const QStringList& presets, const QString& server) {
        const auto value = server.trimmed();
        if (value.isEmpty() || presets.contains(value)) return false;
        if (!history.isEmpty() && history.first() == value) return false;
        history.removeAll(value);
        history.prepend(value);
        while (history.size() > maxEntries) history.removeLast();
        return true;
    }
}

static QString routeDnsProblem(const QString& text) {
    Configs::DnsAddress parsed;
    QString why;
    return Configs::ParseDnsAddress(text, &parsed, &why) ? QString() : why;
}

RouteDnsEditor::RouteDnsEditor(const Configs::RouteProfile& profile, QWidget* parent) : QWidget(parent) {
    customDraft = profile.dns.value_or(Configs::RouteDns::Global());
    opened = customDraft;

    auto* layout = new QVBoxLayout(this);
    layout->setContentsMargins(14, 14, 14, 14);
    layout->setSpacing(10);

    customSwitch = new QCheckBox(tr("Custom DNS for this profile"), this);
    customSwitch->setProperty("role", QStringLiteral("switch"));
    customSwitch->setToolTip(tr("These servers replace the global ones while this routing profile is in use."));
    layout->addWidget(customSwitch);

    dnsObjectNote = new QLabel(tr("Routing uses a custom DNS object, so that object applies instead of the values below."), this);
    dnsObjectNote->setProperty("role", QStringLiteral("hint"));
    dnsObjectNote->setWordWrap(true);
    layout->addWidget(dnsObjectNote);

    fields = new QWidget(this);
    auto* grid = new QGridLayout(fields);
    grid->setContentsMargins(0, 0, 0, 0);
    grid->setHorizontalSpacing(16);
    grid->setVerticalSpacing(6);
    grid->setColumnStretch(0, 1);
    grid->setColumnStretch(1, 1);
    const auto fieldLabel = [](QLabel* label) {
        label->setProperty("role", QStringLiteral("field"));
        return label;
    };
    const auto switchBox = [](QCheckBox* box) {
        box->setProperty("role", QStringLiteral("switch"));
        return box;
    };

    const QString serverHint = tr("<html><head/><body><p>examples:<br/>tls://8.8.8.8<br/>https://domain/path</p>"
                                  "<p>tcp://8.8.8.8:1234</p><p>dhcp://auto</p><p>h3://domain/path</p>"
                                  "<p>quic://domain:4632</p><p>localhost</p></body></html>");
    const auto& settings = *Configs::dataManager->settingsRepo;

    auto* remoteLabel = fieldLabel(new QLabel(tr("Remote DNS"), fields));
    remoteLabel->setToolTip(serverHint);
    remoteDns = new QComboBox(fields);
    remoteDns->setEditable(true);
    remoteDns->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
    DnsServerHistory::Fill(remoteDns, settings.remote_dns_history, DnsServerHistory::RemotePresets(), customDraft.remote_dns);
    remoteLabel->setBuddy(remoteDns);
    remoteDisableIPv6 = switchBox(new QCheckBox(tr("Disable IPv6"), fields));
    remoteDisableIPv6->setToolTip(tr("Answer AAAA queries sent to this server with an empty result."));
    grid->addWidget(remoteLabel, 0, 0);
    grid->addWidget(remoteDns, 1, 0);
    grid->addWidget(remoteDisableIPv6, 2, 0);

    auto* directLabel = fieldLabel(new QLabel(tr("Direct DNS"), fields));
    directLabel->setToolTip(serverHint);
    directDns = new QComboBox(fields);
    directDns->setEditable(true);
    directDns->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Fixed);
    DnsServerHistory::Fill(directDns, settings.direct_dns_history, DnsServerHistory::DirectPresets(), customDraft.direct_dns);
    directLabel->setBuddy(directDns);
    directDisableIPv6 = switchBox(new QCheckBox(tr("Disable IPv6"), fields));
    directDisableIPv6->setToolTip(remoteDisableIPv6->toolTip());
    grid->addWidget(directLabel, 0, 1);
    grid->addWidget(directDns, 1, 1);
    grid->addWidget(directDisableIPv6, 2, 1);

    auto* finalLabel = fieldLabel(new QLabel(tr("Default DNS server"), fields));
    finalOut = new QComboBox(fields);
    finalOut->addItem(tr("Remote"), QStringLiteral("remote"));
    finalOut->addItem(tr("Direct"), QStringLiteral("direct"));
    finalLabel->setBuddy(finalOut);
    dnsRouting = switchBox(new QCheckBox(tr("Enable DNS Routing"), fields));
    dnsRouting->setToolTip(tr("Domains this profile sends direct are resolved with the direct DNS too."));
    grid->addWidget(finalLabel, 3, 0);
    grid->addWidget(finalOut, 4, 0);
    grid->addWidget(dnsRouting, 4, 1);

    fakeIP = switchBox(new QCheckBox(tr("Enable FakeIP"), fields));
    fakeIPDisableIPv6 = switchBox(new QCheckBox(tr("FakeIP Disable IPv6"), fields));
    fakeIPDisableIPv6->setToolTip(tr("Hand out no fake IPv6 addresses; AAAA queries are answered empty."));
    grid->addWidget(fakeIP, 5, 0);
    grid->addWidget(fakeIPDisableIPv6, 5, 1);
    layout->addWidget(fields);

    auto* scopeNote = new QLabel(tr("Local Override, the hosts file, predefined answers and the advanced DNS settings always come from Routing > DNS."), this);
    scopeNote->setProperty("role", QStringLiteral("hint"));
    scopeNote->setWordWrap(true);
    layout->addWidget(scopeNote);
    layout->addStretch();

    showDns(profile.customDns ? customDraft : Configs::RouteDns::Global());
    customSwitch->setChecked(profile.customDns);
    syncState();

    connect(fakeIP, &QCheckBox::toggled, fakeIPDisableIPv6, &QCheckBox::setEnabled);
    connect(customSwitch, &QCheckBox::toggled, this, [this](bool custom) {
        if (custom) {
            showDns(customDraft);
        } else {
            customDraft = shownDns();
            showDns(Configs::RouteDns::Global());
        }
        syncState();
    });
}

void RouteDnsEditor::bindTo(Validation::Form* form) {
    form->bind(remoteDns, routeDnsProblem).label(tr("Remote DNS"));
    form->bind(directDns, routeDnsProblem).label(tr("Direct DNS"));
    connect(customSwitch, &QCheckBox::toggled, form, [form] { form->refresh(); });
}

void RouteDnsEditor::applyTo(Configs::RouteProfile& profile) const {
    profile.customDns = customSwitch->isChecked();
    const auto values = profile.customDns ? shownDns() : customDraft;
    // Untouched seed values stay unstored, so a later switch to custom starts from the global settings of that time.
    if (!profile.customDns && !profile.dns && values == opened) return;
    profile.dns = values;

    auto& settings = *Configs::dataManager->settingsRepo;
    bool remembered = false;
    if (values.remote_dns != opened.remote_dns &&
        DnsServerHistory::Remember(settings.remote_dns_history, DnsServerHistory::RemotePresets(), values.remote_dns))
        remembered = true;
    if (values.direct_dns != opened.direct_dns &&
        DnsServerHistory::Remember(settings.direct_dns_history, DnsServerHistory::DirectPresets(), values.direct_dns))
        remembered = true;
    if (remembered) settings.Save();
}

void RouteDnsEditor::showDns(const Configs::RouteDns& dns) const {
    remoteDns->setCurrentText(dns.remote_dns);
    remoteDisableIPv6->setChecked(dns.remote_dns_disable_ipv6);
    directDns->setCurrentText(dns.direct_dns);
    directDisableIPv6->setChecked(dns.direct_dns_disable_ipv6);
    finalOut->setCurrentIndex(std::max(0, finalOut->findData(dns.dns_final_out)));
    dnsRouting->setChecked(dns.enable_dns_routing);
    fakeIP->setChecked(dns.fake_dns);
    fakeIPDisableIPv6->setChecked(dns.fakeip_disable_ipv6);
}

Configs::RouteDns RouteDnsEditor::shownDns() const {
    Configs::RouteDns dns;
    dns.remote_dns = remoteDns->currentText().trimmed();
    dns.remote_dns_disable_ipv6 = remoteDisableIPv6->isChecked();
    dns.direct_dns = directDns->currentText().trimmed();
    dns.direct_dns_disable_ipv6 = directDisableIPv6->isChecked();
    dns.dns_final_out = finalOut->currentData().toString();
    dns.enable_dns_routing = dnsRouting->isChecked();
    dns.fake_dns = fakeIP->isChecked();
    dns.fakeip_disable_ipv6 = fakeIPDisableIPv6->isChecked();
    return dns;
}

void RouteDnsEditor::syncState() const {
    const bool custom = customSwitch->isChecked();
    fields->setEnabled(custom);
    fakeIPDisableIPv6->setEnabled(fakeIP->isChecked());
    dnsObjectNote->setVisible(!custom && Configs::dataManager->settingsRepo->use_dns_object);
}
