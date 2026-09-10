use gpui::prelude::FluentBuilder;
use gpui::{App, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce, Styled, Window, div, px};
use std::rc::Rc;
use crate::input::ComposerInput;
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct ModelsPage {
    pub default_input: Entity<ComposerInput>,
    pub plan_input: Entity<ComposerInput>,
    pub vision_input: Entity<ComposerInput>,
    pub smol_input: Entity<ComposerInput>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub saving: bool,
    pub error: Option<String>,
}

impl RenderOnce for ModelsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_save = self.on_save.clone();
        div().flex().flex_col().gap(px(16.0)).child(
            div().flex().flex_col().gap(px(4.0)).child(
                div().text_size(px(16.0)).font_weight(gpui::FontWeight::SEMIBOLD).text_color(theme.text).child("Model roles")
            ).child(
                div().text_size(px(12.5)).text_color(theme.text_secondary).child("Choose which model handles everyday work, planning, vision, and lightweight background tasks.")
            )
        ).child(
            div().p(px(14.0)).rounded(px(8.0)).border_1().border_color(theme.border).bg(theme.surface).flex().flex_col().gap(px(12.0))
                .child(Self::field("Default", "Main coding and execution model", self.default_input))
                .child(Self::field("Plan", "Architecture and planning model", self.plan_input))
                .child(Self::field("Vision", "Image and screenshot inspection model", self.vision_input))
                .child(Self::field("Smol", "Fast summaries and session titles model", self.smol_input))
                .child(div().id("save-model-roles").px(px(10.0)).py(px(6.0)).rounded(px(6.0)).bg(theme.accent).text_color(theme.on_inverse).cursor_pointer().on_mouse_down(MouseButton::Left, move |_event, window, cx| (on_save)(window, cx)).child("Save model roles"))
                .when(self.saving, |el| el.child(div().text_size(px(12.0)).text_color(theme.text_secondary).child("Saving…")))
                .when_some(self.error, |el, error| el.child(div().text_size(px(12.0)).text_color(theme.danger).child(error)))
        )
    }
}

impl ModelsPage {
    fn field(label: &'static str, description: &'static str, input: Entity<ComposerInput>) -> impl IntoElement {
        div().flex().flex_col().gap(px(4.0))
            .child(div().text_size(px(12.5)).font_weight(gpui::FontWeight::MEDIUM).child(label))
            .child(div().text_size(px(11.5)).child(description))
            .child(div().p(px(6.0)).rounded(px(6.0)).border_1().child(input))
    }
}
