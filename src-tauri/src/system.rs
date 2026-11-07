//! What Poros writes into the system for the user: starting with Windows and opening `poros://` links. Both point
//! at this copy of the portable Poros. Turning one off removes only an entry that points here, so another copy
//! keeps its own.

use std::path::Path;

/// The settings of the core that the shell applies.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct Integration {
    pub autostart: bool,
    pub start_minimized: bool,
    pub url_scheme: bool,
}

/// The command line Windows runs at sign-in.
pub fn autostart_command(exe: &Path, minimized: bool) -> String {
    let flag = if minimized { " --minimized" } else { "" };
    format!("\"{}\" --autostart{flag}", exe.display())
}

/// The command line Windows runs for a `poros://` link.
pub fn url_command(exe: &Path) -> String {
    format!("\"{}\" \"%1\"", exe.display())
}

/// Whether a command line written by Poros starts this program.
pub fn points_at(command: &str, exe: &Path) -> bool {
    let quoted = format!("\"{}\"", exe.display()).to_lowercase();
    command.to_lowercase().starts_with(&quoted)
}

#[cfg(windows)]
pub use windows::apply;

#[cfg(not(windows))]
pub fn apply(_exe: &Path, _integration: Integration) -> std::io::Result<()> {
    Ok(())
}

#[cfg(windows)]
mod windows {
    use std::io;
    use std::path::Path;

    use windows_sys::Win32::Foundation::{ERROR_FILE_NOT_FOUND, ERROR_SUCCESS, WIN32_ERROR};
    use windows_sys::Win32::System::Registry::{
        HKEY_CURRENT_USER, REG_SZ, RRF_RT_REG_SZ, RegDeleteKeyValueW, RegDeleteTreeW, RegGetValueW,
        RegSetKeyValueW,
    };

    use super::{Integration, autostart_command, points_at, url_command};

    const RUN: &str = r"Software\Microsoft\Windows\CurrentVersion\Run";
    const RUN_VALUE: &str = "Poros";
    const SCHEME: &str = r"Software\Classes\poros";
    const SCHEME_COMMAND: &str = r"Software\Classes\poros\shell\open\command";
    const SCHEME_ICON: &str = r"Software\Classes\poros\DefaultIcon";

    pub fn apply(exe: &Path, integration: Integration) -> io::Result<()> {
        let autostart = set_autostart(exe, integration);
        set_url_scheme(exe, integration.url_scheme)?;
        autostart
    }

    fn set_autostart(exe: &Path, integration: Integration) -> io::Result<()> {
        let current = read(RUN, Some(RUN_VALUE))?;
        if integration.autostart {
            let command = autostart_command(exe, integration.start_minimized);
            if current.as_deref() != Some(command.as_str()) {
                write(RUN, Some(RUN_VALUE), &command)?;
            }
        } else if current.is_some_and(|command| points_at(&command, exe)) {
            // SAFETY: null-terminated key and value names under the user's hive.
            check(unsafe {
                RegDeleteKeyValueW(
                    HKEY_CURRENT_USER,
                    wide(RUN).as_ptr(),
                    wide(RUN_VALUE).as_ptr(),
                )
            })?;
        }
        Ok(())
    }

    fn set_url_scheme(exe: &Path, on: bool) -> io::Result<()> {
        let current = read(SCHEME_COMMAND, None)?;
        if on {
            let command = url_command(exe);
            if current.as_deref() != Some(command.as_str()) {
                write(SCHEME, None, "URL:Poros")?;
                write(SCHEME, Some("URL Protocol"), "")?;
                write(SCHEME_ICON, None, &format!("\"{}\",0", exe.display()))?;
                write(SCHEME_COMMAND, None, &command)?;
            }
        } else if current.is_some_and(|command| points_at(&command, exe)) {
            // SAFETY: a null-terminated key name under the user's hive.
            check(unsafe { RegDeleteTreeW(HKEY_CURRENT_USER, wide(SCHEME).as_ptr()) })?;
        }
        Ok(())
    }

    fn wide(text: &str) -> Vec<u16> {
        text.encode_utf16().chain(Some(0)).collect()
    }

    fn check(status: WIN32_ERROR) -> io::Result<()> {
        if status == ERROR_SUCCESS {
            Ok(())
        } else {
            Err(io::Error::from_raw_os_error(status as i32))
        }
    }

    /// A string value under HKEY_CURRENT_USER, None when the key or the value is missing.
    fn read(key: &str, value: Option<&str>) -> io::Result<Option<String>> {
        let key = wide(key);
        let value = value.map(wide);
        let value_ptr = value.as_ref().map_or(std::ptr::null(), |v| v.as_ptr());
        let mut buffer = vec![0u16; 1024];
        let mut size = (buffer.len() * 2) as u32;
        // SAFETY: the names are null-terminated and the buffer holds `size` bytes.
        let status = unsafe {
            RegGetValueW(
                HKEY_CURRENT_USER,
                key.as_ptr(),
                value_ptr,
                RRF_RT_REG_SZ,
                std::ptr::null_mut(),
                buffer.as_mut_ptr().cast(),
                &mut size,
            )
        };
        if status == ERROR_FILE_NOT_FOUND {
            return Ok(None);
        }
        check(status)?;
        let length = (size as usize / 2).saturating_sub(1);
        Ok(Some(String::from_utf16_lossy(&buffer[..length])))
    }

    /// Writes a string value, creating the key when it is missing.
    fn write(key: &str, value: Option<&str>, data: &str) -> io::Result<()> {
        let key = wide(key);
        let value = value.map(wide);
        let value_ptr = value.as_ref().map_or(std::ptr::null(), |v| v.as_ptr());
        let data = wide(data);
        // SAFETY: the names and the data are null-terminated, and the size counts the data's bytes.
        check(unsafe {
            RegSetKeyValueW(
                HKEY_CURRENT_USER,
                key.as_ptr(),
                value_ptr,
                REG_SZ,
                data.as_ptr().cast(),
                (data.len() * 2) as u32,
            )
        })
    }

    #[cfg(test)]
    mod tests {
        use std::path::PathBuf;

        use super::*;

        /// Writes into the user's registry, so it runs only in the test machine.
        #[test]
        fn registers_and_removes_only_this_copy() {
            if std::env::var_os("POROS_VM_TESTS").is_none() {
                return;
            }
            let exe = PathBuf::from(r"C:\poros-test\Poros.exe");
            let other = PathBuf::from(r"C:\other\Poros.exe");
            let all = Integration {
                autostart: true,
                start_minimized: true,
                url_scheme: true,
            };

            apply(&exe, all).unwrap();
            assert_eq!(
                read(RUN, Some(RUN_VALUE)).unwrap().as_deref(),
                Some(r#""C:\poros-test\Poros.exe" --autostart --minimized"#)
            );
            assert_eq!(
                read(SCHEME_COMMAND, None).unwrap().as_deref(),
                Some(r#""C:\poros-test\Poros.exe" "%1""#)
            );
            assert_eq!(
                read(SCHEME, Some("URL Protocol")).unwrap().as_deref(),
                Some("")
            );

            apply(&other, Integration::default()).unwrap();
            assert!(
                read(RUN, Some(RUN_VALUE)).unwrap().is_some(),
                "another copy removed this one's autostart"
            );
            assert!(
                read(SCHEME_COMMAND, None).unwrap().is_some(),
                "another copy removed this one's links"
            );

            apply(&exe, Integration::default()).unwrap();
            assert_eq!(read(RUN, Some(RUN_VALUE)).unwrap(), None);
            assert_eq!(read(SCHEME_COMMAND, None).unwrap(), None);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;

    #[test]
    fn commands_point_at_the_program() {
        let exe = PathBuf::from(r"D:\Apps\Poros\Poros.exe");
        assert_eq!(
            autostart_command(&exe, false),
            r#""D:\Apps\Poros\Poros.exe" --autostart"#
        );
        assert!(points_at(&autostart_command(&exe, true), &exe));
        assert!(points_at(&url_command(&exe).to_uppercase(), &exe));
        assert!(!points_at(
            &url_command(&PathBuf::from(r"D:\Apps\Poros2\Poros.exe")),
            &exe
        ));
    }
}
