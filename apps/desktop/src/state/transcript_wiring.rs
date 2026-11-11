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

/// Open a turn's changed-file chip as a diff tab in the pane's workspace.
pub(crate) fn wire_open_change_for_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
    pane_id: String,
) {
    let load_entity = entity.clone();
    let load_pane_id = pane_id.clone();
    transcript.set_load_diff(move |path, turn_index, cx| {
        load_change_diff(&load_entity, &load_pane_id, path, turn_index, cx)
    });
    transcript.set_on_open_change(move |path, turn_index, diff_text, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.open_diff_tab_with_text(&pane_id, path, Some(turn_index), diff_text, cx);
            });
        }
    });
}

pub(crate) fn wire_open_change_for_active_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    let load_entity = entity.clone();
    transcript.set_load_diff(move |path, turn_index, cx| {
        let pane_id = load_entity
            .upgrade()
            .and_then(|app| app.read(cx).active_pane_id.clone())
            .unwrap_or_else(|| "pane-main".to_string());
        load_change_diff(&load_entity, &pane_id, path, turn_index, cx)
    });
    transcript.set_on_open_change(move |path, turn_index, diff_text, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                let pane_id = this
                    .active_pane_id
                    .clone()
                    .unwrap_or_else(|| "pane-main".to_string());
                this.open_diff_tab_with_text(&pane_id, path, Some(turn_index), diff_text, cx);
            });
        }
    });
}

/// Fetch one changed file's diff for the pane's session; `None` when the
/// pane has no session or the request fails.
fn load_change_diff(
    entity: &WeakEntity<ConsoleDesktopApp>,
    pane_id: &str,
    path: &str,
    turn_index: u32,
    cx: &mut gpui::App,
) -> gpui::Task<Option<String>> {
    let target = entity.upgrade().and_then(|app| {
        let app = app.read(cx);
        app.active_session_for_pane(pane_id)
            .map(|session_id| (app.client.clone(), session_id))
    });
    let path = path.to_string();
    cx.spawn(async move |_cx| {
        let (client, session_id) = target?;
        client
            .sessions
            .get_change_diff(&session_id, &path, u64::from(turn_index))
            .await
            .ok()
    })
}

/// Fork the pane's session through the clicked assistant message.
pub(crate) fn wire_fork_for_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
    pane_id: String,
) {
    transcript.set_on_fork(move |message_id, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.fork_session_for_pane(pane_id.clone(), message_id, cx);
            });
        }
    });
}

/// Cite the pane transcript's selection into the pane's composer.
pub(crate) fn wire_cite_for_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
    pane_id: String,
) {
    transcript.set_on_cite(move |quote, window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.cite_quote_into_composer(&pane_id, &quote, window, cx);
            });
        }
    });
}

pub(crate) fn wire_cite_for_active_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    transcript.set_on_cite(move |quote, window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                let pane_id = this
                    .active_pane_id
                    .clone()
                    .unwrap_or_else(|| "pane-main".to_string());
                this.cite_quote_into_composer(&pane_id, &quote, window, cx);
            });
        }
    });
}

pub(crate) fn wire_fork_for_active_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    transcript.set_on_fork(move |message_id, _window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                let pane_id = this
                    .active_pane_id
                    .clone()
                    .unwrap_or_else(|| "pane-main".to_string());
                this.fork_session_for_pane(pane_id, message_id, cx);
            });
        }
    });
}

pub(crate) fn wire_open_file_for_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
    pane_id: String,
) {
    let file_entity = entity.clone();
    transcript.set_on_open_file(move |link, _window, cx| {
        if let Some(app) = file_entity.upgrade() {
            app.update(cx, |this, cx| {
                this.open_file_link(link, &pane_id, cx);
            });
        }
    });
    wire_open_url(transcript, entity);
}

pub(crate) fn wire_open_file_for_active_pane(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    let file_entity = entity.clone();
    transcript.set_on_open_file(move |link, _window, cx| {
        if let Some(app) = file_entity.upgrade() {
            app.update(cx, |this, cx| {
                let pane_id = this
                    .active_pane_id
                    .clone()
                    .unwrap_or_else(|| "pane-main".to_string());
                this.open_file_link(link, &pane_id, cx);
            });
        }
    });
    wire_open_url(transcript, entity);
}

fn wire_open_url(transcript: &mut TranscriptView, entity: WeakEntity<ConsoleDesktopApp>) {
    transcript.set_on_open_url(move |url, window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.open_chat_url_in_browser(url, window, cx);
            });
        }
    });
}

pub(crate) fn wire_view_subagent(
    transcript: &mut TranscriptView,
    entity: WeakEntity<ConsoleDesktopApp>,
) {
    transcript.set_on_view_subagent(move |_call_id, window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.open_subagents_popover(window, cx);
            });
        }
    });
}
