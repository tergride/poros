#pragma once

#include <QPointer>
#include <QWidget>
#include <functional>

class QDialog;

// Shows a dialog as a page. Saving or cancelling closes a dialog, so the page builds a fresh one from the saved settings.
class EmbeddedDialog : public QWidget {
    Q_OBJECT

public:
    using Factory = std::function<QDialog *(QWidget *parent)>;

    explicit EmbeddedDialog(Factory factory, QWidget *parent = nullptr);

    // Builds it when needed.
    QDialog *dialog();

    // Drops what is on the page in favour of what is saved: another screen may have changed it.
    void rebuild();

signals:
    void built(QDialog *dialog);

protected:
    void showEvent(QShowEvent *event) override;

private:
    Factory m_factory;
    QPointer<QDialog> m_dialog;
};
