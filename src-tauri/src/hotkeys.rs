//! Shortcuts of Windows that act while Poros runs, its window hidden too. Windows sends `WM_HOTKEY` to the thread
//! that registered a shortcut, so a thread of its own holds them and waits for messages; a new list of shortcuts
//! reaches it as a message too.

use std::sync::{Arc, Mutex};

/// The actions in the order of `Hotkeys` of the core's settings.
pub const ACTIONS: [&str; 5] = ["show", "presets", "system", "tun", "start"];

const MOD_ALT: u32 = 0x1;
const MOD_CONTROL: u32 = 0x2;
const MOD_SHIFT: u32 = 0x4;
const MOD_WIN: u32 = 0x8;

/// A shortcut as the settings write it, "Ctrl+Alt+P", as the modifiers and the virtual key `RegisterHotKey` takes.
pub fn parse(text: &str) -> Option<(u32, u32)> {
    let mut parts: Vec<&str> = text.split('+').collect();
    let key = parts.pop()?;
    let mut modifiers = 0;
    for part in parts {
        modifiers |= match part {
            "Ctrl" => MOD_CONTROL,
            "Alt" => MOD_ALT,
            "Shift" => MOD_SHIFT,
            "Win" => MOD_WIN,
            _ => return None,
        };
    }
    let bytes = key.as_bytes();
    let vk = match key {
        _ if bytes.len() == 1 && (bytes[0].is_ascii_uppercase() || bytes[0].is_ascii_digit()) => {
            u32::from(bytes[0])
        }
        _ if key.starts_with('F') && key.len() > 1 => match key[1..].parse::<u32>() {
            Ok(n @ 1..=24) => 0x6F + n,
            _ => return None,
        },
        "Up" => 0x26,
        "Down" => 0x28,
        "Left" => 0x25,
        "Right" => 0x27,
        "Home" => 0x24,
        "End" => 0x23,
        "PageUp" => 0x21,
        "PageDown" => 0x22,
        "Insert" => 0x2D,
        "Delete" => 0x2E,
        "Space" => 0x20,
        "Enter" => 0x0D,
        "Tab" => 0x09,
        "Backspace" => 0x08,
        _ => return None,
    };
    Some((modifiers, vk))
}

/// The shortcuts of the settings, by action; "" is none.
pub type Wanted = Vec<(String, String)>;

/// The thread of the shortcuts: what the settings ask for, and the actions whose shortcut Windows refused, taken by
/// another program.
pub struct Hotkeys {
    wanted: Arc<Mutex<Wanted>>,
    failed: Arc<Mutex<Vec<String>>>,
    #[cfg(windows)]
    thread: u32,
}

impl Hotkeys {
    /// Starts the thread; `on_action` gets the action of a shortcut pressed, `on_failed` the refused ones after each
    /// new list.
    pub fn start(
        on_action: impl Fn(&str) + Send + 'static,
        on_failed: impl Fn(Vec<String>) + Send + 'static,
    ) -> Self {
        let wanted = Arc::new(Mutex::new(Wanted::new()));
        let failed = Arc::new(Mutex::new(Vec::new()));
        #[cfg(windows)]
        let thread = windows::spawn(wanted.clone(), failed.clone(), on_action, on_failed);
        #[cfg(not(windows))]
        let _ = (on_action, on_failed);
        Self {
            wanted,
            failed,
            #[cfg(windows)]
            thread,
        }
    }

    /// Takes a new list of shortcuts; the same list again changes nothing.
    pub fn set(&self, next: Wanted) {
        let mut wanted = self.wanted.lock().unwrap();
        if *wanted == next {
            return;
        }
        *wanted = next;
        #[cfg(windows)]
        windows::reload(self.thread);
    }

    pub fn failed(&self) -> Vec<String> {
        self.failed.lock().unwrap().clone()
    }
}

#[cfg(windows)]
mod windows {
    use std::ptr::null_mut;
    use std::sync::{Arc, Mutex, mpsc};

    use windows_sys::Win32::System::Threading::GetCurrentThreadId;
    use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
        MOD_NOREPEAT, RegisterHotKey, UnregisterHotKey,
    };
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        GetMessageW, MSG, PM_NOREMOVE, PeekMessageW, PostThreadMessageW, WM_APP, WM_HOTKEY, WM_USER,
    };

    use super::{Wanted, parse};

    const WM_RELOAD: u32 = WM_APP + 1;

    pub fn spawn(
        wanted: Arc<Mutex<Wanted>>,
        failed: Arc<Mutex<Vec<String>>>,
        on_action: impl Fn(&str) + Send + 'static,
        on_failed: impl Fn(Vec<String>) + Send + 'static,
    ) -> u32 {
        let (tx, rx) = mpsc::channel();
        std::thread::spawn(move || unsafe {
            let mut msg: MSG = std::mem::zeroed();
            // The queue of the thread exists once it asks for a message: only then can the list be posted to it.
            PeekMessageW(&mut msg, null_mut(), WM_USER, WM_USER, PM_NOREMOVE);
            let _ = tx.send(GetCurrentThreadId());
            let mut registered: Vec<(i32, String)> = Vec::new();
            while GetMessageW(&mut msg, null_mut(), 0, 0) > 0 {
                match msg.message {
                    WM_HOTKEY => {
                        if let Some((_, action)) =
                            registered.iter().find(|(id, _)| *id as usize == msg.wParam)
                        {
                            on_action(action);
                        }
                    }
                    WM_RELOAD => {
                        for (id, _) in registered.drain(..) {
                            UnregisterHotKey(null_mut(), id);
                        }
                        let list = wanted.lock().unwrap().clone();
                        let mut refused = Vec::new();
                        for (i, (action, text)) in list.iter().enumerate() {
                            if text.is_empty() {
                                continue;
                            }
                            let id = i as i32 + 1;
                            match parse(text) {
                                Some((modifiers, vk))
                                    if RegisterHotKey(
                                        null_mut(),
                                        id,
                                        modifiers | MOD_NOREPEAT,
                                        vk,
                                    ) != 0 =>
                                {
                                    registered.push((id, action.clone()));
                                }
                                _ => refused.push(action.clone()),
                            }
                        }
                        *failed.lock().unwrap() = refused.clone();
                        on_failed(refused);
                    }
                    _ => {}
                }
            }
        });
        rx.recv().unwrap_or(0)
    }

    pub fn reload(thread: u32) {
        unsafe { PostThreadMessageW(thread, WM_RELOAD, 0, 0) };
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reads_the_shortcuts_of_the_settings() {
        assert_eq!(parse("Ctrl+Alt+P"), Some((MOD_CONTROL | MOD_ALT, 0x50)));
        assert_eq!(
            parse("Ctrl+Shift+Win+F12"),
            Some((MOD_CONTROL | MOD_SHIFT | MOD_WIN, 0x7B))
        );
        assert_eq!(parse("Win+Up"), Some((MOD_WIN, 0x26)));
        assert_eq!(parse("Alt+1"), Some((MOD_ALT, 0x31)));
        assert_eq!(parse("F24"), Some((0, 0x87)));
        assert_eq!(parse("Ctrl+F25"), None);
        assert_eq!(parse("Meta+P"), None);
        assert_eq!(parse("Ctrl+p"), None);
        assert_eq!(parse(""), None);
    }
}
