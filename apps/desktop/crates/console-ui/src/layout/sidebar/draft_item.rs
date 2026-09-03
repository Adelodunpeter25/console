use gpui::{
    App, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::primitives::{IconName, app_icon, draft_context_menu};
use crate::theme::Theme;
use crate::utils::format_time_ago;

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

    let folder_name = draft
        .project_name
        .clone()
        .unwrap_or_else(|| "Workspace".to_string());

    let delete_key = draft
        .session_id
        .clone()
        .unwrap_or_else(|| "new_chat".to_string());
    let on_delete = on_delete_draft.clone();

    let row = div()
        .id(ElementId::Name(
            format!(
                "draft-row-{}",
                draft.session_id.as_deref().unwrap_or("new-chat")
            )
            .into(),
        ))
        .w_full()
        .h(px(55.0))
        .px(px(8.0))
        .py(px(6.0))
        .rounded(px(8.0))
        .cursor_default()
        .group("sidebar-draft-card")
        .when(is_active, |s| s.bg(theme.sidebar_item_background))
        .when(!is_active, |s| {
            s.hover(|h| h.bg(theme.sidebar_item_background))
        })
        .on_click(move |_, window, cx| on_click(window, cx))
        .flex()
        .flex_col()
        .justify_between()
        .child(
            div()
                .w_full()
                .flex()
                .items_center()
                .justify_between()
                .gap_x(px(6.0))
                .child(
                    div().flex_1().min_w_0().child(
                        div()
                            .truncate()
                            .text_size(px(13.0))
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(theme.text)
                            .child(draft.title.clone()),
                    ),
                ),
        )
        .child(
            div()
                .w_full()
                .flex()
                .items_center()
                .justify_between()
                .gap_x(px(5.0))
                .text_size(px(11.5))
                .line_height(px(15.0))
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .flex()
                        .items_center()
                        .gap_x(px(4.0))
                        .child(app_icon(IconName::Folder, 11.0, theme.text_tertiary))
                        .child(
                            div()
                                .flex_1()
                                .truncate()
                                .text_color(theme.text_tertiary)
                                .child(folder_name),
                        ),
                )
                .child(
                    div()
                        .flex_none()
                        .text_color(theme.text_ghost)
                        .child(format_time_ago(draft.updated_at)),
                ),
        );

    draft_context_menu(row, move |window, cx| {
        on_delete(delete_key.clone(), window, cx);
    })
    .into_any_element()
}
