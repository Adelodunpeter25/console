//! Shared transcript handler wiring (E consolidation).
//!
//! The pane and main transcripts both wire image preview + file-link opens
//! with only the pane-id source differing. Centralizing the closures here
//! keeps the two call sites from drifting.

use console_ui::chat::TranscriptView;
use gpui::WeakEntity;

use super::ConsoleDesktopApp;

pub(crate) fn wire_preview_image(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    transcript.set_on_preview_image(move |image, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.preview_image_data(image, cx);
            });
        }
    });
}

pub(crate) fn wire_open_file_for_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
    pane_id: String,
) {
    transcript.set_on_open_file(move |link, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.open_file_link(link, &pane_id, cx);
            });
        }
    });
}

pub(crate) fn wire_open_file_for_active_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    transcript.set_on_open_file(move |link, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                let pane_id = this
                    .active_pane_id
                    .clone()
                    .unwrap_or_else(|| "pane-main".to_string());
                this.open_file_link(link, &pane_id, cx);
            });
        }
    });
}

pub(crate) fn wire_view_subagent(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    transcript.set_on_view_subagent(move |call_id, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.view_subagent_in_panel(&call_id, cx);
            });
        }
    });
}
