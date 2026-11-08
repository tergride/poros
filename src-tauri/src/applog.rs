//! The log of the shell and the interface, `logs/app.log` in the data folder next to the core's: what the user saw
//! go wrong, and what happened to the core process. It goes on across runs and moves aside past a size, as the
//! core's does. Before `init`, as in tests, lines go nowhere.

use std::fs::{self, File, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};

const FILE: &str = "app.log";
const OLD_FILE: &str = "app.1.log";
const MAX_SIZE: u64 = 2 << 20;
/// A line from the interface is cut here: an error page or a stack should not fill the log.
const MAX_LINE: usize = 2000;

static LOG: OnceLock<Mutex<File>> = OnceLock::new();
static DIR: OnceLock<PathBuf> = OnceLock::new();

/// Opens the log in `<data_dir>/logs`; a log that cannot be opened leaves Poros without one.
pub fn init(data_dir: &Path) {
    let dir = data_dir.join("logs");
    let _ = DIR.set(dir.clone());
    if fs::create_dir_all(&dir).is_err() {
        return;
    }
    let path = dir.join(FILE);
    if fs::metadata(&path).is_ok_and(|meta| meta.len() > MAX_SIZE) {
        let _ = fs::rename(&path, dir.join(OLD_FILE));
    }
    if let Ok(file) = OpenOptions::new().create(true).append(true).open(&path) {
        let _ = LOG.set(Mutex::new(file));
    }
}

/// The folder of the logs, once `init` ran.
pub fn dir() -> Option<&'static PathBuf> {
    DIR.get()
}

/// Writes a line with the local time, the way the core's log has it.
pub fn write(text: &str) {
    let Some(log) = LOG.get() else {
        return;
    };
    let mut line: String = text
        .chars()
        .map(|c| if c == '\r' || c == '\n' { ' ' } else { c })
        .take(MAX_LINE)
        .collect();
    line.insert_str(0, &format!("{} ", now()));
    line.push('\n');
    if let Ok(mut file) = log.lock() {
        let _ = file.write_all(line.as_bytes());
    }
}

#[cfg(windows)]
fn now() -> String {
    use windows_sys::Win32::Globalization::{GetDateFormatEx, GetTimeFormatEx};

    fn wide(text: &str) -> Vec<u16> {
        text.encode_utf16().chain(Some(0)).collect()
    }
    let date_format = wide("yyyy/MM/dd");
    let time_format = wide("HH:mm:ss");
    let mut date = [0u16; 32];
    let mut time = [0u16; 32];
    // Without a date or time given, both take the current local one.
    // SAFETY: the format strings end with a zero and the buffers are as long as the lengths passed.
    let (date_len, time_len) = unsafe {
        (
            GetDateFormatEx(
                std::ptr::null(),
                0,
                std::ptr::null(),
                date_format.as_ptr(),
                date.as_mut_ptr(),
                date.len() as i32,
                std::ptr::null(),
            ),
            GetTimeFormatEx(
                std::ptr::null(),
                0,
                std::ptr::null(),
                time_format.as_ptr(),
                time.as_mut_ptr(),
                time.len() as i32,
            ),
        )
    };
    let text =
        |buffer: &[u16], len: i32| String::from_utf16_lossy(&buffer[..(len.max(1) as usize - 1)]);
    format!("{} {}", text(&date, date_len), text(&time, time_len))
}

#[cfg(not(windows))]
fn now() -> String {
    let secs = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|since| since.as_secs())
        .unwrap_or_default();
    format!("@{secs}")
}

/// Opens the folder of the logs in the file manager.
pub fn open_dir() -> Result<(), String> {
    let dir = dir().ok_or("the logs folder is not known yet")?;
    fs::create_dir_all(dir).map_err(|err| err.to_string())?;
    #[cfg(windows)]
    let program = "explorer.exe";
    #[cfg(not(windows))]
    let program = "xdg-open";
    std::process::Command::new(program)
        .arg(dir)
        .spawn()
        .map(|_| ())
        .map_err(|err| err.to_string())
}
