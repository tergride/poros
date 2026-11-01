#pragma once

#include <QListWidgetItem>
#include <QWidget>
#include <memory>

#include "include/database/entities/OtpProfile.h"
#include "ui_OtpItem.h"

QT_BEGIN_NAMESPACE
namespace Ui {
    class OtpItem;
}
QT_END_NAMESPACE

class OtpItem : public QWidget {
    Q_OBJECT

public:
    enum class Mode {
        Manage,
        ReadOnly
    };

    explicit OtpItem(QWidget *parent, std::shared_ptr<Configs::OtpProfile> profile, QListWidgetItem *item,
                     Mode mode = Mode::Manage);

    ~OtpItem() override;

    void Refresh() const;

    [[nodiscard]] const std::shared_ptr<Configs::OtpProfile> &Profile() const { return profile; }

signals:
    void copyRequested();

    void menuRequested();

protected:
    void resizeEvent(QResizeEvent *event) override;

private:
    Ui::OtpItem *ui;

    std::shared_ptr<Configs::OtpProfile> profile;

    QListWidgetItem *item;

    Mode mode;

    void updateNameLabels() const;
};
