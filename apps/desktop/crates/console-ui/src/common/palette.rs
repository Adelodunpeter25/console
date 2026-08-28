//! Central command palette modal, built on `gpui_component::command`.
//!
//! [`CommandPaletteModal`] is the shared, app-agnostic shell: it owns the
//! `CommandState` entity, renders the dimmed modal overlay with the search
//! field and item list, and exposes the small API every palette surface needs
//! — show/hide, replace the item list, and react to query changes. Feature
//! surfaces (the ⌘K command palette, ⌘P quick file open, ⌘O project browser)
//! are thin wrappers in this crate that feed it [`PaletteEntry`]s and handle
//! confirmation.
//!
//! [`PaletteEntry`] is a plain row: a stable id, a display label, an optional
//! icon, and the action to run on Enter. `keep_open` marks entries that
//! navigate (e.g. drilling into a folder) instead of dismissing the palette.

use std::rc::Rc;

use gpui::{
    App, Context, Entity, Focusable, IntoElement, ParentElement, Render, SharedString, Styled,
    Window, div, prelude::*, px,
};
use gpui_component::command::{Command, CommandItem, CommandState};

use crate::IconName;

/// One palette row: a stable id, display label, and the action to run on Enter.
#[derive(Clone)]
pub struct PaletteEntry {
    pub id: SharedString,
    pub label: SharedString,
    pub icon: Option<IconName>,
    /// When true the modal stays open after this entry is confirmed (used for
    /// navigation entries such as drilling into a directory).
    pub keep_open: bool,
    #[allow(clippy::type_complexity)]
    pub handler: Rc<dyn Fn(&mut Window, &mut App)>,
}

impl PaletteEntry {
    pub fn new(
        id: impl Into<SharedString>,
        label: impl Into<SharedString>,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            icon: None,
            keep_open: false,
            handler: Rc::new(handler),
        }
    }

    /// Set the leading icon for this row.
    pub fn icon(mut self, icon: IconName) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Keep the palette open after this entry is confirmed. Navigation rows
    /// (folders, …) use this; actions that finish the flow leave it `false`.
    pub fn keep_open(mut self, keep_open: bool) -> Self {
        self.keep_open = keep_open;
        self
    }
}

/// The shared command palette modal shell.
///
/// Renders nothing while closed. The owning wrapper supplies entries via
/// [`Self::set_entries`] (async results call it again — it notifies) and, for
/// search-driven modes, a query handler via [`Self::set_query_handler`] that
/// runs with the trimmed query whenever it changes.
pub struct CommandPaletteModal {
    state: Entity<CommandState>,
    entries: Rc<Vec<PaletteEntry>>,
    open: bool,
    placeholder: SharedString,
    /// Whether the query locally filters the item list. Set to `false` when an
    /// external source already answers the query (e.g. server-side file search).
    filterable: bool,
    /// Async search hook: called with the current query whenever it changes.
    query_handler: Option<Rc<dyn Fn(&str, &mut Window, &mut App)>>,
    /// Navigation hook for `keep_open` rows (e.g. drilling into a folder):
    /// called with the entry's id instead of dismissing the palette.
    browse_callback: Option<Rc<dyn Fn(&str, &mut Window, &mut App)>>,
    /// Monotonic token for debounced async searches: a result is applied only
    /// while its generation is still the latest (see [`Self::set_entries`]).
    search_generation: u64,
}

impl CommandPaletteModal {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let state = cx.new(|cx| CommandState::new(window, cx));
        Self {
            state,
            entries: Rc::new(Vec::new()),
            open: false,
            placeholder: "Type a command or search…".into(),
            filterable: true,
            query_handler: None,
            browse_callback: None,
            search_generation: 0,
        }
    }

    /// Replace the entry list. Safe to call from async contexts — notifies so
    /// the modal re-renders with the fresh rows. When `generation` is `Some`,
    /// the update is dropped if a newer search has been scheduled since.
    pub fn set_entries(&mut self, entries: Vec<PaletteEntry>, cx: &mut Context<Self>) {
        self.entries = Rc::new(entries);
        cx.notify();
    }

    /// Drop the generation token so a fresh open can apply its first result.
    pub fn reset_search_generation(&mut self, cx: &mut Context<Self>) {
        self.search_generation = 0;
        cx.notify();
    }

    /// [`Self::set_entries`] guarded by a search generation token.
    pub fn set_entries_with_generation(
        &mut self,
        generation: u64,
        entries: Vec<PaletteEntry>,
        cx: &mut Context<Self>,
    ) {
        if generation != self.search_generation {
            return;
        }
        self.set_entries(entries, cx);
    }

    /// Set the placeholder shown in the search field.
    pub fn set_placeholder(&mut self, placeholder: impl Into<SharedString>, cx: &mut Context<Self>) {
        self.placeholder = placeholder.into();
        cx.notify();
    }

    /// Turn local query filtering on or off. Defaults to `true`; turn it off
    /// when an external source already answers the query.
    pub fn set_filterable(&mut self, filterable: bool, cx: &mut Context<Self>) {
        self.filterable = filterable;
        cx.notify();
    }

    /// Register the callback run whenever the search query changes. Replace it
    /// on every open so it captures the latest context.
    pub fn set_query_handler(
        &mut self,
        handler: impl Fn(&str, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.query_handler = Some(Rc::new(handler));
    }

    /// Register the navigation hook for `keep_open` rows. It receives the
    /// confirmed entry's id and keeps the modal open (e.g. browsing into a
    /// folder). Replaced on every open.
    pub fn set_browse_callback(
        &mut self,
        callback: impl Fn(&str, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.browse_callback = Some(Rc::new(callback));
    }

    pub fn show(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.open = true;
        self.state.update(cx, |state, cx| state.focus(window, cx));
        cx.notify();
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.open = false;
        cx.notify();
    }

    pub fn toggle(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.open {
            self.hide(cx);
        } else {
            self.show(window, cx);
        }
    }

    pub fn is_open(&self) -> bool {
        self.open
    }
}

impl Focusable for CommandPaletteModal {
    fn focus_handle(&self, cx: &App) -> gpui::FocusHandle {
        self.state.read(cx).focus_handle(cx)
    }
}

impl Render for CommandPaletteModal {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if !self.open {
            return div().into_any_element();
        }

        let entries = self.entries.clone();
        let palette_handle = cx.entity().downgrade();
        let command = Command::new(&self.state)
            .filterable(self.filterable)
            .items(entries.iter().map(|entry| {
                let mut item = CommandItem::new().label(entry.label.clone());
                if let Some(icon) = entry.icon {
                    item = item.child(move |_window, cx| {
                        let theme = crate::Theme::current(cx);
                        icon.render(15.0, theme.text_secondary).into_any_element()
                    });
                }
                item
            }))
            .placeholder(self.placeholder.clone())
            // Escape with a non-empty query clears it; once the query is
            // empty gpui-component hands dismissal to us here.
            .on_cancel({
                let palette_handle = palette_handle.clone();
                move |_, cx| {
                    if let Some(palette) = palette_handle.upgrade() {
                        palette.update(cx, |palette, cx| palette.hide(cx));
                    }
                }
            })
            .on_query({
                let palette_handle = palette_handle.clone();
                move |query, window, cx| {
                    if let Some(palette) = palette_handle.upgrade() {
                        let handler = palette.read(cx).query_handler.clone();
                        if let Some(handler) = handler {
                            handler(query, window, cx);
                        }
                    }
                }
            })
            .on_confirm(move |index, window, cx| {
                let entry = entries.get(index.row).cloned();
                if let Some(palette) = palette_handle.upgrade() {
                    if let Some(entry) = entry {
                        if entry.keep_open {
                            // Navigation row: hand the id to the browse hook
                            // and keep the palette up.
                            if let Some(callback) = palette.read(cx).browse_callback.clone() {
                                callback(entry.id.as_ref(), window, cx);
                            }
                        } else {
                            palette.update(cx, |palette, cx| palette.hide(cx));
                            (entry.handler)(window, cx);
                        }
                    }
                }
            });

        // Modal overlay: dimmed backdrop, palette centered near the top like ⌘K menus.
        // Painted last inside app-root, so no explicit z-index is needed.
        div()
            .absolute()
            .inset_0()
            .bg(gpui::black().opacity(0.35))
            .flex()
            .justify_center()
            .items_start()
            .pt(px(96.0))
            .on_mouse_down(gpui::MouseButton::Left, {
                let entity = cx.entity().downgrade();
                move |_, _, cx| {
                    if let Some(palette) = entity.upgrade() {
                        palette.update(cx, |palette, cx| palette.hide(cx));
                    }
                }
            })
            .child(
                div()
                    .occlude()
                    .w(px(560.0))
                    .max_h(px(420.0))
                    .on_mouse_down(gpui::MouseButton::Left, |_, _, cx| {
                        cx.stop_propagation();
                    })
                    .child(command),
            )
            .into_any_element()
    }
}

/// ⌘K command palette surface: static entries for app commands, built on the
/// shared [`CommandPaletteModal`].
pub struct CommandPalette {
    modal: Entity<CommandPaletteModal>,
}

impl CommandPalette {
    /// Create the palette. Reuse one instance per surface; entries can be
    /// replaced any time via [`Self::set_entries`].
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let modal = cx.new(|cx| CommandPaletteModal::new(window, cx));
        Self { modal }
    }

    /// Replace the entry list. Call before `show()` so filtering and
    /// IndexPath addressing match.
    pub fn set_entries(&mut self, entries: Vec<PaletteEntry>, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.set_entries(entries, cx));
    }

    pub fn show(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.show(window, cx));
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.hide(cx));
    }

    pub fn toggle(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.toggle(window, cx));
    }

    pub fn is_open(&self, cx: &App) -> bool {
        self.modal.read(cx).is_open()
    }
}

impl Render for CommandPalette {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        self.modal.clone()
    }
}
