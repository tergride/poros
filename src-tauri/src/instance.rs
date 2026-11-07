//! One Poros per data folder. A second start hands its arguments, such as a `poros://` link, to the running one
//! over a named pipe and exits, and the running one shows its window.

use std::path::Path;

pub enum Claim {
    /// This is the only Poros for the folder; `Incoming` brings what later starts hand over.
    Primary(Incoming),
    /// The running Poros took the arguments.
    Handed,
}

/// The pipe name for a data folder: the same folder gives the same name in every build.
pub fn pipe_name(data_dir: &Path) -> String {
    // FNV-1a: stable across Rust versions, unlike the standard hasher.
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for byte in data_dir.display().to_string().to_lowercase().bytes() {
        hash ^= u64::from(byte);
        hash = hash.wrapping_mul(0x0100_0000_01b3);
    }
    format!(r"\\.\pipe\poros-app-{hash:016x}")
}

#[cfg(windows)]
pub use windows::{Incoming, claim};

#[cfg(windows)]
mod windows {
    use std::fs::OpenOptions;
    use std::io::{self, Write};
    use std::path::Path;
    use std::time::Duration;

    use tokio::io::{AsyncBufReadExt, AsyncReadExt, BufReader};
    use tokio::net::windows::named_pipe::{NamedPipeServer, ServerOptions};
    use windows_sys::Win32::Foundation::ERROR_PIPE_BUSY;
    use windows_sys::Win32::UI::WindowsAndMessaging::{ASFW_ANY, AllowSetForegroundWindow};

    use super::{Claim, pipe_name};
    use crate::core_launch::pipe_security;

    const MAX_MESSAGE: u64 = 64 * 1024;
    const READ_TIMEOUT: Duration = Duration::from_secs(5);

    pub struct Incoming {
        name: String,
        server: NamedPipeServer,
    }

    /// Must run inside the async runtime: the pipe server registers with it.
    pub fn claim(data_dir: &Path, args: &[String]) -> io::Result<Claim> {
        let name = pipe_name(data_dir);
        for _ in 0..40 {
            match OpenOptions::new().write(true).open(&name) {
                Ok(mut pipe) => {
                    // The running Poros may bring its window to the front: this start came from the user.
                    // SAFETY: a plain call without pointers.
                    unsafe { AllowSetForegroundWindow(ASFW_ANY) };
                    let mut line = serde_json::to_string(args)?;
                    line.push('\n');
                    pipe.write_all(line.as_bytes())?;
                    return Ok(Claim::Handed);
                }
                Err(error) if error.raw_os_error() == Some(ERROR_PIPE_BUSY as i32) => {
                    std::thread::sleep(Duration::from_millis(50));
                }
                Err(error) if error.kind() == io::ErrorKind::NotFound => {
                    match create(&name, true) {
                        Ok(server) => return Ok(Claim::Primary(Incoming { name, server })),
                        // Another start created the pipe a moment earlier: hand over to it.
                        Err(error) if error.kind() == io::ErrorKind::PermissionDenied => {}
                        Err(error) => return Err(error),
                    }
                }
                Err(error) => return Err(error),
            }
        }
        Err(io::Error::new(
            io::ErrorKind::TimedOut,
            "the running Poros does not take the arguments",
        ))
    }

    fn create(name: &str, first: bool) -> io::Result<NamedPipeServer> {
        let mut security = pipe_security::attributes()?;
        // SAFETY: the attributes and their descriptor live until create returns.
        unsafe {
            ServerOptions::new()
                .first_pipe_instance(first)
                .reject_remote_clients(true)
                .create_with_security_attributes_raw(name, (&raw mut security.attributes).cast())
        }
    }

    impl Incoming {
        /// Passes the arguments of every later start to `handler` until the task it gives back is aborted.
        pub fn serve(
            self,
            handler: impl Fn(Vec<String>) + Send + 'static,
        ) -> Option<tauri::async_runtime::JoinHandle<()>> {
            Some(tauri::async_runtime::spawn(async move {
                let Incoming { name, mut server } = self;
                loop {
                    let connected = server.connect().await;
                    // The next instance exists before this one is read, so the name is never free.
                    let Ok(next) = create(&name, false) else {
                        return;
                    };
                    let client = std::mem::replace(&mut server, next);
                    if connected.is_err() {
                        continue;
                    }
                    let mut line = String::new();
                    let mut reader = BufReader::new(client.take(MAX_MESSAGE));
                    if let Ok(Ok(_)) =
                        tokio::time::timeout(READ_TIMEOUT, reader.read_line(&mut line)).await
                        && let Ok(args) = serde_json::from_str::<Vec<String>>(&line)
                    {
                        handler(args);
                    }
                }
            }))
        }
    }
}

#[cfg(not(windows))]
pub struct Incoming;

#[cfg(not(windows))]
impl Incoming {
    pub fn serve(
        self,
        _handler: impl Fn(Vec<String>) + Send + 'static,
    ) -> Option<tauri::async_runtime::JoinHandle<()>> {
        None
    }
}

/// Linux and Android come later; until then every start runs on its own.
#[cfg(not(windows))]
pub fn claim(_data_dir: &Path, _args: &[String]) -> std::io::Result<Claim> {
    Ok(Claim::Primary(Incoming))
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;

    #[test]
    fn one_folder_gives_one_name() {
        let a = pipe_name(&PathBuf::from(r"D:\Apps\Poros\config"));
        assert_eq!(a, pipe_name(&PathBuf::from(r"d:\apps\poros\CONFIG")));
        assert_ne!(a, pipe_name(&PathBuf::from(r"D:\Apps\Poros2\config")));
        assert!(a.starts_with(r"\\.\pipe\poros-app-"));
    }

    #[cfg(windows)]
    #[test]
    fn a_second_start_hands_its_arguments_over() {
        let dir = std::env::temp_dir().join(format!("poros-instance-{}", std::process::id()));
        let (tx, rx) = std::sync::mpsc::channel();
        tauri::async_runtime::block_on(async {
            let Ok(Claim::Primary(incoming)) = claim(&dir, &[]) else {
                panic!("the first start must own the folder")
            };
            incoming.serve(move |args| tx.send(args).unwrap());
        });
        let second =
            tauri::async_runtime::block_on(async { claim(&dir, &["poros://route/x".to_owned()]) })
                .unwrap();
        assert!(matches!(second, Claim::Handed));
        let args = rx.recv_timeout(std::time::Duration::from_secs(5)).unwrap();
        assert_eq!(args, ["poros://route/x"]);
    }
}
