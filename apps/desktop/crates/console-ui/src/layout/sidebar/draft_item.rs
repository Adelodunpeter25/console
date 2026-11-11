use gpui::{App, ElementId, IntoElement, ParentElement, StatefulInteractiveElement, Window};
use std::rc::Rc;

use super::item_row::{row_container, status_slot, title_label};
use crate::primitives::draft_context_menu;
use crate::theme::Theme;

#[derive(Clone, Debug)]
pub struct DraftSummary {
    pub session_id: Option<String>,
    pub title: String,
    pub preview: String,
    pub project_name: Option<String>,
    pub updated_at: i64,
}

pub fn render_sidebar_draft_item(
    draft: &DraftSummary,
    selected_id: Option<&str>,
    on_select: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_new_chat: &Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_delete_draft: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    theme: Theme,
) -> gpui::AnyElement {
    let is_active = match &draft.session_id {
        Some(sid) => selected_id == Some(sid.as_str()),
        None => selected_id.is_none(),
    };
    let sid = draft.session_id.clone();
    let on_select = on_select.clone();
    let on_new = on_new_chat.clone();
    let on_click = move |window: &mut Window, cx: &mut App| {
        if let Some(sid) = &sid {
            (on_select)(sid.clone(), window, cx);
        } else {
            (on_new)(window, cx);
        }
    };

    let delete_key = draft
        .session_id
        .clone()
        .unwrap_or_else(|| "new_chat".to_string());
    let on_delete = on_delete_draft.clone();

    let row = row_container(
        ElementId::Name(
            format!(
                "draft-row-{}",
                draft.session_id.as_deref().unwrap_or("new-chat")
            )
            .into(),
        ),
        is_active,
        theme,
    )
    .on_click(move |_, window, cx| on_click(window, cx))
    .child(status_slot())
    .child(title_label(is_active, theme).child(draft.title.clone()));

    draft_context_menu(row, move |window, cx| {
        on_delete(delete_key.clone(), window, cx);
    })
    .into_any_element()
}
