mod applog;
mod core_launch;
mod core_process;
mod data_dir;
mod fatal;
mod instance;
mod system;
mod tray;

use std::path::PathBuf;
use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Duration;

use serde_json::{Value, json};
use tauri::webview::{PermissionKind, PermissionResponse};
use tauri::{AppHandle, Emitter, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder, WindowEvent};

use crate::core_process::{
    CODE_UPDATE_FAILED, Core, CoreConfig, CoreError, CoreState, CoreStatus, Sink,
};
use crate::instance::Claim;
use crate::system::{Integration, UrlScheme};

/// The version of a release build, from `POROS_VERSION` at build time; a local build has none.
pub const VERSION: &str = match option_env!("POROS_VERSION") {
    Some(version) => version,
    None => "",
};

/// What the shell keeps besides the core.
struct Shell {
    /// The window's close button hides it to the tray; follows the core's settings. Until they come it closes
    /// Poros: a core that never starts gives the tray no menu to quit from.
    close_to_tray: AtomicBool,
    /// `poros://` links from the command line and from later starts, until the interface takes them.
    links: Mutex<Vec<String>>,
    /// What was last written into the system, to write only changes.
    integration: Mutex<Option<Integration>>,
    /// The updater started this Poros after an install, and the connection that ran before comes back.
    reconnect: AtomicBool,
    /// Poros is on its way out: nothing starts any more.
    exiting: AtomicBool,
    /// The server that takes the arguments of later starts; it closes first on the way out, so that a start in
    /// the meantime runs as a Poros of its own instead of handing its links to one that is going away.
    instance: Mutex<Option<tauri::async_runtime::JoinHandle<()>>>,
}

#[tauri::command]
fn app_show(app: AppHandle) {
    show_window(&app);
}

#[tauri::command]
fn app_hide(app: AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.hide();
    }
}

#[tauri::command]
fn app_exit(app: AppHandle) {
    app.exit(0);
}

/// Whether this start should connect as the run before the update did; true once.
#[tauri::command]
fn app_take_reconnect(shell: tauri::State<'_, Shell>) -> bool {
    shell.reconnect.swap(false, Ordering::Relaxed)
}

/// Installs the downloaded release: the updater waits for this process to exit, swaps the files and starts Poros
/// again, connected when `reconnect` says so. It runs as a copy, so that the release may replace updater.exe too.
#[tauri::command]
async fn app_update(
    app: AppHandle,
    core: tauri::State<'_, Core>,
    reconnect: bool,
) -> Result<(), CoreError> {
    let install = core.call("updates.install", Value::Null).await?;
    let archive = install
        .get("archive")
        .and_then(Value::as_str)
        .ok_or_else(|| CoreError::local(CODE_UPDATE_FAILED, "the core gave no archive"))?;
    let hidden = app
        .get_webview_window("main")
        .and_then(|window| window.is_visible().ok())
        .is_some_and(|visible| !visible);
    let failed = |err: std::io::Error| CoreError::local(CODE_UPDATE_FAILED, err.to_string());
    let exe = std::env::current_exe().map_err(failed)?;
    let dir = exe.parent().unwrap_or(std::path::Path::new("."));
    let updater = dir.join("updater.old");
    std::fs::copy(dir.join("updater.exe"), &updater).map_err(failed)?;
    std::process::Command::new(&updater)
        .args(updater_arguments(
            std::process::id(),
            archive,
            dir,
            reconnect,
            hidden,
        ))
        .current_dir(dir)
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .spawn()
        .map_err(failed)?;
    app.exit(0);
    Ok(())
}

/// The updater's arguments, and after `--` those of the Poros it starts.
fn updater_arguments(
    pid: u32,
    archive: &str,
    dir: &std::path::Path,
    reconnect: bool,
    hidden: bool,
) -> Vec<String> {
    let mut args = vec![
        "-pid".to_string(),
        pid.to_string(),
        "-archive".to_string(),
        archive.to_string(),
        "-dir".to_string(),
        dir.display().to_string(),
        "--".to_string(),
    ];
    if reconnect {
        args.push("--reconnect".to_string());
    }
    if hidden {
        args.push("--minimized".to_string());
    }
    args
}

/// Who opens `poros://` links now: this copy, another one or nobody.
/// A line from the interface for the log: an error it showed the user.
#[tauri::command]
fn app_log(text: String) {
    applog::write(&format!("interface: {text}"));
}

/// Opens the folder of the logs in Explorer.
#[tauri::command]
fn app_open_logs() -> Result<(), String> {
    applog::open_dir()
}

#[tauri::command]
fn url_scheme_get() -> Result<UrlScheme, String> {
    let exe = std::env::current_exe().map_err(|e| e.to_string())?;
    system::url_scheme(&exe).map_err(|e| e.to_string())
}

/// Takes `poros://` links for this copy or gives them up; the answer is what Windows holds afterwards.
#[tauri::command]
fn url_scheme_set(on: bool) -> Result<UrlScheme, String> {
    let exe = std::env::current_exe().map_err(|e| e.to_string())?;
    system::set_url_scheme(&exe, on).map_err(|e| e.to_string())
}

/// The links that came since the last call; `app-links` tells the interface that there are new ones.
#[tauri::command]
fn take_links(shell: tauri::State<'_, Shell>) -> Vec<String> {
    std::mem::take(&mut *shell.links.lock().unwrap())
}

#[tauri::command]
async fn core_call(
    core: tauri::State<'_, Core>,
    method: String,
    params: Option<Value>,
) -> Result<Value, CoreError> {
    core.call(&method, params.unwrap_or(Value::Null)).await
}

#[tauri::command]
fn core_status(core: tauri::State<'_, Core>) -> CoreStatus {
    core.status()
}

/// Restarts the core with administrator rights for TUN; the UAC prompt shows over the window.
#[tauri::command]
async fn core_elevate(
    core: tauri::State<'_, Core>,
    window: tauri::WebviewWindow,
) -> Result<(), CoreError> {
    #[cfg(windows)]
    let handle = window.hwnd().ok().map(|hwnd| hwnd.0 as isize);
    #[cfg(not(windows))]
    let handle = {
        let _ = window;
        None
    };
    applog::write("shell: asking for administrator rights");
    let result = core.elevate(handle).await;
    if let Err(error) = &result {
        applog::write(&format!(
            "shell: the core did not get the rights: {}",
            error.message
        ));
    }
    result
}

/// Restarts the core without rights; it also brings back a core that gave up.
#[tauri::command]
async fn core_restart(core: tauri::State<'_, Core>) -> Result<(), CoreError> {
    core.restart().await
}

/// Puts the data file the core cannot read aside and starts a core on clean data; the user asked for it.
#[tauri::command]
async fn core_reset_data(core: tauri::State<'_, Core>) -> Result<(), CoreError> {
    core.reset_data().await
}

/// How often the shell looks whether the page in the window still runs, and how long it waits for the answer.
const PAGE_CHECK: Duration = Duration::from_secs(2);
const PAGE_TIMEOUT: Duration = Duration::from_secs(3);

/// Reloads the page of the window when its renderer is gone: WebView2 then shows a page of its own, with no title
/// bar in a window without decorations, so the user could neither use nor close it. A page that twice in a row does
/// not answer, or answers without Poros in it, is loaded again. Only a visible window is looked at.
///
/// It runs on a thread of its own: asking the window whether it is visible waits for the main thread, and a task of
/// the async runtime that waited so would hold up the others, the core's among them, while Poros exits.
fn watch_page(app: AppHandle) {
    std::thread::spawn(move || {
        let mut silent = 0;
        loop {
            std::thread::sleep(PAGE_CHECK);
            if app.state::<Shell>().exiting.load(Ordering::Relaxed) {
                return;
            }
            let Some(window) = app.get_webview_window("main") else {
                return;
            };
            if !window.is_visible().unwrap_or(false) {
                silent = 0;
                continue;
            }
            let (tx, rx) = std::sync::mpsc::channel();
            let tx = Mutex::new(Some(tx));
            let asked = window.eval_with_callback(
                "typeof window.__TAURI_INTERNALS__ === 'object'",
                move |answer| {
                    if let Some(tx) = tx.lock().unwrap().take() {
                        let _ = tx.send(answer == "true");
                    }
                },
            );
            let answer = asked.ok().and_then(|()| rx.recv_timeout(PAGE_TIMEOUT).ok());
            if answer == Some(true) {
                silent = 0;
                continue;
            }
            silent += 1;
            if silent >= 2 && !app.state::<Shell>().exiting.load(Ordering::Relaxed) {
                eprintln!("poros: the window shows no working Poros page; loading it again");
                applog::write("shell: the window shows no working Poros page; loading it again");
                let _ = window.reload();
                silent = 0;
            }
        }
    });
}

fn show_window(app: &AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

fn add_links(app: &AppHandle, args: &[String]) {
    let links: Vec<String> = args
        .iter()
        .filter(|arg| arg.contains("://"))
        .cloned()
        .collect();
    if links.is_empty() {
        return;
    }
    if let Some(shell) = app.try_state::<Shell>() {
        shell.links.lock().unwrap().extend(links);
    }
    let _ = app.emit("app-links", ());
}

/// The settings the shell acts on: the close button, and autostart once the first setup is done.
fn apply_settings(app: &AppHandle, settings: &Value) {
    let Some(shell) = app.try_state::<Shell>() else {
        return;
    };
    let flag = |name: &str| settings.get(name).and_then(Value::as_bool).unwrap_or(false);
    shell
        .close_to_tray
        .store(flag("closeToTray"), Ordering::Relaxed);
    if !flag("onboardingDone") {
        return;
    }
    let integration = Integration {
        autostart: flag("autostart"),
        start_minimized: flag("startMinimized"),
        logon_core: flag("adminAutostart"),
    };
    let mut applied = shell.integration.lock().unwrap();
    if applied.as_ref() == Some(&integration) {
        return;
    }
    let Ok(exe) = std::env::current_exe() else {
        return;
    };
    match system::apply(&exe, integration) {
        Ok(()) => *applied = Some(integration),
        Err(error) => {
            eprintln!("poros: autostart: {error}");
            applog::write(&format!("shell: autostart: {error}"));
        }
    }
}

/// Core events and state changes go to the interface as `core-event` and `core-status`; the shell takes what it
/// needs from them on the way.
struct WindowSink(AppHandle);

impl Sink for WindowSink {
    fn event(&self, method: &str, params: Value) {
        match method {
            "settings.changed" => apply_settings(&self.0, &params),
            "connection.changed" => tray::set_look(&self.0, tray::Look::of(&params)),
            _ => {}
        }
        let _ = self
            .0
            .emit("core-event", json!({ "method": method, "params": params }));
    }

    fn status(&self, status: &CoreStatus) {
        if status.state == CoreState::Ready {
            let app = self.0.clone();
            tauri::async_runtime::spawn(async move {
                let core = app.state::<Core>().inner().clone();
                if let Ok(settings) = core.call("settings.get", Value::Null).await {
                    apply_settings(&app, &settings);
                }
                if let Ok(connection) = core.call("connection.state", Value::Null).await {
                    tray::set_look(&app, tray::Look::of(&connection));
                }
            });
        } else {
            tray::set_look(&self.0, tray::Look::Off);
        }
        let _ = self.0.emit("core-status", status);
    }
}

/// The core sits next to the program; POROS_CORE_PATH points elsewhere, for `tauri dev`.
fn core_program() -> PathBuf {
    if let Some(path) = std::env::var_os("POROS_CORE_PATH") {
        return path.into();
    }
    let name = if cfg!(windows) {
        "PorosCore.exe"
    } else {
        "PorosCore"
    };
    std::env::current_exe()
        .ok()
        .and_then(|exe| exe.parent().map(|dir| dir.join(name)))
        .unwrap_or_else(|| name.into())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    #[cfg(windows)]
    if tauri::webview_version().is_err() {
        fatal::webview_missing();
        return;
    }

    let args: Vec<String> = std::env::args().skip(1).collect();
    // A second start for the same data hands its links to the running Poros, which shows its window.
    let incoming = match data_dir::portable() {
        Some(dir) => match tauri::async_runtime::block_on(async { instance::claim(&dir, &args) }) {
            Ok(Claim::Primary(incoming)) => Some(incoming),
            Ok(Claim::Handed) => return,
            Err(error) => {
                eprintln!("poros: one copy per folder: {error}");
                None
            }
        },
        None => None,
    };
    let minimized = args.iter().any(|arg| arg == "--minimized");
    // Autostart with rights at sign-in: the core comes from the sign-in task.
    let logon_core = cfg!(windows) && args.iter().any(|arg| arg == "--logon-core");
    let reconnect = args.iter().any(|arg| arg == "--reconnect");

    let app = tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            app_show,
            app_hide,
            app_exit,
            app_take_reconnect,
            app_update,
            app_log,
            app_open_logs,
            take_links,
            url_scheme_get,
            url_scheme_set,
            core_call,
            core_status,
            core_elevate,
            core_restart,
            core_reset_data
        ])
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                let to_tray = window
                    .app_handle()
                    .try_state::<Shell>()
                    .is_some_and(|shell| shell.close_to_tray.load(Ordering::Relaxed));
                if to_tray {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .setup(move |app| {
            let data_dir = match data_dir::resolve(app.handle()) {
                Ok(dir) => dir,
                Err(dir) => fatal::data_dir_unwritable(&dir),
            };
            applog::init(&data_dir);
            applog::write(&format!(
                "shell: Poros {} started{}{}",
                if VERSION.is_empty() {
                    "without a version"
                } else {
                    VERSION
                },
                if minimized { ", minimized" } else { "" },
                if logon_core {
                    ", waits for the core of the sign-in task"
                } else {
                    ""
                }
            ));
            app.manage(Shell {
                close_to_tray: AtomicBool::new(false),
                links: Mutex::new(Vec::new()),
                integration: Mutex::new(None),
                reconnect: AtomicBool::new(reconnect),
                exiting: AtomicBool::new(false),
                instance: Mutex::new(None),
            });
            add_links(app.handle(), &args);
            tray::create(app.handle(), show_window)?;
            let core = Core::start(
                CoreConfig {
                    program: core_program(),
                    data_dir: data_dir.clone(),
                    logon: logon_core,
                },
                WindowSink(app.handle().clone()),
            );
            app.manage(core);

            let builder = WebviewWindowBuilder::new(app, "main", WebviewUrl::default())
                .title("Poros")
                .inner_size(1180.0, 760.0)
                .min_inner_size(900.0, 600.0)
                .center()
                .decorations(false)
                .visible(!minimized)
                // The paste buttons read the clipboard; WebView2 would first ask, as a browser does on a site.
                .on_permission_request(|_, kind| match kind {
                    PermissionKind::ClipboardRead => PermissionResponse::Allow,
                    _ => PermissionResponse::Default,
                });
            // The portable build keeps the WebView2 profile in its own folder instead of %LOCALAPPDATA%.
            #[cfg(windows)]
            let builder = builder.data_directory(data_dir.join("webview"));
            builder.build()?;
            watch_page(app.handle().clone());

            if let Some(incoming) = incoming {
                let handle = app.handle().clone();
                let server = incoming.serve(move |args| {
                    add_links(&handle, &args);
                    show_window(&handle);
                });
                *app.state::<Shell>().instance.lock().unwrap() = server;
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("Poros failed to start");

    app.run(|handle, event| {
        if let RunEvent::Exit = event {
            // Poros is gone for the user at once: the core may take a while to close its connection.
            if let Some(shell) = handle.try_state::<Shell>() {
                shell.exiting.store(true, Ordering::Relaxed);
                if let Some(server) = shell.instance.lock().unwrap().take() {
                    server.abort();
                }
            }
            if let Some(window) = handle.get_webview_window("main") {
                let _ = window.hide();
            }
            let _ = handle.remove_tray_by_id(tray::ID);
            // Let the core close its data before the process goes.
            let core = handle.state::<Core>().inner().clone();
            tauri::async_runtime::block_on(core.shutdown());
        }
    });
}
