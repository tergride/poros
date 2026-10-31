#include "include/ui/advanced/EmbeddedDialog.h"

#include <QDialog>
#include <QDialogButtonBox>
#include <QPushButton>
#include <QTimer>
#include <QVBoxLayout>

EmbeddedDialog::EmbeddedDialog(Factory factory, QWidget *parent) : QWidget(parent), m_factory(std::move(factory)) {
    auto *column = new QVBoxLayout(this);
    column->setContentsMargins(0, 0, 0, 0);
}

QDialog *EmbeddedDialog::dialog() {
    if (m_dialog != nullptr) return m_dialog;
    m_dialog = m_factory(this);
    m_dialog->setWindowFlags(Qt::Widget);
    m_dialog->setProperty("embedded", true);
    layout()->addWidget(m_dialog);
    connect(m_dialog, &QDialog::finished, this, [this] {
        const QPointer<QDialog> closed = m_dialog;
        m_dialog = nullptr;
        if (closed != nullptr) closed->deleteLater();
        QTimer::singleShot(0, this, [this] {
            if (isVisible()) dialog();
        });
    });
    // Moving on from a page is no cancel, so the only button left is the one that saves.
    if (auto *buttons = m_dialog->findChild<QDialogButtonBox *>()) {
        if (auto *ok = buttons->button(QDialogButtonBox::Ok)) ok->setText(tr("Save"));
        if (auto *cancel = buttons->button(QDialogButtonBox::Cancel)) cancel->hide();
    }
    m_dialog->show();
    emit built(m_dialog);
    return m_dialog;
}

void EmbeddedDialog::rebuild() {
    if (m_dialog == nullptr) return;
    auto *old = m_dialog.data();
    m_dialog = nullptr;
    old->disconnect(this);
    old->hide();
    old->deleteLater();
    if (isVisible()) dialog();
}

void EmbeddedDialog::showEvent(QShowEvent *event) {
    QWidget::showEvent(event);
    dialog();
}
