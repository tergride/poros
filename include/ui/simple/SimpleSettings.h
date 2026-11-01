#pragma once

#include <QList>
#include <QWidget>
#include <functional>

class QAbstractButton;
class QJsonObject;
class QLabel;
class QStackedWidget;
class QVBoxLayout;

// The simple mode's settings: seven short sections that apply at once.
class SimpleSettings : public QWidget {
    Q_OBJECT

public:
    // What the settings cannot do themselves: the window owns these actions.
    struct Hooks {
        std::function<void()> back;
        std::function<void(int routeId)> chooseRoute;
        std::function<void()> routeEdited;
        std::function<void(int routeId)> editPreset;
        std::function<void(bool)> setAllowLan;
        std::function<void()> trayChanged;
        std::function<void()> restartProgram;
        std::function<void()> showLog;
        std::function<void()> runWizard;
        std::function<void(const QString &mode)> setUiMode;
        std::function<void()> resetCore;
        std::function<void()> checkForUpdates;
    };

    explicit SimpleSettings(const Hooks &hooks, QWidget *parent = nullptr);

    void showSection(int index);
    // Rereads every section: settings may have changed in the advanced mode.
    void reload();

private:
    void put(const QJsonObject &patch);
    QWidget *buildPresets();
    QWidget *buildConnection();
    QWidget *buildSubscriptions();
    QWidget *buildStartup();
    QWidget *buildAppearance();
    QWidget *buildBackup();
    QWidget *buildHelp();
    void rebuildPresetRows();
    void copyPreset(int routeId);
    void deletePreset(int routeId);
    void importPreset();
    void exportPreset();
    void createBackup();
    void restoreBackup();

    Hooks m_hooks;
    QList<QAbstractButton *> m_nav;
    QStackedWidget *m_pages;
    QVBoxLayout *m_builtinPresets = nullptr;
    QVBoxLayout *m_ownPresets = nullptr;
    QList<std::function<void()>> m_reloaders;
};
