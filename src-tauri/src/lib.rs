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

use serde::Serialize;
use serde_json::{Value, json};
use tauri::{AppHandle, Emitter, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder, WindowEvent};

use crate::core_process::{Core, CoreConfig, CoreError, CoreState, CoreStatus, Sink};
use crate::instance::Claim;
use crate::system::{Integration, UrlScheme};

/// What the shell keeps besides the core.
struct Shell {
    data_dir: PathBuf,
    /// The window's close button hides it to the tray; follows the core's settings.
    close_to_tray: AtomicBool,
    /// `poros://` links from the command line and from later starts, until the interface takes them.
    links: Mutex<Vec<String>>,
    /// What was last written into the system, to write only changes.
    integration: Mutex<Option<Integration>>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct AppInfo {
    version: &'static str,
    data_dir: String,
}

#[tauri::command]
fn app_info(shell: tauri::State<'_, Shell>) -> AppInfo {
    AppInfo {
        version: env!("CARGO_PKG_VERSION"),
        data_dir: shell.data_dir.display().to_string(),
    }
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

/// Who opens `poros://` links now: this copy, another one or nobody.
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
    core.elevate(handle).await
}

/// Restarts the core without rights; it also brings back a core that gave up.
#[tauri::command]
async fn core_restart(core: tauri::State<'_, Core>) -> Result<(), CoreError> {
    core.restart().await
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
        Err(error) => eprintln!("poros: autostart: {error}"),
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

    let app = tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            app_info,
            app_show,
            app_hide,
            app_exit,
            take_links,
            url_scheme_get,
            url_scheme_set,
            core_call,
            core_status,
            core_elevate,
            core_restart
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
            app.manage(Shell {
                data_dir: data_dir.clone(),
                close_to_tray: AtomicBool::new(true),
                links: Mutex::new(Vec::new()),
                integration: Mutex::new(None),
            });
            add_links(app.handle(), &args);
            tray::create(app.handle(), show_window)?;
            let core = Core::start(
                CoreConfig {
                    program: core_program(),
                    data_dir: data_dir.clone(),
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
                .visible(!minimized);
            // The portable build keeps the WebView2 profile in its own folder instead of %LOCALAPPDATA%.
            #[cfg(windows)]
            let builder = builder.data_directory(data_dir.join("webview"));
            builder.build()?;

            if let Some(incoming) = incoming {
                let handle = app.handle().clone();
                incoming.serve(move |args| {
                    add_links(&handle, &args);
                    show_window(&handle);
                });
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("Poros failed to start");

    app.run(|handle, event| {
        if let RunEvent::Exit = event {
            // Let the core close its data before the process goes.
            let core = handle.state::<Core>().inner().clone();
            tauri::async_runtime::block_on(core.shutdown());
        }
    });
}
