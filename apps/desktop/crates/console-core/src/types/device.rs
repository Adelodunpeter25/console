//! Device hardware models (device listing, diagnostics, lifecycle).

use serde::{Deserialize, Serialize};

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

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeviceDescriptor {
    pub id: String,
    pub name: String,
    pub platform: String,
    pub state: DeviceState,
    pub model: Option<String>,
    pub os_version: Option<String>,
    pub is_available: bool,
}

impl DeviceDescriptor {
    pub fn platform_kind(&self) -> DevicePlatform {
        if self.platform.eq_ignore_ascii_case("android") {
            DevicePlatform::Android
        } else {
            DevicePlatform::Ios
        }
    }

    pub fn display_name(&self) -> String {
        if self.name.is_empty() {
            self.id.clone()
        } else {
            self.name.clone()
        }
    }
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeviceDiagnostics {
    #[serde(default)]
    pub xcode_installed: bool,
    #[serde(default)]
    pub xcode_version: Option<String>,
    #[serde(default)]
    pub simctl_available: bool,
    #[serde(default)]
    pub android_sdk_found: bool,
    #[serde(default)]
    pub adb_available: bool,
    #[serde(default)]
    pub emulator_available: bool,
    #[serde(default)]
    pub disk_free_bytes: u64,
    #[serde(default)]
    pub has_enough_disk_space: bool,
    #[serde(default)]
    pub errors: Vec<String>,
}

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeviceActionRequest {
    pub action: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub x: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub y: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub end_x: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub end_y: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub duration_ms: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub text: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub key: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub appearance: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DeviceOpenAppRequest {
    pub app: String,
}
