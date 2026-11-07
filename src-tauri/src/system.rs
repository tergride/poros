//! What Poros writes into the system for the user: starting with Windows and opening `poros://` links. Both point
//! at this copy of the portable Poros. Turning one off removes only an entry that points here, so another copy
//! keeps its own.
//!
//! Autostart follows the core's settings. `poros://` has no setting: the interface shows what Windows holds and
//! changes it when the user asks, and nothing is written at launch.

use std::path::Path;

use serde::Serialize;

/// The settings of the core that the shell applies.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct Integration {
    pub autostart: bool,
    pub start_minimized: bool,
}

/// Who opens `poros://` links now.
#[derive(Serialize, Clone, Copy, Debug, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum SchemeOwner {
    This,
    Other,
    None,
}

#[derive(Serialize, Clone, Debug, PartialEq)]
pub struct UrlScheme {
    pub owner: SchemeOwner,
    /// The program of another copy, "" otherwise.
    pub program: String,
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

/// Reads the registered command of `poros://`.
pub fn scheme_owner(command: Option<&str>, exe: &Path) -> UrlScheme {
    match command {
        None => UrlScheme {
            owner: SchemeOwner::None,
            program: String::new(),
        },
        Some(command) if points_at(command, exe) => UrlScheme {
            owner: SchemeOwner::This,
            program: String::new(),
        },
        Some(command) => {
            let program = match command.strip_prefix('"') {
                Some(rest) => rest.split('"').next().unwrap_or_default(),
                None => command.split_whitespace().next().unwrap_or_default(),
            };
            UrlScheme {
                owner: SchemeOwner::Other,
                program: program.to_owned(),
            }
        }
    }
}

#[cfg(windows)]
pub use windows::{apply, set_url_scheme, url_scheme};

#[cfg(not(windows))]
pub fn apply(_exe: &Path, _integration: Integration) -> std::io::Result<()> {
    Ok(())
}

/// Linux and Android register links their own way, later.
#[cfg(not(windows))]
pub fn url_scheme(exe: &Path) -> std::io::Result<UrlScheme> {
    Ok(scheme_owner(None, exe))
}

#[cfg(not(windows))]
pub fn set_url_scheme(exe: &Path, _on: bool) -> std::io::Result<UrlScheme> {
    url_scheme(exe)
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

    use super::{
        Integration, SchemeOwner, UrlScheme, autostart_command, points_at, scheme_owner,
        url_command,
    };

    const RUN: &str = r"Software\Microsoft\Windows\CurrentVersion\Run";
    const RUN_VALUE: &str = "Poros";
    const SCHEME: &str = r"Software\Classes\poros";
    const SCHEME_COMMAND: &str = r"Software\Classes\poros\shell\open\command";
    const SCHEME_ICON: &str = r"Software\Classes\poros\DefaultIcon";

    pub fn apply(exe: &Path, integration: Integration) -> io::Result<()> {
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

    pub fn url_scheme(exe: &Path) -> io::Result<UrlScheme> {
        Ok(scheme_owner(read(SCHEME_COMMAND, None)?.as_deref(), exe))
    }

    /// Takes the links for this copy, or gives them up when this copy has them.
    pub fn set_url_scheme(exe: &Path, on: bool) -> io::Result<UrlScheme> {
        let current = url_scheme(exe)?;
        if on && current.owner != SchemeOwner::This {
            write(SCHEME, None, "URL:Poros")?;
            write(SCHEME, Some("URL Protocol"), "")?;
            write(SCHEME_ICON, None, &format!("\"{}\",0", exe.display()))?;
            write(SCHEME_COMMAND, None, &url_command(exe))?;
        } else if !on && current.owner == SchemeOwner::This {
            // SAFETY: a null-terminated key name under the user's hive.
            check(unsafe { RegDeleteTreeW(HKEY_CURRENT_USER, wide(SCHEME).as_ptr()) })?;
        }
        url_scheme(exe)
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
            let autostart = Integration {
                autostart: true,
                start_minimized: true,
            };

            apply(&exe, autostart).unwrap();
            assert_eq!(
                read(RUN, Some(RUN_VALUE)).unwrap().as_deref(),
                Some(r#""C:\poros-test\Poros.exe" --autostart --minimized"#)
            );
            assert_eq!(set_url_scheme(&exe, true).unwrap().owner, SchemeOwner::This);
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
            let seen = set_url_scheme(&other, false).unwrap();
            assert_eq!(
                seen.owner,
                SchemeOwner::Other,
                "another copy removed this one's links"
            );
            assert_eq!(seen.program, r"C:\poros-test\Poros.exe");

            apply(&exe, Integration::default()).unwrap();
            assert_eq!(read(RUN, Some(RUN_VALUE)).unwrap(), None);
            assert_eq!(
                set_url_scheme(&exe, false).unwrap().owner,
                SchemeOwner::None
            );
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

    #[test]
    fn tells_who_opens_the_links() {
        let exe = PathBuf::from(r"D:\Apps\Poros\Poros.exe");
        assert_eq!(scheme_owner(None, &exe).owner, SchemeOwner::None);
        assert_eq!(
            scheme_owner(Some(&url_command(&exe)), &exe).owner,
            SchemeOwner::This
        );
        let other = scheme_owner(Some(r#""E:\Poros\Poros.exe" "%1""#), &exe);
        assert_eq!(
            (other.owner, other.program.as_str()),
            (SchemeOwner::Other, r"E:\Poros\Poros.exe")
        );
        assert_eq!(
            scheme_owner(Some(r"C:\Tools\viewer.exe %1"), &exe).program,
            r"C:\Tools\viewer.exe"
        );
    }
}
