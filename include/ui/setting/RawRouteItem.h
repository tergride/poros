#pragma once

#include <QHash>
#include <QPair>
#include <memory>

#include "include/database/entities/RouteProfile.h"
#include "include/ui/widget/json/JsonCodeEdit.h"

#include <QWidget>

class QLineEdit;
class QCheckBox;
class QCompleter;
class QLabel;
class RouteDnsEditor;

namespace JsonEdit {
    class JsonIssueList;
}

namespace Validation {
    class Form;
}

class RawRouteEdit : public JsonEdit::JsonCodeEdit {
    Q_OBJECT

public:
    explicit RawRouteEdit(QWidget* parent = nullptr);
    void setOutboundItems(const QList<QPair<QString, QString>>& items);

protected:
    void keyPressEvent(QKeyEvent* e) override;

private slots:
    void insertOutboundCompletion(const QString& completion);

private:
    [[nodiscard]] bool outboundContext(QString* partial) const;
    void updateCompleter();
    QCompleter* completer = nullptr;
    QHash<QString, QString> outboundIdByDisplay; // popup display text -> id to insert
};

// The editor of one raw routing profile, shown beside the list of profiles.
class RawRouteItem : public QWidget {
    Q_OBJECT

public:
    explicit RawRouteItem(QWidget* parent = nullptr, const std::shared_ptr<Configs::RouteProfile>& routeChain = nullptr);

    std::shared_ptr<Configs::RouteProfile> chain;

    // Writes the fields into chain; false when a problem has to be fixed first.
    bool apply();

private:
    QLineEdit* nameEdit = nullptr;
    RawRouteEdit* jsonEdit = nullptr;
    // Kept to re-run on save: the editor's own check is debounced, so its issues can lag the last keystroke.
    std::shared_ptr<JsonEdit::Validator> routeValidator;
    QCheckBox* preventCheck = nullptr;
    JsonEdit::JsonIssueList* issueList = nullptr;
    QLabel* validateLabel = nullptr;
    RouteDnsEditor* dnsEditor = nullptr;
    Validation::Form* dnsForm = nullptr;
};
