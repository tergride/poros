#pragma once

#include <QtGlobal>

class QWidget;
class TitleBar;

// A native window drawn edge to edge: Windows keeps the shadow, Snap and resizing, and the title bar is ours.
namespace WindowFrame {
    // Before the window is created.
    void Prepare(QWidget *window);

    // Once it has a native handle.
    void Attach(QWidget *window);

    // From nativeEvent(); true when the message was handled and *result holds the answer.
    bool HandleNativeEvent(QWidget *window, TitleBar *bar, void *message, qintptr *result);
} // namespace WindowFrame
