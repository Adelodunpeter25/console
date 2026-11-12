//! The project and branch menu builders shared by the two pickers — the
//! docked footer's project dropdown and the floating composer card's. Like
//! [`crate::common::github_menu`], the parent owns all backend calls and
//! callbacks; these only paint rows and forward taps, so the two surfaces
//! cannot drift apart.

use std::rc::Rc;

use console_core::{GitBranchInfo, ProjectInfo};
use gpui::{App, Window};

use crate::primitives::{IconName, MenuItem};

/// One selectable mode row above the branch list — the floating card's
/// "Project branch" / "New worktree" choices. The docked footer passes none:
/// its branch menu only switches the checked-out branch.
pub struct BranchModeRow {
    pub label: &'static str,
    pub selected: bool,
    #[allow(clippy::type_complexity)]
    pub on_choose: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

/// Rows only the docked footer offers: "New project…" and "No project". The
/// floating card creates sessions from existing projects, so it passes none.
#[allow(clippy::type_complexity)]
pub struct ProjectMenuExtras {
    pub on_new_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_no_project: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    /// Whether "No project" is the current selection.
    pub none_selected: bool,
}

/// The project list, optionally followed by the footer's management rows.
/// `with_icons` is the one cosmetic difference between the surfaces — the
/// card's rows carry folder icons, the footer's keep the plain look.
#[allow(clippy::type_complexity)]
pub fn project_menu_section(
    projects: Rc<Vec<ProjectInfo>>,
    selected_id: Option<String>,
    with_icons: bool,
    on_select: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    extras: Option<&ProjectMenuExtras>,
) -> Vec<MenuItem> {
    let mut items: Vec<MenuItem> = projects
        .iter()
        .map(|project| {
            let id = project.id.clone();
            let on_select = on_select.clone();
            let mut item = MenuItem::new(project.name.clone(), move |window, cx| {
                (on_select)(id.clone(), window, cx);
            })
            .selected(selected_id.as_deref() == Some(project.id.as_str()));
            if with_icons {
                item = item.icon(IconName::Folder.path());
            }
            item
        })
        .collect();

    let Some(extras) = extras else {
        return items;
    };
    if !items.is_empty() {
        items.push(MenuItem::Separator);
    }
    let on_new_project = extras.on_new_project.clone();
    items.push(
        MenuItem::new("New project…", move |window, cx| {
            (on_new_project)(window, cx);
        })
        .icon(IconName::FolderNew.path()),
    );
    let on_no_project = extras.on_no_project.clone();
    items.push(
        MenuItem::new("No project", move |window, cx| {
            (on_no_project)(window, cx);
        })
        .icon(IconName::X.path())
        .selected(extras.none_selected),
    );
    items
}

/// The branch list. The card prepends its mode rows ("Project branch" /
/// "New worktree" — a worktree launch is a choice, not a checkout); the
/// footer instead appends a "New worktree…" action row via `new_worktree`,
/// and renders "No branches" when the list is empty. `is_selected` says
/// which branch row carries the check: the footer compares against the
/// branch actually checked out, the card against the chosen [BranchChoice]
/// mode.
#[allow(clippy::type_complexity)]
pub fn branch_menu_section(
    branches: Rc<Vec<GitBranchInfo>>,
    is_selected: &Rc<dyn Fn(&GitBranchInfo) -> bool + 'static>,
    on_select: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    with_icons: bool,
    modes: &[BranchModeRow],
    new_worktree: Option<(&Rc<dyn Fn(&mut Window, &mut App) + 'static>, &str)>,
) -> Vec<MenuItem> {
    let mut items: Vec<MenuItem> = modes
        .iter()
        .map(|mode| {
            let on_choose = mode.on_choose.clone();
            MenuItem::new(mode.label, move |window, cx| {
                (on_choose)(window, cx);
            })
            .selected(mode.selected)
        })
        .collect();

    if !items.is_empty() && !branches.is_empty() {
        items.push(MenuItem::Separator);
    }
    if items.is_empty() && branches.is_empty() {
        items.push(MenuItem::new("No branches", |_, _| {}).disabled(true));
    }
    for info in branches.iter() {
        // Remote-tracking rows (origin/…) are worktree base choices, not
        // checkouts — `git switch` cannot check them out in place, so the
        // footer and the card's FromBranch mode never offer them.
        if info.remote {
            continue;
        }
        let name = info.name.clone();
        let on_select = on_select.clone();
        let selected = is_selected(info);
        let mut item = MenuItem::new(name.clone(), move |window, cx| {
            (on_select)(name.clone(), window, cx);
        })
        .selected(selected);
        if with_icons {
            item = item.icon(IconName::GitBranch.path());
        }
        items.push(item);
    }
    if let Some((on_new_worktree, label)) = new_worktree {
        if !items.is_empty() {
            items.push(MenuItem::Separator);
        }
        let on_new_worktree = on_new_worktree.clone();
        items.push(
            MenuItem::new(label, move |window, cx| {
                (on_new_worktree)(window, cx);
            })
            .icon(IconName::GitBranch.path()),
        );
    }
    items
}
