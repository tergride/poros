//! Errors shown before the window exists, so they use the system message box and the system language.

use std::path::Path;

#[cfg(windows)]
const WEBVIEW2_PAGE: &str = "https://developer.microsoft.com/microsoft-edge/webview2/";

#[cfg(windows)]
mod native {
    use std::iter::once;
    use std::ptr::null_mut;

    use windows_sys::Win32::Globalization::GetUserDefaultUILanguage;
    use windows_sys::Win32::UI::Shell::ShellExecuteW;
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        IDYES, MB_ICONERROR, MB_ICONINFORMATION, MB_OK, MB_SETFOREGROUND, MB_TOPMOST, MB_YESNO,
        MessageBoxW, SW_SHOWNORMAL,
    };

    const LANG_RUSSIAN: u16 = 0x19;

    fn wide(text: &str) -> Vec<u16> {
        text.encode_utf16().chain(once(0)).collect()
    }

    pub fn russian() -> bool {
        unsafe { GetUserDefaultUILanguage() & 0x3ff == LANG_RUSSIAN }
    }

    pub fn error(text: &str) {
        let (text, caption) = (wide(text), wide("Poros"));
        unsafe {
            MessageBoxW(
                null_mut(),
                text.as_ptr(),
                caption.as_ptr(),
                MB_OK | MB_ICONERROR,
            )
        };
    }

    /// On top: the running Poros brings its window forward at the same moment.
    pub fn info(text: &str) {
        let (text, caption) = (wide(text), wide("Poros"));
        unsafe {
            MessageBoxW(
                null_mut(),
                text.as_ptr(),
                caption.as_ptr(),
                MB_OK | MB_ICONINFORMATION | MB_SETFOREGROUND | MB_TOPMOST,
            )
        };
    }

    pub fn ask(text: &str) -> bool {
        let (text, caption) = (wide(text), wide("Poros"));
        unsafe {
            MessageBoxW(
                null_mut(),
                text.as_ptr(),
                caption.as_ptr(),
                MB_YESNO | MB_ICONERROR,
            ) == IDYES
        }
    }

    pub fn open(url: &str) {
        let (verb, url) = (wide("open"), wide(url));
        unsafe {
            ShellExecuteW(
                null_mut(),
                verb.as_ptr(),
                url.as_ptr(),
                null_mut(),
                null_mut(),
                SW_SHOWNORMAL,
            );
        }
    }
}

#[cfg(not(windows))]
mod native {
    pub fn russian() -> bool {
        false
    }

    pub fn error(text: &str) {
        eprintln!("{text}");
    }

    pub fn info(text: &str) {
        eprintln!("{text}");
    }
}

#[cfg(windows)]
pub fn webview_missing() {
    let text = if native::russian() {
        "Для работы Poros нужна среда Microsoft Edge WebView2, а в системе её нет.\n\nОткрыть страницу, где её можно скачать?"
    } else {
        "Poros needs the Microsoft Edge WebView2 Runtime, and it is not installed.\n\nOpen the page where you can download it?"
    };
    if native::ask(text) {
        native::open(WEBVIEW2_PAGE);
    }
}

pub fn data_dir_unwritable(dir: &Path) -> ! {
    let dir = dir.display();
    let text = if native::russian() {
        format!(
            "Poros не может записывать в папку данных:\n{dir}\n\nПереместите папку Poros туда, где у вас есть права на запись."
        )
    } else {
        format!(
            "Poros cannot write to its data folder:\n{dir}\n\nMove the Poros folder to a place where you can write."
        )
    };
    native::error(&text);
    std::process::exit(1)
}

/// This user runs Poros from another folder: one Poros per user, or the two would fight over the port, the TUN and
/// the proxy of Windows.
pub fn other_copy(path: &str) {
    let text = if native::russian() {
        format!(
            "Poros уже работает из другой папки:
{path}

Чтобы запустить эту копию, сначала закройте ту: «Выход» в меню её значка в трее."
        )
    } else {
        format!(
            "Poros is already running from another folder:
{path}

To start this copy, quit that one first: Exit in the menu of its tray icon."
        )
    };
    native::info(&text);
}

/// Another Windows user runs Poros from this folder: its data is theirs while it runs.
pub fn folder_of_other_user() {
    let text = if native::russian() {
        "Эту копию Poros уже запустил другой пользователь Windows.

У каждого пользователя должна быть своя копия: распакуйте Poros в другую папку."
    } else {
        "Another Windows user is already running this copy of Poros.

Each user needs a copy of their own: unpack Poros into another folder."
    };
    native::info(text);
}
