use std::fs;
use std::path::{Path, PathBuf};

use tauri::AppHandle;

/// Windows builds are portable: the data lives in `config` next to the program.
#[cfg(windows)]
pub fn portable() -> Option<PathBuf> {
    let exe = std::env::current_exe().ok()?;
    Some(exe.parent()?.join("config"))
}

#[cfg(not(windows))]
pub fn portable() -> Option<PathBuf> {
    None
}

#[cfg(windows)]
pub fn resolve(_app: &AppHandle) -> Result<PathBuf, PathBuf> {
    let dir = portable().unwrap_or_else(|| PathBuf::from("config"));
    ensure_writable(&dir).map(|()| dir.clone()).map_err(|_| dir)
}

#[cfg(not(windows))]
pub fn resolve(app: &AppHandle) -> Result<PathBuf, PathBuf> {
    use tauri::Manager;
    let dir = app.path().app_data_dir().map_err(|_| PathBuf::from("."))?;
    ensure_writable(&dir).map(|()| dir.clone()).map_err(|_| dir)
}

fn ensure_writable(dir: &Path) -> std::io::Result<()> {
    fs::create_dir_all(dir)?;
    let probe = dir.join(".write-test");
    fs::write(&probe, b"")?;
    fs::remove_file(&probe)
}
