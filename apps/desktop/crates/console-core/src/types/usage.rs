//! Usage and quota DTOs. Canonical wire types come from the shared protobuf
//! schema (proto/console/v1/usage.proto): trimmed to what the UIs render,
//! with unit/status as plain strings (see the proto comments).
//!
//! The fraction resolvers lived on the hand-written struct as inherent
//! methods; generated types are foreign so they move to UsageLimitExt,
//! re-exported alongside the types.

use serde::{Deserialize, Serialize};

pub use console_proto::{UsageAmount, UsageLimit, UsageReport, UsageScope, UsageWindow};

pub trait UsageLimitExt {
    fn resolved_used_fraction(&self) -> f64;
    fn resolved_remaining_fraction(&self) -> f64;
}

impl UsageLimitExt for UsageLimit {
    fn resolved_used_fraction(&self) -> f64 {
        let Some(amount) = self.amount.as_ref() else {
            return 0.0;
        };
        if let Some(frac) = amount.used_fraction {
            return frac.clamp(0.0, 1.0);
        }
        if let (Some(used), Some(limit)) = (amount.used, amount.limit) {
            if limit > 0.0 {
                return (used / limit).clamp(0.0, 1.0);
            }
        }
        if amount.unit == "percent" {
            if let Some(used) = amount.used {
                return (used / 100.0).clamp(0.0, 1.0);
            }
        }
        if let Some(rem_frac) = amount.remaining_fraction {
            return (1.0 - rem_frac).clamp(0.0, 1.0);
        }
        0.0
    }

    fn resolved_remaining_fraction(&self) -> f64 {
        if let Some(rem) = self
            .amount
            .as_ref()
            .and_then(|amount| amount.remaining_fraction)
        {
            return rem.clamp(0.0, 1.0);
        }
        1.0 - self.resolved_used_fraction()
    }
}

/// Estimated context-window occupancy for one session, served by
/// `GET /api/sessions/:id/context` and pushed as `contextUpdate` frames.
/// Wire shape owned by the shared schema (console.v1.ContextSnapshot):
/// counts stay JSON numbers, so this hand type decodes unchanged.
/// Not part of the usage domain.
#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct ContextSnapshot {
    #[serde(default)]
    pub used_tokens: i64,
    #[serde(default)]
    pub context_window: i64,
    #[serde(default)]
    pub percent_used: f64,
    #[serde(default)]
    pub threshold_ratio: f64,
    #[serde(default)]
    pub model_id: String,
    #[serde(default)]
    pub provider: String,
    #[serde(default)]
    pub source: String,
}

impl ContextSnapshot {
    pub fn used_fraction(&self) -> f64 {
        (self.percent_used / 100.0).clamp(0.0, 1.0)
    }

    /// "23.1k/1.0M" style rendering for the panel header.
    pub fn used_vs_window(&self) -> String {
        format!("{}/{}", format_tokens(self.used_tokens), format_tokens(self.context_window))
    }
}

fn format_tokens(v: i64) -> String {
    let f = v as f64;
    if f >= 1_000_000.0 {
        format!("{:.1}M", f / 1_000_000.0)
    } else if f >= 1_000.0 {
        format!("{:.1}k", f / 1_000.0)
    } else {
        format!("{v}")
    }
}
