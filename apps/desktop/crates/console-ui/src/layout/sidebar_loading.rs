//! Loading-state presentation for sidebar session rows.
//!
//! The sidebar receives session headers from the backend, while the active
//! desktop run and its permission/question prompts live in app state. Keeping
//! the merge here gives rows one source of truth for Working/Waiting display
//! without coupling the sidebar layout to run orchestration.

use console_core::{SessionHeader, SessionStatus};
use gpui::{AnyElement, IntoElement, SharedString};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;
use crate::utils::format_working_elapsed;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SidebarLoadingState {
    Working { started_at: i64 },
    Waiting,
}

/// Resolve the visible loading state for one session.
///
/// The backend status covers sessions restored from an existing server run.
/// The explicit per-session inputs cover the local optimistic window while a
/// newly submitted run is connecting, and while a run is waiting for a
/// permission or question response. Each row is resolved independently so
/// multiple panes can each show their own running/waiting chat.
pub fn session_loading_state(
    session: &SessionHeader,
    running_started_at: Option<i64>,
    is_waiting: bool,
) -> Option<SidebarLoadingState> {
    if is_waiting || session.status.as_ref() == Some(&SessionStatus::NeedsAttention) {
        return Some(SidebarLoadingState::Waiting);
    }

    if let Some(started_at) = running_started_at {
        let started_at = if started_at > 0 {
            normalize_timestamp(started_at)
        } else {
            unix_time()
        };
        return Some(SidebarLoadingState::Working { started_at });
    }

    if session.status.as_ref() == Some(&SessionStatus::Working) {
        return Some(SidebarLoadingState::Working {
            started_at: normalize_timestamp(session.updated_at),
        });
    }

    None
}

/// Render the compact status icon used beside a session title.
pub fn status_indicator(state: SidebarLoadingState, theme: Theme) -> Option<AnyElement> {
    match state {
        // The elapsed Working-for label is sufficient for active runs; avoid
        // leasing a repainting spinner for every working sidebar row.
        SidebarLoadingState::Working { .. } => None,
        SidebarLoadingState::Waiting => {
            Some(app_icon(IconName::Alert, 12.0, theme.warning).into_any_element())
        }
    }
}

/// Render the live elapsed label for a working row. Waiting rows deliberately
/// retain the ordinary session time label because they are paused for input.
pub fn working_label(state: SidebarLoadingState, now: i64) -> Option<SharedString> {
    let SidebarLoadingState::Working { started_at } = state else {
        return None;
    };
    let now = normalize_timestamp(now);
    let started_at = normalize_timestamp(started_at);
    let elapsed = (now - started_at).max(0) as u64;
    Some(SharedString::from(format!(
        "Working for {}",
        format_working_elapsed(elapsed)
    )))
}

fn normalize_timestamp(timestamp: i64) -> i64 {
    if timestamp > 10_000_000_000 {
        timestamp / 1000
    } else {
        timestamp
    }
}

fn unix_time() -> i64 {
    chrono::Utc::now().timestamp()
}
