//! ⌘K command palette, built on `gpui_component::command`.
//!
//! Drop-in and app-agnostic: the owner supplies [`PaletteEntry`]s (label +
//! closure) and renders the palette conditionally. Keyboard nav (↑↓/Enter/Esc)
//! and filtering come from gpui-component's `Command`; entries here are plain
//! callbacks rather than gpui `Action`s so callers don't need to register
//! actions for dynamic items (sessions, tabs).
//!
//! **Not wired into `ConsoleDesktopApp` yet.** Integration requires:
//! 1. `gpui_component::init(cx)` in `src/main.rs` before opening the window.
//! 2. Wrapping the window root: `cx.new(|cx| Root::new(app_view.into(), window, cx))`
//!    — overlays (this palette, dialogs) render through `Root`.

use std::rc::Rc;

use gpui::{
    App, Context, Entity, Focusable, IntoElement, ParentElement, Render, SharedString, Styled,
    Window, div, prelude::*, px,
};
use gpui_component::command::{Command, CommandItem, CommandState};

/// One palette row: a stable id, display label, and the action to run on Enter.
pub struct PaletteEntry {
    pub id: SharedString,
    pub label: SharedString,
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
            handler: Rc::new(handler),
        }
    }
}

pub struct CommandPalette {
    state: Entity<CommandState>,
    entries: Rc<Vec<PaletteEntry>>,
    open: bool,
}

impl CommandPalette {
    /// Create the palette. Reuse one instance per surface; entries can be
    /// replaced any time via [`Self::set_entries`].
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let state = cx.new(|cx| CommandState::new(window, cx));
        Self {
            state,
            entries: Rc::new(Vec::new()),
            open: false,
        }
    }

    /// Replace the entry list. Call before `show()` so filtering and
    /// IndexPath addressing match.
    pub fn set_entries(&mut self, entries: Vec<PaletteEntry>, _cx: &mut Context<Self>) {
        self.entries = Rc::new(entries);
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

impl Focusable for CommandPalette {
    fn focus_handle(&self, cx: &App) -> gpui::FocusHandle {
        self.state.read(cx).focus_handle(cx)
    }
}

impl Render for CommandPalette {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if !self.open {
            return div().into_any_element();
        }

        let entries = self.entries.clone();

        let palette_handle = cx.entity().downgrade();
        let command = Command::new(&self.state)
            .items(entries.iter().map(|entry| {
                CommandItem::new().label(entry.label.clone())
            }))
            .placeholder("Type a command or search…")
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
            .on_confirm(move |index, window, cx| {
                if let Some(entry) = entries.get(index.row) {
                    (entry.handler)(window, cx);
                }
                // Enter or item click both land here; the palette is a hosted
                // overlay, so dismissal is our job (gpui-component never hides it).
                if let Some(palette) = palette_handle.upgrade() {
                    palette.update(cx, |palette, cx| palette.hide(cx));
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
