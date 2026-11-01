#include "include/ui/profile/dialog_profile_type.h"

#include "include/database/entities/Profile.h"
#include "include/ui/setting/PorosTheme.h"

#include <QGridLayout>
#include <QLabel>
#include <QPushButton>
#include <QVBoxLayout>

namespace {
    constexpr int tileColumns = 4;
    constexpr int tileWidth = 145;

    struct Tile {
        QString label;
        QString type;
        DialogProfileType::Preset preset = {};
    };
}

DialogProfileType::DialogProfileType(QWidget *parent) : QDialog(parent) {
    setWindowTitle(tr("New profile"));
    auto *column = new QVBoxLayout(this);
    column->setSizeConstraint(QLayout::SetFixedSize);
    column->setContentsMargins(20, 16, 20, 18);
    column->setSpacing(0);
    auto *title = new QLabel(windowTitle(), this);
    title->setProperty("role", QStringLiteral("dialogTitle"));
    column->addWidget(title);
    column->addSpacing(10);

    const auto tileGrid = [this](QVBoxLayout *into, const QList<Tile> &tiles) {
        auto *grid = new QGridLayout;
        grid->setSpacing(6);
        for (int i = 0; i < tiles.size(); ++i) {
            const Tile tile = tiles[i];
            auto *button = new QPushButton(tile.label, this);
            button->setProperty("role", QStringLiteral("typeTile"));
            button->setFixedWidth(tileWidth);
            connect(button, &QPushButton::clicked, this, [this, tile] {
                accept();
                emit chosen(tile.type, tile.preset);
            });
            grid->addWidget(button, i / tileColumns, i % tileColumns);
        }
        // An empty column drops its spacing, which would shift a short row out of line with the others.
        for (auto i = tiles.size(); i < tileColumns; ++i) {
            auto *gap = new QWidget(this);
            gap->setFixedWidth(tileWidth);
            grid->addWidget(gap, 0, int(i));
        }
        into->addLayout(grid);
    };
    const auto section = [this, column, &tileGrid](const QString &heading, const QList<Tile> &tiles) {
        auto *label = new QLabel(heading, this);
        label->setProperty("role", QStringLiteral("typeHeading"));
        column->addWidget(label);
        tileGrid(column, tiles);
        column->addSpacing(8);
    };

    const auto l = [](const char *name) { return QString::fromLatin1(name); };
    section(tr("Proxy"), {{l("VLESS"), l("vless")}, {l("VMess"), l("vmess")}, {l("Trojan"), l("trojan")}, {l("Shadowsocks"), l("shadowsocks")},
                          {l("ShadowTLS"), l("shadowtls")}, {l("AnyTLS"), l("anytls")}, {l("SOCKS"), l("socks")}, {l("HTTP"), l("http")},
                          {l("Naive"), l("naive")}, {l("SSH"), l("ssh")}});
    section(tr("QUIC"), {{l("Hysteria"), l("hysteria")}, {l("TUIC"), l("tuic")}, {l("Masque"), l("masque")}});
    section(tr("VPN"), {{l("WireGuard"), l("wireguard")},
                        {l("AmneziaWG"), l("wireguard"), [](Configs::Profile &profile) { profile.Wireguard()->enable_amnezia = true; }},
                        {l("OpenVPN"), l("openvpn")}, {l("OpenConnect"), l("openconnect")}, {l("TrustTunnel"), l("trusttunnel")}});
    section(tr("Composite"), {{tr("Chain"), l("chain")}, {tr("Auto selector"), l("autoselector")}, {l("Xray VLESS"), l("xrayvless")},
                              {l("Custom JSON"), l(Configs::Custom::CustomOutbound)}, {l("Direct"), l("direct")}});

    auto *more = new QPushButton(PorosTheme::Icon(QStringLiteral("right")), tr("Others"), this);
    more->setObjectName(QStringLiteral("more_types"));
    more->setProperty("role", QStringLiteral("typeMore"));
    more->setIconSize({14, 14});
    more->setCheckable(true);
    column->addWidget(more);
    auto *others = new QWidget(this);
    auto *othersColumn = new QVBoxLayout(others);
    othersColumn->setContentsMargins(0, 4, 0, 0);
    tileGrid(othersColumn, {{l("Mieru"), l("mieru")}, {l("Snell"), l("snell")}, {l("Juicity"), l("juicity")},
                            {l("Hysteria Realm"), l("hysteria"), [](Configs::Profile &profile) {
                                 auto *hysteria = profile.Hysteria();
                                 hysteria->protocol_version = QStringLiteral("2");
                                 hysteria->realm_enabled = true;
                             }}});
    column->addWidget(others);
    others->hide();
    connect(more, &QPushButton::toggled, this, [more, others](bool open) {
        more->setIcon(PorosTheme::Icon(open ? QStringLiteral("down") : QStringLiteral("right")));
        others->setVisible(open);
    });
}
