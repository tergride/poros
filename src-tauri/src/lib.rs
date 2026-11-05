mod core_launch;
mod core_process;
mod data_dir;
mod fatal;

use std::path::PathBuf;

use serde::Serialize;
use serde_json::{Value, json};
use tauri::{AppHandle, Emitter, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};

use crate::core_process::{Core, CoreConfig, CoreError, CoreStatus, Sink};

struct AppState {
    data_dir: PathBuf,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct AppInfo {
    version: &'static str,
    data_dir: String,
}

#[tauri::command]
fn app_info(state: tauri::State<'_, AppState>) -> AppInfo {
    AppInfo {
        version: env!("CARGO_PKG_VERSION"),
        data_dir: state.data_dir.display().to_string(),
    }
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

/// Core events and state changes go to the interface as `core-event` and `core-status`.
struct WindowSink(AppHandle);

impl Sink for WindowSink {
    fn event(&self, method: &str, params: Value) {
        let _ = self
            .0
            .emit("core-event", json!({ "method": method, "params": params }));
    }

    fn status(&self, status: &CoreStatus) {
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

    let app = tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            app_info,
            core_call,
            core_status,
            core_elevate,
            core_restart
        ])
        .setup(|app| {
            let data_dir = match data_dir::resolve(app.handle()) {
                Ok(dir) => dir,
                Err(dir) => fatal::data_dir_unwritable(&dir),
            };
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
                .decorations(false);
            // The portable build keeps the WebView2 profile in its own folder instead of %LOCALAPPDATA%.
            #[cfg(windows)]
            let builder = builder.data_directory(data_dir.join("webview"));
            builder.build()?;
            app.manage(AppState { data_dir });
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
