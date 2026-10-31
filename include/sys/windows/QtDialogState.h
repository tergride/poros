#pragma once

#include <memory>

// QFileDialog always saves its history under HKCU\Software\QtProject, in a key other Qt programs share, and Qt offers no way to move it.
// Leaves that key as it was found, or removes it if there was none. Must outlive the dialog.
class QtDialogStateGuard {
public:
    QtDialogStateGuard();
    ~QtDialogStateGuard();
    QtDialogStateGuard(const QtDialogStateGuard &) = delete;
    QtDialogStateGuard &operator=(const QtDialogStateGuard &) = delete;

private:
    struct Saved;
    std::unique_ptr<Saved> saved;
};
