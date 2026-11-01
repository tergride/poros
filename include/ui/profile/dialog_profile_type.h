#pragma once

#include <QDialog>

#include <functional>

namespace Configs {
    class Profile;
}

// Opens a new profile: a tile for every kind, as in the mockups.
class DialogProfileType : public QDialog {
    Q_OBJECT

public:
    using Preset = std::function<void(Configs::Profile &)>;

    explicit DialogProfileType(QWidget *parent = nullptr);

signals:
    // A preset switches on an option of the type, for the tiles that are not types of their own.
    void chosen(const QString &type, const DialogProfileType::Preset &preset);
};
