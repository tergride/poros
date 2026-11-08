//! Starting the core process: as a child of the shell, or with administrator rights through UAC, which TUN needs,
//! or taking the one the sign-in task started with rights. The window always runs without rights; only the core is
//! elevated.

use std::path::Path;

use tokio::process::{Child, Command};

/// The arguments of the core: its channel, the token the shell greets it with, the data folder, the app's version,
/// the shell's process, the core it replaces, which it waits for, and a test's release feed.
pub struct Arguments<'a> {
    pub channel: &'a str,
    pub token: &'a str,
    pub data_dir: &'a Path,
    pub wait_pid: Option<u32>,
}

impl Arguments<'_> {
    fn list(&self) -> Vec<String> {
        let mut args = vec![
            "--pipe".to_string(),
            self.channel.to_string(),
            "--token".to_string(),
            self.token.to_string(),
            "--data-dir".to_string(),
            self.data_dir.display().to_string(),
            "--host-pid".to_string(),
            std::process::id().to_string(),
        ];
        if !crate::VERSION.is_empty() {
            args.push("--version".to_string());
            args.push(crate::VERSION.to_string());
        }
        if let Some(pid) = self.wait_pid {
            args.push("--wait-pid".to_string());
            args.push(pid.to_string());
        }
        // A test points the update check at its own release feed.
        if let Some(feed) = std::env::var_os("POROS_UPDATE_FEED") {
            args.push("--update-feed".to_string());
            args.push(feed.to_string_lossy().into_owned());
        }
        args
    }
}

/// How a core's process ended: in words, and its exit code when it had one.
pub struct Exit {
    pub text: String,
    pub code: Option<i64>,
}

/// A running core.
pub enum Process {
    Child(Box<Child>),
    /// A core elevated through UAC is no child of the shell: there is a handle, its exit code, and no stderr.
    #[cfg(windows)]
    Elevated(elevated::Process),
    /// The core of the sign-in task before it connects: it starts on its own, with rights, and is no process here
    /// until then.
    Awaited,
}

impl Process {
    pub fn pid(&self) -> Option<u32> {
        match self {
            Process::Child(child) => child.id(),
            #[cfg(windows)]
            Process::Elevated(process) => Some(process.pid),
            Process::Awaited => None,
        }
    }

    /// Whether it has rights; the core of the sign-in task counts once it came.
    pub fn elevated(&self) -> bool {
        #[cfg(windows)]
        if let Process::Elevated(_) = self {
            return true;
        }
        false
    }

    /// Waits for the exit and says how it went. Safe to cancel and call again.
    pub async fn wait(&mut self) -> Exit {
        match self {
            Process::Child(child) => match child.wait().await {
                Ok(status) => Exit {
                    text: format!("the core exited with {status}"),
                    code: status.code().map(i64::from),
                },
                Err(err) => Exit {
                    text: format!("the core is gone: {err}"),
                    code: None,
                },
            },
            #[cfg(windows)]
            Process::Elevated(process) => {
                let code = process.wait().await;
                Exit {
                    text: format!("the core exited with code {code}"),
                    code: Some(i64::from(code)),
                }
            }
            Process::Awaited => std::future::pending().await,
        }
    }

    /// Ends a core that did not exit on its own. An elevated one may refuse: it then exits by itself, once it sees
    /// the shell gone or its channel closed.
    pub async fn kill(&mut self) {
        match self {
            Process::Child(child) => {
                let _ = child.kill().await;
            }
            #[cfg(windows)]
            Process::Elevated(process) => process.terminate(),
            Process::Awaited => {}
        }
    }
}

/// Starts the core as a child, without a window.
pub fn spawn(program: &Path, args: &Arguments) -> std::io::Result<Process> {
    let mut command = Command::new(program);
    command
        .args(args.list())
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::piped())
        .kill_on_drop(true);
    #[cfg(windows)]
    {
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        command.creation_flags(CREATE_NO_WINDOW);
    }
    command.spawn().map(|child| Process::Child(Box::new(child)))
}

/// Why an elevated start did not happen.
#[derive(Debug)]
pub enum ElevationError {
    /// The user said no in the UAC prompt.
    Cancelled,
    Failed(String),
}

/// Starts the core with administrator rights: Windows shows the UAC prompt over `window`, and the call waits for the
/// answer. It must run on a thread that may block.
#[cfg(windows)]
pub fn spawn_elevated(
    program: &Path,
    args: &Arguments,
    window: Option<isize>,
) -> Result<Process, ElevationError> {
    elevated::spawn(program, &args.list(), window).map(Process::Elevated)
}

#[cfg(not(windows))]
pub fn spawn_elevated(
    _program: &Path,
    _args: &Arguments,
    _window: Option<isize>,
) -> Result<Process, ElevationError> {
    Err(ElevationError::Failed(
        "starting the core with administrator rights is done on Windows only".to_string(),
    ))
}

/// Takes the core of the sign-in task that connected from `pid`, once its program is this copy's core.
#[cfg(windows)]
pub fn adopt(pid: u32, program: &Path) -> Result<Process, String> {
    elevated::adopt(pid, program).map(Process::Elevated)
}

#[cfg(not(windows))]
pub fn adopt(_pid: u32, _program: &Path) -> Result<Process, String> {
    Err("the sign-in task is for Windows".to_string())
}

/// The process on the other end of the core channel, to make sure it is the core that was started.
#[cfg(windows)]
pub fn client_pid(pipe: &impl std::os::windows::io::AsRawHandle) -> Option<u32> {
    use windows_sys::Win32::System::Pipes::GetNamedPipeClientProcessId;
    let mut pid = 0u32;
    // SAFETY: the handle is the server end of a connected pipe, owned by the caller for the whole call.
    let ok = unsafe { GetNamedPipeClientProcessId(pipe.as_raw_handle(), &mut pid) };
    (ok != 0).then_some(pid)
}

#[cfg(windows)]
mod elevated {
    use std::os::windows::ffi::OsStrExt;
    use std::path::Path;

    use tokio::sync::watch;
    use windows_sys::Win32::Foundation::{CloseHandle, ERROR_CANCELLED, GetLastError, HANDLE};
    use windows_sys::Win32::System::Threading::{
        GetExitCodeProcess, GetProcessId, INFINITE, OpenProcess, PROCESS_NAME_WIN32,
        PROCESS_QUERY_LIMITED_INFORMATION, PROCESS_SYNCHRONIZE, PROCESS_TERMINATE,
        QueryFullProcessImageNameW, TerminateProcess, WaitForSingleObject,
    };
    use windows_sys::Win32::UI::Shell::{
        SEE_MASK_FLAG_NO_UI, SEE_MASK_NOASYNC, SEE_MASK_NOCLOSEPROCESS, SHELLEXECUTEINFOW,
        ShellExecuteExW,
    };
    use windows_sys::Win32::UI::WindowsAndMessaging::SW_HIDE;

    use super::ElevationError;

    /// The process handle UAC gave back; it moves between threads and is closed once.
    struct Handle(HANDLE);

    // SAFETY: a process handle may be used from any thread.
    unsafe impl Send for Handle {}
    // SAFETY: the handle is only read; the process object synchronizes itself.
    unsafe impl Sync for Handle {}

    impl Drop for Handle {
        fn drop(&mut self) {
            // SAFETY: the handle came from ShellExecuteExW and is closed only here.
            unsafe { CloseHandle(self.0) };
        }
    }

    pub struct Process {
        pub pid: u32,
        handle: std::sync::Arc<Handle>,
        exit: watch::Receiver<Option<u32>>,
    }

    impl Process {
        pub async fn wait(&mut self) -> u32 {
            match self.exit.wait_for(Option::is_some).await {
                Ok(code) => code.unwrap_or_default(),
                Err(_) => u32::MAX,
            }
        }

        pub fn terminate(&self) {
            // SAFETY: the handle is open; without the right to terminate the call fails and nothing happens.
            unsafe { TerminateProcess(self.handle.0, 1) };
        }
    }

    fn wide(text: &str) -> Vec<u16> {
        text.encode_utf16().chain(std::iter::once(0)).collect()
    }

    /// Quotes an argument the way CommandLineToArgvW and Go read it back.
    fn quote(arg: &str) -> String {
        if !arg.is_empty() && !arg.contains([' ', '\t', '"']) {
            return arg.to_string();
        }
        let mut quoted = String::from('"');
        let mut backslashes = 0;
        for c in arg.chars() {
            match c {
                '\\' => backslashes += 1,
                '"' => {
                    quoted.extend(std::iter::repeat_n('\\', backslashes * 2 + 1));
                    quoted.push('"');
                    backslashes = 0;
                }
                _ => {
                    quoted.extend(std::iter::repeat_n('\\', backslashes));
                    quoted.push(c);
                    backslashes = 0;
                }
            }
        }
        quoted.extend(std::iter::repeat_n('\\', backslashes * 2));
        quoted.push('"');
        quoted
    }

    pub fn spawn(
        program: &Path,
        args: &[String],
        window: Option<isize>,
    ) -> Result<Process, ElevationError> {
        let verb = wide("runas");
        let file: Vec<u16> = program
            .as_os_str()
            .encode_wide()
            .chain(std::iter::once(0))
            .collect();
        let parameters = wide(
            &args
                .iter()
                .map(|arg| quote(arg))
                .collect::<Vec<_>>()
                .join(" "),
        );
        let directory: Vec<u16> = program
            .parent()
            .unwrap_or(Path::new("."))
            .as_os_str()
            .encode_wide()
            .chain(std::iter::once(0))
            .collect();
        let mut info = SHELLEXECUTEINFOW {
            cbSize: std::mem::size_of::<SHELLEXECUTEINFOW>() as u32,
            // No error dialogs of the shell: a failure comes back here and the window tells it.
            fMask: SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC | SEE_MASK_FLAG_NO_UI,
            hwnd: window.unwrap_or_default() as _,
            lpVerb: verb.as_ptr(),
            lpFile: file.as_ptr(),
            lpParameters: parameters.as_ptr(),
            lpDirectory: directory.as_ptr(),
            nShow: SW_HIDE,
            ..Default::default()
        };
        // SAFETY: every pointer in info points into a buffer that lives until the call returns.
        if unsafe { ShellExecuteExW(&mut info) } == 0 {
            // SAFETY: reads the error of the call just made on this thread.
            let error = unsafe { GetLastError() };
            return Err(if error == ERROR_CANCELLED {
                ElevationError::Cancelled
            } else {
                ElevationError::Failed(std::io::Error::from_raw_os_error(error as i32).to_string())
            });
        }
        if info.hProcess.is_null() {
            return Err(ElevationError::Failed(
                "Windows started the core without a process handle".to_string(),
            ));
        }
        let handle = Handle(info.hProcess);
        // SAFETY: the handle is a live process handle.
        let pid = unsafe { GetProcessId(handle.0) };
        Ok(watched(pid, handle))
    }

    /// Opens the process of a core that started on its own and checks that it runs `program`.
    pub fn adopt(pid: u32, program: &Path) -> Result<Process, String> {
        // SAFETY: plain calls; a handle that opens is owned by Handle from here on. A core with rights may refuse
        // the right to end it to a shell without them: it then exits on its own once the channel closes.
        let raw = unsafe {
            let wanted = PROCESS_SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION;
            match OpenProcess(wanted | PROCESS_TERMINATE, 0, pid) {
                raw if raw.is_null() => OpenProcess(wanted, 0, pid),
                raw => raw,
            }
        };
        if raw.is_null() {
            return Err(format!(
                "cannot open process {pid}: {}",
                std::io::Error::last_os_error()
            ));
        }
        let handle = Handle(raw);
        let mut buffer = vec![0u16; 32768];
        let mut size = buffer.len() as u32;
        // SAFETY: the buffer holds `size` characters, and the call writes at most that many.
        if unsafe {
            QueryFullProcessImageNameW(handle.0, PROCESS_NAME_WIN32, buffer.as_mut_ptr(), &mut size)
        } == 0
        {
            return Err(format!(
                "the program of process {pid}: {}",
                std::io::Error::last_os_error()
            ));
        }
        let image = String::from_utf16_lossy(&buffer[..size as usize]);
        if !image.eq_ignore_ascii_case(&program.display().to_string()) {
            return Err(format!("{image} connected to the core channel"));
        }
        Ok(watched(pid, handle))
    }

    fn watched(pid: u32, handle: Handle) -> Process {
        let handle = std::sync::Arc::new(handle);
        // A thread waits for the exit and hands the code over; waiting on the receiver is safe to cancel.
        let (tx, exit) = watch::channel(None);
        let waiting = handle.clone();
        std::thread::spawn(move || {
            let mut code = 0u32;
            // SAFETY: the handle stays open while this thread holds its Arc.
            unsafe {
                WaitForSingleObject(waiting.0, INFINITE);
                GetExitCodeProcess(waiting.0, &mut code);
            }
            let _ = tx.send(Some(code));
        });
        Process { pid, handle, exit }
    }

    #[cfg(test)]
    mod tests {
        use super::quote;

        #[test]
        fn quotes_arguments_for_the_command_line() {
            assert_eq!(quote("plain"), "plain");
            assert_eq!(quote(r"C:\Poros Data\config"), r#""C:\Poros Data\config""#);
            assert_eq!(quote(r"C:\dir with space\"), r#""C:\dir with space\\""#);
            assert_eq!(quote(r#"a"b c"#), r#""a\"b c""#);
            assert_eq!(quote(""), r#""""#);
        }
    }
}

/// The explicit access list of the core channel: only this account, and administrators for a core elevated under
/// another one. Without it the default list lets everyone read.
#[cfg(windows)]
pub mod pipe_security {
    use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, LocalFree};
    use windows_sys::Win32::Security::Authorization::{
        ConvertSidToStringSidW, ConvertStringSecurityDescriptorToSecurityDescriptorW,
        SDDL_REVISION_1,
    };
    use windows_sys::Win32::Security::{
        GetTokenInformation, PSECURITY_DESCRIPTOR, SECURITY_ATTRIBUTES, TOKEN_QUERY, TOKEN_USER,
        TokenUser,
    };
    use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

    /// Security attributes that live as long as this value.
    pub struct Attributes {
        pub attributes: SECURITY_ATTRIBUTES,
        descriptor: PSECURITY_DESCRIPTOR,
    }

    impl Drop for Attributes {
        fn drop(&mut self) {
            // SAFETY: the descriptor came from ConvertStringSecurityDescriptorToSecurityDescriptorW.
            unsafe { LocalFree(self.descriptor) };
        }
    }

    fn user_sid() -> std::io::Result<String> {
        // SAFETY: plain Win32 calls on buffers sized by the first GetTokenInformation call; every handle and
        // allocation is released before returning.
        unsafe {
            let mut token: HANDLE = std::ptr::null_mut();
            if OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) == 0 {
                return Err(std::io::Error::last_os_error());
            }
            let mut size = 0u32;
            GetTokenInformation(token, TokenUser, std::ptr::null_mut(), 0, &mut size);
            let mut buffer = vec![0u64; (size as usize).div_ceil(8)];
            let ok = GetTokenInformation(
                token,
                TokenUser,
                buffer.as_mut_ptr().cast(),
                size,
                &mut size,
            );
            CloseHandle(token);
            if ok == 0 {
                return Err(std::io::Error::last_os_error());
            }
            let user = &*(buffer.as_ptr() as *const TOKEN_USER);
            let mut text: *mut u16 = std::ptr::null_mut();
            if ConvertSidToStringSidW(user.User.Sid, &mut text) == 0 {
                return Err(std::io::Error::last_os_error());
            }
            let length = (0..).take_while(|&i| *text.add(i) != 0).count();
            let sid = String::from_utf16_lossy(std::slice::from_raw_parts(text, length));
            LocalFree(text.cast());
            Ok(sid)
        }
    }

    pub fn attributes() -> std::io::Result<Attributes> {
        let sddl: Vec<u16> = format!("D:P(A;;GA;;;{})(A;;GA;;;BA)", user_sid()?)
            .encode_utf16()
            .chain(std::iter::once(0))
            .collect();
        let mut descriptor: PSECURITY_DESCRIPTOR = std::ptr::null_mut();
        // SAFETY: sddl is a NUL-terminated string; the descriptor is freed by Attributes.
        let ok = unsafe {
            ConvertStringSecurityDescriptorToSecurityDescriptorW(
                sddl.as_ptr(),
                SDDL_REVISION_1,
                &mut descriptor,
                std::ptr::null_mut(),
            )
        };
        if ok == 0 {
            return Err(std::io::Error::last_os_error());
        }
        Ok(Attributes {
            attributes: SECURITY_ATTRIBUTES {
                nLength: std::mem::size_of::<SECURITY_ATTRIBUTES>() as u32,
                lpSecurityDescriptor: descriptor,
                bInheritHandle: 0,
            },
            descriptor,
        })
    }
}
