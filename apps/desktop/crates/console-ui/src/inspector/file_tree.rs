//! Hierarchical Project Directory Tree Viewer for the Right Inspector.

use std::collections::{HashMap, HashSet};
use std::path::Path;
use std::rc::Rc;

use console_core::types::{FsTreeEntry, GitFileEntry};
use gpui::Hsla;
use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::primitives::file_icon;
use crate::primitives::file_icons::file_icon_for_name;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

/// A file's Git state, reduced to what the tree paints. Variants are ordered
/// by how loudly a folder should report them: a folder holding a deleted file
/// and a new one shows the deletion.
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub enum GitBadge {
    Untracked,
    Added,
    Renamed,
    Modified,
    Deleted,
    Conflict,
}

impl GitBadge {
    /// Maps the server's status code (`M`/`A`/`D`/`R`/`U`/`?`); anything else
    /// gets no badge rather than a guess.
    pub fn from_code(code: &str) -> Option<Self> {
        Some(match code {
            "?" => Self::Untracked,
            "A" => Self::Added,
            "R" | "C" => Self::Renamed,
            "M" => Self::Modified,
            "D" => Self::Deleted,
            "U" => Self::Conflict,
            _ => return None,
        })
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::Untracked => "U",
            Self::Added => "A",
            Self::Renamed => "R",
            Self::Modified => "M",
            Self::Deleted => "D",
            Self::Conflict => "!",
        }
    }

    pub fn color(self, theme: &Theme) -> Hsla {
        match self {
            Self::Untracked | Self::Added => theme.success,
            Self::Renamed => theme.text_tertiary,
            Self::Modified => theme.warning,
            Self::Deleted | Self::Conflict => theme.danger,
        }
    }
}

/// Git badge per absolute path, with every changed file's ancestors carrying
/// the strongest badge beneath them (VS Code's folder tinting).
///
/// Built once per status update, not per frame. The walk up from a file stops
/// at the first ancestor already showing an equal or stronger badge: that
/// ancestor's own ancestors were raised to at least that level when it was
/// set, so the rest of the chain cannot change. A thousand files in one
/// folder therefore cost one walk, not a thousand.
pub fn aggregate_git_badges(changes: &[GitFileEntry]) -> HashMap<String, GitBadge> {
    let mut badges: HashMap<String, GitBadge> = HashMap::with_capacity(changes.len() * 2);
    for change in changes {
        let Some(badge) = GitBadge::from_code(&change.status) else {
            continue;
        };
        raise(&mut badges, &change.path, badge);
        for ancestor in Path::new(&change.path).ancestors().skip(1) {
            if ancestor.file_name().is_none() {
                break;
            }
            let Some(path) = ancestor.to_str() else {
                break;
            };
            if badges.get(path).is_some_and(|existing| *existing >= badge) {
                break;
            }
            raise(&mut badges, path, badge);
        }
    }
    badges
}

fn raise(badges: &mut HashMap<String, GitBadge>, path: &str, badge: GitBadge) {
    match badges.get_mut(path) {
        Some(existing) => *existing = (*existing).max(badge),
        None => {
            badges.insert(path.to_owned(), badge);
        }
    }
}

#[derive(Clone, Debug)]
pub struct FileTreeNode {
    pub name: String,
    pub path: String,
    pub is_dir: bool,
    pub children: Vec<FileTreeNode>,
}

#[derive(IntoElement)]
pub struct FileTreeView {
    tree: Rc<Vec<FileTreeNode>>,
    expanded_folders: HashSet<String>,
    selected_path: Option<String>,
    search_query: String,
    on_toggle_folder: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    git_badges: Rc<HashMap<String, GitBadge>>,
    on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
}

impl FileTreeView {
    pub fn new(
        tree: Rc<Vec<FileTreeNode>>,
        expanded_folders: HashSet<String>,
        selected_path: Option<String>,
        search_query: String,
        on_toggle_folder: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        git_badges: Rc<HashMap<String, GitBadge>>,
        on_select_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            tree,
            expanded_folders,
            selected_path,
            search_query,
            on_toggle_folder,
            git_badges,
            on_select_file,
        }
    }

    fn render_node(
        node: &FileTreeNode,
        depth: usize,
        expanded_folders: &HashSet<String>,
        selected_path: &Option<String>,
        on_toggle_folder: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_select_file: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        theme: &Theme,
        search_query: &str,
        git_badges: &HashMap<String, GitBadge>,
    ) -> Vec<gpui::AnyElement> {
        let mut elements = Vec::new();
        let query_lower = search_query.to_lowercase();
        let matches_filter =
            search_query.is_empty() || node.name.to_lowercase().contains(&query_lower);

        let path = node.path.clone();
        let is_expanded = expanded_folders.contains(&node.path);
        let is_selected = selected_path.as_deref() == Some(&node.path);

        if matches_filter || node.is_dir {
            let badge = git_badges.get(&node.path).copied();
            let status_color = badge.map(|badge| badge.color(theme));
            let on_toggle = on_toggle_folder.clone();
            let on_select = on_select_file.clone();
            let node_path = path.clone();
            let is_dir = node.is_dir;

            let row =
                div()
                    .id(format!("file-tree-item-{}", path))
                    .flex()
                    .items_center()
                    .h(px(26.0))
                    .w_full()
                    .px(px(8.0))
                    .pl(px(8.0 + (depth as f32 * 14.0)))
                    .rounded(px(4.0))
                    .cursor_pointer()
                    .hover(|s| s.bg(theme.overlay))
                    .when(is_selected, |s| s.bg(theme.overlay_strong))
                    .on_click(move |_, window, cx| {
                        if is_dir {
                            (on_toggle)(node_path.clone(), window, cx);
                        } else {
                            (on_select)(node_path.clone(), window, cx);
                        }
                    })
                    .child(
                        div()
                            .w(px(16.0))
                            .flex_none()
                            .flex()
                            .items_center()
                            .justify_center()
                            .when(node.is_dir, |el| {
                                el.child(if is_expanded {
                                    app_icon(IconName::ChevronDown, 12.0, theme.text_tertiary)
                                } else {
                                    app_icon(IconName::ChevronRight, 12.0, theme.text_tertiary)
                                })
                            }),
                    )
                    .child(div().mr(px(6.0)).flex_none().flex().items_center().child(
                        if node.is_dir {
                            if is_expanded {
                                app_icon(
                                    IconName::FolderOpen,
                                    14.0,
                                    status_color.unwrap_or(theme.text_secondary),
                                )
                                .into_any_element()
                            } else {
                                app_icon(
                                    IconName::Folder,
                                    14.0,
                                    status_color.unwrap_or(theme.text_secondary),
                                )
                                .into_any_element()
                            }
                        } else {
                            file_icon(file_icon_for_name(&node.name), 14.0).into_any_element()
                        },
                    ))
                    .child(
                        div()
                            .flex_1()
                            .truncate()
                            .text_size(px(12.0))
                            .text_color(if is_selected {
                                theme.text
                            } else {
                                status_color.unwrap_or(theme.text_secondary)
                            })
                            .child(node.name.clone()),
                    )
                    .when_some(badge, |el, badge| {
                        el.child(
                            div()
                                .w(px(14.0))
                                .flex_none()
                                .text_size(px(11.0))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(badge.color(theme))
                                .child(badge.label()),
                        )
                    });

            elements.push(row.into_any_element());
        }

        if node.is_dir && (is_expanded || !search_query.is_empty()) {
            for child in &node.children {
                let mut child_els = Self::render_node(
                    child,
                    depth + 1,
                    expanded_folders,
                    selected_path,
                    on_toggle_folder,
                    on_select_file,
                    theme,
                    search_query,
                    git_badges,
                );
                elements.append(&mut child_els);
            }
        }

        elements
    }
}

pub fn build_tree_from_entries(entries: &[FsTreeEntry]) -> Vec<FileTreeNode> {
    if entries.is_empty() {
        return Vec::new();
    }

    let mut all_paths = HashSet::new();
    let mut children_by_parent: HashMap<String, Vec<FsTreeEntry>> = HashMap::new();

    for entry in entries {
        all_paths.insert(entry.path.clone());
        if let Some(parent) = Path::new(&entry.path).parent() {
            children_by_parent
                .entry(parent.to_string_lossy().to_string())
                .or_default()
                .push(entry.clone());
        }
    }

    // Sort every children list: directories first, then case-insensitive alphabetical
    for list in children_by_parent.values_mut() {
        list.sort_by(|a, b| {
            b.is_dir
                .cmp(&a.is_dir)
                .then_with(|| a.name.to_lowercase().cmp(&b.name.to_lowercase()))
        });
    }

    // Root entries are those whose parent directory is not in all_paths
    let mut root_entries: Vec<FsTreeEntry> = entries
        .iter()
        .filter(|e| {
            Path::new(&e.path)
                .parent()
                .map(|p| !all_paths.contains(&p.to_string_lossy().to_string()))
                .unwrap_or(true)
        })
        .cloned()
        .collect();

    root_entries.sort_by(|a, b| {
        b.is_dir
            .cmp(&a.is_dir)
            .then_with(|| a.name.to_lowercase().cmp(&b.name.to_lowercase()))
    });

    fn assemble(
        entry: FsTreeEntry,
        children_by_parent: &HashMap<String, Vec<FsTreeEntry>>,
    ) -> FileTreeNode {
        let mut children = Vec::new();
        if entry.is_dir {
            if let Some(child_entries) = children_by_parent.get(&entry.path) {
                for child in child_entries {
                    children.push(assemble(child.clone(), children_by_parent));
                }
            }
        }
        FileTreeNode {
            name: entry.name,
            path: entry.path,
            is_dir: entry.is_dir,
            children,
        }
    }

    root_entries
        .into_iter()
        .map(|r| assemble(r, &children_by_parent))
        .collect()
}

impl RenderOnce for FileTreeView {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let mut children = Vec::new();

        for node in self.tree.as_ref() {
            let mut els = Self::render_node(
                node,
                0,
                &self.expanded_folders,
                &self.selected_path,
                &self.on_toggle_folder,
                &self.on_select_file,
                &theme,
                &self.search_query,
                &self.git_badges,
            );
            children.append(&mut els);
        }

        div()
            .id("file-tree-container")
            .flex_1()
            .w_full()
            .h_full()
            .min_h_0()
            .overflow_y_scroll()
            .p(px(6.0))
            .child(if children.is_empty() {
                div()
                    .flex_1()
                    .flex()
                    .items_center()
                    .justify_center()
                    .py(px(24.0))
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child(if self.search_query.is_empty() {
                        "No files found"
                    } else {
                        "No matching files"
                    })
                    .into_any_element()
            } else {
                div()
                    .flex()
                    .flex_col()
                    .children(children)
                    .into_any_element()
            })
    }
}
