#include "include/ui/setting/OtpItem.h"

#include "include/ui/setting/PorosTheme.h"

namespace {
    constexpr int READONLY_CODE_WIDTH = 110;
    constexpr int MANAGE_ROW_HEIGHT = 56;

    // The clipboard gets the code without the space.
    QString GroupedCode(const QString &code) {
        const int half = (code.size() + 1) / 2;
        return code.left(half) + QLatin1Char(' ') + code.mid(half);
    }

    // QLabel clips rather than elides.
    void SetElided(QLabel *label, const QString &text) {
        label->setText(label->fontMetrics().elidedText(text, Qt::ElideRight, label->width()));
    }
}

OtpItem::OtpItem(QWidget *parent, std::shared_ptr<Configs::OtpProfile> profile_, QListWidgetItem *item_,
                 const Mode mode_)
    : QWidget(parent), ui(new Ui::OtpItem), profile(std::move(profile_)), item(item_), mode(mode_) {
    ui->setupUi(this);
    setLayoutDirection(Qt::LeftToRight);

    if (mode == Mode::ReadOnly) {
        ui->actions->hide();
        ui->code->setStyleSheet(QStringLiteral("font-family: \"JetBrains Mono\", \"Consolas\"; font-size: 12pt; letter-spacing: 2px;"));
        ui->code->setMinimumWidth(READONLY_CODE_WIDTH);
    } else {
        ui->layout_row->setContentsMargins(0, 0, 0, 0);
    }

    // A child that consumes the press stops the click reaching the list, which selects, copies and drags.
    for (QWidget *passive : {static_cast<QWidget *>(ui->name), static_cast<QWidget *>(ui->account),
                             static_cast<QWidget *>(ui->code), static_cast<QWidget *>(ui->timer_text)})
        passive->setAttribute(Qt::WA_TransparentForMouseEvents);

    ui->copy->setIcon(PorosTheme::Icon(QStringLiteral("copy")));
    ui->more->setIcon(PorosTheme::Icon(QStringLiteral("more")));
    connect(ui->copy, &QToolButton::clicked, this, [this] { emit copyRequested(); });
    connect(ui->more, &QToolButton::clicked, this, [this] { emit menuRequested(); });

    setToolTip(profile->DisplayName());
    Refresh();

    adjustSize();
    if (item != nullptr) item->setSizeHint(mode == Mode::Manage ? QSize(sizeHint().width(), MANAGE_ROW_HEIGHT) : sizeHint());
}

OtpItem::~OtpItem() {
    delete ui;
}

void OtpItem::updateNameLabels() const {
    const bool twoLines = mode == Mode::Manage && !profile->issuer.isEmpty() && !profile->name.isEmpty();
    SetElided(ui->name, twoLines ? profile->issuer : profile->DisplayName());
    ui->account->setVisible(twoLines);
    if (twoLines) SetElided(ui->account, profile->name);
}

void OtpItem::resizeEvent(QResizeEvent *event) {
    updateNameLabels();
    QWidget::resizeEvent(event);
}

void OtpItem::Refresh() const {
    updateNameLabels();

    const auto code = profile->CurrentCode();
    ui->timer_text->setVisible(!code.isEmpty());
    if (code.isEmpty()) {
        ui->code->setText(tr("invalid"));
        return;
    }

    ui->code->setText(GroupedCode(code));
    ui->timer_text->setText(profile->type == OTP::Type::HOTP ? "#" + QString::number(profile->counter)
                                                             : tr("%1s").arg(profile->SecondsRemaining()));
}
