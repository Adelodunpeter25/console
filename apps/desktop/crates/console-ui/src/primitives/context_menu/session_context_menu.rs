//! Context-menu presentation for sidebar sessions and drafts.
//!
//! Built on the app's own [`context_menu`] surface rather than a component
//! library, so the sidebar menus share the composer menu's card, row metrics,
//! palette and keyboard handling.

use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, ParentElement, Styled, Window,
};

use crate::layout::sidebar::session_item::SETTLE_SHORTCUT_LABEL;
use crate::primitives::IconName;
use crate::primitives::menu::{ContextMenuHandle, MenuItem, context_menu};

/// The menu's open/closed state, kept across frames for as long as the row it
/// belongs to keeps rendering.
fn menu_handle(id: &ElementId, window: &mut Window, cx: &mut App) -> ContextMenuHandle {
    window
        .use_keyed_state(id.clone(), cx, |_, cx| ContextMenuHandle::new(cx))
        .read(cx)
        .clone()
}

/// Attach the session context menu to a sidebar session row.
///
/// The row owns the session-specific callbacks; this component only owns the
/// presentation and action ordering, so it can remain independent of the app
/// state and backend client. `id` must be unique per row.
#[allow(clippy::too_many_arguments)]
pub fn session_context_menu<E>(
    element: E,
    id: impl Into<ElementId>,
    window: &mut Window,
    cx: &mut App,
    on_open_in_new_window: impl Fn(&mut Window, &mut App) + 'static,
    on_rename: impl Fn(&mut Window, &mut App) + 'static,
    on_toggle_pin: impl Fn(&mut Window, &mut App) + 'static,
    on_settle: impl Fn(&mut Window, &mut App) + 'static,
    on_unsettle: impl Fn(&mut Window, &mut App) + 'static,
    on_delete: impl Fn(&mut Window, &mut App) + 'static,
    pinned: bool,
    settled: bool,
) -> AnyElement
where
    E: InteractiveElement + ParentElement + Styled + IntoElement + 'static,
{
    let id: ElementId = id.into();
    let handle = menu_handle(&id, window, cx);
    let on_open_in_new_window = Rc::new(on_open_in_new_window);
    let on_rename = Rc::new(on_rename);
    let on_toggle_pin = Rc::new(on_toggle_pin);
    let on_settle = Rc::new(on_settle);
    let on_unsettle = Rc::new(on_unsettle);
    let on_delete = Rc::new(on_delete);

    context_menu(element, id, &handle, move |_| {
        let pin_label = if pinned { "Unpin" } else { "Pin" };
        let (settle_label, settle_icon) = if settled {
            ("Move to Active", IconName::ArrowUp)
        } else {
            ("Settle", IconName::Check)
        };
        let on_open_in_new_window = on_open_in_new_window.clone();
        let on_rename = on_rename.clone();
        let on_toggle_pin = on_toggle_pin.clone();
        let on_settle = on_settle.clone();
        let on_unsettle = on_unsettle.clone();
        let on_delete = on_delete.clone();
        vec![
            MenuItem::new("Open in New Window", move |window, cx| {
                on_open_in_new_window(window, cx)
            })
            .icon(IconName::OpenInWindow.path()),
            MenuItem::Separator,
            MenuItem::new(pin_label, move |window, cx| on_toggle_pin(window, cx))
                .icon(IconName::Pin.path()),
            MenuItem::new("Rename", move |window, cx| on_rename(window, cx))
                .icon(IconName::Pencil.path()),
            MenuItem::new(settle_label, move |window, cx| {
                if settled {
                    on_unsettle(window, cx)
                } else {
                    on_settle(window, cx)
                }
            })
            .icon(settle_icon.path())
            .shortcut(SETTLE_SHORTCUT_LABEL),
            MenuItem::Separator,
            MenuItem::new("Delete", move |window, cx| on_delete(window, cx))
                .icon(IconName::TrashBinMinimalistic.path())
                .destructive(true),
        ]
    })
}

/// Attach the draft context menu to a sidebar draft row.
///
/// Drafts only need a destructive discard action — no rename.
pub fn draft_context_menu<E>(
    element: E,
    id: impl Into<ElementId>,
    window: &mut Window,
    cx: &mut App,
    on_delete: impl Fn(&mut Window, &mut App) + 'static,
) -> AnyElement
where
    E: InteractiveElement + ParentElement + Styled + IntoElement + 'static,
{
    let id: ElementId = id.into();
    let handle = menu_handle(&id, window, cx);
    let on_delete = Rc::new(on_delete);

    context_menu(element, id, &handle, move |_| {
        let on_delete = on_delete.clone();
        vec![
            MenuItem::new("Delete Draft", move |window, cx| on_delete(window, cx))
                .icon(IconName::TrashBinMinimalistic.path())
                .destructive(true),
        ]
    })
}
