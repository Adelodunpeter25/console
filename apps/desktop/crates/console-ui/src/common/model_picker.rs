use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use crate::input::ComposerInput;
use crate::primitives::{
    ContextMenuHandle, IconName, MenuAlign, app_icon, provider_app_icon, provider_color,
};
use crate::theme::Theme;
use console_core::{Model, ProviderCatalogEntry, SelectedModel};
use gpui::{
    App, ElementId, Entity, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum PickerTab {
    Favorites,
    Provider(String),
}

pub fn provider_svg_path(provider: &str) -> &'static str {
    let provider = provider.to_ascii_lowercase();
    match provider.as_str() {
        "google" | "antigravity" | "opencode" | "codex" | "openai" | "chatgpt" | "claude"
        | "anthropic" | "deepseek" | "grok" | "cursor" | "amp" | "pi" => {
            IconName::provider(&provider).path()
        }
        _ => IconName::Sparkle.path(),
    }
}

pub fn format_model_name(model_id: &str) -> String {
    model_id
        .split(['-', '_'])
        .map(|part| {
            if !part.is_empty() {
                let mut chars = part.chars();
                let first = chars.next().unwrap().to_uppercase().to_string();
                format!("{}{}", first, chars.as_str())
            } else {
                part.to_string()
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}

#[derive(Clone, IntoElement)]
pub struct ModelDropdownMenu {
    providers: Rc<Vec<ProviderCatalogEntry>>,
    selected: Option<SelectedModel>,
    active_tab: PickerTab,
    favorites: Rc<HashSet<String>>,
    /// Live-fetched models per provider, keyed by provider name. When an entry
    /// exists it overrides the static `models` in [`providers`]; until then the
    /// static list shows as a fallback so the picker is never blank. Mirrors
    /// the Electron picker's `providerModels` helper.
    models_by_provider: Rc<HashMap<String, Vec<Model>>>,
    /// Editable one-line filter rendered at the top of the model list. Owned
    /// by the app so the query survives the per-frame rebuild of this struct.
    search: Entity<ComposerInput>,
    /// Snapshot of [`search`] read in `view.rs` this frame; the list filters
    /// against it so typing re-renders without re-reading the entity here.
    search_query: String,
    on_select: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_tab: Rc<dyn Fn(PickerTab, &mut Window, &mut App) + 'static>,
    on_favorite: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
}

impl ModelDropdownMenu {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        providers: Rc<Vec<ProviderCatalogEntry>>,
        selected: Option<SelectedModel>,
        active_tab: PickerTab,
        favorites: Rc<HashSet<String>>,
        models_by_provider: Rc<HashMap<String, Vec<Model>>>,
        search: Entity<ComposerInput>,
        search_query: String,
        on_select: impl Fn(String, String, &mut Window, &mut App) + 'static,
        on_tab: impl Fn(PickerTab, &mut Window, &mut App) + 'static,
        on_favorite: impl Fn(String, String, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            providers,
            selected,
            active_tab,
            favorites,
            models_by_provider,
            search,
            search_query,
            on_select: Rc::new(on_select),
            on_tab: Rc::new(on_tab),
            on_favorite: Rc::new(on_favorite),
        }
    }
}

impl RenderOnce for ModelDropdownMenu {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let selected_model = self.selected;
        let active_tab = self.active_tab.clone();
        let on_tab = self.on_tab;
        let on_select = self.on_select;
        let on_favorite = self.on_favorite;
        let search = self.search.clone();
        // Lowercased once; the filter checks `model.id.contains(query)`, like
        // the Electron picker's `model.id.toLowerCase().includes(query)`.
        let query = self.search_query.trim().to_lowercase();

        // Resolve a provider's display models: live-fetched list if the cache
        // has an entry, otherwise the static catalog list. The picker is never
        // blank — static models show immediately and are silently replaced when
        // the live fetch lands.
        let live = &self.models_by_provider;

        let mut visible_models: Vec<(String, String, Model)> = match &active_tab {
            PickerTab::Favorites => {
                let favs: Vec<_> = self
                    .providers
                    .iter()
                    .flat_map(|p| {
                        let models = live.get(&p.name).map(|v| v.as_slice()).unwrap_or(&p.models);
                        models.iter().filter_map(|m| {
                            let key = format!("{}:{}", p.name, m.id);
                            if self.favorites.contains(&key) {
                                Some((p.name.clone(), p.display_name.clone(), m.clone()))
                            } else {
                                None
                            }
                        })
                    })
                    .collect();

                // Empty favourites shows the picker's empty state — no
                // fallback to another provider's models.
                favs
            }
            PickerTab::Provider(prov) => self
                .providers
                .iter()
                .find(|p| &p.name == prov)
                .map(|p| {
                    let models = live.get(&p.name).map(|v| v.as_slice()).unwrap_or(&p.models);
                    models
                        .iter()
                        .map(|m| (p.name.clone(), p.display_name.clone(), m.clone()))
                        .collect()
                })
                .unwrap_or_default(),
        };

        if !query.is_empty() {
            visible_models.retain(|(_, _, m)| m.id.to_lowercase().contains(&query));
        }

        div()
            .id("model-dropdown-card")
            .w(px(400.0))
            .h(px(350.0))
            .rounded(px(13.0))
            .bg(theme.canvas)
            .border_1()
            .border_color(theme.border)
            .shadow_xl()
            .overflow_hidden()
            .flex()
            .on_click(|_, _, cx| {
                cx.stop_propagation();
            })
            // Left Provider Icon Sidebar (50px width, matching Waku)
            .child(
                div()
                    .w(px(50.0))
                    .h_full()
                    .flex_none()
                    .flex()
                    .flex_col()
                    .items_center()
                    .gap(px(4.0))
                    .p(px(5.0))
                    .rounded_tl(px(12.0))
                    .rounded_bl(px(12.0))
                    .bg(theme.canvas)
                    .border_r_1()
                    .border_color(theme.border)
                    // Favorites Star Tab
                    .child({
                        let is_fav_active = active_tab == PickerTab::Favorites;
                        let on_t = on_tab.clone();
                        div()
                            .id("model-tab-favorites")
                            .w(px(38.0))
                            .h(px(38.0))
                            .rounded(px(7.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_default()
                            .when(is_fav_active, |s| s.bg(theme.overlay_strong))
                            .hover(|h| h.bg(theme.overlay))
                            .on_click(move |_, window, cx| {
                                (on_t)(PickerTab::Favorites, window, cx);
                            })
                            .child(app_icon(
                                IconName::Star,
                                16.0,
                                if is_fav_active {
                                    theme.text
                                } else {
                                    theme.text_tertiary
                                },
                            ))
                    })
                    // 1px Divider
                    .child(div().w(px(34.0)).h(px(1.0)).my(px(3.0)).bg(theme.border))
                    // Provider SVG icon buttons
                    .children(self.providers.iter().map(|provider| {
                        let is_prov_active = match &active_tab {
                            PickerTab::Provider(p) => p == &provider.name,
                            _ => false,
                        };
                        let prov_name = provider.name.clone();
                        let on_t = on_tab.clone();
                        div()
                            .id(ElementId::Name(format!("model-tab-{}", prov_name).into()))
                            .w(px(38.0))
                            .h(px(38.0))
                            .rounded(px(7.0))
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_default()
                            .when(is_prov_active, |s| s.bg(theme.overlay_strong))
                            .hover(|h| h.bg(theme.overlay))
                            .on_click({
                                let p = prov_name.clone();
                                move |_, window, cx| {
                                    (on_t)(PickerTab::Provider(p.clone()), window, cx);
                                }
                            })
                            .child(provider_app_icon(
                                &prov_name,
                                17.0,
                                if is_prov_active {
                                    theme.text
                                } else {
                                    theme.text_secondary
                                },
                            ))
                    })),
            )
            // Right Content: Search bar + Models List
            .child(
                div()
                    .id("model-dropdown-list")
                    .flex_1()
                    .h_full()
                    .flex()
                    .flex_col()
                    .overflow_hidden()
                    // Search filter input — a bordered shell with a leading
                    // magnifier icon and the focused one-line field. Mirrors
                    // the Electron picker's search row. The field entity is
                    // owned by the app, so focus and query persist across the
                    // per-frame rebuild of this dropdown.
                    .child(
                        div()
                            .id("model-search-shell")
                            .flex_none()
                            .mx(px(8.0))
                            .mt(px(8.0))
                            .h(px(30.0))
                            .px(px(8.0))
                            .rounded(px(7.0))
                            .border_1()
                            .border_color(theme.border_strong)
                            .bg(theme.inset)
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(app_icon(IconName::Search, 13.0, theme.text_ghost))
                            .child(div().flex_1().min_w_0().child(search)),
                    )
                    // Scrollable list of models
                    .child(
                        div()
                            .id("models-scroll-list")
                            .flex_1()
                            .w_full()
                            .p(px(8.0))
                            .overflow_y_scroll()
                            .flex()
                            .flex_col()
                            .gap_y(px(3.0))
                            .when(visible_models.is_empty(), |el| {
                                let is_favorites = active_tab == PickerTab::Favorites;
                                el.size_full()
                                    .flex()
                                    .flex_col()
                                    .items_center()
                                    .justify_center()
                                    .gap_y(px(4.0))
                                    .when(!is_favorites, |el| {
                                        el.child(app_icon(IconName::Star, 18.0, theme.text_ghost))
                                    })
                                    .child(
                                        div()
                                            .text_size(px(11.5))
                                            .text_color(theme.text_tertiary)
                                            .child(if is_favorites {
                                                "No Favourites"
                                            } else {
                                                "No models found"
                                            }),
                                    )
                            })
                            .children(visible_models.into_iter().map(
                                |(prov_name, prov_display, m)| {
                                    let is_selected = selected_model.as_ref().map_or(false, |s| {
                                        s.provider == prov_name && s.model_id == m.id
                                    });
                                    let is_fav =
                                        self.favorites.contains(&format!("{}:{}", prov_name, m.id));
                                    let prov_clone = prov_name.clone();
                                    let m_id_clone = m.id.clone();
                                    let on_sel = on_select.clone();
                                    let on_fav = on_favorite.clone();
                                    div()
                                        .id(ElementId::Name(
                                            format!("model-row-{}-{}", prov_name, m.id).into(),
                                        ))
                                        .h(px(54.0))
                                        .min_h(px(54.0))
                                        .flex_none()
                                        .px(px(12.0))
                                        .rounded(px(8.0))
                                        .flex()
                                        .items_center()
                                        .justify_between()
                                        .cursor_default()
                                        .when(is_selected, |s| s.bg(theme.overlay_strong))
                                        .when(!is_selected, |s| s.hover(|h| h.bg(theme.overlay)))
                                        .active(|a| a.opacity(0.85))
                                        .on_click({
                                            let p = prov_clone.clone();
                                            let mid = m_id_clone.clone();
                                            let on_s = on_sel.clone();
                                            move |_, window, cx| {
                                                (on_s)(p.clone(), mid.clone(), window, cx);
                                            }
                                        })
                                        .child(
                                            div()
                                                .flex_1()
                                                .min_w_0()
                                                .flex()
                                                .flex_col()
                                                .gap_y(px(3.0))
                                                // Model Name
                                                .child(
                                                    div()
                                                        .truncate()
                                                        .text_size(px(13.0))
                                                        .font_weight(FontWeight::SEMIBOLD)
                                                        .text_color(theme.text)
                                                        .child(format_model_name(&m.id)),
                                                )
                                                // Subtitle: Provider SVG + Context Tokens
                                                .child(
                                                    div()
                                                        .flex()
                                                        .items_center()
                                                        .gap_x(px(5.0))
                                                        .child(provider_app_icon(
                                                            &prov_name,
                                                            10.5,
                                                            theme.text_tertiary,
                                                        ))
                                                        .child(
                                                            div()
                                                                .truncate()
                                                                .text_size(px(11.0))
                                                                .text_color(theme.text_tertiary)
                                                                .child(format!(
                                                                    "{} · {}k context",
                                                                    prov_display,
                                                                    m.context_window / 1000
                                                                )),
                                                        ),
                                                ),
                                        )
                                        // Favorite Star
                                        .child(
                                            div()
                                                .id(ElementId::Name(
                                                    format!("fav-{}-{}", prov_clone, m_id_clone)
                                                        .into(),
                                                ))
                                                .p(px(4.0))
                                                .rounded(px(4.0))
                                                .cursor_default()
                                                .hover(|s| s.bg(theme.raised))
                                                .on_click({
                                                    let p = prov_clone;
                                                    let mid = m_id_clone;
                                                    move |_, window, cx| {
                                                        cx.stop_propagation();
                                                        (on_fav)(
                                                            p.clone(),
                                                            mid.clone(),
                                                            window,
                                                            cx,
                                                        );
                                                    }
                                                })
                                                .child(app_icon(
                                                    if is_fav {
                                                        IconName::StarFilled
                                                    } else {
                                                        IconName::Star
                                                    },
                                                    13.0,
                                                    if is_fav {
                                                        theme.favorite
                                                    } else {
                                                        theme.text_ghost
                                                    },
                                                )),
                                        )
                                },
                            )),
                    ),
            )
    }
}

/// Compact provider/model selector used by settings role configuration.
#[derive(Clone, IntoElement)]
pub struct ModelRolePicker {
    pub label: &'static str,
    pub description: &'static str,
    pub selected: Option<SelectedModel>,
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub models_by_provider: Rc<HashMap<String, Vec<Model>>>,
    pub active_tab: PickerTab,
    pub favorites: Rc<HashSet<String>>,
    pub menu: ContextMenuHandle,
    pub search: Entity<ComposerInput>,
    pub search_query: String,
    pub on_select: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    pub on_clear: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    pub on_tab: Rc<dyn Fn(PickerTab, &mut Window, &mut App) + 'static>,
    pub on_favorite: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
}

impl RenderOnce for ModelRolePicker {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let selected = self.selected;
        let selected_text = selected
            .as_ref()
            .map(|model| {
                format!(
                    "{} / {}",
                    model.provider,
                    format_model_name(&model.model_id)
                )
            })
            .unwrap_or_else(|| "Uses Default".to_string());
        let selected_provider = selected.as_ref().map(|s| s.provider.clone());
        let providers = self.providers;
        let models_by_provider = self.models_by_provider;
        let active_tab = self.active_tab;
        let favorites = self.favorites;
        let on_select = self.on_select;
        let on_clear = self.on_clear;
        let on_tab = self.on_tab;
        let on_favorite = self.on_favorite;
        let menu = self.menu;
        let search = self.search;
        let query = self.search_query.trim().to_lowercase();

        let trigger = div()
            .id(ElementId::Name(
                format!("model-role-trigger-{}", self.label).into(),
            ))
            .w_full()
            .p(px(9.0))
            .rounded(px(7.0))
            .border_1()
            .border_color(theme.border)
            .bg(theme.inset)
            .flex()
            .items_center()
            .gap(px(7.0))
            .cursor_pointer()
            .when_some(selected_provider.as_deref(), |el, prov| {
                el.child(provider_app_icon(prov, 14.0, provider_color(&theme, prov)))
            })
            .child(
                div()
                    .flex_1()
                    .text_size(px(12.5))
                    .text_color(if selected.is_some() {
                        theme.text
                    } else {
                        theme.text_secondary
                    })
                    .child(selected_text),
            )
            .child(
                div()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("▾"),
            );

        let panel = {
            let providers = providers.clone();
            let models_by_provider = models_by_provider.clone();
            let active_tab = active_tab.clone();
            let search = search.clone();
            let on_tab = on_tab.clone();
            let on_select = on_select.clone();
            let on_clear = on_clear.clone();
            let on_favorite = on_favorite.clone();
            let favorites = favorites.clone();
            let selected_model = selected.clone();
            let query = query.clone();

            move |handle: &ContextMenuHandle, _window: &mut Window, _cx: &mut App| {
                let close_handle = handle.clone();
                let live = &models_by_provider;
                let mut visible_models: Vec<(String, String, Model)> = match &active_tab {
                    PickerTab::Favorites => {
                        let favs: Vec<_> = providers
                            .iter()
                            .flat_map(|p| {
                                let models = live.get(&p.name).map(|v| v.as_slice()).unwrap_or(&p.models);
                                models.iter().filter_map(|m| {
                                    let key = format!("{}:{}", p.name, m.id);
                                    if favorites.contains(&key) {
                                        Some((p.name.clone(), p.display_name.clone(), m.clone()))
                                    } else {
                                        None
                                    }
                                })
                            })
                            .collect();
                        favs
                    }
                    PickerTab::Provider(prov) => providers
                        .iter()
                        .find(|p| &p.name == prov)
                        .map(|p| {
                            let models = live.get(&p.name).map(|v| v.as_slice()).unwrap_or(&p.models);
                            models
                                .iter()
                                .map(|m| (p.name.clone(), p.display_name.clone(), m.clone()))
                                .collect()
                        })
                        .unwrap_or_default(),
                };

                if !query.is_empty() {
                    visible_models.retain(|(_, _, m)| m.id.to_lowercase().contains(&query));
                }

                div()
                    .id("model-role-dropdown-card")
                    .w(px(460.0))
                    .h(px(360.0))
                    .rounded(px(12.0))
                    .bg(theme.canvas)
                    .border_1()
                    .border_color(theme.border)
                    .shadow_xl()
                    .overflow_hidden()
                    .flex()
                    .on_click(|_, _, cx| {
                        cx.stop_propagation();
                    })
                    // Left Provider Icon Sidebar
                    .child(
                        div()
                            .w(px(48.0))
                            .h_full()
                            .flex_none()
                            .flex()
                            .flex_col()
                            .items_center()
                            .gap(px(4.0))
                            .p(px(5.0))
                            .bg(theme.canvas)
                            .border_r_1()
                            .border_color(theme.border)
                            // Favorites Tab
                            .child({
                                let is_fav = active_tab == PickerTab::Favorites;
                                let on_t = on_tab.clone();
                                div()
                                    .id("role-tab-fav")
                                    .size(px(36.0))
                                    .rounded(px(6.0))
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .cursor_pointer()
                                    .when(is_fav, |s| s.bg(theme.overlay_strong))
                                    .hover(|h| h.bg(theme.overlay))
                                    .on_click(move |_, window, cx| {
                                        (on_t)(PickerTab::Favorites, window, cx);
                                    })
                                    .child(app_icon(
                                        IconName::Star,
                                        15.0,
                                        if is_fav {
                                            theme.text
                                        } else {
                                            theme.text_tertiary
                                        },
                                    ))
                            })
                            .child(div().w(px(32.0)).h(px(1.0)).my(px(2.0)).bg(theme.border))
                            // Provider icons
                            .children(providers.iter().map(|prov| {
                                let is_active = match &active_tab {
                                    PickerTab::Provider(p) => p == &prov.name,
                                    _ => false,
                                };
                                let on_t = on_tab.clone();
                                let prov_name = prov.name.clone();
                                div()
                                    .id(ElementId::Name(format!("role-tab-{}", prov.name).into()))
                                    .size(px(36.0))
                                    .rounded(px(6.0))
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .cursor_pointer()
                                    .when(is_active, |s| s.bg(theme.overlay_strong))
                                    .hover(|h| h.bg(theme.overlay))
                                    .on_click(move |_, window, cx| {
                                        (on_t)(PickerTab::Provider(prov_name.clone()), window, cx);
                                    })
                                    .child(provider_app_icon(
                                        &prov.name,
                                        18.0,
                                        provider_color(&theme, &prov.name),
                                    ))
                            })),
                    )
                    // Right Content Area
                    .child(
                        div()
                            .flex_1()
                            .h_full()
                            .flex()
                            .flex_col()
                            .min_w_0()
                            // Search box
                            .child(
                                div()
                                    .h(px(34.0))
                                    .px(px(8.0))
                                    .border_b_1()
                                    .border_color(theme.border)
                                    .flex()
                                    .items_center()
                                    .child(search.clone()),
                            )
                            // Scrollable list
                            .child(
                                div()
                                    .id("role-model-list-scroll")
                                    .flex_1()
                                    .overflow_y_scroll()
                                    .p(px(6.0))
                                    .flex()
                                    .flex_col()
                                    .gap(px(2.0))
                                    // Optional "Use Default" button
                                    .when_some(on_clear.clone(), |list, on_c| {
                                        let h = close_handle.clone();
                                        list.child(
                                            div()
                                                .id("role-clear-option")
                                                .px(px(8.0))
                                                .py(px(6.0))
                                                .rounded(px(5.0))
                                                .cursor_pointer()
                                                .hover(|el| el.bg(theme.overlay))
                                                .on_click(move |_, window, cx| {
                                                    h.close(window, cx);
                                                    (on_c)(window, cx);
                                                })
                                                .flex()
                                                .items_center()
                                                .gap(px(7.0))
                                                .child(app_icon(
                                                    IconName::Restart,
                                                    13.0,
                                                    theme.text_tertiary,
                                                ))
                                                .child(
                                                    div()
                                                        .flex_1()
                                                        .text_size(px(12.0))
                                                        .font_weight(FontWeight::MEDIUM)
                                                        .text_color(theme.text_secondary)
                                                        .child("Use Default Model"),
                                                ),
                                        )
                                    })
                                    .children(visible_models.into_iter().map(|(prov_id, prov_disp, model)| {
                                        let is_sel = selected_model.as_ref().is_some_and(|s| {
                                            s.provider == prov_id && s.model_id == model.id
                                        });
                                        let on_s = on_select.clone();
                                        let on_f = on_favorite.clone();
                                        let p_id = prov_id.clone();
                                        let m_id = model.id.clone();
                                        let h = close_handle.clone();
                                        let fav_key = format!("{}:{}", prov_id, model.id);
                                        let is_fav = favorites.contains(&fav_key);
                                        let star_id = format!("role-fav-btn-{}-{}", prov_id, model.id);

                                        div()
                                            .id(ElementId::Name(format!("role-m-{}-{}", prov_id, model.id).into()))
                                            .px(px(8.0))
                                            .py(px(6.0))
                                            .rounded(px(5.0))
                                            .cursor_pointer()
                                            .when(is_sel, |el| el.bg(theme.overlay_strong))
                                            .hover(|el| el.bg(theme.overlay))
                                            .on_click(move |_, window, cx| {
                                                h.close(window, cx);
                                                (on_s)(p_id.clone(), m_id.clone(), window, cx);
                                            })
                                            .flex()
                                            .items_center()
                                            .gap(px(8.0))
                                            .child(provider_app_icon(&prov_id, 14.0, provider_color(&theme, &prov_id)))
                                            .child(
                                                div()
                                                    .flex_1()
                                                    .flex()
                                                    .flex_col()
                                                    .min_w_0()
                                                    .child(
                                                        div()
                                                            .text_size(px(12.0))
                                                            .text_color(theme.text)
                                                            .truncate()
                                                            .child(format_model_name(&model.id)),
                                                    )
                                                    .child(
                                                        div()
                                                            .text_size(px(10.5))
                                                            .text_color(theme.text_ghost)
                                                            .child(prov_disp),
                                                    ),
                                            )
                                            .child(
                                                div()
                                                    .text_size(px(10.5))
                                                    .text_color(theme.text_ghost)
                                                    .child(if model.context_window >= 1_000_000 {
                                                        format!("{}M", model.context_window / 1_000_000)
                                                    } else {
                                                        format!("{}k", model.context_window / 1000)
                                                    }),
                                            )
                                            .child({
                                                let p = prov_id.clone();
                                                let m = model.id.clone();
                                                div()
                                                    .id(ElementId::Name(star_id.into()))
                                                    .p(px(2.0))
                                                    .cursor_pointer()
                                                    .hover(|el| el.bg(theme.overlay))
                                                    .on_click(move |_, window, cx| {
                                                        cx.stop_propagation();
                                                        (on_f)(p.clone(), m.clone(), window, cx);
                                                    })
                                                    .child(app_icon(
                                                        IconName::Star,
                                                        13.0,
                                                        if is_fav {
                                                            theme.warning
                                                        } else {
                                                            theme.text_ghost
                                                        },
                                                    ))
                                            })
                                    })),
                            ),
                    )
                    .into_any_element()
            }
        };

        div()
            .flex()
            .flex_col()
            .gap(px(5.0))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(1.0))
                    .child(
                        div()
                            .text_size(px(13.0))
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(theme.text)
                            .child(self.label),
                    )
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(theme.text_secondary)
                            .child(self.description),
                    ),
            )
            .child(crate::primitives::popover(
                trigger,
                &menu,
                MenuAlign::BelowLeft,
                panel,
            ))
    }
}
