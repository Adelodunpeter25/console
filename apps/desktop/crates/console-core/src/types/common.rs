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
/// UI concern; presentation (labels, icons) stays in `console-ui`.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub enum ApprovalMode {
    #[default]
    AlwaysAsk,
    AcceptEdits,
    PlanMode,
    FullAccess,
}

impl ApprovalMode {
    pub const ALL: [Self; 4] = [
        Self::AlwaysAsk,
        Self::AcceptEdits,
        Self::PlanMode,
        Self::FullAccess,
    ];

    pub fn value(self) -> &'static str {
        match self {
            Self::AlwaysAsk => "always-ask",
            Self::AcceptEdits => "accept-edits",
            Self::PlanMode => "plan-mode",
            Self::FullAccess => "full-access",
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::AlwaysAsk => "Normal",
            Self::AcceptEdits => "Accept Edits",
            Self::PlanMode => "Plan Mode",
            Self::FullAccess => "Full access",
        }
    }

    pub fn description(self) -> &'static str {
        match self {
            Self::AlwaysAsk => "Always ask for confirmation on edit & execute actions",
            Self::AcceptEdits => "Auto-approve safe edits, confirm shell execution",
            Self::PlanMode => "Read-only mode, no file edits or command execution",
            Self::FullAccess => "Full autonomy, auto-approve all tools",
        }
    }

    pub fn from_value(val: &str) -> Self {
        match val {
            "accept-edits" => Self::AcceptEdits,
            "plan-mode" => Self::PlanMode,
            "full-access" => Self::FullAccess,
            _ => Self::AlwaysAsk,
        }
    }
}
