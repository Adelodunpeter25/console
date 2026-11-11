use gpui::{
    AnyView, App, AppContext, Context, IntoElement, ParentElement, Render, SharedString,
    Styled, Window, div, px, rgb,
};
use std::sync::atomic::{AtomicI64, Ordering};
use std::time::Duration;

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;
use crate::utils::format_time_ago;

static LAST_HOVER_TS: AtomicI64 = AtomicI64::new(0);

/// Initial delay before the first hover popup opens.
const INITIAL_DELAY: Duration = Duration::from_millis(180);
/// Grace period: moving to another chat within this window opens instantly.
const GRACE_PERIOD_MS: i64 = 450;

pub fn current_hover_delay() -> Duration {
    let now = chrono::Utc::now().timestamp_millis();
    let last = LAST_HOVER_TS.load(Ordering::Relaxed);
    if last > 0 && (now - last) < GRACE_PERIOD_MS {
        Duration::from_millis(0)
    } else {
        INITIAL_DELAY
    }
}

pub fn touch_hover() {
    let now = chrono::Utc::now().timestamp_millis();
    LAST_HOVER_TS.store(now, Ordering::Relaxed);
}

/// Hover details card for a sidebar session item.
pub struct SessionHoverCard {
    title: SharedString,
    created_at: i64,
    updated_at: i64,
    project_name: SharedString,
}

impl SessionHoverCard {
    pub fn new(
        title: impl Into<SharedString>,
        created_at: i64,
        updated_at: i64,
        project_name: impl Into<SharedString>,
    ) -> Self {
        touch_hover();
        Self {
            title: title.into(),
            created_at,
            updated_at,
            project_name: project_name.into(),
        }
    }

    pub fn build(self, _window: &mut Window, cx: &mut App) -> AnyView {
        cx.new(|_| self).into()
    }
}

impl Render for SessionHoverCard {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        touch_hover();

        let created_ago = if self.created_at > 0 {
            let rel = format_time_ago(self.created_at);
            if rel == "just now" {
                "just now".to_string()
            } else {
                format!("{rel} ago")
            }
        } else {
            "recently".to_string()
        };

        let updated_str = if self.updated_at > 0 {
            let rel = format_time_ago(self.updated_at);
            if rel == "just now" {
                "updated now".to_string()
            } else {
                format!("updated {rel} ago")
            }
        } else {
            "updated recently".to_string()
        };

        let subtitle = format!("Created {created_ago}, {updated_str}");

        div().pt(px(4.0)).pl(px(4.0)).child(
            div()
                .w(px(280.0))
                .p(px(12.0))
                .rounded(px(8.0))
                .border_1()
                .border_color(theme.border_strong)
                // Dark background matching the menu surface
                .bg(rgb(0x0C0C0C))
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
                                .child(self.title.clone()),
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
                                .child(self.project_name.clone()),
                        ),
                ),
        )
    }
}
