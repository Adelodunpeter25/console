//! Device inspector surface: stream viewer plus GPUI chrome.
//!
//! The live screen comes from the server-vendored `expo-device-hub` proxy
//! (see the device-simulator service spec). The wry child webview in
//! [`DeviceViewer`] only decodes hub frames (WebCodecs) or polls the
//! screenshot endpoint until the hub proxy lands; everything the user
//! touches — switcher, status pill, hardware rail — is native GPUI.

mod player;
mod view;

pub use player::{PLAYER_HTML, PlayerConfig};
pub use view::{DeviceViewer, DeviceViewerPending, select_device};
