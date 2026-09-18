//! The usage meter: a clickable circular gauge showing the active provider's
//! quota status at a glance. Clicking opens a panel with detailed limit breakdowns.

use std::rc::Rc;

use console_core::UsageReport;
use gpui::{
    App, IntoElement, InteractiveElement, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div,
};

use crate::primitives::{IconName, MenuChip};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct UsageMeter {
    provider: String,
    usage_report: Option<UsageReport>,
    on_click: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl UsageMeter {
    pub fn new(
        provider: String,
        usage_report: Option<UsageReport>,
        on_click: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            provider,
            usage_report,
            on_click: Rc::new(on_click),
        }
    }
}

impl RenderOnce for UsageMeter {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        // Determine status color from primary limit if available
        let status_color = self
            .usage_report
            .as_ref()
            .and_then(|report| report.limits.first())
            .and_then(|limit| limit.status)
            .map(|status| match status {
                console_core::UsageStatus::Ok => theme.text_secondary,
                console_core::UsageStatus::Warning => theme.warning,
                console_core::UsageStatus::Exhausted => theme.danger,
                console_core::UsageStatus::Unknown => theme.text_tertiary,
            })
            .unwrap_or(theme.text_tertiary);

        let label = match self.usage_report.as_ref() {
            Some(report) => report
                .limits
                .first()
                .and_then(|limit| limit.window.as_ref())
                .and_then(|window| window.reset_label.as_ref())
                .map(|label| label.clone())
                .unwrap_or_else(|| "Quota".to_string()),
            None => "Quota".to_string(),
        };

        let on_click = self.on_click.clone();
        let meter = MenuChip::new("usage-meter")
            .icon(IconName::Zap.path(), status_color)
            .label(label)
            .caret(false)
            .flex_none();

        div()
            .on_mouse_down(gpui::MouseButton::Left, move |_, window, cx| {
                (on_click)(window, cx);
            })
            .child(meter)
    }
}
