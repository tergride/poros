#pragma once

#include <QDialog>
#include <QMenu>

#include <atomic>
#include <memory>

#include "include/engine/EditLocks.h"
#include "include/global/Configs.hpp"
#include "include/ui/setting/RouteItem.h"
#include "include/ui/utils/ValidationForm.h"
#include "ui_dialog_manage_routes.h"
#include "include/database/entities/RouteProfile.h"

QT_BEGIN_NAMESPACE
namespace Ui {
    class DialogManageRoutes;
}
QT_END_NAMESPACE

class RawRouteItem;

class DialogManageRoutes : public QDialog {
    Q_OBJECT

public:
    explicit DialogManageRoutes(QWidget *parent = nullptr);

    ~DialogManageRoutes() override;

private:
    Ui::DialogManageRoutes *ui;

    Validation::Form* form = nullptr;

    Engine::EditLock settingsLock{Engine::EditTarget::Settings};
    Engine::EditLock routesLock{Engine::EditTarget::Routes};

    // One of the two is open, on editedRoute; chainList gets its changes through applyEditor().
    RouteItem* routeEditor = nullptr;
    RawRouteItem* rawEditor = nullptr;
    std::shared_ptr<Configs::RouteProfile> editedRoute;

    void showRouteEditor(const std::shared_ptr<Configs::RouteProfile>& profile);

    // False when the open editor has a problem to fix first.
    bool applyEditor();

    // select defaults to the profile being edited, then to the active one.
    void reloadProfileItems(const std::shared_ptr<Configs::RouteProfile>& select = nullptr);

    void applyImportedProfile(const std::shared_ptr<Configs::RouteProfile>& profile, bool wasOldArray);

    bool tryImportRemoteRoutesLink(const QString& text);

    void updateRemoteProfiles(const QList<std::shared_ptr<Configs::RouteProfile>>& profiles);

    void downloadRouteProfile(const QString& country);

    // routeUpdateRunning is UI-thread only; routeUpdateCancel is set there and polled by the worker, which may outlive the page.
    bool routeUpdateRunning = false;
    std::shared_ptr<std::atomic<bool>> routeUpdateCancel = std::make_shared<std::atomic<bool>>(false);

    QList<std::shared_ptr<Configs::RouteProfile>> chainList;

    std::shared_ptr<Configs::RouteProfile> currentRoute;

    int tooltipID = 0;

    void show_dns_advanced_editor();

    void show_dns_object_editor();

    void generate_warp_config();

    struct DnsAdvancedDraft {
        int cache_capacity;
        bool disable_cache;
        bool disable_expire;
        bool persist_cache;
        bool reverse_mapping;
        bool optimistic;
        QString optimistic_timeout;
        QString query_timeout;
    };

    // Held until accept() so the popups can be cancelled without touching the settings.
    DnsAdvancedDraft dns_advanced{};
    QString dns_object_text;

    QShortcut* deleteShortcut;
public slots:
    void accept() override;

    void updateCurrentRouteProfile(int idx);

    void on_new_route_clicked();

    void on_export_route_clicked();

    void on_import_route_clicked();

    void on_clone_route_clicked();

    void on_delete_route_clicked();

    void on_update_route_clicked();
};
