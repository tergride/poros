#pragma once

#include <QWidget>

class ChoiceCard;
class QButtonGroup;
class QCheckBox;
class QLabel;
class QLineEdit;
class QPushButton;
class QStackedWidget;
class QTimer;

// The first-launch wizard: interface mode, subscription, how to start, routing preset. Every step can be skipped and it can run again later.
class OnboardingPage : public QWidget {
    Q_OBJECT

public:
    explicit OnboardingPage(QWidget *parent = nullptr);

    // From the start, with the settings as they are now.
    void restart();

    void setLink(const QString &link);

signals:
    // Skipping keeps the mode once its step is passed, and the subscription. serverLink is a one-server link to import, if any.
    void finished(const QString &uiMode, const QString &serverLink);
    void routeChosen(int routeId);
    void qrScanRequested();

protected:
    void paintEvent(QPaintEvent *event) override;

private:
    void showStep(int step);
    void checkLink();
    void setLinkStatus(const QString &text, const QString &icon, bool problem);
    void dropCheckedGroup();
    void applyChoices();
    [[nodiscard]] QString chosenMode() const;
    [[nodiscard]] QString serverLink() const;

    QWidget *m_stepper;
    QStackedWidget *m_steps;
    QPushButton *m_back;
    QPushButton *m_next;
    int m_step = 0;

    QLineEdit *m_link;
    QLabel *m_linkIcon;
    QLabel *m_linkStatus;
    QWidget *m_linkLine;
    QTimer *m_linkTimer;
    int m_linkToken = 0;
    int m_groupId = -1;

    QCheckBox *m_autoStart;
    QCheckBox *m_remember;
    QCheckBox *m_links;

    ChoiceCard *m_simple;
    ChoiceCard *m_advanced;

    QButtonGroup *m_routes;

    QPixmap m_art;
    QPixmap m_wordmark;
};
