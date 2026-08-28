//! ⌘P quick file open palette, built on the central [`crate::CommandPaletteModal`].
//!
//! Debounces the query, asks the server for fuzzy file matches scoped to the
//! active chat's project root, and hands the confirmed file to the app via
//! the `on_open_file` callback (which opens it as a workspace file tab).

use std::rc::Rc;

use console_core::{ConsoleClient, FileSearchResult};
use gpui::{App, AppContext, Context, Entity, IntoElement, ParentElement, Render, Window, div};

use crate::CommandPaletteModal;
use crate::IconName;
use crate::PaletteEntry;

/// Debounce between the query changing and the server search firing.
const SEARCH_DEBOUNCE_MS: u64 = 200;

pub struct QuickOpenPalette {
    modal: Entity<CommandPaletteModal>,
    client: ConsoleClient,
    /// Project root the search is scoped to, captured when the palette opens.
    root: Option<String>,
    /// Confirmed file callback: receives the absolute path.
    on_open_file: Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
    /// Monotonic token so a stale in-flight search never overwrites a newer
    /// one, and the debounce timer is always anchored to the latest query.
    search_generation: u64,
    /// The generation the modal last applied, to keep the query callback
    /// from re-firing the same search on every render.
    applied_generation: u64,
}

impl QuickOpenPalette {
    pub fn new(
        client: ConsoleClient,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let modal = cx.new(|cx| CommandPaletteModal::new(window, cx));
        Self {
            modal,
            client,
            root: None,
            on_open_file: None,
            search_generation: 0,
            applied_generation: 0,
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
    pub fn open(&mut self, root: Option<String>, window: &mut Window, cx: &mut Context<Self>) {
        self.root = root;
        // Reset the debounce tokens so every open (even with an unchanged
        // query, e.g. reopening with an empty field) fires a fresh search.
        self.search_generation = 0;
        self.applied_generation = 0;
        let this = cx.entity().downgrade();
        self.modal.update(cx, |modal, cx| {
            modal.reset_search_generation(cx);
            modal.set_placeholder("Search files…", cx);
            modal.set_query_handler(
                {
                    let this = this.clone();
                    move |query, _window, cx| {
                        if let Some(this) = this.upgrade() {
                            this.update(cx, |this, cx| this.schedule_search(query, cx));
                        }
                    }
                },
                cx,
            );
            modal.set_entries(Vec::new(), cx);
            modal.show(window, cx);
        });
        // Prime the list with the empty-query search.
        self.schedule_search("", cx);
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.hide(cx));
    }

    pub fn is_open(&self, cx: &App) -> bool {
        self.modal.read(cx).is_open()
    }

    fn schedule_search(&mut self, query: &str, cx: &mut Context<Self>) {
        self.search_generation += 1;
        let generation = self.search_generation;
        if generation == self.applied_generation {
            return;
        }
        self.applied_generation = generation;
        let modal = self.modal.clone();
        let client = self.client.clone();
        let root = self.root.clone();
        let query = query.to_string();
        let on_open_file = self.on_open_file.clone();

        cx.spawn(async move |_, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_millis(SEARCH_DEBOUNCE_MS))
                .await;
            let items = match root {
                Some(root) => client.assist.search_files(None, &query, Some(&root)).await,
                None => return,
            };
            let Ok(items) = items else { return };
            cx.update(|cx| {
                let modal = modal.clone();
                modal.update(cx, |m, cx| {
                    if m.is_open() {
                        m.set_entries_with_generation(
                            generation,
                            entries_from_results(items.items, &modal, &on_open_file),
                            cx,
                        );
                    }
                });
            });
        })
        .detach();
    }
}

impl Render for QuickOpenPalette {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        div().child(self.modal.clone())
    }
}

/// Convert server search results into palette rows. The label carries the
/// path relative to the project root; confirming runs the `on_open_file`
/// callback with the absolute path and hides the palette.
fn entries_from_results(
    items: Vec<FileSearchResult>,
    modal: &Entity<CommandPaletteModal>,
    on_open_file: &Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
) -> Vec<PaletteEntry> {
    let modal = modal.clone();
    let on_open_file = on_open_file.clone();
    items
        .into_iter()
        .map(|item| {
            let icon = if item.is_dir {
                IconName::Folder
            } else {
                IconName::File
            };
            let path = item.absolute_path;
            let label = item.relative_path;
            let modal = modal.clone();
            let on_open_file = on_open_file.clone();
            PaletteEntry::new(path.clone(), label, move |window, cx| {
                if let Some(on_open_file) = &on_open_file {
                    on_open_file(path.clone(), window, cx);
                }
                modal.update(cx, |modal, cx| modal.hide(cx));
            })
            .icon(icon)
        })
        .collect()
}
