use std::time::Duration;

use gpui::{
    AnyElement, App, Entity, IntoElement, ParentElement, Styled, div, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;
use crate::utils::format_time_ago;

const OPEN_DELAY: Duration = Duration::from_millis(130);
const HIDE_DELAY: Duration = Duration::from_millis(90);

/// Manages hover card state across all session items in the sidebar.
pub struct SidebarHoverManager {
    pub active_id: Option<String>,
    pub hovered_id: Option<String>,
    pub card_hovered: bool,
    pub generation: u64,
}

impl SidebarHoverManager {
    pub fn new() -> Self {
        Self {
            active_id: None,
            hovered_id: None,
            card_hovered: false,
            generation: 0,
        }
    }
}

pub fn on_row_hover(
    state: &Entity<SidebarHoverManager>,
    session_id: &str,
    hovered: bool,
    cx: &mut App,
) {
    let session_id = session_id.to_string();
    let (generation, is_active) = state.update(cx, |s, _| {
        s.generation += 1;
        if hovered {
            s.hovered_id = Some(session_id.clone());
            if s.active_id.is_some() {
                // If a popup is already active on another item, switch immediately without delay
                s.active_id = Some(session_id.clone());
            }
            (s.generation, s.active_id.is_some())
        } else {
            if s.hovered_id.as_deref() == Some(&session_id) {
                s.hovered_id = None;
            }
            (s.generation, s.active_id.is_some())
        }
    });

    if hovered {
        if is_active {
            cx.notify(state.entity_id());
        } else {
            let state = state.clone();
            cx.spawn(async move |cx| {
                cx.background_executor().timer(OPEN_DELAY).await;
                cx.update(|cx| {
                    state.update(cx, |s, cx| {
                        if s.generation == generation && s.hovered_id.as_deref() == Some(&session_id) {
                            s.active_id = Some(session_id);
                            cx.notify();
                        }
                    });
                });
            })
            .detach();
        }
    } else {
        let state = state.clone();
        cx.spawn(async move |cx| {
            cx.background_executor().timer(HIDE_DELAY).await;
            cx.update(|cx| {
                state.update(cx, |s, cx| {
                    if s.generation == generation && s.hovered_id.is_none() && !s.card_hovered {
                        s.active_id = None;
                        cx.notify();
                    }
                });
            });
        })
        .detach();
    }
}

pub fn on_card_hover(
    state: &Entity<SidebarHoverManager>,
    hovered: bool,
    cx: &mut App,
) {
    let generation = state.update(cx, |s, _| {
        s.generation += 1;
        s.card_hovered = hovered;
        s.generation
    });

    if !hovered {
        let state = state.clone();
        cx.spawn(async move |cx| {
            cx.background_executor().timer(HIDE_DELAY).await;
            cx.update(|cx| {
                state.update(cx, |s, cx| {
                    if s.generation == generation && s.hovered_id.is_none() && !s.card_hovered {
                        s.active_id = None;
                        cx.notify();
                    }
                });
            });
        })
        .detach();
    }
}

/// Render the content inside the session hover card.
pub fn render_session_card_content(
    title: &str,
    created_at: i64,
    updated_at: i64,
    project_name: &str,
    theme: &Theme,
) -> AnyElement {
    let created_ago = if created_at > 0 {
        let rel = format_time_ago(created_at);
        if rel == "just now" {
            "just now".to_string()
        } else {
            format!("{rel} ago")
        }
    } else {
        "recently".to_string()
    };

    let updated_str = if updated_at > 0 {
        let rel = format_time_ago(updated_at);
        if rel == "just now" {
            "updated now".to_string()
        } else {
            format!("updated {rel} ago")
        }
    } else {
        "updated recently".to_string()
    };

    let subtitle = format!("Created {created_ago}, {updated_str}");

    div()
        .w(px(280.0))
        .p(px(12.0))
        .rounded(px(8.0))
        .border_1()
        .border_color(theme.border_strong)
        .bg(theme.menu)
        .shadow_lg()
        .flex()
        .flex_col()
        .gap_y(px(8.0))
        .child(
            div()
                .flex()
                .flex_col()
                .gap_y(px(2.0))
                .child(
                    div()
                        .text_size(px(13.0))
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .line_height(px(17.0))
                        .child(title.to_string()),
                )
                .child(
                    div()
                        .text_size(px(11.0))
                        .text_color(theme.text_tertiary)
                        .child(subtitle),
                ),
        )
        .child(div().h(px(1.0)).bg(theme.border))
        .child(
            div()
                .flex()
                .items_center()
                .text_size(px(12.0))
                .child(
                    div()
                        .w(px(65.0))
                        .text_color(theme.text_tertiary)
                        .child("Project"),
                )
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap_x(px(5.0))
                        .text_color(theme.text_secondary)
                        .child(app_icon(IconName::Folder, 13.0, theme.text_tertiary))
                        .child(project_name.to_string()),
                ),
        )
        .into_any_element()
}
