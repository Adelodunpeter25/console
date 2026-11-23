//! Hierarchical Project Directory Tree Viewer for the Right Inspector.

use std::collections::{HashMap, HashSet};
use std::path::Path;
use std::rc::Rc;

use console_core::types::{FsTreeEntry, GitFileEntry};
use gpui::Hsla;
use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, ScrollHandle,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::primitives::file_icon;
use crate::primitives::file_icons::file_icon_for_name;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;

// The badge rules live in console-shared, which the Android client reaches
// over JNI, so a file cannot be green on one client and grey on the other.
// Only the palette is the desktop's own.
pub use console_shared::git_badges::{GitBadge, GitBadgeRole};

/// The color a badge is drawn in: the desktop's theme, by the role the shared
/// rules give it.
pub fn git_badge_color(badge: GitBadge, theme: &Theme) -> Hsla {
    match badge.role() {
        GitBadgeRole::Success => theme.success,
        GitBadgeRole::Warning => theme.warning,
        GitBadgeRole::Danger => theme.danger,
        GitBadgeRole::Muted => theme.text_tertiary,
    }
}

/// Git badge per absolute path, with every changed file's ancestors carrying
/// the strongest badge beneath them (VS Code's folder tinting).
///
/// A thin adapter over the shared rules: the desktop's own type becomes the
/// shared one, and the rules run unchanged.
pub fn aggregate_git_badges(changes: &[GitFileEntry]) -> HashMap<String, GitBadge> {
    let files: Vec<console_shared::git_badges::ChangedFile<'_>> = changes
        .iter()
        .map(|change| console_shared::git_badges::ChangedFile {
            path: &change.path,
            status: &change.status,
        })
        .collect();
    console_shared::git_badges::aggregate_git_badges(&files)
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
    on_open_browser: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    scroll_handle: Option<ScrollHandle>,
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
            on_open_browser: None,
            scroll_handle: None,
        }
    }

    pub fn on_open_browser(
        mut self,
        handler: Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
    ) -> Self {
        self.on_open_browser = handler;
        self
    }

    pub fn track_scroll(mut self, scroll_handle: ScrollHandle) -> Self {
        self.scroll_handle = Some(scroll_handle);
        self
    }

    fn render_node(
        node: &FileTreeNode,
        depth: usize,
        expanded_folders: &HashSet<String>,
        selected_path: &Option<String>,
        on_toggle_folder: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_select_file: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_open_browser: &Option<Rc<dyn Fn(String, &mut Window, &mut App) + 'static>>,
        theme: &Theme,
        search_query: &str,
        git_badges: &HashMap<String, GitBadge>,
        window: &mut Window,
        cx: &mut App,
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
            let status_color = badge.map(|badge| git_badge_color(badge, theme));
            let on_toggle = on_toggle_folder.clone();
            let on_select = on_select_file.clone();
            let node_path = path.clone();
            let is_dir = node.is_dir;
            let click_path = path.clone();

            let on_select_click = on_select.clone();
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
                            (on_toggle)(click_path.clone(), window, cx);
                        } else {
                            (on_select_click)(click_path.clone(), window, cx);
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
                                .text_color(git_badge_color(badge, theme))
                                .child(badge.label()),
                        )
                    });

            let is_html = !is_dir
                && std::path::Path::new(&node_path)
                    .extension()
                    .and_then(|e| e.to_str())
                    .is_some_and(|ext| {
                        ext.eq_ignore_ascii_case("html") || ext.eq_ignore_ascii_case("htm")
                    });
            let element: gpui::AnyElement = if is_html {
                let p_browser = node_path.clone();
                let p_editor = node_path;
                let on_browser = on_open_browser.clone();
                let on_editor_for_browser = on_select.clone();
                let on_editor_direct = on_select.clone();
                crate::primitives::html_file_context_menu(
                    row,
                    format!("html-tree-item-{}", path),
                    window,
                    cx,
                    move |w, cx| {
                        if let Some(h) = &on_browser {
                            h(p_browser.clone(), w, cx);
                        } else {
                            on_editor_for_browser(p_browser.clone(), w, cx);
                        }
                    },
                    move |w, cx| {
                        on_editor_direct(p_editor.clone(), w, cx);
                    },
                )
            } else {
                row.into_any_element()
            };
            elements.push(element);
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
                    on_open_browser,
                    theme,
                    search_query,
                    git_badges,
                    window,
                    cx,
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

    let mut all_paths: HashSet<&str> = HashSet::new();
    let mut children_by_parent: HashMap<String, Vec<FsTreeEntry>> = HashMap::new();

    for entry in entries {
        all_paths.insert(&entry.path);
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
                .and_then(|p| p.to_str())
                .map(|p| !all_paths.contains(p))
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
        if entry.is_dir
            && let Some(child_entries) = children_by_parent.get(&entry.path)
        {
            for child in child_entries {
                children.push(assemble(child.clone(), children_by_parent));
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
                &self.on_open_browser,
                &theme,
                &self.search_query,
                &self.git_badges,
                _window,
                cx,
            );
            children.append(&mut els);
        }

        let mut container = div()
            .id("file-tree-container")
            .flex_1()
            .w_full()
            .h_full()
            .min_h_0()
            .overflow_y_scroll()
            .p(px(6.0));

        if let Some(scroll_handle) = self.scroll_handle {
            container = container.track_scroll(&scroll_handle);
        }

        container.child(if children.is_empty() {
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
