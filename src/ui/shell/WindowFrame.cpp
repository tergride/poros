#include "include/ui/shell/WindowFrame.h"

#include <QAbstractButton>
#include <QWidget>

#include <algorithm>

#include "include/ui/shell/TitleBar.h"

#include <windows.h>
#include <windowsx.h>
#include <dwmapi.h>

namespace {
    constexpr int resizeBorder = 6;

    // A maximized thick-frame window overflows the monitor by its frame; trim only what actually overflows.
    void fitMaximizedClient(HWND hwnd, NCCALCSIZE_PARAMS *params) {
        if (!IsZoomed(hwnd)) return;
        MONITORINFO info{sizeof(MONITORINFO)};
        GetMonitorInfoW(MonitorFromWindow(hwnd, MONITOR_DEFAULTTONEAREST), &info);
        auto &rect = params->rgrc[0];
        rect.left = std::max(rect.left, info.rcWork.left);
        rect.top = std::max(rect.top, info.rcWork.top);
        rect.right = std::min(rect.right, info.rcWork.right);
        rect.bottom = std::min(rect.bottom, info.rcWork.bottom);
    }

    LRESULT hitTest(QWidget *window, TitleBar *bar, const MSG *msg) {
        POINT point{GET_X_LPARAM(msg->lParam), GET_Y_LPARAM(msg->lParam)};
        RECT rect;
        GetWindowRect(msg->hwnd, &rect);
        if (!IsZoomed(msg->hwnd)) {
            const int edge = qRound(resizeBorder * window->devicePixelRatioF());
            const bool left = point.x < rect.left + edge;
            const bool right = point.x >= rect.right - edge;
            const bool top = point.y < rect.top + edge;
            const bool bottom = point.y >= rect.bottom - edge;
            if (top && left) return HTTOPLEFT;
            if (top && right) return HTTOPRIGHT;
            if (bottom && left) return HTBOTTOMLEFT;
            if (bottom && right) return HTBOTTOMRIGHT;
            if (left) return HTLEFT;
            if (right) return HTRIGHT;
            if (top) return HTTOP;
            if (bottom) return HTBOTTOM;
        }
        ScreenToClient(msg->hwnd, &point);
        const qreal ratio = window->devicePixelRatioF();
        const QPoint local = bar->mapFrom(window, QPoint(qRound(point.x / ratio), qRound(point.y / ratio)));
        if (!bar->rect().contains(local)) return HTCLIENT;
        if (bar->maximizeButton()->geometry().contains(local)) return HTMAXBUTTON;
        return bar->isCaption(local) ? HTCAPTION : HTCLIENT;
    }
} // namespace

namespace WindowFrame {
    void Prepare(QWidget *window) {
        window->setWindowFlags(Qt::Window | Qt::FramelessWindowHint | Qt::WindowMinMaxButtonsHint | Qt::WindowSystemMenuHint);
    }

    void Attach(QWidget *window) {
        const auto hwnd = reinterpret_cast<HWND>(window->winId());
        const auto style = GetWindowLongPtrW(hwnd, GWL_STYLE);
        SetWindowLongPtrW(hwnd, GWL_STYLE, style | WS_THICKFRAME | WS_CAPTION | WS_MINIMIZEBOX | WS_MAXIMIZEBOX | WS_SYSMENU);
        // One pixel of frame in the client area is what makes DWM keep the drop shadow.
        const MARGINS shadow{0, 0, 1, 0};
        DwmExtendFrameIntoClientArea(hwnd, &shadow);
        SetWindowPos(hwnd, nullptr, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER | SWP_FRAMECHANGED);
    }

    bool HandleNativeEvent(QWidget *window, TitleBar *bar, void *message, qintptr *result) {
        const auto *msg = static_cast<MSG *>(message);
        switch (msg->message) {
            case WM_NCCALCSIZE:
                if (msg->wParam == FALSE) return false;
                fitMaximizedClient(msg->hwnd, reinterpret_cast<NCCALCSIZE_PARAMS *>(msg->lParam));
                *result = 0;
                return true;
            case WM_NCHITTEST:
                *result = hitTest(window, bar, msg);
                return true;
            case WM_NCMOUSEMOVE:
                bar->setMaximizeHover(msg->wParam == HTMAXBUTTON);
                if (msg->wParam != HTMAXBUTTON) return false;
                {
                    TRACKMOUSEEVENT track{sizeof(TRACKMOUSEEVENT), TME_LEAVE | TME_NONCLIENT, msg->hwnd, 0};
                    TrackMouseEvent(&track);
                }
                *result = 0;
                return true;
            case WM_NCMOUSELEAVE:
                bar->setMaximizeHover(false);
                return false;
            case WM_NCLBUTTONDOWN:
                if (msg->wParam != HTMAXBUTTON) return false;
                *result = 0;
                return true;
            case WM_NCLBUTTONUP:
                if (msg->wParam != HTMAXBUTTON) return false;
                emit bar->maximizeToggled();
                *result = 0;
                return true;
            default:
                return false;
        }
    }
} // namespace WindowFrame
