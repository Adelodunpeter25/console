//! Device hardware models (device listing, diagnostics, lifecycle).
//!
//! The wire types are now shared protobuf (console_proto, see
//! proto/console/v1/device.proto) and re-exported below. `platform` and
//! `state` stay plain strings on the wire — open vocabularies the clients
//! branch on directly — so DevicePlatform / DeviceState remain hand-written
//! UI-side enums, reached through the boundary helpers at the bottom.

use serde::{Deserialize, Serialize};

pub use console_proto::{
    DeviceActionRequest, DeviceDescriptor, DeviceDiagnostics, DeviceOpenAppRequest,
    DeviceOpenAppResponse, DeviceStreamMeta,
};

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DevicePlatform {
    Ios,
    Android,
}

impl DevicePlatform {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Ios => "ios",
            Self::Android => "android",
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::Ios => "iOS",
            Self::Android => "Android",
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DeviceState {
    Booted,
    Shutdown,
    Booting,
}

impl DeviceState {
    pub fn label(&self) -> &'static str {
        match self {
            Self::Booted => "Booted",
            Self::Shutdown => "Shutdown",
            Self::Booting => "Booting",
        }
    }

    pub fn is_booted(&self) -> bool {
        matches!(self, Self::Booted)
    }
}

/// Boundary helpers: the wire carries strings, the UI wants enums.

/// The device's platform as a UI-side enum. Anything that isn't "android" is
/// treated as iOS, matching the old `platform_kind`.
pub fn device_platform(device: &DeviceDescriptor) -> DevicePlatform {
    if device.platform.eq_ignore_ascii_case("android") {
        DevicePlatform::Android
    } else {
        DevicePlatform::Ios
    }
}

/// True when the device's state is "booted" (case-insensitive, matching the
/// old enum deserialization of a lowercase wire value).
pub fn device_state_is_booted(state: &str) -> bool {
    state.eq_ignore_ascii_case("booted")
}

/// The UI label for a device state. Unrecognized values fall back to the raw
/// string so a new simulator state still renders something honest.
pub fn device_state_label(state: &str) -> String {
    match state.to_ascii_lowercase().as_str() {
        "booted" => DeviceState::Booted.label().to_owned(),
        "shutdown" => DeviceState::Shutdown.label().to_owned(),
        "booting" => DeviceState::Booting.label().to_owned(),
        _ => state.to_owned(),
    }
}

/// The label to show for a device: its name, falling back to its id.
pub fn device_display_name(device: &DeviceDescriptor) -> String {
    if device.name.is_empty() {
        device.id.clone()
    } else {
        device.name.clone()
    }
}