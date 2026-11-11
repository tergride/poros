//! Writes text to the clipboard for the tray, whose items act while the window may be hidden: WebView2 writes the
//! clipboard only from a page that has the focus.

#[cfg(windows)]
pub use windows::write;

/// Linux and Android get their clipboard later.
#[cfg(not(windows))]
pub fn write(_text: &str) -> std::io::Result<()> {
    Err(std::io::Error::from(std::io::ErrorKind::Unsupported))
}

#[cfg(windows)]
mod windows {
    use std::io;
    use std::ptr::null_mut;
    use std::thread::sleep;
    use std::time::Duration;

    use windows_sys::Win32::Foundation::GlobalFree;
    use windows_sys::Win32::System::DataExchange::{
        CloseClipboard, EmptyClipboard, OpenClipboard, SetClipboardData,
    };
    use windows_sys::Win32::System::Memory::{
        GMEM_MOVEABLE, GlobalAlloc, GlobalLock, GlobalUnlock,
    };

    const CF_UNICODETEXT: u32 = 13;

    pub fn write(text: &str) -> io::Result<()> {
        let wide: Vec<u16> = text.encode_utf16().chain(Some(0)).collect();
        // Another program may hold the clipboard for a moment.
        let mut tries = 0;
        while unsafe { OpenClipboard(null_mut()) } == 0 {
            tries += 1;
            if tries == 10 {
                return Err(io::Error::last_os_error());
            }
            sleep(Duration::from_millis(20));
        }
        let result = put(&wide);
        unsafe { CloseClipboard() };
        result
    }

    fn put(wide: &[u16]) -> io::Result<()> {
        unsafe {
            if EmptyClipboard() == 0 {
                return Err(io::Error::last_os_error());
            }
            let memory = GlobalAlloc(GMEM_MOVEABLE, std::mem::size_of_val(wide));
            if memory.is_null() {
                return Err(io::Error::last_os_error());
            }
            let target = GlobalLock(memory) as *mut u16;
            if target.is_null() {
                let error = io::Error::last_os_error();
                GlobalFree(memory);
                return Err(error);
            }
            target.copy_from_nonoverlapping(wide.as_ptr(), wide.len());
            GlobalUnlock(memory);
            // The clipboard owns the memory once it takes it.
            if SetClipboardData(CF_UNICODETEXT, memory).is_null() {
                let error = io::Error::last_os_error();
                GlobalFree(memory);
                return Err(error);
            }
        }
        Ok(())
    }
}
