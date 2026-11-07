//! The tray icon. Its menu comes from the interface, which has the servers, the presets and the words; the icon
//! follows the connection here, so it is right while the window is hidden or still loading.

use serde_json::Value;
use tauri::image::Image;
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Runtime};

pub const ID: &str = "main";

#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Look {
    Off,
    Connecting,
    Tun,
    Proxy,
}

impl Look {
    /// The look of a `connection.changed` event or a `connection.state` answer.
    pub fn of(connection: &Value) -> Self {
        match connection.get("state").and_then(Value::as_str) {
            Some("starting" | "stopping") => Self::Connecting,
            Some("running") if connection.get("tun").and_then(Value::as_bool) == Some(true) => {
                Self::Tun
            }
            Some("running") => Self::Proxy,
            _ => Self::Off,
        }
    }

    fn image(self) -> Image<'static> {
        match self {
            Self::Off => tauri::include_image!("icons/tray/off.png"),
            Self::Connecting => tauri::include_image!("icons/tray/connecting.png"),
            Self::Tun => tauri::include_image!("icons/tray/tun.png"),
            Self::Proxy => tauri::include_image!("icons/tray/proxy.png"),
        }
    }
}

/// A click shows the window; the right button opens the menu.
pub fn create<R: Runtime>(
    app: &AppHandle<R>,
    on_click: impl Fn(&AppHandle<R>) + Send + Sync + 'static,
) -> tauri::Result<()> {
    TrayIconBuilder::with_id(ID)
        .icon(Look::Off.image())
        .tooltip("Poros")
        .show_menu_on_left_click(false)
        .on_tray_icon_event(move |tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                on_click(tray.app_handle());
            }
        })
        .build(app)?;
    Ok(())
}

pub fn set_look<R: Runtime>(app: &AppHandle<R>, look: Look) {
    if let Some(tray) = app.tray_by_id(ID) {
        let _ = tray.set_icon(Some(look.image()));
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn the_icon_follows_the_connection() {
        assert_eq!(Look::of(&json!({ "state": "idle" })), Look::Off);
        assert_eq!(
            Look::of(&json!({ "state": "stopping", "tun": true })),
            Look::Connecting
        );
        assert_eq!(
            Look::of(&json!({ "state": "running", "tun": true, "systemProxy": true })),
            Look::Tun
        );
        assert_eq!(
            Look::of(&json!({ "state": "running", "tun": false })),
            Look::Proxy
        );
    }
}
