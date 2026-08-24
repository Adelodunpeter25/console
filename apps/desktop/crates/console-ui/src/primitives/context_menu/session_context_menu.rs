//! Context-menu presentation for sidebar sessions.

use std::rc::Rc;

use gpui::{App, InteractiveElement, IntoElement, ParentElement, Styled, Window};

use crate::primitives::{ContextMenu, ContextMenuExt, IconName, MenuItem};

/// Attach the session context menu to a sidebar session row.
///
/// The row owns the session-specific callbacks; this component only owns the
/// presentation and action ordering, so it can remain independent of the app
/// state and backend client.
pub fn session_context_menu<E>(
    element: E,
    on_rename: impl Fn(&mut Window, &mut App) + 'static,
    on_delete: impl Fn(&mut Window, &mut App) + 'static,
) -> ContextMenu<E>
where
    E: InteractiveElement + ParentElement + Styled + IntoElement + 'static,
{
    let on_rename = Rc::new(on_rename);
    let on_delete = Rc::new(on_delete);
    ContextMenuExt::context_menu(element, move |_| {
        let on_rename = on_rename.clone();
        let on_delete = on_delete.clone();
        vec![
            MenuItem::new("Rename", move |window, cx| on_rename(window, cx))
                .icon(IconName::Pencil.path())
                .shortcut("F2"),
            MenuItem::Separator,
            MenuItem::new("Delete", move |window, cx| on_delete(window, cx))
                .icon(IconName::TrashBinMinimalistic.path())
                .destructive(true),
        ]
    })
}
