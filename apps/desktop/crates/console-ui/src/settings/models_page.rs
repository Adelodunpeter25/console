use crate::common::ModelRolePicker;
use crate::input::ComposerInput;
use console_core::{Model, ProviderCatalogEntry, SelectedModel};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, Entity, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::collections::HashMap;
use std::rc::Rc;

#[derive(IntoElement)]
pub struct ModelsPage {
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub models_by_provider: Rc<HashMap<String, Vec<Model>>>,
    pub default_input: Entity<ComposerInput>,
    pub plan_input: Entity<ComposerInput>,
    pub vision_input: Entity<ComposerInput>,
    pub smol_input: Entity<ComposerInput>,
    pub on_select:
        Rc<dyn Fn(String, String, String, Entity<ComposerInput>, &mut Window, &mut App) + 'static>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub saving: bool,
    pub error: Option<String>,
}

impl RenderOnce for ModelsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = crate::theme::Theme::current(cx);
        let make_selected = |input: &Entity<ComposerInput>| {
            let value = input.read(cx).content().trim().to_string();
            let (provider, model_id) = value
                .split_once('/')
                .map(|(p, m)| (p.to_string(), m.to_string()))
                .unwrap_or_default();
            (!provider.is_empty() && !model_id.is_empty())
                .then_some(SelectedModel { provider, model_id })
        };
        let on_save = self.on_save.clone();
        let picker = |label: &'static str,
                      description: &'static str,
                      role: &'static str,
                      input: Entity<ComposerInput>,
                      providers: Rc<Vec<ProviderCatalogEntry>>,
                      models: Rc<HashMap<String, Vec<Model>>>,
                      on_select: Rc<
            dyn Fn(String, String, String, Entity<ComposerInput>, &mut Window, &mut App) + 'static,
        >| {
            let selected = make_selected(&input);
            let callback = on_select.clone();
            ModelRolePicker {
                label,
                description,
                selected,
                providers,
                models_by_provider: models,
                on_select: Rc::new(move |provider, model, window, cx| {
                    callback(role.to_string(), provider, model, input.clone(), window, cx)
                }),
            }
        };
        div().flex().flex_col().gap(px(16.0))
            .child(div().flex().flex_col().gap(px(4.0)).child(div().text_size(px(16.0)).font_weight(gpui::FontWeight::SEMIBOLD).text_color(theme.text).child("Model roles")).child(div().text_size(px(12.5)).text_color(theme.text_secondary).child("Choose the model used for each harness role. Unset roles use the default model.")))
            .child(div().p(px(14.0)).rounded(px(8.0)).border_1().border_color(theme.border).bg(theme.surface).flex().flex_col().gap(px(14.0))
                .child(picker("Default", "Main coding and execution model", "default", self.default_input, self.providers.clone(), self.models_by_provider.clone(), self.on_select.clone()))
                .child(picker("Plan", "Architecture and planning model", "plan", self.plan_input, self.providers.clone(), self.models_by_provider.clone(), self.on_select.clone()))
                .child(picker("Vision", "Image and screenshot inspection model", "vision", self.vision_input, self.providers.clone(), self.models_by_provider.clone(), self.on_select.clone()))
                .child(picker("Smol", "Fast summaries and session titles model", "smol", self.smol_input, self.providers.clone(), self.models_by_provider.clone(), self.on_select.clone()))
                .child(div().id("save-model-roles").px(px(10.0)).py(px(6.0)).rounded(px(6.0)).bg(theme.accent).text_color(theme.on_inverse).cursor_pointer().on_click(move |_, window, cx| (on_save)(window, cx)).child("Save model roles"))
                .when(self.saving, |el| el.child(div().text_size(px(12.0)).text_color(theme.text_secondary).child("Saving…")))
                .when_some(self.error, |el, error| el.child(div().text_size(px(12.0)).text_color(theme.danger).child(error))))
    }
}
