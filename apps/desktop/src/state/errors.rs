//! Error banner surfacing and the Cmd+C fallback that copies transcript or
//! banner selections.

use console_ui::CopySelection;
use gpui::{ClipboardItem, Context, Window};

use super::ConsoleDesktopApp;

/// A surfaced banner message plus the generation token it was created with,
/// so neither a stale auto-dismiss timer nor another chat settling its run
/// can clear a newer error.
#[derive(Clone)]
pub struct BannerError {
    pub message: String,
    pub(crate) generation: u64,
}

impl ConsoleDesktopApp {
    /// Surface an app-level error banner (bootstrap, projects, composer
    /// actions — anything not tied to one chat). It renders in every pane and
    /// auto-dismisses after five seconds.
    pub fn set_error(&mut self, message: impl Into<String>, cx: &mut Context<Self>) {
        self.error_generation = self.error_generation.wrapping_add(1);
        self.error_message = Some(BannerError {
            message: message.into(),
            generation: self.error_generation,
        });
        self.schedule_dismissal(None, self.error_generation, cx);
        cx.notify();
    }

    /// Surface an error owned by one chat. It renders only in panes whose
    /// active tab shows that session (mirroring `agent_notices`), so one
    /// chat's failure no longer appears across every open split. Auto-dismiss
    /// applies to that chat's banner alone.
    pub(crate) fn set_error_for_session(
        &mut self,
        session_id: &str,
        message: impl Into<String>,
        cx: &mut Context<Self>,
    ) {
        self.error_generation = self.error_generation.wrapping_add(1);
        self.session_errors.insert(
            session_id.to_string(),
            BannerError {
                message: message.into(),
                generation: self.error_generation,
            },
        );
        self.schedule_dismissal(Some(session_id.to_string()), self.error_generation, cx);
        cx.notify();
    }

    fn schedule_dismissal(
        &mut self,
        session_id: Option<String>,
        generation: u64,
        cx: &mut Context<Self>,
    ) {
        cx.spawn(async move |entity, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_secs(5))
                .await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.dismiss_if_stale(session_id, generation, cx);
                    });
                }
            });
        })
        .detach();
    }

    /// Honor an auto-dismiss timer only if the exact error it was scheduled
    /// for is still the one on display.
    fn dismiss_if_stale(
        &mut self,
        session_id: Option<String>,
        generation: u64,
        cx: &mut Context<Self>,
    ) {
        let stale = match &session_id {
            None => self
                .error_message
                .as_ref()
                .is_some_and(|error| error.generation == generation),
            Some(session_id) => self
                .session_errors
                .get(session_id)
                .is_some_and(|error| error.generation == generation),
        };
        if stale {
            match session_id {
                None => self.clear_error(cx),
                Some(session_id) => self.clear_error_for_session(&session_id, cx),
            }
        }
    }

    /// The banner currently visible in `pane_id`: its active chat's error if
    /// one exists, falling back to the app-level banner.
    pub(crate) fn error_for_pane(&self, pane_id: &str) -> Option<&BannerError> {
        self.active_session_for_pane(pane_id)
            .and_then(|session_id| self.session_errors.get(&session_id))
            .or(self.error_message.as_ref())
    }

    pub(crate) fn clear_error_for_session(&mut self, session_id: &str, cx: &mut Context<Self>) {
        if self.session_errors.remove(session_id).is_some() {
            self.error_selection.clear();
            cx.notify();
        }
    }

    /// Clear whichever banner `pane_id` is currently showing.
    pub(crate) fn clear_error_for_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        if let Some(session_id) = self.active_session_for_pane(pane_id) {
            if self.session_errors.contains_key(&session_id) {
                self.clear_error_for_session(&session_id, cx);
                return;
            }
        }
        self.clear_error(cx);
    }

    /// Dismiss the app-level banner (explicit close/copy or a stale timer).
    pub fn clear_error(&mut self, cx: &mut Context<Self>) {
        if self.error_message.take().is_some() {
            self.error_selection.clear();
            cx.notify();
        }
    }

    /// Fallback leg of the Cmd+C shortcut: the composer answers the keystroke
    /// first and propagates when it has nothing selected of its own; here the
    /// transcript's text selection answers instead.
    pub fn copy_selection_action(
        &mut self,
        _: &CopySelection,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let selected = self
            .error_selection
            .selection
            .borrow()
            .selected_text()
            .or_else(|| self.active_transcript_view().read(cx).selected_text());
        match selected {
            Some(text) => cx.write_to_clipboard(ClipboardItem::new_string(text)),
            None => cx.propagate(),
        }
    }
}
