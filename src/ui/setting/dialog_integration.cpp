#include "include/ui/setting/dialog_integration.h"

#include <include/ui/utils/GuiUtils.hpp>

#include "include/global/Validation.hpp"
#include "include/ui/mainwindow.h"
#include "include/ui/utils/ValidationForm.h"
#include <QAction>
#include <QLabel>
#include <QMessageBox>
#include <QScreen>

namespace {
    // Mirrors GlobalHotkeys::setActions, which registers only the first chord and refuses keys that would swallow typing.
    QString integrationGlobalHotkeyError(const QKeySequence &key) {
        if (key.isEmpty()) return {};
        if (key.count() > 1) return DialogIntegration::tr("Global hotkeys can only be a single key combination.");
        const QKeyCombination chord = key[0];
        if (!(chord.keyboardModifiers() & (Qt::ControlModifier | Qt::AltModifier | Qt::MetaModifier)) && chord.key() < Qt::Key_F1)
            return DialogIntegration::tr("\"%1\" needs Ctrl, Alt or Meta, otherwise it would block typing in other applications.")
                .arg(key.toString(QKeySequence::NativeText));
        return {};
    }

    QString integrationFieldLabel(QFormLayout *layout, QWidget *field) {
        const auto *label = qobject_cast<QLabel *>(layout->labelForField(field));
        return label != nullptr ? label->text() : QString();
    }
}

DialogIntegration::DialogIntegration(QWidget *parent, const QList<QAction*>& actions) : QDialog(parent), ui(new Ui::DialogIntegration) {
    ui->setupUi(this);
    form = new Validation::Form(this);
    form->attachStatus(ui->buttonBox);
    ui->show_mainwindow->setKeySequence(Configs::dataManager->settingsRepo->hotkey_mainwindow);
    ui->show_groups->setKeySequence(Configs::dataManager->settingsRepo->hotkey_group);
    ui->show_routes->setKeySequence(Configs::dataManager->settingsRepo->hotkey_route);
    ui->system_proxy->setKeySequence(Configs::dataManager->settingsRepo->hotkey_system_proxy_menu);
    ui->toggle_proxy->setKeySequence(Configs::dataManager->settingsRepo->hotkey_toggle_system_proxy);
    ui->toggle_connection->setKeySequence(Configs::dataManager->settingsRepo->hotkey_toggle_connection);
    ui->toggle_tun->setKeySequence(Configs::dataManager->settingsRepo->hotkey_toggle_tun);

    generateShortcutItems(actions);
    bindHotkeyValidation();

    if (!GetMainWindow()->IsGlobalHotkeySupported()) {
        auto note = new QLabel(QCoreApplication::translate("GlobalHotkeys", "Global hotkeys are not supported in this desktop session."), ui->global_box);
        note->setWordWrap(true);
        ui->formLayout->insertRow(0, note);
    }

    const auto *scr = screen() != nullptr ? screen() : QGuiApplication::primaryScreen();
    if (scr != nullptr) resize(sizeHint().boundedTo(scr->availableGeometry().size()));
    form->markAll();
}

void DialogIntegration::generateShortcutItems(const QList<QAction*>& actions)
{
    auto widget = new QWidget(ui->shortcut_area);
    auto layout = new QFormLayout(widget);
    widget->setLayout(layout);
    ui->shortcut_area->setWidget(widget);
    for (auto action : actions)
    {
        auto kseq = new QtExtKeySequenceEdit(this);
        if (!action->shortcut().isEmpty()) kseq->setKeySequence(action->shortcut());
        seqEdit2ID[kseq] = action->data().toString();
        layout->addRow(action->text(), kseq);
        hotkeyEdits.append({kseq, action->text(), false});
    }
}

void DialogIntegration::bindHotkeyValidation()
{
    QList<HotkeyEdit> globals;
    for (auto *edit : {ui->show_mainwindow, ui->show_groups, ui->show_routes, ui->system_proxy, ui->toggle_proxy, ui->toggle_connection, ui->toggle_tun})
        globals.append({edit, integrationFieldLabel(ui->formLayout, edit), true});
    hotkeyEdits = globals + hotkeyEdits;

    for (const auto &field : std::as_const(hotkeyEdits)) {
        // QKeySequenceEdit is its line edit's focus proxy: the line edit carries the mark, the edit reports when recording ends.
        auto *display = field.edit->findChild<QLineEdit *>();
        QWidget *target = display != nullptr ? static_cast<QWidget *>(display) : field.edit;
        form->addRule({target}, [this, field] {
            const auto key = field.edit->keySequence();
            if (key.isEmpty()) return QString();
            if (field.global) {
                if (auto problem = integrationGlobalHotkeyError(key); !problem.isEmpty()) return problem;
            }
            for (const auto &other : std::as_const(hotkeyEdits)) {
                if (other.edit != field.edit && other.edit->keySequence() == key)
                    return tr("\"%1\" is also assigned to \"%2\".").arg(key.toString(QKeySequence::NativeText), other.label);
            }
            return QString();
        }, this, Validation::Severity::Error, field.label);
        const auto recheck = [this, target] {
            form->revalidate(target);
            form->refresh();
        };
        connect(field.edit, &QKeySequenceEdit::editingFinished, this, recheck);
        connect(field.edit, &QKeySequenceEdit::keySequenceChanged, this, [recheck](const QKeySequence &key) {
            if (key.isEmpty()) recheck();
        });
    }
}

void DialogIntegration::accept()
{
    if (!form->validateAll()) return;

    const auto &settings = Configs::dataManager->settingsRepo;
    settings->hotkey_mainwindow = ui->show_mainwindow->keySequence().toString();
    settings->hotkey_group = ui->show_groups->keySequence().toString();
    settings->hotkey_route = ui->show_routes->keySequence().toString();
    settings->hotkey_system_proxy_menu = ui->system_proxy->keySequence().toString();
    settings->hotkey_toggle_system_proxy = ui->toggle_proxy->keySequence().toString();
    settings->hotkey_toggle_connection = ui->toggle_connection->keySequence().toString();
    settings->hotkey_toggle_tun = ui->toggle_tun->keySequence().toString();

    auto mp = seqEdit2ID.toStdMap();
    for (const auto& [kseq, actionID] : mp)
    {
        settings->shortcuts[actionID] = kseq->keySequence();
    }

    settings->Save();

    MW_dialog_message(MwMessage::UpdateShortcuts, {});
    hotkeysReleased = false;
    const auto failures = GetMainWindow()->RegisterHotkey(false);
    if (!failures.isEmpty() && GetMainWindow()->IsGlobalHotkeySupported()) {
        QMessageBox box(QMessageBox::Warning, tr("Hotkey"), tr("These global hotkeys could not be registered:") + "\n\n" + failures.join('\n'), QMessageBox::Ok, this);
        box.setTextFormat(Qt::PlainText);
        box.exec();
    }
    QDialog::accept();
}

void DialogIntegration::showEvent(QShowEvent *event) {
    QDialog::showEvent(event);
    // A combination being typed here must not fire the action it is bound to now.
    if (!hotkeysReleased) GetMainWindow()->RegisterHotkey(true);
    hotkeysReleased = true;
}

void DialogIntegration::hideEvent(QHideEvent *event) {
    QDialog::hideEvent(event);
    if (hotkeysReleased) GetMainWindow()->RegisterHotkey(false);
    hotkeysReleased = false;
}

DialogIntegration::~DialogIntegration() {
    delete ui;
}
