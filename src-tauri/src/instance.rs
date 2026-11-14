//! One Poros per Windows user, and one per data folder. A second start of the same folder hands its arguments, such as
//! a `poros://` link, to the running one over a named pipe and exits, and the running one shows its window. A start
//! from another folder of the same user learns where the running Poros is, which shows its window, and exits. A
//! folder that another Windows user runs is told as such: its pipe lets in no one else. Another user's Poros in a
//! folder of their own is none of this: each user has a pipe of their own.
//!
//! `POROS_MULTI_INSTANCE=1` lets a build for development run beside the user's Poros: only the folder stays one.

use std::path::Path;

pub enum Claim {
    /// This is the only Poros for the folder and the user; `Incoming` brings what later starts hand over.
    Primary(Incoming),
    /// The running Poros took the arguments.
    Handed,
    /// This user runs Poros from another folder: the path of its program.
    OtherCopy(String),
    /// Another Windows user runs Poros from this folder.
    OtherUser,
}

/// The variable that lets a build for development run beside the user's Poros.
#[cfg(windows)]
const MULTI_INSTANCE: &str = "POROS_MULTI_INSTANCE";

// FNV-1a: stable across Rust versions, unlike the standard hasher.
fn fnv(text: &str) -> u64 {
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for byte in text.bytes() {
        hash ^= u64::from(byte);
        hash = hash.wrapping_mul(0x0100_0000_01b3);
    }
    hash
}

/// The pipe name for a data folder: the same folder gives the same name in every build.
pub fn pipe_name(data_dir: &Path) -> String {
    format!(
        r"\\.\pipe\poros-app-{:016x}",
        fnv(&data_dir.display().to_string().to_lowercase())
    )
}

/// The pipe name for a Windows account, from its SID.
#[cfg_attr(not(windows), allow(dead_code))]
pub fn user_pipe_name(sid: &str) -> String {
    format!(r"\\.\pipe\poros-user-{:016x}", fnv(&sid.to_uppercase()))
}

#[cfg(windows)]
fn multi_instance() -> bool {
    std::env::var_os(MULTI_INSTANCE).is_some_and(|value| value == "1")
}

#[cfg(windows)]
pub use windows::{Incoming, claim};

#[cfg(windows)]
mod windows {
    use std::fs::OpenOptions;
    use std::io::{self, BufRead, Write};
    use std::path::Path;
    use std::time::Duration;

    use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
    use tokio::net::windows::named_pipe::{NamedPipeServer, ServerOptions};
    use windows_sys::Win32::Foundation::ERROR_PIPE_BUSY;
    use windows_sys::Win32::UI::WindowsAndMessaging::{ASFW_ANY, AllowSetForegroundWindow};

    use super::{Claim, multi_instance, pipe_name, user_pipe_name};
    use crate::core_launch::pipe_security;

    const MAX_MESSAGE: u64 = 64 * 1024;
    const READ_TIMEOUT: Duration = Duration::from_secs(5);

    pub struct Incoming {
        folder: Server,
        user: Option<Server>,
    }

    struct Server {
        name: String,
        server: NamedPipeServer,
    }

    /// Must run inside the async runtime: the pipe servers register with it.
    pub fn claim(data_dir: &Path, args: &[String]) -> io::Result<Claim> {
        let user = if multi_instance() {
            None
        } else {
            Some(user_pipe_name(&pipe_security::user_sid()?))
        };
        claim_names(&pipe_name(data_dir), user.as_deref(), args)
    }

    fn busy(error: &io::Error) -> bool {
        error.raw_os_error() == Some(ERROR_PIPE_BUSY as i32)
    }

    pub(super) fn claim_names(
        folder: &str,
        user: Option<&str>,
        args: &[String],
    ) -> io::Result<Claim> {
        for _ in 0..40 {
            match OpenOptions::new().write(true).open(folder) {
                Ok(mut pipe) => {
                    // The running Poros may bring its window to the front: this start came from the user.
                    // SAFETY: a plain call without pointers.
                    unsafe { AllowSetForegroundWindow(ASFW_ANY) };
                    let mut line = serde_json::to_string(args)?;
                    line.push('\n');
                    pipe.write_all(line.as_bytes())?;
                    return Ok(Claim::Handed);
                }
                Err(error) if busy(&error) => {
                    std::thread::sleep(Duration::from_millis(50));
                    continue;
                }
                // The pipe lets in its own account alone: the folder is another user's.
                Err(error) if error.kind() == io::ErrorKind::PermissionDenied => {
                    return Ok(Claim::OtherUser);
                }
                Err(error) if error.kind() == io::ErrorKind::NotFound => {}
                Err(error) => return Err(error),
            }
            if let Some(user) = user {
                match OpenOptions::new().read(true).write(true).open(user) {
                    Ok(pipe) => {
                        // SAFETY: a plain call without pointers.
                        unsafe { AllowSetForegroundWindow(ASFW_ANY) };
                        return Ok(Claim::OtherCopy(read_path(pipe)?));
                    }
                    Err(error) if busy(&error) => {
                        std::thread::sleep(Duration::from_millis(50));
                        continue;
                    }
                    Err(error) if error.kind() == io::ErrorKind::NotFound => {}
                    Err(error) => return Err(error),
                }
            }
            // Another start may create a pipe a moment earlier: then it is asked again from the top.
            let folder_server = match create(folder, true) {
                Ok(server) => server,
                Err(error) if error.kind() == io::ErrorKind::PermissionDenied => continue,
                Err(error) => return Err(error),
            };
            let user_server = match user {
                None => None,
                Some(name) => match create(name, true) {
                    Ok(server) => Some(Server {
                        name: name.to_owned(),
                        server,
                    }),
                    Err(error) if error.kind() == io::ErrorKind::PermissionDenied => continue,
                    Err(error) => return Err(error),
                },
            };
            return Ok(Claim::Primary(Incoming {
                folder: Server {
                    name: folder.to_owned(),
                    server: folder_server,
                },
                user: user_server,
            }));
        }
        Err(io::Error::new(
            io::ErrorKind::TimedOut,
            "the running Poros does not answer",
        ))
    }

    /// The running Poros writes the path of its program at once; one that hangs is not waited for.
    fn read_path(pipe: std::fs::File) -> io::Result<String> {
        let (tx, rx) = std::sync::mpsc::channel();
        std::thread::spawn(move || {
            let mut line = String::new();
            let read = io::BufReader::new(pipe).read_line(&mut line);
            let _ = tx.send(read.map(|_| line.trim().to_owned()));
        });
        rx.recv_timeout(READ_TIMEOUT).unwrap_or_else(|_| {
            Err(io::Error::new(
                io::ErrorKind::TimedOut,
                "the running Poros did not say where it is",
            ))
        })
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
        /// Passes the arguments of every later start of the folder to `handler`, and an empty list for a start of
        /// another folder of this user, until the task it gives back is aborted.
        pub fn serve(
            self,
            handler: impl Fn(Vec<String>) + Send + Sync + 'static,
        ) -> Option<tauri::async_runtime::JoinHandle<()>> {
            let handler = std::sync::Arc::new(handler);
            let Incoming { folder, user } = self;
            let on_args = handler.clone();
            Some(tauri::async_runtime::spawn(async move {
                let folder = folder.serve(move |client| {
                    let handler = on_args.clone();
                    async move {
                        let mut line = String::new();
                        let mut reader = BufReader::new(client.take(MAX_MESSAGE));
                        if let Ok(Ok(_)) =
                            tokio::time::timeout(READ_TIMEOUT, reader.read_line(&mut line)).await
                            && let Ok(args) = serde_json::from_str::<Vec<String>>(&line)
                        {
                            handler(args);
                        }
                    }
                });
                let user = async move {
                    let Some(user) = user else { return };
                    user.serve(move |mut client| {
                        let handler = handler.clone();
                        async move {
                            let mut path = std::env::current_exe()
                                .map(|exe| exe.display().to_string())
                                .unwrap_or_default();
                            path.push('\n');
                            let _ = tokio::time::timeout(
                                READ_TIMEOUT,
                                client.write_all(path.as_bytes()),
                            )
                            .await;
                            handler(Vec::new());
                        }
                    })
                    .await
                };
                tokio::join!(folder, user);
            }))
        }
    }

    impl Server {
        async fn serve<F, Fut>(self, on_client: F)
        where
            F: Fn(NamedPipeServer) -> Fut,
            Fut: std::future::Future<Output = ()>,
        {
            let Server { name, mut server } = self;
            loop {
                let connected = server.connect().await;
                // The next instance exists before this one is answered, so the name is never free.
                let Ok(next) = create(&name, false) else {
                    return;
                };
                let client = std::mem::replace(&mut server, next);
                if connected.is_ok() {
                    on_client(client).await;
                }
            }
        }
    }
}

#[cfg(not(windows))]
pub struct Incoming;

#[cfg(not(windows))]
impl Incoming {
    pub fn serve(
        self,
        _handler: impl Fn(Vec<String>) + Send + Sync + 'static,
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
        let user = user_pipe_name("S-1-5-21-1-2-3-1001");
        assert_eq!(user, user_pipe_name("s-1-5-21-1-2-3-1001"));
        assert_ne!(user, user_pipe_name("S-1-5-21-1-2-3-1002"));
        assert!(user.starts_with(r"\\.\pipe\poros-user-"));
    }

    // The tests own their pipe names: a Poros running on this computer must neither answer them nor show its window.
    #[cfg(windows)]
    fn names(test: &str) -> (String, String, String) {
        let id = format!("{test}-{}", std::process::id());
        (
            format!(r"\\.\pipe\poros-test-folder-{id}"),
            format!(r"\\.\pipe\poros-test-folder2-{id}"),
            format!(r"\\.\pipe\poros-test-user-{id}"),
        )
    }

    #[cfg(windows)]
    fn primary(folder: &str, user: Option<&str>) -> std::sync::mpsc::Receiver<Vec<String>> {
        let (tx, rx) = std::sync::mpsc::channel();
        let tx = std::sync::Mutex::new(tx);
        tauri::async_runtime::block_on(async {
            let Ok(Claim::Primary(incoming)) = windows::claim_names(folder, user, &[]) else {
                panic!("the first start must own the folder")
            };
            incoming.serve(move |args| tx.lock().unwrap().send(args).unwrap());
        });
        rx
    }

    #[cfg(windows)]
    #[test]
    fn a_second_start_hands_its_arguments_over() {
        let (folder, _, user) = names("hand");
        let rx = primary(&folder, Some(&user));
        let second = tauri::async_runtime::block_on(async {
            windows::claim_names(&folder, Some(&user), &["poros://route/x".to_owned()])
        })
        .unwrap();
        assert!(matches!(second, Claim::Handed));
        let args = rx.recv_timeout(std::time::Duration::from_secs(5)).unwrap();
        assert_eq!(args, ["poros://route/x"]);
    }

    #[cfg(windows)]
    #[test]
    fn a_start_from_another_folder_learns_where_poros_runs() {
        let (folder, other, user) = names("copy");
        let rx = primary(&folder, Some(&user));
        let second = tauri::async_runtime::block_on(async {
            windows::claim_names(&other, Some(&user), &["poros://route/x".to_owned()])
        })
        .unwrap();
        let Claim::OtherCopy(path) = second else {
            panic!("a start from another folder of the user must not run")
        };
        assert_eq!(path, std::env::current_exe().unwrap().display().to_string());
        // The running one shows its window, and the links of the other copy are not its own.
        let args = rx.recv_timeout(std::time::Duration::from_secs(5)).unwrap();
        assert!(args.is_empty());
    }

    #[cfg(windows)]
    #[test]
    fn a_build_for_development_runs_beside() {
        let (folder, other, user) = names("multi");
        let _rx = primary(&folder, Some(&user));
        let second =
            tauri::async_runtime::block_on(async { windows::claim_names(&other, None, &[]) })
                .unwrap();
        assert!(matches!(second, Claim::Primary(_)));
    }

    // A folder pipe only SYSTEM may open stands for the Poros of another Windows user in this folder.
    #[cfg(windows)]
    #[test]
    fn a_folder_another_user_runs_is_told() {
        use tokio::net::windows::named_pipe::ServerOptions;

        let (folder, _, user) = names("user");
        let mut security = crate::core_launch::pipe_security::from_sddl("D:P(A;;GA;;;SY)").unwrap();
        let _server = tauri::async_runtime::block_on(async {
            // SAFETY: the attributes and their descriptor live until create returns.
            unsafe {
                ServerOptions::new()
                    .first_pipe_instance(true)
                    .create_with_security_attributes_raw(
                        &folder,
                        (&raw mut security.attributes).cast(),
                    )
            }
            .unwrap()
        });
        let second = tauri::async_runtime::block_on(async {
            windows::claim_names(&folder, Some(&user), &[])
        })
        .unwrap();
        assert!(matches!(second, Claim::OtherUser));
    }
}
