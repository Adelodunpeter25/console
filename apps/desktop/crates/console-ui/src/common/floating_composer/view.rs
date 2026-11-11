//! The floating composer's card: a scrim, a bordered surface, a header of
//! selectors, the prompt input, and a footer row that ends in `Create ↵`.
//!
//! Purely presentational. All selection lives on the owning
//! [`super::FloatingComposer`]; this file only paints a snapshot and reports
//! clicks back.

use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use console_core::{
    ApprovalMode, GitBranchInfo, GithubRepo, ImageAttachment, Model, ProjectInfo,
    ProviderCatalogEntry, SelectedModel, ThinkingLevel,
};
use gpui::{
    App, Entity, ExternalPaths, FontWeight, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use super::BranchChoice;
use crate::common::composer_view::composer_attachment_row;
use crate::common::plan_mode_stripes;
use crate::common::{
    AutocompleteConfirm, AutocompleteDismiss, AutocompleteNext, AutocompletePrevious,
    AutocompleteView, ModelDropdownMenu, PickerTab, format_model_name, provider_svg_path,
};
use crate::input::ComposerInput;
use crate::primitives::{
    ContextMenuHandle, IconName, MenuAlign, MenuChip, MenuItem, app_icon, dropdown_menu, popover,
};
use crate::theme::Theme;

use super::super::github_menu::github_menu_section;
use super::super::picker_menus::{
    BranchModeRow, ProjectMenuExtras, branch_menu_section, project_menu_section,
};

/// One frame of state to paint.
pub struct Data {
    pub projects: Rc<Vec<ProjectInfo>>,
    pub selected_project_id: Option<String>,
    /// The connected GitHub account's repos for the project menu's GitHub
    /// section, with load/error flags (None error + None repos = not
    /// connected). Mirrored from app state on every frame, so a refresh from
    /// any menu reaches an open card.
    pub github_repos: Option<Rc<Vec<GithubRepo>>>,
    pub github_repos_loading: bool,
    pub github_repos_error: Option<String>,
    pub branches: Rc<Vec<GitBranchInfo>>,
    pub branch: BranchChoice,
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub models_by_provider: Rc<HashMap<String, Vec<Model>>>,
    pub favorites: Rc<HashSet<String>>,
    pub selected_model: Option<SelectedModel>,
    pub picker_tab: PickerTab,
    pub model_search_query: String,
    /// The repo a GitHub tap is cloning right now, if any. Painted as a
    /// "Cloning <name>…" line because the card's menu closes on tap.
    pub repo_cloning: Option<String>,
    pub thinking_level: Option<ThinkingLevel>,
    pub supported_thinking_levels: Vec<ThinkingLevel>,
    pub approval_mode: ApprovalMode,
    pub attachments: Rc<Vec<ImageAttachment>>,
    pub autocomplete: Option<AutocompleteView>,
    pub submitting: bool,
    pub error: Option<String>,
}

pub struct FloatingComposerView {
    input: Entity<ComposerInput>,
    project_menu: ContextMenuHandle,
    branch_menu: ContextMenuHandle,
    model_menu: ContextMenuHandle,
    thinking_menu: ContextMenuHandle,
    model_dropdown: ModelDropdownMenu,
    data: Data,
    on_dismiss: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_choose_project: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_no_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_new_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_choose_github_repo: Rc<dyn Fn(GithubRepo, &mut Window, &mut App) + 'static>,
    on_refresh_github_repos: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_connect_github: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_choose_branch: Rc<dyn Fn(BranchChoice, &mut Window, &mut App) + 'static>,
    on_choose_model: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_choose_thinking: Rc<dyn Fn(ThinkingLevel, &mut Window, &mut App) + 'static>,
    on_submit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_pick_image: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_drop_files: Rc<dyn Fn(&ExternalPaths, &mut Window, &mut App) + 'static>,
    on_remove_attachment: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_preview_attachment: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_autocomplete_next: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_autocomplete_previous: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_autocomplete_confirm: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_autocomplete_dismiss: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_cancel: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl FloatingComposerView {
    pub fn new(
        input: Entity<ComposerInput>,
        project_menu: ContextMenuHandle,
        branch_menu: ContextMenuHandle,
        model_menu: ContextMenuHandle,
        thinking_menu: ContextMenuHandle,
        model_dropdown: ModelDropdownMenu,
    ) -> Self {
        Self {
            input,
            project_menu,
            branch_menu,
            model_menu,
            thinking_menu,
            model_dropdown,
            data: Data {
                projects: Rc::new(Vec::new()),
                selected_project_id: None,
                github_repos: None,
                github_repos_loading: false,
                github_repos_error: None,
                branches: Rc::new(Vec::new()),
                branch: BranchChoice::default(),
                providers: Rc::new(Vec::new()),
                models_by_provider: Rc::new(HashMap::new()),
                favorites: Rc::new(HashSet::new()),
                selected_model: None,
                picker_tab: PickerTab::Favorites,
                model_search_query: String::new(),
                repo_cloning: None,
                thinking_level: None,
                supported_thinking_levels: Vec::new(),
                approval_mode: ApprovalMode::default(),
                attachments: Rc::new(Vec::new()),
                autocomplete: None,
                submitting: false,
                error: None,
            },
            on_dismiss: Rc::new(|_, _| {}),
            on_choose_project: Rc::new(|_, _, _| {}),
            on_no_project: Rc::new(|_, _| {}),
            on_new_project: Rc::new(|_, _| {}),
            on_choose_github_repo: Rc::new(|_, _, _| {}),
            on_refresh_github_repos: Rc::new(|_, _| {}),
            on_connect_github: Rc::new(|_, _| {}),
            on_choose_branch: Rc::new(|_, _, _| {}),
            on_choose_model: Rc::new(|_, _, _, _| {}),
            on_choose_thinking: Rc::new(|_, _, _| {}),
            on_submit: Rc::new(|_, _, _| {}),
            on_pick_image: Rc::new(|_, _| {}),
            on_drop_files: Rc::new(|_, _, _| {}),
            on_remove_attachment: Rc::new(|_, _, _| {}),
            on_preview_attachment: Rc::new(|_, _, _| {}),
            on_autocomplete_next: Rc::new(|_, _| {}),
            on_autocomplete_previous: Rc::new(|_, _| {}),
            on_autocomplete_confirm: Rc::new(|_, _| {}),
            on_autocomplete_dismiss: Rc::new(|_, _| {}),
            on_cancel: Rc::new(|_, _| {}),
        }
    }

    pub fn data(mut self, data: Data) -> Self {
        self.data = data;
        self
    }

    /// Every view callback takes `&mut Window` because that is what gpui's
    /// click and menu-item handlers hand us; the ones the entity registers on
    /// the app side drop it, since the card keeps no window-scoped state.
    pub fn on_dismiss(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_dismiss = Rc::new(f);
        self
    }

    pub fn on_choose_project(
        mut self,
        f: impl Fn(String, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_choose_project = Rc::new(f);
        self
    }

    pub fn on_no_project(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_no_project = Rc::new(f);
        self
    }

    pub fn on_new_project(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_new_project = Rc::new(f);
        self
    }

    /// A GitHub repo row was clicked: clone it (server-side) and select the
    /// resulting project for this card's submit.
    pub fn on_choose_github_repo(
        mut self,
        f: impl Fn(GithubRepo, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_choose_github_repo = Rc::new(f);
        self
    }

    pub fn on_refresh_github_repos(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_refresh_github_repos = Rc::new(f);
        self
    }

    pub fn on_connect_github(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_connect_github = Rc::new(f);
        self
    }

    pub fn on_choose_branch(
        mut self,
        f: impl Fn(BranchChoice, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_choose_branch = Rc::new(f);
        self
    }

    pub fn on_choose_model(
        mut self,
        f: impl Fn(String, String, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_choose_model = Rc::new(f);
        self
    }

    pub fn on_choose_thinking(
        mut self,
        f: impl Fn(ThinkingLevel, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_choose_thinking = Rc::new(f);
        self
    }

    pub fn on_submit(mut self, f: impl Fn(String, &mut Window, &mut App) + 'static) -> Self {
        self.on_submit = Rc::new(f);
        self
    }

    pub fn on_pick_image(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_pick_image = Rc::new(f);
        self
    }

    pub fn on_drop_files(
        mut self,
        f: impl Fn(&ExternalPaths, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_drop_files = Rc::new(f);
        self
    }

    pub fn on_remove_attachment(
        mut self,
        f: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_remove_attachment = Rc::new(f);
        self
    }

    pub fn on_preview_attachment(
        mut self,
        f: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_preview_attachment = Rc::new(f);
        self
    }

    pub fn on_autocomplete_next(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_autocomplete_next = Rc::new(f);
        self
    }

    pub fn on_autocomplete_previous(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_autocomplete_previous = Rc::new(f);
        self
    }

    pub fn on_autocomplete_confirm(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_autocomplete_confirm = Rc::new(f);
        self
    }

    pub fn on_autocomplete_dismiss(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_autocomplete_dismiss = Rc::new(f);
        self
    }

    pub fn on_cancel(mut self, f: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_cancel = Rc::new(f);
        self
    }

    pub fn render(self, _window: &mut Window, cx: &mut App) -> gpui::AnyElement {
        let theme = Theme::current(cx);
        // Cloned out of `self` so the menu closures below can be `'static`
        // without borrowing the view.
        let data = self.data;
        let plan_mode = data.approval_mode == ApprovalMode::PlanMode;

        // Pulled out before the menu closures below, which move the `Rc`s.
        let selected_project_id = data.selected_project_id.clone();
        let has_project = selected_project_id.is_some();
        let submitting = data.submitting;
        let error = data.error.clone();
        let repo_cloning = data.repo_cloning.clone();

        // ---- header: project + branch selectors on the left ----
        // Both answer "where does this work happen", so they sit together
        // above the prompt; the footer keeps the behavioural choices.
        let (project_label, project_is_none) = match data
            .projects
            .iter()
            .find(|p| Some(&p.id) == selected_project_id.as_ref())
        {
            Some(p) => (p.name.clone(), false),
            None => match selected_project_id.as_ref() {
                Some(_) => ("Select project".to_owned(), false),
                None => ("No project".to_owned(), true),
            },
        };
        let project_trigger = MenuChip::new("floating-project-chip")
            .height(px(28.0))
            .icon(
                if project_is_none {
                    IconName::X
                } else {
                    IconName::Folder
                }
                .path(),
                theme.text_tertiary,
            )
            .label(project_label);
        let on_choose_project = self.on_choose_project.clone();
        let on_no_project = self.on_no_project.clone();
        let on_new_project = self.on_new_project.clone();
        let on_choose_github_repo = self.on_choose_github_repo.clone();
        let on_refresh_github_repos = self.on_refresh_github_repos.clone();
        let on_connect_github = self.on_connect_github.clone();
        let project_control: gpui::AnyElement = dropdown_menu(
            project_trigger,
            "floating-project-menu",
            &self.project_menu,
            MenuAlign::AboveLeft,
            move |_| {
                // The GitHub section needs no local projects to make sense —
                // cloning from GitHub is how the list grows in the first
                // place — so the menu always offers it.
                let mut items = project_menu_section(
                    data.projects.clone(),
                    selected_project_id.clone(),
                    true,
                    &on_choose_project,
                    Some(&ProjectMenuExtras {
                        on_new_project: on_new_project.clone(),
                        on_no_project: on_no_project.clone(),
                        none_selected: selected_project_id.is_none(),
                    }),
                );
                let github_repos = data.github_repos.clone();
                let github_repos_loading = data.github_repos_loading;
                let github_repos_error = data.github_repos_error.clone();
                items.extend(github_menu_section(
                    github_repos,
                    github_repos_loading,
                    github_repos_error.is_some(),
                    &on_choose_github_repo,
                    &on_refresh_github_repos,
                    &on_connect_github,
                ));
                items
            },
        );

        // ---- branch selector, painted in the header beside the project ----
        // Same treatment as the project chip beside it: no outline, no filled
        // background, same height — the header reads as one control group.
        let branch_trigger = MenuChip::new("floating-branch-chip")
            .height(px(28.0))
            .icon(IconName::GitBranch.path(), theme.text_tertiary)
            .label(data.branch.label())
            // The header shares its row with the project chip, so a long
            // branch name is capped rather than pushing the card wider.
            .max_w(px(220.0));
        let on_choose_branch = self.on_choose_branch.clone();
        let branch_control = dropdown_menu(
            branch_trigger,
            "floating-branch-menu",
            &self.branch_menu,
            MenuAlign::AboveLeft,
            move |_| {
                let current = data.branch.clone();
                let current_for_check = current.clone();
                let is_choice: Rc<dyn Fn(&GitBranchInfo) -> bool> =
                    Rc::new(move |info: &GitBranchInfo| {
                        current_for_check == BranchChoice::FromBranch(info.name.clone())
                    });
                let on_pick_branch: Rc<dyn Fn(String, &mut Window, &mut App)> = {
                    let on_choose_branch = on_choose_branch.clone();
                    Rc::new(move |name, window, cx| {
                        on_choose_branch(BranchChoice::FromBranch(name), window, cx);
                    })
                };
                let mode = |label: &'static str, choice: BranchChoice, selected: bool| {
                    let on_choose_branch = on_choose_branch.clone();
                    BranchModeRow {
                        label,
                        selected,
                        on_choose: Rc::new(move |window, cx| {
                            on_choose_branch(choice.clone(), window, cx);
                        }),
                    }
                };
                branch_menu_section(
                    data.branches.clone(),
                    &is_choice,
                    &on_pick_branch,
                    true,
                    &[
                        mode(
                            "Project branch",
                            BranchChoice::Project,
                            current == BranchChoice::Project,
                        ),
                        mode(
                            "New worktree",
                            BranchChoice::NewWorktree,
                            current == BranchChoice::NewWorktree,
                        ),
                    ],
                    None,
                )
            },
        );

        let (model_label, model_provider) = data
            .selected_model
            .as_ref()
            .map(|m| (format_model_name(&m.model_id), m.provider.clone()))
            .unwrap_or_else(|| ("Select Model".to_owned(), "antigravity".to_owned()));
        let model_is_open = self.model_menu.is_open();
        let model_trigger = MenuChip::new("floating-model-chip")
            .height(px(26.0))
            .outlined()
            // Matches the approval chip beside it, so the footer controls sit
            // on the same surface instead of one looking recessed.
            .background(theme.composer)
            .selected(model_is_open)
            .icon(provider_svg_path(&model_provider), theme.text)
            .label(model_label);
        let model_dropdown = self.model_dropdown.clone();
        let model_control = popover(
            model_trigger,
            &self.model_menu,
            MenuAlign::AboveLeft,
            move |_handle, _window, _cx| model_dropdown.clone().into_any_element(),
        );

        let thinking_control = if data.supported_thinking_levels.is_empty() {
            div().into_any_element()
        } else {
            let level = data
                .thinking_level
                .unwrap_or(data.supported_thinking_levels[0]);
            let thinking_is_open = self.thinking_menu.is_open();
            let trigger = MenuChip::new("floating-thinking-chip")
                .height(px(26.0))
                .outlined()
                // Same fill as the model and approval chips, so the whole
                // footer row sits on one surface.
                .background(theme.composer)
                .selected(thinking_is_open)
                .label(level.label());
            let on_choose_thinking = self.on_choose_thinking.clone();
            let levels = data.supported_thinking_levels.clone();
            dropdown_menu(
                trigger,
                "floating-thinking-menu",
                &self.thinking_menu,
                MenuAlign::AboveLeft,
                move |_| {
                    levels
                        .iter()
                        .map(|candidate| {
                            let on_choose_thinking = on_choose_thinking.clone();
                            // `levels` is a snapshot the closure can't borrow
                            // from, so copy the Copy level out per row.
                            let value = *candidate;
                            MenuItem::new(value.label(), move |window, cx| {
                                on_choose_thinking(value, window, cx);
                            })
                            .selected(value == level)
                        })
                        .collect()
                },
            )
        };

        // ---- Create button ----
        // The prompt is read at click time rather than captured here, so the
        // button reflects the live input even though this struct is built
        // per frame.
        let can_submit = !submitting && !self.input.read(cx).content().trim().is_empty();
        let on_submit = self.on_submit.clone();
        // Captured so the click reads the live text instead of a per-frame copy.
        let submit_input = self.input.clone();
        let label_color = if can_submit {
            theme.canvas
        } else {
            theme.text_tertiary
        };
        let hint_color = if can_submit {
            theme.canvas.opacity(0.6)
        } else {
            theme.text_ghost
        };

        // The click handlers are attached unconditionally so the button keeps a
        // stable element id across frames; `can_submit` only gates the paint,
        // and a click while disabled is a no-op because the text is empty or a
        // request is already in flight.
        let create_button = div()
            .id("floating-create")
            .h(px(28.0))
            .px(px(12.0))
            .rounded(px(7.0))
            .flex()
            .items_center()
            .gap(px(6.0))
            .cursor_default()
            .when(can_submit, |el| el.bg(theme.text))
            .when(!can_submit, |el| el.opacity(0.4))
            // Stop the mousedown so pressing the button doesn't read as a
            // backdrop click and dismiss the card before the click lands.
            .on_mouse_down(
                gpui::MouseButton::Left,
                move |_, _window: &mut Window, cx: &mut App| cx.stop_propagation(),
            )
            .on_click(move |_, window, cx| {
                if !can_submit {
                    return;
                }
                let prompt = submit_input.read(cx).content().to_string();
                on_submit(prompt, window, cx);
            })
            .child(
                div()
                    .text_size(px(12.5))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(label_color)
                    .child("Create"),
            )
            .child(div().text_size(px(11.0)).text_color(hint_color).child("↵"));

        // ---- staged images ----
        let attachments = (!data.attachments.is_empty()).then(|| {
            composer_attachment_row(
                data.attachments.clone(),
                self.on_remove_attachment.clone(),
                self.on_preview_attachment.clone(),
                theme,
            )
        });

        // ---- attach button ----
        let on_pick = self.on_pick_image.clone();
        let attach_button = div()
            .id("floating-attach")
            .p(px(5.0))
            .rounded(px(6.0))
            .cursor_default()
            .hover(|style| style.bg(theme.overlay))
            .on_click(move |_, window, cx| {
                on_pick(window, cx);
            })
            .child(app_icon(IconName::Plus, 13.0, theme.text_tertiary));

        // ---- assemble ----
        let on_dismiss = self.on_dismiss.clone();
        let on_drop_files = self.on_drop_files.clone();
        let input = self.input.clone();
        let autocomplete = data.autocomplete.clone();
        let autocomplete_open = autocomplete.is_some();
        let autocomplete_anchor = autocomplete.as_ref().map(AutocompleteView::anchor_cell);
        let on_autocomplete_next = self.on_autocomplete_next.clone();
        let on_autocomplete_previous = self.on_autocomplete_previous.clone();
        let on_autocomplete_confirm = self.on_autocomplete_confirm.clone();
        let on_autocomplete_dismiss = self.on_autocomplete_dismiss.clone();
        let on_cancel = self.on_cancel.clone();

        div()
            .absolute()
            .inset_0()
            .bg(gpui::black().opacity(0.35))
            .flex()
            .justify_center()
            // Nudged left of center so it clears the eye-line of the sidebar.
            .pr(px(64.0))
            .items_center()
            // Above true center reads better under the popup.
            .pb(px(160.0))
            .on_mouse_down(gpui::MouseButton::Left, move |_, window, cx| {
                on_dismiss(window, cx);
            })
            .child(
                div()
                    .occlude()
                    .key_context(super::CONTEXT)
                    .track_focus(&self.input.read(cx).focus())
                    .on_action(
                        move |_: &super::Cancel, window: &mut Window, cx: &mut App| {
                            on_cancel(window, cx)
                        },
                    )
                    .w(px(560.0))
                    .rounded(px(13.0))
                    .bg(theme.composer)
                    .border_1()
                    .border_color(theme.border_strong)
                    .shadow_lg()
                    .overflow_hidden()
                    // Dropping image files stages them as chips; the card
                    // highlights while a drag hovers over it.
                    .drag_over::<ExternalPaths>(move |style, _, _, _| {
                        style
                            .bg(theme.composer.opacity(1.0))
                            .border_color(theme.accent)
                    })
                    .on_drop(move |paths: &ExternalPaths, window, cx| {
                        on_drop_files(paths, window, cx);
                    })
                    .on_mouse_down(gpui::MouseButton::Left, |_, _, cx| {
                        cx.stop_propagation();
                    })
                    .when(plan_mode, |el| el.child(plan_mode_stripes(theme)))
                    // header
                    .child(
                        div()
                            .h(px(44.0))
                            .px(px(12.0))
                            .flex()
                            .items_center()
                            .gap(px(8.0))
                            .border_b_1()
                            .border_color(theme.border)
                            .child(project_control)
                            .when(has_project, |el| el.child(branch_control))
                            .child(div().flex_1()),
                    )
                    // staged images
                    .when_some(attachments, |el, attachments| el.child(attachments))
                    // prompt
                    .child(
                        div()
                            .w_full()
                            .min_h(px(72.0))
                            .px(px(14.0))
                            .pt(px(10.0))
                            .pb(px(6.0))
                            .relative()
                            // The suggestion popup is anchored to the caret via
                            // a bounds probe, so it needs this element to be
                            // the positioning parent.
                            .when_some(autocomplete_anchor, |el, anchor_bounds| {
                                el.child(AutocompleteView::bounds_probe(anchor_bounds))
                            })
                            .when(autocomplete_open, |el| {
                                el.key_context(crate::common::AUTOCOMPLETE_CONTEXT)
                                    .on_action(move |_: &AutocompleteNext, window, cx| {
                                        on_autocomplete_next(window, cx);
                                        cx.stop_propagation();
                                    })
                                    .on_action(move |_: &AutocompletePrevious, window, cx| {
                                        on_autocomplete_previous(window, cx);
                                        cx.stop_propagation();
                                    })
                                    .on_action(move |_: &AutocompleteConfirm, window, cx| {
                                        on_autocomplete_confirm(window, cx);
                                        cx.stop_propagation();
                                    })
                                    .on_action(move |_: &AutocompleteDismiss, window, cx| {
                                        on_autocomplete_dismiss(window, cx);
                                        cx.stop_propagation();
                                    })
                            })
                            .child(input.clone()),
                    )
                    // suggestion popup
                    .when_some(autocomplete, |el, popup| el.child(popup))
                    // cloning — the GitHub section's tap starts a clone that
                    // outlives the menu, so this keeps it visible in the card.
                    .when_some(repo_cloning, |el, name| {
                        el.child(
                            div()
                                .px(px(14.0))
                                .pb(px(6.0))
                                .text_size(px(11.5))
                                .text_color(theme.text_secondary)
                                .child(format!("Cloning {name}…")),
                        )
                    })
                    // error — a failed launch keeps the card open with the
                    // prompt intact, so this is the retry affordance.
                    .when_some(error, |el, message| {
                        el.child(
                            div()
                                .px(px(14.0))
                                .pb(px(6.0))
                                .text_size(px(11.5))
                                .text_color(theme.danger)
                                .child(message),
                        )
                    })
                    // footer
                    .child(
                        div()
                            .px(px(10.0))
                            .pt(px(6.0))
                            .pb(px(10.0))
                            .flex()
                            .items_center()
                            .gap(px(6.0))
                            .child(attach_button)
                            .child(model_control)
                            .child(thinking_control)
                            .child(div().flex_1())
                            // Spinner replaces nothing — it sits just before
                            // Create, which reads as the in-flight action.
                            .when(submitting, |el| {
                                el.child(crate::primitives::motion::spin(app_icon(
                                    IconName::LoaderCircle,
                                    13.0,
                                    theme.text_secondary,
                                )))
                            })
                            .child(create_button),
                    ),
            )
            .into_any_element()
    }
}
