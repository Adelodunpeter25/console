use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct ApiResponse<T> {
    pub success: bool,
    pub data: Option<T>,
    pub error: Option<String>,
    /// Set by the backend when the request was user-cancelled rather than a
    /// failure (e.g. dismissing the native folder picker).
    #[serde(default)]
    pub cancelled: Option<bool>,
}

/// How much autonomy an agent run has. Persisted on the session as
/// `approval_mode` via [`ApprovalMode::value`], so it is a domain value, not a
/// UI concern; presentation (labels, icons) stays in `console-ui`. Plan is the
/// default; any older stored value (always-ask, accept-edits) reads as Plan.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub enum ApprovalMode {
    #[default]
    PlanMode,
    FullAccess,
}

impl ApprovalMode {
    pub const ALL: [Self; 2] = [Self::PlanMode, Self::FullAccess];

    pub fn value(self) -> &'static str {
        match self {
            Self::PlanMode => "plan-mode",
            Self::FullAccess => "full-access",
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::PlanMode => "Plan",
            Self::FullAccess => "Full access",
        }
    }

    pub fn description(self) -> &'static str {
        match self {
            Self::PlanMode => "Read-only mode, no file edits or command execution",
            Self::FullAccess => "Full autonomy, runs everything without asking",
        }
    }

    pub fn from_value(val: &str) -> Self {
        match val {
            "full-access" => Self::FullAccess,
            _ => Self::PlanMode,
        }
    }

    /// The other mode: what Shift+Tab flips to.
    pub fn toggled(self) -> Self {
        match self {
            Self::PlanMode => Self::FullAccess,
            Self::FullAccess => Self::PlanMode,
        }
    }
}
