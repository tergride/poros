#include "include/ui/simple/SimpleHome.h"

#include <QFrame>
#include <QHBoxLayout>
#include <QLabel>
#include <QLineEdit>
#include <QPainter>
#include <QPushButton>
#include <QStackedWidget>
#include <QTimer>
#include <QVBoxLayout>

#include "include/ui/setting/PorosTheme.h"
#include "include/ui/simple/PortalPanel.h"
#include "include/ui/simple/ServerListView.h"
#include "include/ui/simple/ServerTreeModel.h"

namespace {
    QPushButton *simpleHomeButton(const QString &icon, const QString &text, QWidget *parent) {
        auto *button = new QPushButton(PorosTheme::Icon(icon), text, parent);
        button->setIconSize({16, 16});
        return button;
    }

    QWidget *simpleHomeEmptyState(QLineEdit **link, QPushButton **add, QPushButton **wizard, QWidget *parent) {
        auto *page = new QWidget(parent);
        auto *column = new QVBoxLayout(page);
        column->setContentsMargins(40, 40, 40, 40);
        column->setSpacing(14);
        column->addStretch();

        auto *mark = new QLabel(page);
        const qreal ratio = page->devicePixelRatioF();
        QPixmap pixmap = QPixmap(QStringLiteral(":/poros/img/mark.png")).scaledToHeight(qRound(64 * ratio), Qt::SmoothTransformation);
        pixmap.setDevicePixelRatio(ratio);
        mark->setPixmap(pixmap);
        mark->setAlignment(Qt::AlignCenter);
        column->addWidget(mark);

        auto *title = new QLabel(SimpleHome::tr("No servers yet"), page);
        title->setProperty("role", QStringLiteral("heading"));
        title->setAlignment(Qt::AlignCenter);
        column->addWidget(title);

        auto *text = new QLabel(SimpleHome::tr("Paste a subscription link and the servers will appear here."), page);
        text->setAlignment(Qt::AlignCenter);
        text->setWordWrap(true);
        column->addWidget(text);

        auto *row = new QHBoxLayout;
        row->setSpacing(8);
        *link = new QLineEdit(page);
        (*link)->setProperty("role", QStringLiteral("big"));
        (*link)->setPlaceholderText(QStringLiteral("https://…"));
        row->addWidget(*link, 1);
        *add = new QPushButton(SimpleHome::tr("Add"), page);
        (*add)->setProperty("variant", QStringLiteral("primary"));
        (*add)->setProperty("role", QStringLiteral("big"));
        row->addWidget(*add);
        auto *rowHolder = new QWidget(page);
        rowHolder->setMaximumWidth(520);
        rowHolder->setLayout(row);
        row->setContentsMargins(0, 0, 0, 0);
        column->addWidget(rowHolder, 0, Qt::AlignHCenter);
        rowHolder->setMinimumWidth(420);

        *wizard = simpleHomeButton(QStringLiteral("bolt"), SimpleHome::tr("Run quick setup"), page);
        (*wizard)->setProperty("variant", QStringLiteral("ghost"));
        column->addWidget(*wizard, 0, Qt::AlignHCenter);
        column->addStretch();
        return page;
    }
} // namespace

SimpleHome::SimpleHome(ServerTreeModel *model, QWidget *parent) : QWidget(parent), m_model(model) {
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(0, 0, 0, 0);
    row->setSpacing(0);

    m_portal = new PortalPanel(this);
    row->addWidget(m_portal);

    m_body = new QStackedWidget(this);
    row->addWidget(m_body, 1);

    auto *listPage = new QWidget(m_body);
    auto *listColumn = new QVBoxLayout(listPage);
    listColumn->setContentsMargins(0, 0, 0, 0);
    listColumn->setSpacing(0);

    auto *top = new QFrame(listPage);
    top->setProperty("role", QStringLiteral("toolbar"));
    auto *topRow = new QHBoxLayout(top);
    topRow->setContentsMargins(16, 14, 16, 14);
    topRow->setSpacing(8);
    m_search = new QLineEdit(top);
    m_search->setPlaceholderText(tr("Find a server"));
    m_search->addAction(PorosTheme::Icon(QStringLiteral("search")), QLineEdit::LeadingPosition);
    topRow->addWidget(m_search, 1);
    auto *test = simpleHomeButton(QStringLiteral("gauge"), tr("Test servers"), top);
    connect(test, &QPushButton::clicked, this, &SimpleHome::testRequested);
    topRow->addWidget(test);
    auto *add = simpleHomeButton(QStringLiteral("plus"), tr("Subscription"), top);
    connect(add, &QPushButton::clicked, this, [this] { emit addRequested({}); });
    topRow->addWidget(add);
    listColumn->addWidget(top);

    m_list = new ServerListView(model, listPage);
    listColumn->addWidget(m_list, 1);
    connect(m_list, &ServerListView::addRequested, this, [this] { emit addRequested({}); });
    m_body->addWidget(listPage);

    auto *searchDebounce = new QTimer(this);
    searchDebounce->setSingleShot(true);
    searchDebounce->setInterval(120);
    connect(m_search, &QLineEdit::textChanged, searchDebounce, qOverload<>(&QTimer::start));
    connect(searchDebounce, &QTimer::timeout, this, [this] { m_list->setFilterText(m_search->text()); });

    QPushButton *emptyAdd = nullptr;
    QPushButton *emptyWizard = nullptr;
    m_body->addWidget(simpleHomeEmptyState(&m_emptyLink, &emptyAdd, &emptyWizard, m_body));
    const auto submit = [this] {
        emit addRequested(m_emptyLink->text().trimmed());
        m_emptyLink->clear();
    };
    connect(emptyAdd, &QPushButton::clicked, this, submit);
    connect(m_emptyLink, &QLineEdit::returnPressed, this, submit);
    connect(emptyWizard, &QPushButton::clicked, this, &SimpleHome::wizardRequested);

    connect(model, &QAbstractItemModel::modelReset, this, &SimpleHome::syncEmptyState);
    syncEmptyState();
}

void SimpleHome::syncEmptyState() {
    m_body->setCurrentIndex(m_model->hasGroups() ? 0 : 1);
}
