#pragma once

#include <QEvent>
#include <QKeyEvent>
#include <QLineEdit>

// The search above the profile table: it keeps its keys from the window's shortcuts and hands the focus over to the table.
class ProfilesSearchEdit : public QLineEdit {
    Q_OBJECT
public:
    using QLineEdit::QLineEdit;

signals:
    void focusTableRequested(bool selectFirst);

protected:
    bool event(QEvent *event) override {
        // Window shortcuts resolve before the key reaches the field, so bare Return/Del would fire menu actions.
        if (event->type() == QEvent::ShortcutOverride && isTextEditingKey(static_cast<QKeyEvent *>(event))) {
            event->accept();
            return true;
        }
        return QLineEdit::event(event);
    }

    void keyPressEvent(QKeyEvent *event) override {
        switch (event->key()) {
        case Qt::Key_Escape:
            clear();
            emit focusTableRequested(false);
            return;
        case Qt::Key_Down:
            emit focusTableRequested(true);
            return;
        default:
            QLineEdit::keyPressEvent(event);
        }
    }

private:
    static bool isTextEditingKey(QKeyEvent *key) {
        if (!(key->modifiers() & (Qt::ControlModifier | Qt::AltModifier | Qt::MetaModifier))) {
            return key->key() < Qt::Key_F1 || key->key() > Qt::Key_F35;
        }
        for (auto standard : {QKeySequence::SelectAll, QKeySequence::Copy, QKeySequence::Cut,
                              QKeySequence::Paste, QKeySequence::Undo, QKeySequence::Redo,
                              QKeySequence::MoveToStartOfLine, QKeySequence::MoveToEndOfLine,
                              QKeySequence::SelectStartOfLine, QKeySequence::SelectEndOfLine,
                              QKeySequence::DeleteStartOfWord, QKeySequence::DeleteEndOfWord}) {
            if (key->matches(standard)) return true;
        }
        return false;
    }
};
