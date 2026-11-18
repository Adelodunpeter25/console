//! Context-menu presentation for HTML files (in file tree and links).
//! Offers "Open in browser" and "Open in code editor".

use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, ParentElement, Styled, Window,
};

use crate::primitives::menu::{context_menu, ContextMenuHandle, MenuItem};
use crate::primitives::IconName;

fn menu_handle(id: &ElementId, window: &mut Window, cx: &mut App) -> ContextMenuHandle {
    window
        .use_keyed_state(id.clone(), cx, |_, cx| ContextMenuHandle::new(cx))
        .read(cx)
        .clone()
}

/// Attach an HTML-file context menu to an element (right-click / context menu).
pub fn html_file_context_menu<E>(
    element: E,
    id: impl Into<ElementId>,
    window: &mut Window,
    cx: &mut App,
    on_open_browser: impl Fn(&mut Window, &mut App) + 'static,
    on_open_editor: impl Fn(&mut Window, &mut App) + 'static,
) -> AnyElement
where
    E: InteractiveElement + ParentElement + Styled + IntoElement + 'static,
{
    let id: ElementId = id.into();
    let handle = menu_handle(&id, window, cx);
    let on_open_browser = Rc::new(on_open_browser);
    let on_open_editor = Rc::new(on_open_editor);

    context_menu(element, id, &handle, move |_| {
        let on_open_browser = on_open_browser.clone();
        let on_open_editor = on_open_editor.clone();
        vec![
            MenuItem::new("Open in browser", move |window, cx| {
                on_open_browser(window, cx)
            })
            .icon(IconName::Globe.path()),
            MenuItem::new("Open in editor", move |window, cx| {
                on_open_editor(window, cx)
            })
            .icon(IconName::File.path()),
        ]
    })
}
