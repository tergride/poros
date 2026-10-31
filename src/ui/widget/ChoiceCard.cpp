#include "include/ui/widget/ChoiceCard.h"

#include <QLabel>
#include <QPainter>
#include <QVBoxLayout>

#include "include/ui/setting/PorosTheme.h"

namespace {
    QLabel *choiceCardLabel(const QString &text, const char *role, QWidget *parent) {
        auto *label = new QLabel(text, parent);
        label->setProperty("role", QString::fromLatin1(role));
        label->setWordWrap(true);
        label->setAttribute(Qt::WA_TransparentForMouseEvents);
        return label;
    }
} // namespace

ChoiceCard::ChoiceCard(const QString &title, const QString &text, const QStringList &points, QWidget *parent) : QAbstractButton(parent) {
    setCheckable(true);
    setAttribute(Qt::WA_Hover);
    setCursor(Qt::PointingHandCursor);
    setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Maximum);

    m_column = new QVBoxLayout(this);
    m_column->setContentsMargins(16, 14, 16, 14);
    m_column->setSpacing(6);
    m_column->addWidget(choiceCardLabel(title, "cardTitle", this));
    if (!text.isEmpty()) m_column->addWidget(choiceCardLabel(text, "hint", this));
    for (const auto &point : points) m_column->addWidget(choiceCardLabel(QStringLiteral("•  ") + point, "hint", this));
    m_column->addStretch();
}

QSize ChoiceCard::sizeHint() const {
    return m_column->sizeHint();
}

void ChoiceCard::paintEvent(QPaintEvent *) {
    using namespace PorosTheme;
    QPainter painter(this);
    painter.setRenderHint(QPainter::Antialiasing);
    const bool hover = underMouse();
    const QRectF box = QRectF(rect()).adjusted(1, 1, -1, -1);
    painter.setBrush(QColor(isChecked() || hover ? S3 : S2));
    painter.setPen(QPen(QColor(isChecked() ? Blue : hover ? Line2 : Line), isChecked() ? 2 : 1));
    painter.drawRoundedRect(box, 12, 12);
}
