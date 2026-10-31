#include "include/ui/shell/SegmentSwitch.h"

#include <QHBoxLayout>
#include <QToolButton>

SegmentSwitch::SegmentSwitch(const QStringList &items, QWidget *parent) : QFrame(parent) {
    auto *row = new QHBoxLayout(this);
    row->setContentsMargins(3, 3, 3, 3);
    row->setSpacing(2);
    for (int i = 0; i < items.size(); i++) {
        auto *button = new QToolButton(this);
        button->setText(items[i]);
        button->setCheckable(true);
        button->setFocusPolicy(Qt::TabFocus);
        button->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Expanding);
        connect(button, &QToolButton::clicked, this, [this, i] {
            setCurrent(i);
            emit activated(i);
        });
        row->addWidget(button);
        m_buttons << button;
    }
}

void SegmentSwitch::setCurrent(int index) {
    m_current = index;
    for (int i = 0; i < m_buttons.size(); i++) m_buttons[i]->setChecked(i == index);
}
