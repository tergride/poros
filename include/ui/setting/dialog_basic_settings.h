#pragma once

#include <QDialog>
#include <QJsonObject>
#include <QTimer>
#include <memory>
#include "ui_dialog_basic_settings.h"

namespace Ui {
    class DialogBasicSettings;
}

namespace Validation {
    class Form;
}

class DialogBasicSettings : public QDialog {
    Q_OBJECT

public:
    explicit DialogBasicSettings(QWidget *parent = nullptr);

    ~DialogBasicSettings();

public slots:

    void accept();

private:
    Ui::DialogBasicSettings *ui;

    Validation::Form *form = nullptr;


    void bindValidation();

    void refreshUrlSchemeStatus();

    void downloadXrayGeoAsset(const QString &url, const QString &fileName);

    void rememberGeoAssetUrl(QComboBox *combo, const QString &url, QStringList &history);

    // Inspect, choose the parts, apply and restart; fallbackDeviceName names the creator when the archive does not.
    void restoreBackupFile(const QString &filePath, const QString &fallbackDeviceName);

    struct {
        bool needRestart = false;
        bool updateDisableTray = false;
        bool updateMaxLogLines = false;
        bool updateDisableAdmin = false;
    } CACHE;

private slots:
    void on_backup_create_clicked();
    void on_backup_restore_clicked();
    void on_xray_geoip_download_clicked();
    void on_xray_geosite_download_clicked();
};