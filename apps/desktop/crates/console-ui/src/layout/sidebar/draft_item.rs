use gpui::{
    App, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

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

    // Same single-row anatomy as `SidebarSessionItem`: the empty leading slot
    // keeps draft titles aligned with the status icons on session rows.
    let row = div()
        .id(ElementId::Name(
            format!(
                "draft-row-{}",
                draft.session_id.as_deref().unwrap_or("new-chat")
            )
            .into(),
        ))
        .w_full()
        .min_w_0()
        .h(px(34.0))
        .px(px(6.0))
        .rounded(px(6.0))
        .cursor_default()
        .when(is_active, |s| s.bg(theme.sidebar_item_background))
        .when(!is_active, |s| {
            s.hover(|h| h.bg(theme.sidebar_item_background))
        })
        .on_click(move |_, window, cx| on_click(window, cx))
        .flex()
        .flex_row()
        .items_center()
        .gap_x(px(6.0))
        .child(div().size(px(16.0)).flex_none())
        .child(
            div()
                .flex_1()
                .min_w_0()
                .truncate()
                .text_size(px(13.5))
                .font_weight(FontWeight::MEDIUM)
                .text_color(if is_active {
                    theme.text
                } else {
                    theme.text_secondary
                })
                .child(draft.title.clone()),
        );

    draft_context_menu(row, move |window, cx| {
        on_delete(delete_key.clone(), window, cx);
    })
    .into_any_element()
}
