//! Backend-facing state for composer autocomplete.
//!
//! Each workspace pane owns an independent command/file query state so split
//! panes never display or insert suggestions from another active session.

use std::cell::Cell;
use std::rc::Rc;

use console_core::{FileSearchResult, SlashCommandInfo};
use console_ui::{
    AutocompleteItem, AutocompleteTrigger, AutocompleteView, detect_trigger, filter_items,
};
use gpui::{Bounds, Context, Pixels, Window};

use super::ConsoleDesktopApp;

#[derive(Default)]
pub(crate) struct PaneAutocompleteState {
    commands: Vec<SlashCommandInfo>,
    command_key: Option<String>,
    command_loading: bool,
    files: Vec<FileSearchResult>,
    file_key: Option<(String, String, String)>,
    file_loading: bool,
    trigger: Option<AutocompleteTrigger>,
    highlighted: usize,
    dismissed: bool,
    anchor_bounds: Rc<Cell<Option<Bounds<Pixels>>>>,
}

impl ConsoleDesktopApp {
    /// Reconcile the active trigger, start missing backend requests, and build
    /// the popup for one pane. Requests are keyed by session/project/query so
    /// late responses cannot overwrite another pane's active suggestions.
    pub(crate) fn composer_autocomplete_for_pane(
        &mut self,
        pane_id: &str,
        session_id: Option<&str>,
        window: &Window,
        cx: &mut Context<Self>,
    ) -> Option<AutocompleteView> {
        let composer = self.composer_for_pane(pane_id);
        let trigger = {
            let input = composer.read(cx);
            if input.focus().is_focused(window) {
                detect_trigger(input.content(), input.cursor())
            } else {
                None
            }
        };

        let project_root = self
            .selected_project_for_pane(pane_id)
            .map(|project| project.path.clone())
            .unwrap_or_default();
        let state = self
            .autocomplete_states
            .entry(pane_id.to_owned())
            .or_default();
        if state.trigger != trigger {
            state.trigger = trigger.clone();
            state.highlighted = 0;
            state.dismissed = false;
        }
        let Some(trigger) = trigger else {
            return None;
        };
        if state.dismissed {
            return None;
        }

        let session_key = session_id.unwrap_or_default().to_owned();
        let session_for_request = (!session_key.is_empty()).then(|| session_key.clone());
        let command_key = session_key.clone();
        let load_commands = if trigger.kind == console_ui::AutocompleteKind::Command
            && state.command_key.as_deref() != Some(command_key.as_str())
        {
            state.command_key = Some(command_key.clone());
            state.commands.clear();
            state.command_loading = true;
            true
        } else {
            false
        };

        let file_key = (
            session_key.clone(),
            trigger.query.clone(),
            project_root.clone(),
        );
        let load_files = if trigger.kind == console_ui::AutocompleteKind::File
            && state.file_key.as_ref() != Some(&file_key)
        {
            state.file_key = Some(file_key.clone());
            state.files.clear();
            state.file_loading = true;
            true
        } else {
            false
        };

        let session_for_file = session_for_request.clone();
        if load_commands {
            let client = self.client.clone();
            let entity = cx.entity().downgrade();
            let pane_id = pane_id.to_owned();
            let apply_key = command_key.clone();
            cx.spawn(async move |_, cx| {
                let commands = client
                    .assist
                    .list_commands(session_for_request.as_deref())
                    .await
                    .unwrap_or_default();
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(state) = this.autocomplete_states.get_mut(&pane_id)
                                && state.command_key.as_deref() == Some(apply_key.as_str())
                            {
                                state.commands = commands;
                                state.command_loading = false;
                                cx.notify();
                            }
                        });
                    }
                });
            })
            .detach();
        }

        if load_files {
            let client = self.client.clone();
            let entity = cx.entity().downgrade();
            let pane_id = pane_id.to_owned();
            let apply_key = file_key.clone();
            let query = trigger.query.clone();
            let root = (!project_root.is_empty()).then_some(project_root.clone());
            cx.spawn(async move |_, cx| {
                let files = client
                    .assist
                    .search_files(session_for_file.as_deref(), &query, root.as_deref())
                    .await
                    .map(|response| response.items)
                    .unwrap_or_default();
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(state) = this.autocomplete_states.get_mut(&pane_id)
                                && state.file_key.as_ref() == Some(&apply_key)
                            {
                                state.files = files;
                                state.file_loading = false;
                                cx.notify();
                            }
                        });
                    }
                });
            })
            .detach();
        }

        let (highlighted, anchor_bounds, loading, items) = {
            let state = self
                .autocomplete_states
                .get(pane_id)
                .expect("autocomplete state inserted above");
            let loading = match trigger.kind {
                console_ui::AutocompleteKind::Command => state.command_loading,
                console_ui::AutocompleteKind::File => state.file_loading,
            };
            // Borrow straight from state — cloning the whole files vec (potentially
            // thousands of FileSearchResult with paths) per frame was the hot path
            // for jank. `filter_items` caps to MAX_AUTOCOMPLETE_ITEMS and short-
            // circuits, so we only clone the ~80 that will be painted.
            let items = filter_items(&state.commands, &state.files, &trigger);
            (
                state.highlighted,
                state.anchor_bounds.clone(),
                loading,
                items,
            )
        };
        if items.is_empty() && !loading {
            return None;
        }

        let pane_id = pane_id.to_owned();
        let entity = cx.entity().downgrade();
        Some(
            AutocompleteView::new(items, highlighted, loading)
                .with_anchor_cell(anchor_bounds)
                .on_select(move |item, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.accept_autocomplete_for_pane(&pane_id, item, cx);
                        });
                    }
                }),
        )
    }

    pub(crate) fn move_autocomplete_for_pane(
        &mut self,
        pane_id: &str,
        next: bool,
        cx: &mut Context<Self>,
    ) {
        let Some(state) = self.autocomplete_states.get_mut(pane_id) else {
            return;
        };
        let Some(trigger) = state.trigger.clone() else {
            return;
        };
        if state.dismissed {
            return;
        }
        let items = filter_items(&state.commands, &state.files, &trigger);
        if items.is_empty() {
            return;
        }
        state.highlighted = if next {
            (state.highlighted + 1) % items.len()
        } else {
            state.highlighted.checked_sub(1).unwrap_or(items.len() - 1)
        };
        cx.notify();
    }

    pub(crate) fn accept_highlighted_autocomplete_for_pane(
        &mut self,
        pane_id: &str,
        cx: &mut Context<Self>,
    ) {
        let Some(state) = self.autocomplete_states.get(pane_id) else {
            return;
        };
        let Some(trigger) = state.trigger.clone() else {
            return;
        };
        let items = filter_items(&state.commands, &state.files, &trigger);
        let Some(item) = items
            .get(state.highlighted.min(items.len().saturating_sub(1)))
            .cloned()
        else {
            return;
        };
        self.accept_autocomplete_for_pane(pane_id, item, cx);
    }

    pub(crate) fn accept_autocomplete_for_pane(
        &mut self,
        pane_id: &str,
        item: AutocompleteItem,
        cx: &mut Context<Self>,
    ) {
        let composer = self.composer_for_pane(pane_id);
        // Prefer the stored trigger from the frame that built the popup: click can
        // land after the caret moved or focus changed, so re-detecting from the
        // current cursor often returns None and silently drops the insert.
        let (content, cursor, stored_trigger) = {
            let input = composer.read(cx);
            (
                input.content().to_owned(),
                input.cursor(),
                self.autocomplete_states
                    .get(pane_id)
                    .and_then(|state| state.trigger.clone()),
            )
        };
        let trigger = stored_trigger
            .or_else(|| detect_trigger(&content, cursor))
            .or_else(|| {
                // Fallback: truncated or stale cursor — find last @/ trigger in content.
                detect_trigger(&content, content.len())
            });
        let Some(trigger) = trigger else {
            return;
        };
        let insert = item.insert_text();
        composer.update(cx, |input, cx| {
            // Clamp to current content so a stale range from an earlier frame can't panic.
            let range = trigger.range.start.min(input.content().len())
                ..trigger.range.end.min(input.content().len());
            input.replace_range(range, &insert, cx);
        });
        if let Some(state) = self.autocomplete_states.get_mut(pane_id) {
            state.trigger = None;
            state.dismissed = false;
            state.highlighted = 0;
        }
        cx.notify();
    }

    pub(crate) fn dismiss_autocomplete_for_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        if let Some(state) = self.autocomplete_states.get_mut(pane_id) {
            state.dismissed = true;
            cx.notify();
        }
    }
}
