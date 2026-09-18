//! Compact Click-to-Cycle Thinking Level Stepper.
//!
//! Renders an ultra-compact pill (`[ ✨ 3/5 ]`) inside the composer toolbar.
//! Clicking cycles through the model's supported thinking levels in a loop.

use std::rc::Rc;

use console_core::ThinkingLevel;
use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct ThinkingStepper {
    current_level: Option<ThinkingLevel>,
    supported_levels: Vec<ThinkingLevel>,
    on_cycle: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl ThinkingStepper {
    pub fn new(
        current_level: Option<ThinkingLevel>,
        supported_levels: Vec<ThinkingLevel>,
        on_cycle: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            current_level,
            supported_levels,
            on_cycle: Rc::new(on_cycle),
        }
    }
}

impl RenderOnce for ThinkingStepper {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        if self.supported_levels.is_empty() {
            return div().into_any_element();
        }

        let total_steps = self.supported_levels.len();
        let current = self
            .current_level
            .unwrap_or(self.supported_levels[0]);

        // Find index of current level in supported levels
        let current_index = self
            .supported_levels
            .iter()
            .position(|&lvl| lvl == current)
            .unwrap_or(0);

        let is_off = current == ThinkingLevel::None;

        // Display text: e.g. "0/6" if None, or "3/5"
        let step_label = if self.supported_levels.first() == Some(&ThinkingLevel::None) {
            if is_off {
                format!("0/{}", total_steps - 1)
            } else {
                format!("{}/{}", current_index, total_steps - 1)
            }
        } else {
            format!("{}/{}", current_index + 1, total_steps)
        };

        let tooltip_text = current.label().to_string();

        let icon_color = thinking_level_color(current, &theme);

        let text_color = if is_off {
            theme.text_ghost
        } else {
            theme.text_secondary
        };

        let on_cycle = self.on_cycle.clone();

        div()
            .id("thinking-stepper-chip")
            .h(px(22.0))
            .px(px(5.0))
            .rounded(px(5.0))
            .border_1()
            .border_color(theme.border_strong)
            .bg(theme.composer)
            .flex()
            .items_center()
            .gap(px(3.5))
            .cursor_default()
            .hover(|element| element.bg(theme.overlay).border_color(theme.border))
            .active(|element| element.opacity(0.85))
            .tooltip(Tooltip::text(tooltip_text))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                (on_cycle)(window, cx);
            })
            .child(app_icon(IconName::Brain, 11.0, icon_color))
            .child(
                div()
                    .text_size(px(10.5))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(text_color)
                    .child(SharedString::from(step_label)),
            )
            .into_any_element()
    }
}

/// Highlight colors for the brain icon across thinking levels.
fn thinking_level_color(level: ThinkingLevel, theme: &Theme) -> gpui::Hsla {
    match level {
        ThinkingLevel::None => theme.text_ghost,
        ThinkingLevel::Minimal => {
            if theme.is_dark {
                gpui::rgb(0x2DD4BF).into() // Mint / Teal
            } else {
                gpui::rgb(0x0D9488).into()
            }
        }
        ThinkingLevel::Low => {
            if theme.is_dark {
                gpui::rgb(0x38BDF8).into() // Sky Blue
            } else {
                gpui::rgb(0x0284C7).into()
            }
        }
        ThinkingLevel::Medium => {
            if theme.is_dark {
                gpui::rgb(0x818CF8).into() // Indigo
            } else {
                gpui::rgb(0x4F46E5).into()
            }
        }
        ThinkingLevel::High => {
            if theme.is_dark {
                gpui::rgb(0xC084FC).into() // Violet / Purple
            } else {
                gpui::rgb(0x9333EA).into()
            }
        }
        ThinkingLevel::XHigh => {
            if theme.is_dark {
                gpui::rgb(0xF472B6).into() // Fuchsia / Magenta
            } else {
                gpui::rgb(0xDB2777).into()
            }
        }
        ThinkingLevel::Max => {
            if theme.is_dark {
                gpui::rgb(0xFBBF24).into() // Warm Amber / Gold
            } else {
                gpui::rgb(0xD97706).into()
            }
        }
    }
}
