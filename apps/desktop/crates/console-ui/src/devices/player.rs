//! Embedded device-stream player for the Devices inspector tab.
//!
//! Speaks the sim-go stream wire (served by the Go server at
//! `GET /api/devices/:id/stream?platform=` as a websocket), identical for iOS
//! and Android:
//!
//! - server → player text: `{"type":"meta","width","height","name","codec"}`
//! - server → player binary: tag byte + payload; `1` avcC description,
//!   `2` key frame, `3` delta (AVCC H.264). Unknown tags are ignored.
//! - player → server text: `{"type":"touch","action":"down|move|up","x","y"}`,
//!   `scroll`, `key` (Android keycode), `text`, `button`, `reset`; x/y in
//!   stream pixels.
//!
//! No screenshot fallback: a dead stream reports its state instead of a
//! frozen picture. Status changes are posted to the host over IPC.
//!
//! The page itself lives on the server (`GET /api/devices/player`, see
//! `routes/device_player.go`): a same-origin page can always open the stream
//! websocket, while Chromium (CEF) refuses `ws://` from a locally embedded
//! `data:` page, which left the Devices tab reporting "disconnected" with the
//! stream itself perfectly healthy.

/// Config passed to `window.__consoleDevice.start(...)`.
#[derive(Clone, Debug)]
pub struct PlayerConfig {
    /// Fully-qualified `ws://` / `wss://` stream URL.
    pub stream_url: String,
}

impl PlayerConfig {
    /// The player page URL: the server serves it (`GET /api/devices/player`)
    /// so the page and its stream websocket are same-origin.
    pub fn player_page_url(base: &str) -> String {
        format!("{}/api/devices/player", base.trim_end_matches('/'))
    }

    /// Build the stream URL from the HTTP backend origin.
    ///
    /// The player page opens the socket from a webview, which cannot attach an
    /// Authorization header, so the backend accepts the device token in the
    /// query string for this upgrade only.
    pub fn for_device(
        base: &str,
        id: &str,
        platform: &str,
        auth_token: Option<&str>,
    ) -> Self {
        let base = base.trim_end_matches('/');
        let ws_base = if let Some(rest) = base.strip_prefix("https://") {
            format!("wss://{rest}")
        } else if let Some(rest) = base.strip_prefix("http://") {
            format!("ws://{rest}")
        } else {
            base.to_string()
        };
        let token = auth_token
            .map(|token| format!("&token={}", urlencoding_component(token)))
            .unwrap_or_default();
        Self {
            stream_url: format!(
                "{ws_base}/api/devices/{}/stream?platform={}{token}",
                urlencoding_component(id),
                urlencoding_component(platform)
            ),
        }
    }

    pub fn start_script(&self) -> String {
        let config = serde_json::json!({ "streamUrl": self.stream_url }).to_string();
        format!(
            "window.__consoleDevice && window.__consoleDevice.start({})",
            serde_json::to_string(&config).unwrap_or_else(|_| "\"{}\"".to_string())
        )
    }

    pub fn stop_script() -> &'static str {
        "window.__consoleDevice && window.__consoleDevice.stop()"
    }

    /// Press a hardware button over the live socket (home, back, power, ...).
    pub fn button_script(name: &str) -> String {
        format!(
            "window.__consoleDevice && window.__consoleDevice.button({})",
            serde_json::to_string(name).unwrap_or_else(|_| "\"\"".to_string())
        )
    }
}

fn urlencoding_component(value: &str) -> String {
    let mut out = String::with_capacity(value.len());
    for byte in value.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(byte as char)
            }
            _ => out.push_str(&format!("%{byte:02X}")),
        }
    }
    out
}
