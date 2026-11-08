use console_core::ApprovalMode;

use crate::primitives::IconName;

/// UI-only presentation for the domain [`ApprovalMode`]: the icon shown on the
/// floating composer's mode chip. The enum itself lives in `console-core`.
pub trait ApprovalModeIconExt {
    fn icon(self) -> IconName;
}

impl ApprovalModeIconExt for ApprovalMode {
    fn icon(self) -> IconName {
        match self {
            Self::PlanMode => IconName::List,
            Self::FullAccess => IconName::Zap,
        }
    }
}
