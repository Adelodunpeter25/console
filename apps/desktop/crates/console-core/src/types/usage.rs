//! Usage and quota DTOs mirroring server `/api/usage` and `@console/types`.

use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum UsageUnit {
    Percent,
    Tokens,
    Requests,
    Usd,
    Minutes,
    Bytes,
    #[serde(other)]
    Unknown,
}

impl Default for UsageUnit {
    fn default() -> Self {
        Self::Unknown
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum UsageStatus {
    Ok,
    Warning,
    Exhausted,
    #[serde(other)]
    Unknown,
}

impl Default for UsageStatus {
    fn default() -> Self {
        Self::Unknown
    }
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UsageWindow {
    pub id: String,
    pub label: String,
    #[serde(default)]
    pub duration_ms: Option<i64>,
    #[serde(default)]
    pub resets_at: Option<i64>,
    #[serde(default)]
    pub reset_label: Option<String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct UsageAmount {
    #[serde(default)]
    pub used: Option<f64>,
    #[serde(default)]
    pub limit: Option<f64>,
    #[serde(default)]
    pub remaining: Option<f64>,
    #[serde(default)]
    pub used_fraction: Option<f64>,
    #[serde(default)]
    pub remaining_fraction: Option<f64>,
    #[serde(default)]
    pub unit: UsageUnit,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UsageScope {
    pub provider: String,
    #[serde(default)]
    pub account_id: Option<String>,
    #[serde(default)]
    pub project_id: Option<String>,
    #[serde(default)]
    pub org_id: Option<String>,
    #[serde(default)]
    pub model_id: Option<String>,
    #[serde(default)]
    pub tier: Option<String>,
    #[serde(default)]
    pub window_id: Option<String>,
    #[serde(default)]
    pub shared: Option<bool>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct UsageLimit {
    pub id: String,
    pub label: String,
    pub scope: UsageScope,
    #[serde(default)]
    pub window: Option<UsageWindow>,
    pub amount: UsageAmount,
    #[serde(default)]
    pub status: Option<UsageStatus>,
    #[serde(default)]
    pub notes: Option<Vec<String>>,
}

impl UsageLimit {
    pub fn resolved_used_fraction(&self) -> f64 {
        if let Some(frac) = self.amount.used_fraction {
            return frac.clamp(0.0, 1.0);
        }
        if let (Some(used), Some(limit)) = (self.amount.used, self.amount.limit) {
            if limit > 0.0 {
                return (used / limit).clamp(0.0, 1.0);
            }
        }
        if self.amount.unit == UsageUnit::Percent {
            if let Some(used) = self.amount.used {
                return (used / 100.0).clamp(0.0, 1.0);
            }
        }
        if let Some(rem_frac) = self.amount.remaining_fraction {
            return (1.0 - rem_frac).clamp(0.0, 1.0);
        }
        0.0
    }

    pub fn resolved_remaining_fraction(&self) -> f64 {
        if let Some(rem) = self.amount.remaining_fraction {
            return rem.clamp(0.0, 1.0);
        }
        1.0 - self.resolved_used_fraction()
    }
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct UsageReport {
    pub provider: String,
    pub fetched_at: i64,
    #[serde(default)]
    pub limits: Vec<UsageLimit>,
    #[serde(default)]
    pub notes: Option<Vec<String>>,
    #[serde(default)]
    pub metadata: Option<serde_json::Value>,
}
