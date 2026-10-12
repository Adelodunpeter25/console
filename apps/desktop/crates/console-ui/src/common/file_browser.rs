//! Finder-style folder chooser for a path on the SERVER's filesystem.
//!
//! Deliberately NOT a native OS panel. The backend is often a remote host (a
//! tunnel to a Linux box), and a macOS panel browses the Mac — so every path it
//! offers is one the server cannot use. This lists the backend's real
//! directories through `/api/fs/browse`, which runs server-side.
//!
//! Deliberately folders only: this picks a directory, so files are dropped
//! from the listing rather than shown disabled.
//!
//! Presented as its OWN window rather than an in-window overlay, for two
//! reasons that both cost us time before:
//!   - an overlay built inline during render is rebuilt every frame, so its
//!     open state never sticks and it simply never appears;
//!   - the New Project dialog is itself a separate window, and an overlay
//!     belonging to one window can surface behind another. A plain window
//!     opens in front by construction.
//!
//! Interaction mirrors Finder's folder browser:
//! - **single click** highlights a row
//! - **double click** opens that folder
//! - **..** goes to the parent

use console_core::{ConsoleClient, FsTreeEntry};
use gpui::{
    App, Context, FocusHandle, Focusable, InteractiveElement, IntoElement, KeyDownEvent,
    MouseButton, ParentElement, Render, StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

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
    /// Row highlighted by a single click.
    selected: Option<String>,
    loading: bool,
    /// Confirmed by the user: the highlighted row, or the directory being
    /// listed when nothing is highlighted.
    on_choose: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_cancel: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    focus_handle: FocusHandle,
}

impl FileBrowser {
    pub fn new(
        client: ConsoleClient,
        on_choose: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_cancel: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle, cx);
        Self {
            client,
            current_path: None,
            parent_path: None,
            entries: Vec::new(),
            selected: None,
            loading: false,
            on_choose,
            on_cancel,
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

    /// The folder a confirm should use: the highlighted row, or the directory
    /// being listed when nothing is highlighted. The fallback means opening a
    /// folder and confirming without clicking still means something sensible
    /// rather than silently doing nothing.
    pub fn chosen(&self) -> Option<String> {
        self.selected.clone().or_else(|| self.current_path.clone())
    }

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
        let entity = cx.entity().downgrade();
        let current = self.current_path.clone();
        let parent = self.parent_path.clone();
        let selected = self.selected.clone();
        let entries = self.entries.clone();
        let loading = self.loading;

        let on_choose = self.on_choose.clone();
        let on_cancel = self.on_cancel.clone();

        let cancel_button = {
            let cancel = on_cancel.clone();
            let b = div()
                .id("file-browser-cancel")
                .px(px(12.0))
                .py(px(6.0))
                .rounded(px(6.0))
                .border_1()
                .border_color(theme.border)
                .bg(theme.surface)
                .cursor_pointer()
                .child(
                    div()
                        .text_size(px(12.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.text)
                        .child("Cancel"),
                )
                .on_mouse_down(MouseButton::Left, move |_e, window, cx| {
                    cx.stop_propagation();
                    cancel(window, cx);
                });
            b.into_any_element()
        };

        let choose_button = {
            let choose = on_choose.clone();
            let confirm_entity = entity.clone();
            let b = div()
                .id("file-browser-choose")
                .px(px(12.0))
                .py(px(6.0))
                .rounded(px(6.0))
                .bg(theme.accent)
                .cursor_pointer()
                .child(
                    div()
                        .text_size(px(12.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.on_inverse)
                        .child("Use this folder"),
                )
                .on_mouse_down(MouseButton::Left, move |_e, window, cx| {
                    cx.stop_propagation();
                    let chosen = confirm_entity
                        .upgrade()
                        .and_then(|browser| browser.read(cx).chosen());
                    if let Some(chosen) = chosen {
                        choose(chosen, window, cx);
                    }
                });
            b.into_any_element()
        };

        div()
            .size_full()
            .bg(theme.canvas)
            .track_focus(&self.focus_handle)
            .on_key_down(cx.listener({
                // Cloned: the listener must own it, and it outlives render.
                let on_cancel = on_cancel.clone();
                move |_this, event: &KeyDownEvent, window, cx| match event
                    .keystroke
                    .key
                    .as_str()
                {
                    "escape" => on_cancel(window, cx),
                    _ => {}
                }
            }))
            .child(
                div()
                    .mx(px(16.0))
                    .mt(px(48.0))
                    .flex()
                    .flex_col()
                    .gap(px(12.0))
                    .child(
                        div()
                            .text_size(px(16.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child("Choose a folder"),
                    )
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
                    .child(build_list(
                        parent, selected, entries, loading, entity.clone(), &theme,
                    ))
                    .child(
                        div()
                            .px(px(2.0))
                            .text_size(px(11.0))
                            .text_color(theme.text_ghost)
                            .child("Double-click a folder to open it. These are folders on the server, not on this Mac."),
                    )
                    .child(
                        div()
                            .flex()
                            .gap(px(8.0))
                            .justify_end()
                            .child(cancel_button)
                            .child(choose_button),
                    ),
            )
    }
}

/// The scrollable listing: "..", then folders.
fn build_list(
    parent: Option<String>,
    selected: Option<String>,
    entries: Vec<FsTreeEntry>,
    loading: bool,
    entity: gpui::WeakEntity<FileBrowser>,
    theme: &Theme,
) -> impl IntoElement {
    // `.id` makes the element stateful, which is what provides
    // `overflow_y_scroll` — without it that method does not exist on Div.
    let mut list = div()
        .id("file-browser-list")
        .flex()
        .flex_col()
        .flex_1()
        .min_h(px(220.0))
        .border_1()
        .border_color(theme.border)
        .bg(theme.surface)
        .rounded(px(6.0))
        .overflow_y_scroll();

    if let Some(parent) = parent.clone() {
        list = list.child(row(
            "..",
            "Parent folder",
            false,
            false,
            Some(parent),
            entity.clone(),
            theme,
        ));
    }

    for entry in &entries {
        let is_selected = selected.as_deref() == Some(entry.path.as_str());
        let open_path = entry.path.clone();
        list = list.child(row(
            &entry.name,
            &entry.path,
            is_selected,
            true,
            Some(open_path),
            entity.clone(),
            theme,
        ));
    }

    // Distinguish "still loading" from "genuinely empty": showing "No
    // subfolders" during a request would flash on every navigation.
    if entries.is_empty() && parent.is_none() {
        list = list.child(
            div()
                .px(px(8.0))
                .py(px(10.0))
                .text_size(px(11.5))
                .text_color(theme.text_ghost)
                .child(if loading {
                    "Loading…"
                } else {
                    "No subfolders here."
                }),
        );
    }

    list
}

/// One row: folder name over its full path, highlighted when selected.
#[allow(clippy::too_many_arguments)]
fn row(
    name: &str,
    path: &str,
    selected: bool,
    navigable: bool,
    open_path: Option<String>,
    entity: gpui::WeakEntity<FileBrowser>,
    theme: &Theme,
) -> impl IntoElement {
    let name = name.to_string();
    let path = path.to_string();
    // Cloned for the click handler; `path` is still read as the row's caption.
    let click_path = path.clone();

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
            // ".." and folders both navigate on a double click, but ".." has
            // nothing to select, so it acts on the first click instead.
            if event.click_count >= 2 || !navigable {
                let Some(path) = open_path.clone() else {
                    return;
                };
                browser.update(cx, |this, cx| this.open(Some(path), cx));
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
