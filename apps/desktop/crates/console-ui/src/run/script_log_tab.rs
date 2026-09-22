//! Dedicated output tab for a project script.

use console_core::ScriptRunStatus;
use gpui::{App, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};

use crate::markdown::render::MONO_FAMILY;
use crate::theme::Theme;
use crate::viewer::LogsViewer;
use crate::run::RunOutputView;

#[derive(IntoElement)]
pub struct ScriptLogTab {
    pub label: String,
    pub command: String,
    pub status: Option<ScriptRunStatus>,
    pub exit_code: Option<i32>,
    pub starting: bool,
    pub output: String,
    pub view: Option<RunOutputView>,
}

impl RenderOnce for ScriptLogTab {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let body = match self.view {
            Some(view) if !self.output.is_empty() => LogsViewer::new(
                format!("script-log-{}", self.label), view.lines, view.list_state,
            )
            .selection(view.selection)
            .scrollbar_state(view.scrollbar_state)
            .into_any_element(),
            _ => div()
                .flex_1()
                .p(px(12.0))
                .font_family(MONO_FAMILY)
                .text_size(px(11.0))
                .text_color(theme.text_tertiary)
                .child(if self.output.is_empty() { "(no output yet)".to_string() } else { self.output })
                .into_any_element(),
        };
        div()
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.canvas)
            .child(div().flex_1().min_h_0().overflow_hidden().child(body))
    }
}
