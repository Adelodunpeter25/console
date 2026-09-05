//! Context-menu presentation for sidebar sessions.

use std::rc::Rc;

use gpui::{App, InteractiveElement, IntoElement, ParentElement, Styled, Window, div, px};
use gpui_component::menu::{ContextMenu, ContextMenuExt as _, PopupMenuItem};

use crate::primitives::{IconName, icon};
use crate::theme::Theme;

/// Attach the session context menu to a sidebar session row.
///
/// The row owns the session-specific callbacks; this component only owns the
/// presentation and action ordering, so it can remain independent of the app
/// state and backend client. Open/close, placement, keyboard navigation, and
/// focus restoration come from `gpui_component::menu`.
pub fn session_context_menu<E>(
    element: E,
    on_open_in_new_window: impl Fn(&mut Window, &mut App) + 'static,
    on_rename: impl Fn(&mut Window, &mut App) + 'static,
    on_delete: impl Fn(&mut Window, &mut App) + 'static,
) -> ContextMenu<E>
where
    E: InteractiveElement + ParentElement + Styled + IntoElement + 'static,
{
    let on_open_in_new_window = Rc::new(on_open_in_new_window);
    let on_rename = Rc::new(on_rename);
    let on_delete = Rc::new(on_delete);
    element.context_menu(move |menu, _, _| {
        let on_open_in_new_window = on_open_in_new_window.clone();
        let on_rename = on_rename.clone();
        let on_delete = on_delete.clone();
        menu.min_w(px(200.0))
            .item(
                entry("Open in New Window", IconName::WindowRestore, false)
                    .on_click(move |_, window, cx| on_open_in_new_window(window, cx)),
            )
            .separator()
            .item(
                entry("Rename", IconName::Pencil, false)
                    .on_click(move |_, window, cx| on_rename(window, cx)),
            )
            .separator()
            .item(
                entry("Delete", IconName::TrashBinMinimalistic, true)
                    .on_click(move |_, window, cx| on_delete(window, cx)),
            )
    })
}

/// Attach the draft context menu to a sidebar draft row.
///
/// Drafts only need a destructive discard action — no rename.
pub fn draft_context_menu<E>(
    element: E,
    on_delete: impl Fn(&mut Window, &mut App) + 'static,
) -> ContextMenu<E>
where
    E: InteractiveElement + ParentElement + Styled + IntoElement + 'static,
{
    let on_delete = Rc::new(on_delete);
    element.context_menu(move |menu, _, _| {
        let on_delete = on_delete.clone();
        menu.min_w(px(200.0)).item(
            entry("Delete Draft", IconName::TrashBinMinimalistic, true)
                .on_click(move |_, window, cx| on_delete(window, cx)),
        )
    })
}

/// One menu entry drawn with the app's own icon set and palette.
///
/// The component library's stock item has no destructive styling, and its icon
/// slot renders at its own size, so both entries draw their body here and let
/// `PopupMenu` supply the chrome: hover, selection, click, and keyboard
/// activation.
fn entry(label: &'static str, icon_name: IconName, destructive: bool) -> PopupMenuItem {
    PopupMenuItem::element(move |_, cx| {
        let theme = Theme::current(cx);
        let color = if destructive {
            theme.danger
        } else {
            theme.text_secondary
        };
        div()
            .flex()
            .items_center()
            .gap(px(8.0))
            .text_size(px(13.0))
            .line_height(px(15.0))
            .text_color(color)
            .child(icon(icon_name.path(), 12.0, color))
            .child(label)
    })
}
