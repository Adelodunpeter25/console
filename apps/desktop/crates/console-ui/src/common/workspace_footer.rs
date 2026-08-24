//! The bottom strip under the composer: project and branch selectors.
//!
//! Mirrors Waku's `render_workspace_footer`: a slim row of chips below the
//! composer card. The project chip lists the backend's known projects plus
//! "New project…" (native folder picker) and "No project"; the branch chip
//! lists the selected project's Git branches and marks the checked-out one.
//! The parent owns all backend calls — this component only paints state and
//! forwards callbacks.

use std::rc::Rc;

use console_core::{GitBranchInfo, ProjectInfo};
use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px,
};

use crate::primitives::{
    ContextMenuHandle, IconName, MenuAlign, MenuChip, MenuItem, dropdown_menu,
};
use crate::theme::Theme;

#[derive(IntoElement)]
pub struct WorkspaceFooter {
    projects: Rc<Vec<ProjectInfo>>,
    selected_project_id: Option<String>,
    branches: Rc<Vec<GitBranchInfo>>,
    branch_loaded: bool,
    is_git_repository: bool,
    branch_pending: bool,
    project_locked: bool,
    project_menu: ContextMenuHandle,
    branch_menu: ContextMenuHandle,
    on_select_project: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_new_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_no_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_select_branch: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl WorkspaceFooter {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        projects: Rc<Vec<ProjectInfo>>,
        selected_project_id: Option<String>,
        branches: Rc<Vec<GitBranchInfo>>,
        branch_loaded: bool,
        is_git_repository: bool,
        branch_pending: bool,
        project_menu: ContextMenuHandle,
        branch_menu: ContextMenuHandle,
        on_select_project: impl Fn(String, &mut Window, &mut App) + 'static,
        on_new_project: impl Fn(&mut Window, &mut App) + 'static,
        on_no_project: impl Fn(&mut Window, &mut App) + 'static,
        on_select_branch: impl Fn(String, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            projects,
            selected_project_id,
            branches,
            branch_loaded,
            is_git_repository,
            branch_pending,
            project_locked: false,
            project_menu,
            branch_menu,
            on_select_project: Rc::new(on_select_project),
            on_new_project: Rc::new(on_new_project),
            on_no_project: Rc::new(on_no_project),
            on_select_branch: Rc::new(on_select_branch),
        }
    }

    /// Lock the project picker — the session already has messages so the
    /// working directory can no longer change. Shows a lock icon and disables
    /// the chip.
    pub fn project_locked(mut self, locked: bool) -> Self {
        self.project_locked = locked;
        self
    }
}

impl RenderOnce for WorkspaceFooter {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let selected_project = self
            .selected_project_id
            .as_deref()
            .and_then(|id| self.projects.iter().find(|project| project.id == id));
        let project_label = selected_project
            .map(|project| project.name.clone())
            .unwrap_or_else(|| "No project".to_owned());
        let current_branch = self
            .branches
            .iter()
            .find(|branch| branch.current)
            .map(|branch| branch.name.clone());

        let on_select_project = self.on_select_project.clone();
        let on_new_project = self.on_new_project.clone();
        let on_no_project = self.on_no_project.clone();
        let project_locked = self.project_locked;
        let project_selector = {
            let projects = self.projects.clone();
            let selected_id = self.selected_project_id.clone();
            let on_select_project = on_select_project.clone();
            let on_new_project = on_new_project.clone();
            let on_no_project = on_no_project.clone();
            let project_chip = MenuChip::new("workspace-project")
                .icon(
                    if project_locked {
                        IconName::Lock
                    } else {
                        IconName::Folder
                    }
                    .path(),
                    theme.text_tertiary,
                )
                .label(project_label)
                .caret(false)
                .disabled(project_locked)
                .selected(!project_locked && self.project_menu.is_open())
                .max_w(px(190.0));
            if project_locked {
                project_chip.into_any_element()
            } else {
                dropdown_menu(
                    project_chip,
                "workspace-project-menu",
                &self.project_menu,
                MenuAlign::AboveLeft,
                move |_| {
                    let mut items = projects
                        .iter()
                        .map(|project| {
                            let project_id = project.id.clone();
                            let on_select_project = on_select_project.clone();
                            let selected = Some(project.id.as_str()) == selected_id.as_deref();
                            MenuItem::new(project.name.clone(), move |window, cx| {
                                (on_select_project)(project_id.clone(), window, cx);
                            })
                            .selected(selected)
                        })
                        .collect::<Vec<_>>();
                    if !items.is_empty() {
                        items.push(MenuItem::Separator);
                    }
                    let on_new_project = on_new_project.clone();
                    let on_no_project = on_no_project.clone();
                    items.push(
                        MenuItem::new("New project…", move |window, cx| {
                            (on_new_project)(window, cx);
                        })
                        .icon(IconName::FolderNew.path()),
                    );
                    items.push(
                        MenuItem::new("No project", move |window, cx| {
                            (on_no_project)(window, cx);
                        })
                        .icon(IconName::X.path())
                        .selected(selected_id.is_none()),
                    );
                    items
                },
            )
            }
        };

        // The branch chip is a plain label until there is a project to read a
        // repository from — opening a picker with nothing to pick is noise.
        let branch_enabled = selected_project.is_some()
            && self.branch_loaded
            && self.is_git_repository
            && !self.branch_pending
            && !self.branches.is_empty();
        let branch_label = if self.branch_pending {
            "Switching…".to_owned()
        } else if selected_project.is_none() {
            "No branch".to_owned()
        } else if !self.branch_loaded {
            "Loading…".to_owned()
        } else if !self.is_git_repository {
            "No Git repository".to_owned()
        } else if self.branches.is_empty() || current_branch.is_none() {
            "No branch".to_owned()
        } else {
            current_branch.unwrap_or_default()
        };
        let branch_trigger = MenuChip::new("workspace-branch")
            .icon(IconName::GitBranch.path(), theme.text_tertiary)
            .label(branch_label)
            .caret(false)
            .disabled(!branch_enabled)
            .selected(branch_enabled && self.branch_menu.is_open())
            .max_w(px(200.0));
        let branch_selector = if branch_enabled {
            let branches = self.branches.clone();
            let on_select_branch = self.on_select_branch.clone();
            dropdown_menu(
                branch_trigger,
                "workspace-branch-menu",
                &self.branch_menu,
                MenuAlign::AboveLeft,
                move |_| {
                    let mut items = branches
                        .iter()
                        .map(|branch| {
                            let branch_name = branch.name.clone();
                            let on_select_branch = on_select_branch.clone();
                            MenuItem::new(branch.name.clone(), move |window, cx| {
                                (on_select_branch)(branch_name.clone(), window, cx);
                            })
                            .selected(branch.current)
                        })
                        .collect::<Vec<_>>();
                    if items.is_empty() {
                        items.push(MenuItem::new("No branches", |_, _| {}).disabled(true));
                    }
                    items
                },
            )
        } else {
            branch_trigger.into_any_element()
        };

        div()
            .id("workspace-footer")
            .w_full()
            .px(px(20.0))
            .pt(px(4.0))
            .pb(px(8.0))
            .bg(theme.composer)
            .child(
                div()
                    .w_full()
                    .max_w(px(768.0))
                    .mx_auto()
                    .h(px(28.0))
                    .pl(px(10.0))
                    .pr(px(10.0))
                    .flex()
                    .items_center()
                    .gap(px(2.0))
                    .text_size(px(11.0))
                    .line_height(px(14.0))
                    .child(project_selector)
                    .child(branch_selector)
                    .child(div().flex_1()),
            )
    }
}
