use std::rc::Rc;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    Styled, Window, div, px,
};
use console_core::types::SessionHeader;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct DeletedChatsPage {
    pub deleted_sessions: Vec<SessionHeader>,
    pub on_restore: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    pub on_permanent_delete: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl RenderOnce for DeletedChatsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_restore = self.on_restore.clone();
        let on_permanent_delete = self.on_permanent_delete.clone();

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(16.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child("Deleted Chats (Trash)"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .text_color(theme.text_secondary)
                            .child("Browse deleted conversations. Restoring returns them to your active sidebar."),
                    ),
            )
            .child(
                if self.deleted_sessions.is_empty() {
                    div()
                        .p(px(32.0))
                        .rounded(px(8.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.surface)
                        .flex()
                        .flex_col()
                        .items_center()
                        .justify_center()
                        .gap(px(8.0))
                        .child(app_icon(IconName::TrashBinMinimalistic, 24.0, theme.text_ghost))
                        .child(
                            div()
                                .text_size(px(13.0))
                                .text_color(theme.text_secondary)
                                .child("Trash is empty."),
                        )
                } else {
                    div()
                        .flex()
                        .flex_col()
                        .gap(px(8.0))
                        .children(self.deleted_sessions.into_iter().map(|session| {
                            let session_id = session.id.clone();
                            let session_id_restore = session.id.clone();
                            let session_id_delete = session.id.clone();
                            let on_rest = on_restore.clone();
                            let on_perm = on_permanent_delete.clone();

                            let title = if session.title.is_empty() { "Untitled conversation".to_string() } else { session.title };
                            let date_str = chrono::DateTime::from_timestamp_millis(session.updated_at)
                                .map(|dt| dt.format("%b %d, %Y · %H:%M").to_string())
                                .unwrap_or_default();

                            div()
                                .id(ElementId::from(format!("deleted-chat-{}", session_id)))
                                .p(px(12.0))
                                .rounded(px(8.0))
                                .border_1()
                                .border_color(theme.border)
                                .bg(theme.surface)
                                .flex()
                                .items_center()
                                .justify_between()
                                .child(
                                    div()
                                        .flex()
                                        .flex_col()
                                        .gap(px(2.0))
                                        .child(
                                            div()
                                                .text_size(px(13.5))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .text_color(theme.text)
                                                .child(title),
                                        )
                                        .child(
                                            div()
                                                .text_size(px(11.5))
                                                .text_color(theme.text_tertiary)
                                                .child(date_str),
                                        ),
                                )
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .gap(px(8.0))
                                        .child(
                                            div()
                                                .id(ElementId::from(format!("btn-restore-{}", session_id_restore)))
                                                .size(px(26.0))
                                                .rounded(px(5.0))
                                                .border_1()
                                                .border_color(theme.border_strong)
                                                .bg(theme.raised)
                                                .cursor_pointer()
                                                .flex()
                                                .items_center()
                                                .justify_center()
                                                .hover(|s| s.bg(theme.overlay))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_rest)(session_id_restore.clone(), window, cx);
                                                })
                                                .child(app_icon(IconName::Restart, 14.0, theme.accent)),
                                        )
                                        .child(
                                            div()
                                                .id(ElementId::from(format!("btn-perm-delete-{}", session_id_delete)))
                                                .size(px(26.0))
                                                .rounded(px(5.0))
                                                .cursor_pointer()
                                                .flex()
                                                .items_center()
                                                .justify_center()
                                                .hover(|s| s.bg(theme.overlay_strong))
                                                .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                                                    cx.stop_propagation();
                                                    (on_perm)(session_id_delete.clone(), window, cx);
                                                })
                                                .child(app_icon(IconName::TrashBinMinimalistic, 14.0, theme.danger)),
                                        ),
                                )
                        }))
                },
            )
    }
}
