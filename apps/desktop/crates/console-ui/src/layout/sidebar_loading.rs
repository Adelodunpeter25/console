//! Loading-state presentation for sidebar session rows.
//!
//! The sidebar receives session headers from the backend, while the active
//! desktop run and its permission/question prompts live in app state. Keeping
//! the merge here gives rows one source of truth for Working/Waiting display
//! without coupling the sidebar layout to run orchestration.

use console_core::{SessionHeader, SessionHeaderExt};
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
    if is_waiting || session.status == "needs_attention" {
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

    if session.status == "working" {
        return Some(SidebarLoadingState::Working {
            started_at: normalize_timestamp(session.updated_at),
        });
    }

    None
}

/// What the status slot at the left of a sidebar row shows.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum RowStatus {
    Running,
    NeedsAttention,
    /// A run finished after the chat was last shown: the dot.
    Unread,
    /// Finished and already seen: the check.
    Done,
    /// Never run, or idle: the slot stays empty.
    Quiet,
}

/// Choose the row's status icon. Precedence is fixed: running, then needs
/// attention (a pending question, or an errored run not yet opened), then
/// unread (the dot), then done and read (the check), then nothing. The
/// server's `unread` flag is the only unread input, so the sidebar never
/// re-derives it.
pub fn row_status(
    session: &SessionHeader,
    loading_state: Option<SidebarLoadingState>,
) -> RowStatus {
    let running = matches!(loading_state, Some(SidebarLoadingState::Working { .. }))
        || session.status == "working"
        || session.status == "running";
    if running {
        return RowStatus::Running;
    }
    let needs_attention = matches!(loading_state, Some(SidebarLoadingState::Waiting))
        || session.status == "needs_attention"
        || session.status == "waiting";
    if needs_attention {
        return RowStatus::NeedsAttention;
    }
    if session.unread {
        return RowStatus::Unread;
    }
    if session.status == "done" || session.status == "completed" {
        return RowStatus::Done;
    }
    RowStatus::Quiet
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

/// Placeholder titles the server's autotitle treats as unset. Mirrors
/// `titles.IsGenericTitle` in the server; keep the two lists in step.
pub fn is_generic_title(title: &str) -> bool {
    matches!(
        title.trim(),
        "" | "New Session" | "New mobile session" | "New Chat" | "New chat" | "Untitled"
    )
}

/// The session's title is still the placeholder but the first prompt is
/// already running or sent, so the server is generating a real one: show a loading
/// label instead of the placeholder text. The same update that delivers the
/// generated title clears the state, so no event link is needed.
pub fn title_is_pending(
    session: &SessionHeader,
    loading_state: Option<SidebarLoadingState>,
) -> bool {
    let is_active = loading_state.is_some();
    let has_messages = session.message_count.unwrap_or(0) >= 1;
    is_generic_title(session.display_title()) && (is_active || has_messages)
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
