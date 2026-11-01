#include "include/ui/setting/RawRouteItem.h"

#include "include/global/Configs.hpp"
#include "include/database/ProfilesRepo.h"
#include "include/database/GroupsRepo.h"
#include "include/ui/widget/json/JsonIssueList.h"
#include "include/ui/widget/json/SchemaStore.h"
#include "include/ui/setting/RouteDnsEditor.h"
#include "include/ui/setting/ThemeManager.hpp"
#include "include/ui/utils/ValidationForm.h"

#include <QAbstractItemView>
#include <QCheckBox>
#include <QCompleter>
#include <QFrame>
#include <QHBoxLayout>
#include <QJsonDocument>
#include <QKeyEvent>
#include <QLabel>
#include <QLineEdit>
#include <QMap>
#include <QMessageBox>
#include <QPushButton>
#include <QRegularExpression>
#include <QScrollBar>
#include <QStringListModel>
#include <QTabWidget>
#include <QTextBlock>

#include <algorithm>

RawRouteEdit::RawRouteEdit(QWidget* parent) : JsonCodeEdit(parent) {
    completer = new QCompleter(this);
    completer->setModel(new QStringListModel(completer));
    completer->setWidget(this);
    completer->setCompletionMode(QCompleter::PopupCompletion);
    completer->setCaseSensitivity(Qt::CaseInsensitive);
    completer->setFilterMode(Qt::MatchContains);
    connect(completer, static_cast<void (QCompleter::*)(const QString&)>(&QCompleter::activated),
            this, &RawRouteEdit::insertOutboundCompletion);
}

void RawRouteEdit::setOutboundItems(const QList<QPair<QString, QString>>& items) {
    QStringList display;
    outboundIdByDisplay.clear();
    for (const auto& [text, id] : items) {
        display << text;
        outboundIdByDisplay.insert(text, id);
    }
    completer->setModel(new QStringListModel(display, completer));
}

bool RawRouteEdit::outboundContext(QString* partial) const {
    const QTextCursor tc = textCursor();
    const QString line = tc.block().text().left(tc.positionInBlock());
    // cursor is in a bare (unquoted) value right after "outbound": or "final":
    static const QRegularExpression re(QStringLiteral(R"rx("(?:outbound|final)"\s*:\s*([^",}\]\s]*)$)rx"));
    const auto m = re.match(line);
    if (!m.hasMatch()) return false;
    if (partial) *partial = m.captured(1);
    return true;
}

void RawRouteEdit::insertOutboundCompletion(const QString& completion) {
    // completion is the popup display text (e.g. "[Group] Name"); insert its mapped id.
    const QString id = outboundIdByDisplay.value(completion);
    if (id.isEmpty()) return;
    QTextCursor tc = textCursor();
    tc.movePosition(QTextCursor::Left, QTextCursor::KeepAnchor, completer->completionPrefix().length());
    tc.removeSelectedText();
    tc.insertText(id);
    setTextCursor(tc);
}

void RawRouteEdit::updateCompleter() {
    QString partial;
    if (!outboundContext(&partial)) {
        completer->popup()->hide();
        return;
    }
    if (partial != completer->completionPrefix()) {
        completer->setCompletionPrefix(partial);
        completer->popup()->setCurrentIndex(completer->completionModel()->index(0, 0));
    }
    QRect cr = cursorRect();
    cr.setWidth(completer->popup()->sizeHintForColumn(0) + completer->popup()->verticalScrollBar()->sizeHint().width());
    completer->complete(cr);
}

void RawRouteEdit::keyPressEvent(QKeyEvent* e) {
    if (completer->popup()->isVisible()) {
        switch (e->key()) {
            case Qt::Key_Enter:
            case Qt::Key_Return:
            case Qt::Key_Escape:
            case Qt::Key_Tab:
            case Qt::Key_Backtab:
                e->ignore();
                return;
            default:
                break;
        }
    }

    JsonCodeEdit::keyPressEvent(e);
    updateCompleter();
}

RawRouteItem::RawRouteItem(QWidget* parent, const std::shared_ptr<Configs::RouteProfile>& routeChain) : QWidget(parent) {
    chain = std::make_shared<Configs::RouteProfile>(*routeChain);
    chain->isRaw = true;

    auto* layout = new QVBoxLayout(this);
    layout->setContentsMargins(0, 0, 0, 0);
    layout->setSpacing(0);

    auto* toolbar = new QFrame(this);
    toolbar->setProperty("role", QStringLiteral("toolbar"));
    auto* toolbarRow = new QHBoxLayout(toolbar);
    toolbarRow->setContentsMargins(14, 10, 14, 10);
    toolbarRow->setSpacing(8);
    nameEdit = new QLineEdit(chain->name, toolbar);
    nameEdit->setPlaceholderText(tr("Name"));
    nameEdit->setFixedWidth(200);
    toolbarRow->addWidget(nameEdit);
    layout->addWidget(toolbar);

    auto* tabs = new QTabWidget(this);
    auto* routePage = new QWidget(tabs);
    auto* routeLayout = new QVBoxLayout(routePage);
    routeLayout->setContentsMargins(14, 14, 14, 14);
    tabs->addTab(routePage, tr("Route"));
    dnsEditor = new RouteDnsEditor(*chain, tabs);
    tabs->addTab(dnsEditor, tr("DNS"));
    layout->addWidget(tabs, 1);

    preventCheck = new QCheckBox(tr("Prevent modifications"), this);
    preventCheck->setProperty("role", QStringLiteral("switch"));
    preventCheck->setChecked(chain->preventModifications);
    preventCheck->setToolTip(tr("Use the route object exactly as written (outbound ids are still resolved to tags).\n"
                                "Poros will NOT add its DNS-hijack or xray bridge plumbing, so DNS, chained/xray\n"
                                "outbounds and other Poros features may break. For advanced users only."));
    routeLayout->addWidget(preventCheck);

    jsonEdit = new RawRouteEdit(this);
    if (auto validator = JsonEdit::SingBoxValidator(JsonEdit::SingBox::Route)) {
        // Poros writes profile ids where sing-box writes outbound tags; TranslateRawOutbounds swaps them at build time.
        validator->AllowExtraType(QStringLiteral("outbound"), JsonEdit::ValueType::Number);
        validator->AllowExtraType(QStringLiteral("final"), JsonEdit::ValueType::Number);
        routeValidator = validator;
        jsonEdit->setValidator(validator);
    }
    jsonEdit->setPlainText(chain->rawRoute.isEmpty()
        ? QStringLiteral("{\n"
                         "  \"rules\": [\n"
                         "    {\n"
                         "      \"domain_suffix\": [\".example.com\"],\n"
                         "      \"outbound\": -2\n"
                         "    }\n"
                         "  ],\n"
                         "  \"final\": -1\n"
                         "}")
        : chain->rawRoute);
    routeLayout->addWidget(jsonEdit, 1);

    QList<QPair<QString, QString>> items;
    items.append({QStringLiteral("proxy"), QString::number(-1)});
    items.append({QStringLiteral("direct"), QString::number(-2)});
    items.append({QStringLiteral("warp-bypass"), QString::number(Configs::warpBypassID)});
    QMap<int, QString> idToName;
    for (const auto& [pid, pname] : Configs::dataManager->profilesRepo->GetAllProfileIDNameMapped())
        idToName.insert(pid, pname);
    for (int groupID : Configs::dataManager->groupsRepo->GetGroupsTabOrder()) {
        auto group = Configs::dataManager->groupsRepo->GetGroup(groupID);
        if (!group) continue;
        for (int profileID : group->profiles) {
            if (!idToName.contains(profileID)) continue;
            items.append({QString("[%1] %2").arg(group->name, idToName[profileID]), QString::number(profileID)});
        }
    }
    jsonEdit->setOutboundItems(items);

    issueList = new JsonEdit::JsonIssueList(this);
    issueList->attach(jsonEdit);
    routeLayout->addWidget(issueList);

    validateLabel = new QLabel(this);
    const auto refreshStatus = [this] {
        validateLabel->setText(jsonEdit->statusText());
        const auto &tk = themeManager()->tokens;
        validateLabel->setStyleSheet(QStringLiteral("color: %1;")
                                         .arg((jsonEdit->hasErrors() ? tk.danger : tk.success).name()));
    };
    connect(jsonEdit, &JsonEdit::JsonCodeEdit::issuesChanged, this, refreshStatus);
    refreshStatus();

    auto* formatBtn = new QPushButton(tr("Format JSON"), this);
    connect(formatBtn, &QPushButton::clicked, this, [this] {
        if (!jsonEdit->formatDocument()) MessageBoxInfo(tr("Raw route"), tr("The route must be a valid JSON object"));
    });

    auto* bottom = new QHBoxLayout();
    bottom->addWidget(validateLabel);
    bottom->addStretch();
    bottom->addWidget(formatBtn);
    routeLayout->addLayout(bottom);

    dnsForm = new Validation::Form(this);
    dnsForm->attachStatus(toolbarRow);
    dnsEditor->bindTo(dnsForm);
    dnsForm->markAll();
}

bool RawRouteItem::apply() {
    if (!dnsForm->validateAll()) return false;
    chain->name = nameEdit->text().trimmed();
    if (chain->name.isEmpty()) {
        MessageBoxWarning(tr("Invalid operation"), tr("Cannot create Route Profile with empty name"));
        return false;
    }
    const auto doc = QJsonDocument::fromJson(jsonEdit->toPlainText().toUtf8());
    if (!doc.isObject()) {
        MessageBoxWarning(tr("Invalid route"), tr("The route must be a valid JSON object"));
        return false;
    }
    // setValidator re-checks right away.
    if (routeValidator != nullptr) jsonEdit->setValidator(routeValidator);
    const auto problems = std::count_if(jsonEdit->issues().cbegin(), jsonEdit->issues().cend(),
                                        [](const JsonEdit::Issue& issue) { return issue.severity == JsonEdit::Severity::Error; });
    if (problems > 0) {
        QMessageBox box(QMessageBox::Warning, tr("Raw routing profile"),
                        tr("The route has %n schema problem(s), and the core will likely refuse to start with it. Save anyway?", "", static_cast<int>(problems)),
                        QMessageBox::Save | QMessageBox::Cancel, this);
        box.setDefaultButton(QMessageBox::Cancel);
        if (box.exec() != QMessageBox::Save) return false;
    }
    chain->isRaw = true;
    chain->preventModifications = preventCheck->isChecked();
    chain->rawRoute = QJsonObject2QString(doc.object(), false);
    dnsEditor->applyTo(*chain);
    return true;
}
