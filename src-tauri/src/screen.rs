//! Keeps the window out of snapshots of the screen while the core reads the QR codes on it: the window stays where it
//! is, and nothing flickers.

use tauri::{Runtime, WebviewWindow};

#[cfg(windows)]
pub fn exclude_from_capture<R: Runtime>(
    window: &WebviewWindow<R>,
    on: bool,
) -> std::io::Result<()> {
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        SetWindowDisplayAffinity, WDA_EXCLUDEFROMCAPTURE, WDA_NONE,
    };

    let hwnd = window.hwnd().map_err(std::io::Error::other)?;
    let affinity = if on { WDA_EXCLUDEFROMCAPTURE } else { WDA_NONE };
    // Windows before 10 2004 refuses it: the window is then on the snapshot, which only hides what it covers.
    if unsafe { SetWindowDisplayAffinity(hwnd.0, affinity) } == 0 {
        return Err(std::io::Error::last_os_error());
    }
    Ok(())
}

/// Linux and Android take their snapshots their own way, later.
#[cfg(not(windows))]
pub fn exclude_from_capture<R: Runtime>(
    _window: &WebviewWindow<R>,
    _on: bool,
) -> std::io::Result<()> {
    Ok(())
}
