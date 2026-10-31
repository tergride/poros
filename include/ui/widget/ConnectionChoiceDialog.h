#pragma once

#include <QDialog>

#include "include/ui/utils/ConnectionModeController.h"

// The first Start in the simple mode: Poros has no default, so it asks once.
class ConnectionChoiceDialog : public QDialog {
    Q_OBJECT

public:
    explicit ConnectionChoiceDialog(QWidget *parent = nullptr);

    ConnectionModeController::Mode choice() const { return m_choice; }

private:
    ConnectionModeController::Mode m_choice = ConnectionModeController::Mode::Unset;
};
