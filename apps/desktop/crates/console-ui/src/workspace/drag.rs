use std::sync::atomic::{AtomicU64, Ordering};

use console_core::WorkspaceTabConfig;
use gpui::{IntoElement, ParentElement, Render, SharedString, Styled, Window, div, px};

static DRAG_CANCEL_EPOCH: AtomicU64 = AtomicU64::new(0);

/// Invalidate every workspace drag currently in flight. GPUI owns the native
/// drag loop, so the drop handler also checks the epoch; this prevents a drop
/// queued just after Escape from mutating the workspace.
pub fn cancel_workspace_drags() {
    DRAG_CANCEL_EPOCH.fetch_add(1, Ordering::Relaxed);
}

/// Where a dragged tab is dropped relative to an existing pane.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum WorkspaceDropAction {
    SplitLeft,
    AddTab,
    SplitRight,
}

/// A tab being dragged toward another workspace pane.
#[derive(Clone, Debug)]
pub struct WorkspaceDrag {
    pub tab: WorkspaceTabConfig,
    /// `None` means the tab originated in the sidebar rather than another pane.
    pub source_pane_id: Option<String>,
    cancel_epoch: u64,
}

impl WorkspaceDrag {
    pub fn new(tab: WorkspaceTabConfig, source_pane_id: Option<String>) -> Self {
        Self {
            tab,
            source_pane_id,
            cancel_epoch: DRAG_CANCEL_EPOCH.load(Ordering::Relaxed),
        }
    }

    pub fn is_cancelled(&self) -> bool {
        self.cancel_epoch != DRAG_CANCEL_EPOCH.load(Ordering::Relaxed)
    }
}

/// The lightweight view shown under the pointer during a workspace drag.
pub struct WorkspaceDragPreview {
    title: SharedString,
}

impl WorkspaceDragPreview {
    pub fn new(title: impl Into<SharedString>) -> Self {
        Self {
            title: title.into(),
        }
    }
}

impl Render for WorkspaceDragPreview {
    fn render(&mut self, _window: &mut Window, cx: &mut gpui::Context<Self>) -> impl IntoElement {
        let theme = crate::theme::Theme::current(cx);
        div()
            .px(px(10.0))
            .py(px(6.0))
            .rounded(px(6.0))
            .border_1()
            .border_color(theme.border_strong)
            .bg(theme.raised)
            .shadow_md()
            .text_size(px(12.0))
            .text_color(theme.text)
            .child(self.title.clone())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cancelling_a_drag_invalidates_old_payloads_but_not_new_ones() {
        let drag = WorkspaceDrag::new(
            WorkspaceTabConfig::Chat {
                session_id: "session-a".into(),
                title: "Session A".into(),
                project_id: None,
            },
            None,
        );
        assert!(!drag.is_cancelled());

        cancel_workspace_drags();
        assert!(drag.is_cancelled());

        let fresh_drag = WorkspaceDrag::new(
            WorkspaceTabConfig::Chat {
                session_id: "session-b".into(),
                title: "Session B".into(),
                project_id: None,
            },
            None,
        );
        assert!(!fresh_drag.is_cancelled());
    }
}
