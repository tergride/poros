mod data_dir;
mod fatal;

use std::path::PathBuf;

use serde::Serialize;
use tauri::{Manager, WebviewUrl, WebviewWindowBuilder};

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

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    #[cfg(windows)]
    if tauri::webview_version().is_err() {
        fatal::webview_missing();
        return;
    }

    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![app_info])
        .setup(|app| {
            let data_dir = match data_dir::resolve(app.handle()) {
                Ok(dir) => dir,
                Err(dir) => fatal::data_dir_unwritable(&dir),
            };
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
        .run(tauri::generate_context!())
        .expect("Poros failed to start");
}
