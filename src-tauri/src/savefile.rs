//! The «Save as» window of the system for a file the interface made, such as a backup. The shell writes the file, so
//! it belongs to the user even while the core runs with the rights of an administrator.

use std::io;
use std::path::PathBuf;

/// Asks where to save `text` as a file named `name` of the kind `kind`, matched by `pattern` such as "*.json", over the
/// window `owner`, and writes it there; None when the user cancels.
#[cfg(windows)]
pub fn save(
    owner: isize,
    name: &str,
    kind: &str,
    pattern: &str,
    text: &str,
) -> io::Result<Option<PathBuf>> {
    use std::os::windows::ffi::OsStringExt;

    use windows_sys::Win32::UI::Controls::Dialogs::{
        CommDlgExtendedError, GetSaveFileNameW, OFN_EXPLORER, OFN_NOCHANGEDIR, OFN_OVERWRITEPROMPT,
        OFN_PATHMUSTEXIST, OPENFILENAMEW,
    };

    let wide = |s: &str| s.encode_utf16().collect::<Vec<u16>>();
    // The kind and its pattern, each ended by a zero, and the list by another.
    let mut filter = wide(&format!("{kind} ({pattern})"));
    filter.push(0);
    filter.extend(wide(pattern));
    filter.extend([0, 0]);
    let extension: Vec<u16> = wide(pattern.trim_start_matches("*."))
        .into_iter()
        .chain(Some(0))
        .collect();
    let mut file = vec![0u16; 32_768];
    let initial = wide(name);
    let length = initial.len().min(file.len() - 1);
    file[..length].copy_from_slice(&initial[..length]);

    let mut dialog = OPENFILENAMEW {
        lStructSize: std::mem::size_of::<OPENFILENAMEW>() as u32,
        hwndOwner: owner as _,
        lpstrFilter: filter.as_ptr(),
        lpstrFile: file.as_mut_ptr(),
        nMaxFile: file.len() as u32,
        lpstrDefExt: extension.as_ptr(),
        Flags: OFN_EXPLORER | OFN_OVERWRITEPROMPT | OFN_PATHMUSTEXIST | OFN_NOCHANGEDIR,
        ..Default::default()
    };
    if unsafe { GetSaveFileNameW(&mut dialog) } == 0 {
        // Zero is a cancel too: only an error code says otherwise.
        let code = unsafe { CommDlgExtendedError() };
        return if code == 0 {
            Ok(None)
        } else {
            Err(io::Error::other(format!(
                "the «Save as» window failed: {code:#x}"
            )))
        };
    }
    let end = file.iter().position(|&c| c == 0).unwrap_or(file.len());
    let path = PathBuf::from(std::ffi::OsString::from_wide(&file[..end]));
    std::fs::write(&path, text)?;
    Ok(Some(path))
}

/// Linux and Android ask their own way, later.
#[cfg(not(windows))]
pub fn save(
    _owner: isize,
    _name: &str,
    _kind: &str,
    _pattern: &str,
    _text: &str,
) -> io::Result<Option<PathBuf>> {
    Err(io::Error::from(io::ErrorKind::Unsupported))
}
