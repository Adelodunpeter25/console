//! ⌘P quick file open palette, built on the central [`crate::CommandPaletteModal`].
//!
//! Debounces the query, asks the server for fuzzy file matches scoped to the
//! active chat's project root, and hands the confirmed file to the app via
//! the `on_open_file` callback (which opens it as a workspace file tab).

use std::rc::Rc;

use console_core::{ConsoleClient, FileSearchResult};
use gpui::{App, AppContext, Context, Entity, IntoElement, Render, Window};

use crate::CommandPaletteModal;
use crate::IconName;
use crate::PaletteEntry;
use crate::primitives::base_name;

/// Debounce for typed queries (first open fires immediately).
const SEARCH_DEBOUNCE_MS: u64 = 80;
/// Hard cap on rows painted — server already caps ~20, this guards growth.
const MAX_RESULTS: usize = 40;

pub struct QuickOpenPalette {
    modal: Entity<CommandPaletteModal>,
    client: ConsoleClient,
    /// Project root the search is scoped to, captured when the palette opens.
    root: Option<String>,
    /// Confirmed file callback: receives the absolute path.
    on_open_file: Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
    /// Monotonic token so a stale in-flight search never overwrites a newer one.
    search_generation: u64,
    /// Last query we actually dispatched — skip identical re-fires from
    /// CommandState re-renders / focus churn.
    last_dispatched_query: Option<String>,
}

impl QuickOpenPalette {
    pub fn new(client: ConsoleClient, window: &mut Window, cx: &mut Context<Self>) -> Self {
        let modal = cx.new(|cx| CommandPaletteModal::new(window, cx));
        Self {
            modal,
            client,
            root: None,
            on_open_file: None,
            search_generation: 0,
            last_dispatched_query: None,
        }
    }

    /// Set the callback invoked with a confirmed file's absolute path.
    pub fn set_on_open_file(
        &mut self,
        callback: impl Fn(String, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_open_file = Some(Rc::new(callback));
    }

    /// Open the palette scoped to `root` and immediately list its top files.
    /// `None` shows an empty state instead of searching the app's cwd.
    pub fn open(&mut self, root: Option<String>, window: &mut Window, cx: &mut Context<Self>) {
        self.root = root.clone();
        self.search_generation = 0;
        self.last_dispatched_query = None;
        if root.is_none() {
            let this = cx.entity().downgrade();
            self.modal.update(cx, |modal, cx| {
                modal.reset_state(window, cx);
                modal.set_placeholder("No project selected — press ⌘O to add one", cx);
                modal.set_filterable(false, cx);
                modal.set_query_handler(
                    {
                        let this = this.clone();
                        move |query, _window, cx| {
                            if let Some(this) = this.upgrade() {
                                this.update(cx, |this, cx| {
                                    this.schedule_search(query, /*immediate*/ false, cx)
                                });
                            }
                        }
                    },
                    cx,
                );
                modal.set_entries(Vec::new(), cx);
                modal.show(window, cx);
            });
            return;
        }
        let this = cx.entity().downgrade();
        self.modal.update(cx, |modal, cx| {
            modal.reset_state(window, cx);
            modal.set_placeholder("Search files…", cx);
            // Server-side search already answers the query; local filtering
            // would hide rows whose labels don't substring-match the query.
            modal.set_filterable(false, cx);
            modal.set_query_handler(
                {
                    let this = this.clone();
                    move |query, _window, cx| {
                        if let Some(this) = this.upgrade() {
                            this.update(cx, |this, cx| {
                                this.schedule_search(query, /*immediate*/ false, cx)
                            });
                        }
                    }
                },
                cx,
            );
            modal.set_entries(Vec::new(), cx);
            modal.show(window, cx);
        });
        // First paint: no debounce so the list appears ASAP.
        self.schedule_search("", /*immediate*/ true, cx);
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.hide(cx));
    }

    pub fn is_open(&self, cx: &App) -> bool {
        self.modal.read(cx).is_open()
    }

    fn schedule_search(&mut self, query: &str, immediate: bool, cx: &mut Context<Self>) {
        let query = query.to_string();
        // Drop no-op re-fires (CommandState often re-emits the same query).
        if self.last_dispatched_query.as_deref() == Some(query.as_str()) {
            return;
        }
        self.last_dispatched_query = Some(query.clone());
        self.search_generation += 1;
        let generation = self.search_generation;
        let modal = self.modal.clone();
        let client = self.client.clone();
        let root = self.root.clone();
        let on_open_file = self.on_open_file.clone();
        let delay_ms = if immediate { 0 } else { SEARCH_DEBOUNCE_MS };

        cx.spawn(async move |this, cx| {
            if delay_ms > 0 {
                cx.background_executor()
                    .timer(std::time::Duration::from_millis(delay_ms))
                    .await;
            }
            // Bail early if a newer keystroke already superseded us during the wait.
            let still_current = this
                .read_with(cx, |this, _| this.search_generation == generation)
                .unwrap_or(false);
            if !still_current {
                return;
            }

            let items = match root {
                Some(root) => client.assist.search_files(None, &query, Some(&root)).await,
                None => return,
            };
            let Ok(items) = items else { return };

            let _ = this.update(cx, |this, cx| {
                if generation != this.search_generation {
                    return;
                }
                this.modal.update(cx, |m, cx| {
                    if m.is_open() {
                        m.set_entries(entries_from_results(items.items, &modal, &on_open_file), cx);
                    }
                });
            });
        })
        .detach();
    }
}

impl Render for QuickOpenPalette {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        // Return the modal entity directly — same as CommandPalette.
        self.modal.clone()
    }
}

/// Convert server search results into palette rows. The label is the path
/// relative to the project root; icons come from the file-type map (folders
/// use the monochrome folder glyph). Confirming opens the file and hides.
fn entries_from_results(
    items: Vec<FileSearchResult>,
    modal: &Entity<CommandPaletteModal>,
    on_open_file: &Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
) -> Vec<PaletteEntry> {
    let modal = modal.clone();
    let on_open_file = on_open_file.clone();
    items
        .into_iter()
        .take(MAX_RESULTS)
        .map(|item| {
            let path = item.absolute_path;
            let label = item.relative_path;
            let modal = modal.clone();
            let on_open_file = on_open_file.clone();
            let mut entry = PaletteEntry::new(path.clone(), label.clone(), move |window, cx| {
                if let Some(on_open_file) = &on_open_file {
                    on_open_file(path.clone(), window, cx);
                }
                modal.update(cx, |modal, cx| modal.hide(cx));
            });
            if item.is_dir {
                entry = entry.icon(IconName::Folder);
            } else {
                // Prefer basename so extension/name rules hit (Cargo.toml, *.rs, …).
                let name = base_name(&label).to_string();
                entry = entry.file_icon(name);
            }
            entry
        })
        .collect()
}
