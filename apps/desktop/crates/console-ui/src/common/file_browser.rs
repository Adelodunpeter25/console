//! Finder-style folder chooser for a path on the SERVER's filesystem.
//!
//! Deliberately NOT a native OS panel. The backend is often a remote host
//! (a tunnel to a Linux box), and a macOS panel browses the Mac — so every
//! path it offers is one the server cannot use. This lists the backend's real
//! directories through `/api/fs/browse`, which runs server-side.
//!
//! Deliberately folders only. This picks a directory, so files are filtered
//! out of the listing rather than shown greyed out.
//!
//! Interaction mirrors Finder's column browser, minus the columns:
//! - **single click** highlights a row
//! - **double click** opens that folder
//! - **..** goes to the parent
//! - the caller decides what "chosen" means via [`FileBrowser::chosen`]

use console_core::{ConsoleClient, FsTreeEntry};
use gpui::{
    App, Context, Entity, FocusHandle, Focusable, InteractiveElement, IntoElement, MouseButton,
    ParentElement, Render, StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::theme::Theme;

pub struct FileBrowser {
    client: ConsoleClient,
    /// Directory currently listed. None before the first response lands, at
    /// which point it is the server's home directory.
    current_path: Option<String>,
    /// Parent of `current_path`, rendered as the ".." row.
    parent_path: Option<String>,
    /// Folders only — files are dropped in `open`.
    entries: Vec<FsTreeEntry>,
    /// Row highlighted by a single click. Empty until the user picks one.
    selected: Option<String>,
    loading: bool,
    focus_handle: FocusHandle,
}

impl FileBrowser {
    pub fn new(client: ConsoleClient, window: &mut Window, cx: &mut Context<Self>) -> Self {
        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle, cx);
        Self {
            client,
            current_path: None,
            parent_path: None,
            entries: Vec::new(),
            selected: None,
            loading: false,
            focus_handle,
        }
    }

    /// List `path`, or the server's home directory when `None`.
    pub fn open(&mut self, path: Option<String>, cx: &mut Context<Self>) {
        self.loading = true;
        cx.notify();

        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_this, cx| {
            let result = client.fs.browse(path.as_deref()).await;
            let Some(browser) = entity.upgrade() else {
                return;
            };
            cx.update(|cx| {
                browser.update(cx, |this, cx| {
                    this.loading = false;
                    if let Ok(listing) = result {
                        this.current_path = Some(listing.current_path);
                        this.parent_path = listing.parent_path;
                        this.entries = listing
                            .entries
                            .into_iter()
                            .filter(|entry| entry.is_dir)
                            .collect();
                        // The highlight refers to a row that no longer exists.
                        this.selected = None;
                    }
                    cx.notify();
                });
            });
        })
        .detach();
    }

    /// The folder a caller should treat as chosen: the highlighted row, or the
    /// directory being listed when nothing is highlighted. Falling back means
    /// opening a folder and confirming without clicking anything still means
    /// something sensible rather than silently doing nothing.
    pub fn chosen(&self) -> Option<String> {
        self.selected.clone().or_else(|| self.current_path.clone())
    }

    /// Path currently listed, for a "you are here" label.
    pub fn current_path(&self) -> Option<&str> {
        self.current_path.as_deref()
    }
}

impl Focusable for FileBrowser {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for FileBrowser {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let loading = self.loading;
        let has_parent = self.parent_path.is_some();
        let current = self.current_path.clone();
        let selected = self.selected.clone();
        let entries = self.entries.clone();
        let entity = cx.entity().downgrade();

        // ".." first, then folders. Both navigate on a single click: going up
        // is a one-off action, and requiring a double click for it would be
        // needlessly fiddly.
        let mut list = div()
            // `.id` makes the element stateful, which is what provides
            // `overflow_y_scroll` — without it the method does not exist.
            .id("file-browser-list")
            .flex()
            .flex_col()
            .min_h(px(220.0))
            .max_h(px(300.0))
            .overflow_y_scroll();

        if let Some(parent) = self.parent_path.clone() {
            list = list.child(row(
                "..",
                "Parent folder",
                false,
                entity.clone(),
                theme,
                move |dialog_entity, cx| {
                    dialog_entity.update(cx, |this, cx| this.open(Some(parent.clone()), cx));
                },
            ));
        }

        for entry in entries {
            let is_selected = selected.as_deref() == Some(entry.path.as_str());
            let path = entry.path.clone();
            let name = entry.name.clone();
            // Cloned for the closure so the `&path` borrows above stay valid.
            let open_path = path.clone();
            list = list.child(row(
                &name,
                &path,
                is_selected,
                entity.clone(),
                theme,
                move |browser_entity, cx| {
                    browser_entity.update(cx, |this, cx| {
                        // A double click opens; the first click of that pair
                        // already set the highlight, which is what `chosen`
                        // reads — so only navigation is left to do here.
                        this.open(Some(open_path.clone()), cx);
                    });
                },
            ));
        }

        // Distinguish "still loading" from "genuinely empty": showing "No
        // folders" during a request would flash on every navigation.
        if has_parent || (!loading && self.entries.is_empty()) {
            list = list.child(
                div()
                    .px(px(8.0))
                    .py(px(6.0))
                    .text_size(px(11.5))
                    .text_color(theme.text_ghost)
                    .child("No subfolders here."),
            );
        }

        div()
            .flex()
            .flex_col()
            .gap(px(8.0))
            .track_focus(&self.focus_handle)
            .child(
                div()
                    .px(px(8.0))
                    .py(px(6.0))
                    .rounded(px(5.0))
                    .bg(theme.surface)
                    .border_1()
                    .border_color(theme.border)
                    .text_size(px(11.5))
                    .text_color(theme.text_secondary)
                    .child(match current {
                        Some(path) => path,
                        None => "Loading…".to_string(),
                    }),
            )
            .child(list)
            .child(
                div()
                    .px(px(2.0))
                    .text_size(px(11.0))
                    .text_color(theme.text_ghost)
                    .child("Double-click a folder to open it."),
            )
    }
}

/// One row: folder name over its full path, highlighted when selected.
fn row(
    name: &str,
    path: &str,
    selected: bool,
    entity: gpui::WeakEntity<FileBrowser>,
    theme: Theme,
    on_open: impl Fn(&mut Entity<FileBrowser>, &mut App) + 'static,
) -> impl IntoElement {
    let name = name.to_string();
    let path = path.to_string();
    // Cloned for the click handler; `path` is still read as the row's caption.
    let click_path = path.clone();
    let selectable = name != "..";

    let row = div()
        .id(gpui::ElementId::from(format!("file-browser-row-{path}")))
        .px(px(8.0))
        .py(px(5.0))
        .flex()
        .flex_col()
        .gap(px(1.0))
        .cursor_pointer()
        .on_mouse_down(MouseButton::Left, move |event, _window, cx| {
            cx.stop_propagation();
            let Some(browser) = entity.upgrade() else {
                return;
            };
            // ".." navigates immediately; a real folder needs a double click
            // to open, because a single click only highlights it.
            if !selectable {
                on_open(&mut { browser.clone() }, cx);
                return;
            }
            if event.click_count >= 2 {
                on_open(&mut { browser.clone() }, cx);
                return;
            }
            browser.update(cx, |this, cx| {
                this.selected = Some(click_path.clone());
                cx.notify();
            });
        })
        .child(
            div()
                .text_size(px(12.5))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(if selected { theme.accent } else { theme.text })
                .child(name),
        )
        .child(
            div()
                .text_size(px(10.5))
                .text_color(theme.text_ghost)
                .child(path),
        );

    if selected {
        row.bg(theme.overlay).into_any_element()
    } else {
        row.hover(|s| s.bg(theme.overlay)).into_any_element()
    }
}
