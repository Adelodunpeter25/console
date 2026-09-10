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
    pub menus: [crate::primitives::ContextMenuHandle; 4],
    pub searches: [Entity<ComposerInput>; 4],
    pub on_select:
        Rc<dyn Fn(String, String, String, Entity<ComposerInput>, &mut Window, &mut App) + 'static>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub saving: bool,
    pub error: Option<String>,
}

impl RenderOnce for ModelsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = crate::theme::Theme::current(cx);
        let on_save = self.on_save.clone();
        let picker = |label: &'static str,
                      description: &'static str,
                      input: Entity<ComposerInput>,
                      menu: crate::primitives::ContextMenuHandle,
                       search: Entity<ComposerInput>| {
            let value = input.read(cx).content().trim().to_string();
            let selected = value
                .split_once('/')
                .map(|(provider, model_id)| SelectedModel {
                    provider: provider.to_string(),
                    model_id: model_id.to_string(),
                });
            let callback = self.on_select.clone();
            ModelRolePicker {
                label,
                description,
                selected,
                providers: self.providers.clone(),
                models_by_provider: self.models_by_provider.clone(),
                menu,
                search,
                search_query: String::new(),
                on_select: Rc::new(move |provider, model, window, cx| {
                    callback(
                        label.to_lowercase(),
                        provider,
                        model,
                        input.clone(),
                        window,
                        cx,
                    )
                }),
            }
        };
        let [default_menu, plan_menu, vision_menu, smol_menu] = self.menus;
        let [default_search, plan_search, vision_search, smol_search] = self.searches;
        div().flex().flex_col().gap(px(16.0))
            .child(div().flex().flex_col().gap(px(4.0)).child(div().text_size(px(16.0)).font_weight(gpui::FontWeight::SEMIBOLD).text_color(theme.text).child("Model roles")).child(div().text_size(px(12.5)).text_color(theme.text_secondary).child("Choose the model used for each harness role. Unset roles use the default model.")))
            .child(div().p(px(14.0)).rounded(px(8.0)).border_1().border_color(theme.border).bg(theme.surface).flex().flex_col().gap(px(14.0))
                .child(picker("Default", "Main coding and execution model", self.default_input, default_menu, default_search))
                .child(picker("Plan", "Architecture and planning model", self.plan_input, plan_menu, plan_search))
                .child(picker("Vision", "Image and screenshot inspection model", self.vision_input, vision_menu, vision_search))
                .child(picker("Smol", "Fast summaries and session titles model", self.smol_input, smol_menu, smol_search))
                .child(div().id("save-model-roles").px(px(10.0)).py(px(6.0)).rounded(px(6.0)).bg(theme.accent).text_color(theme.on_inverse).cursor_pointer().on_click(move |_, window, cx| (on_save)(window, cx)).child("Save model roles"))
                .when(self.saving, |el| el.child(div().text_size(px(12.0)).text_color(theme.text_secondary).child("Saving…")))
                .when_some(self.error, |el, error| el.child(div().text_size(px(12.0)).text_color(theme.danger).child(error))))
    }
}
