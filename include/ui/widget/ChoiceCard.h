#pragma once

#include <QAbstractButton>
#include <QStringList>

class QVBoxLayout;

// A whole block that is the control: pressing anywhere on it chooses it.
class ChoiceCard : public QAbstractButton {
    Q_OBJECT

public:
    ChoiceCard(const QString &title, const QString &text, const QStringList &points, QWidget *parent = nullptr);

    QSize sizeHint() const override;

protected:
    void paintEvent(QPaintEvent *event) override;

private:
    QVBoxLayout *m_column;
};
