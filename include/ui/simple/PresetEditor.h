#pragma once

#include <QList>
#include <QWidget>

#include "include/configs/PresetRules.h"

class QComboBox;
class QLabel;
class QLineEdit;
class QVBoxLayout;

namespace Configs {
    class RouteProfile;
}

// Edits the user's copy of a preset: name, own sites and programs, DNS.
class PresetEditor : public QWidget {
    Q_OBJECT

public:
    explicit PresetEditor(QWidget *parent = nullptr);

    void edit(const std::shared_ptr<Configs::RouteProfile> &preset);

signals:
    void closed();
    void saved(int presetId);

private:
    void rebuildRules();
    void addRule(bool program, QLineEdit *input);
    void save();

    std::shared_ptr<Configs::RouteProfile> m_preset;
    QList<Configs::PresetRules::UserRule> m_rules;

    QLabel *m_base;
    QLineEdit *m_name;
    QLineEdit *m_siteInput;
    QLineEdit *m_programInput;
    QVBoxLayout *m_sites;
    QVBoxLayout *m_programs;
    QVBoxLayout *m_locked;
    QComboBox *m_dns;
    QLineEdit *m_dnsCustom;
    QLabel *m_problem;
};
