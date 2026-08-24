//! Pure operations on the workspace pane tree — the Rust equivalent of the
//! desktop app's `useWorkspaceStore` actions. They take and return the tree so
//! the app state stays a plain owner; UI callbacks call these and notify.

use console_core::{LeafPaneNode, SplitDirection, WorkspaceNode, WorkspaceTabConfig, split_node};

/// The pane a tab lands in: the active pane, or the first leaf when there are
/// no panes yet.
pub fn active_leaf<'a>(
    root: &'a mut WorkspaceNode,
    active_pane_id: Option<&str>,
) -> &'a mut LeafPaneNode {
    // Resolve the target pane id with only immutable peeks, then take the
    // single mutable borrow at the end.
    let target_id = match active_pane_id {
        Some(id) if root.leaves().iter().any(|leaf| leaf.id == id) => Some(id.to_string()),
        _ => root.first_leaf().map(|leaf| leaf.id.clone()),
    };
    match target_id {
        Some(id) => root.leaf_mut(&id).expect("target pane exists"),
        None => {
            // No leaves at all: swap the root for a fresh default leaf.
            *root = WorkspaceNode::leaf("pane-main");
            root.leaf_mut("pane-main").expect("fresh leaf exists")
        }
    }
}

/// Open a tab in the given pane (creating the pane when needed). If the tab
/// is already open there, it is only activated.
pub fn open_tab(root: &mut WorkspaceNode, pane_id: &str, tab: console_core::WorkspaceTabConfig) {
    let leaf = active_leaf(root, Some(pane_id));
    let tab_id = tab.id();
    if !leaf.tabs.iter().any(|t| t.id() == tab_id) {
        leaf.tabs.push(tab);
    }
    leaf.active_tab_id = Some(tab_id);
}

/// Move a tab into an existing pane, removing it from its source first.
/// Existing tabs are deduplicated and activated instead of duplicated.
pub fn move_tab_to_pane(
    root: &mut WorkspaceNode,
    source_pane_id: Option<&str>,
    target_pane_id: &str,
    tab: WorkspaceTabConfig,
) -> Option<String> {
    if root.leaf_mut(target_pane_id).is_none() {
        return None;
    }

    let tab_id = tab.id();
    if source_pane_id != Some(target_pane_id) {
        if let Some(source_pane_id) = source_pane_id {
            let _ = close_tab(root, source_pane_id, &tab_id);
        }
    }
    open_tab(root, target_pane_id, tab);
    Some(target_pane_id.to_string())
}

/// Close a tab in a pane, activating the neighbor (left-first, like the
/// desktop app). Returns the id of the tab that became active, if any.
pub fn close_tab(root: &mut WorkspaceNode, pane_id: &str, tab_id: &str) -> Option<String> {
    let leaf = root.leaf_mut(pane_id)?;
    let closed_index = leaf.tabs.iter().position(|t| t.id() == tab_id)?;
    leaf.tabs.remove(closed_index);

    if leaf.active_tab_id.as_deref() == Some(tab_id) {
        let fallback = leaf
            .tabs
            .get(closed_index.saturating_sub(1))
            .or_else(|| leaf.tabs.first());
        leaf.active_tab_id = fallback.map(|tab| tab.id());
    }
    leaf.active_tab_id.clone()
}

/// Remove a pane and collapse its parent split into the remaining sibling.
/// The final pane is never removed, so the workspace always has a host for a
/// new chat.
pub fn close_pane(root: &mut WorkspaceNode, pane_id: &str) -> bool {
    if root.leaves().len() <= 1 {
        return false;
    }

    fn remove_from(node: &mut WorkspaceNode, pane_id: &str) -> bool {
        let WorkspaceNode::Split(split) = node else {
            return false;
        };

        for child_index in 0..2 {
            let contains_target = split.children[child_index]
                .leaves()
                .iter()
                .any(|leaf| leaf.id == pane_id);
            if !contains_target {
                continue;
            }

            let direct_target = matches!(
                split.children[child_index].as_ref(),
                WorkspaceNode::Leaf(leaf) if leaf.id == pane_id
            );
            if direct_target {
                let sibling = split.children[1 - child_index].as_ref().clone();
                *node = sibling;
                return true;
            }

            if remove_from(&mut split.children[child_index], pane_id) {
                return true;
            }
        }
        false
    }

    remove_from(root, pane_id)
}

/// Activate a tab in a pane.
pub fn select_tab(root: &mut WorkspaceNode, pane_id: &str, tab_id: &str) {
    if let Some(leaf) = root.leaf_mut(pane_id) {
        if leaf.tabs.iter().any(|t| t.id() == tab_id) {
            leaf.active_tab_id = Some(tab_id.to_string());
        }
    }
}

/// Rename every tab whose id matches `predicate` across the whole tree.
/// Used to keep open chat tabs in step with the session's title.
pub fn rename_tabs(
    root: &mut WorkspaceNode,
    predicate: impl Fn(&console_core::WorkspaceTabConfig) -> bool,
    title: impl Into<String>,
) {
    let title = title.into();
    for leaf in root.leaves_mut() {
        for tab in &mut leaf.tabs {
            if predicate(tab) {
                tab.set_title(title.clone());
            }
        }
    }
}

/// Close every tab whose id matches `predicate` across the whole tree,
/// fixing up active ids as it goes.
pub fn close_matching_tabs(
    root: &mut WorkspaceNode,
    predicate: impl Fn(&console_core::WorkspaceTabConfig) -> bool,
) {
    let mut to_close: Vec<(String, String)> = Vec::new();
    for leaf in root.leaves() {
        for tab in &leaf.tabs {
            if predicate(tab) {
                to_close.push((leaf.id.clone(), tab.id()));
            }
        }
    }
    for (pane_id, tab_id) in to_close {
        let _ = close_tab(root, &pane_id, &tab_id);
    }
}

/// Split a leaf pane into two, keeping the original leaf and adding a fresh
/// empty one beside it. Returns the id of the new pane.
pub fn split_pane(
    root: &mut WorkspaceNode,
    pane_id: &str,
    direction: SplitDirection,
) -> Option<String> {
    let new_id = format!("pane-{}", unique_suffix());
    let split_id = format!("split-{}", unique_suffix());
    let new_leaf = WorkspaceNode::leaf(new_id.clone());

    fn inject(
        node: &mut WorkspaceNode,
        pane_id: &str,
        split_id: &str,
        direction: SplitDirection,
        new_leaf: WorkspaceNode,
    ) -> bool {
        match node {
            WorkspaceNode::Leaf(leaf) if leaf.id == pane_id => {
                *node = split_node(split_id, direction, node.clone(), new_leaf);
                true
            }
            WorkspaceNode::Leaf(_) => false,
            WorkspaceNode::Split(split) => {
                inject(
                    &mut split.children[0],
                    pane_id,
                    split_id,
                    direction,
                    new_leaf.clone(),
                ) || inject(
                    &mut split.children[1],
                    pane_id,
                    split_id,
                    direction,
                    new_leaf,
                )
            }
        }
    }

    if inject(root, pane_id, &split_id, direction, new_leaf) {
        Some(new_id)
    } else {
        None
    }
}

/// Move a tab into a new left/right split adjacent to `target_pane_id`.
///
/// The source pane is optional so sidebar sessions can be dropped into a new
/// pane without first existing in the workspace tree. Existing workspace tabs
/// are detached without disposing their underlying resource.
pub fn move_tab_to_split(
    root: &mut WorkspaceNode,
    source_pane_id: Option<&str>,
    target_pane_id: &str,
    tab: WorkspaceTabConfig,
    insert_left: bool,
) -> Option<String> {
    if root.leaf_mut(target_pane_id).is_none() {
        return None;
    }

    let tab_id = tab.id();
    if let Some(source_pane_id) = source_pane_id {
        let _ = close_tab(root, source_pane_id, &tab_id);
    }

    let new_pane_id = format!("pane-{}", unique_suffix());
    let split_id = format!("split-{}", unique_suffix());
    let new_leaf = WorkspaceNode::Leaf(LeafPaneNode {
        id: new_pane_id.clone(),
        tabs: vec![tab],
        active_tab_id: Some(tab_id),
    });

    fn inject(
        node: &mut WorkspaceNode,
        target_pane_id: &str,
        split_id: &str,
        insert_left: bool,
        new_leaf: WorkspaceNode,
    ) -> bool {
        match node {
            WorkspaceNode::Leaf(leaf) if leaf.id == target_pane_id => {
                let original = node.clone();
                *node = if insert_left {
                    split_node(split_id, SplitDirection::Horizontal, new_leaf, original)
                } else {
                    split_node(split_id, SplitDirection::Horizontal, original, new_leaf)
                };
                true
            }
            WorkspaceNode::Leaf(_) => false,
            WorkspaceNode::Split(split) => {
                inject(
                    &mut split.children[0],
                    target_pane_id,
                    split_id,
                    insert_left,
                    new_leaf.clone(),
                ) || inject(
                    &mut split.children[1],
                    target_pane_id,
                    split_id,
                    insert_left,
                    new_leaf,
                )
            }
        }
    }

    inject(root, target_pane_id, &split_id, insert_left, new_leaf).then_some(new_pane_id)
}

/// A short unique suffix for generated pane/split ids.
fn unique_suffix() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.subsec_nanos())
        .unwrap_or(0);
    format!("{nanos:x}")
}

#[cfg(test)]
mod tests {
    use super::*;
    use console_core::WorkspaceTabConfig;

    fn chat_tab(id: &str) -> WorkspaceTabConfig {
        WorkspaceTabConfig::Chat {
            session_id: id.into(),
            title: format!("Chat {id}"),
            project_id: None,
        }
    }

    #[test]
    fn open_activates_and_dedupes_tabs() {
        let mut root = WorkspaceNode::leaf("pane-main");
        open_tab(&mut root, "pane-main", chat_tab("a"));
        open_tab(&mut root, "pane-main", chat_tab("b"));
        open_tab(&mut root, "pane-main", chat_tab("a")); // dedupe

        let leaf = root.leaf_mut("pane-main").unwrap();
        assert_eq!(leaf.tabs.len(), 2);
        assert_eq!(leaf.active_tab_id.as_deref(), Some("chat:a"));
    }

    #[test]
    fn close_activates_neighbor() {
        let mut root = WorkspaceNode::leaf("pane-main");
        open_tab(&mut root, "pane-main", chat_tab("a"));
        open_tab(&mut root, "pane-main", chat_tab("b"));
        open_tab(&mut root, "pane-main", chat_tab("c"));

        // Closing the active (last) tab activates its left neighbor.
        let next = close_tab(&mut root, "pane-main", "chat:c");
        assert_eq!(next.as_deref(), Some("chat:b"));
        // Closing a middle tab keeps the active tab untouched.
        let next = close_tab(&mut root, "pane-main", "chat:a");
        assert_eq!(next.as_deref(), Some("chat:b"));
    }

    #[test]
    fn rename_updates_matching_tabs_across_panes() {
        let mut root = split_node(
            "split-1",
            SplitDirection::Horizontal,
            WorkspaceNode::leaf("pane-a"),
            WorkspaceNode::leaf("pane-b"),
        );
        open_tab(&mut root, "pane-a", chat_tab("x"));
        open_tab(&mut root, "pane-b", chat_tab("x"));
        open_tab(&mut root, "pane-b", chat_tab("y"));

        rename_tabs(
            &mut root,
            |t| matches!(t, WorkspaceTabConfig::Chat { session_id, .. } if session_id == "x"),
            "Renamed chat",
        );

        let pane_a = root.leaf_mut("pane-a").unwrap();
        assert_eq!(pane_a.tabs[0].title(), "Renamed chat");
        let pane_b = root.leaf_mut("pane-b").unwrap();
        assert_eq!(pane_b.tabs[0].title(), "Renamed chat");
        assert_eq!(pane_b.tabs[1].title(), "Chat y");
    }

    #[test]
    fn close_matching_spans_all_panes() {
        let mut root = split_node(
            "split-1",
            SplitDirection::Horizontal,
            WorkspaceNode::leaf("pane-a"),
            WorkspaceNode::leaf("pane-b"),
        );
        open_tab(&mut root, "pane-a", chat_tab("x"));
        open_tab(&mut root, "pane-b", chat_tab("x"));
        open_tab(&mut root, "pane-b", chat_tab("y"));

        close_matching_tabs(&mut root, |t| t.id() == "chat:x");

        assert_eq!(root.leaf_mut("pane-a").unwrap().tabs.len(), 0);
        let pane_b = root.leaf_mut("pane-b").unwrap();
        assert_eq!(pane_b.tabs.len(), 1);
        assert_eq!(pane_b.active_tab_id.as_deref(), Some("chat:y"));
    }

    #[test]
    fn close_pane_collapses_split_and_preserves_sibling() {
        let mut root = split_node(
            "split-1",
            SplitDirection::Horizontal,
            WorkspaceNode::leaf("pane-a"),
            WorkspaceNode::leaf("pane-b"),
        );
        open_tab(&mut root, "pane-a", chat_tab("a"));
        open_tab(&mut root, "pane-b", chat_tab("b"));

        assert!(close_pane(&mut root, "pane-a"));
        assert_eq!(root.leaves().len(), 1);
        assert_eq!(root.leaves()[0].id, "pane-b");
        assert!(!close_pane(&mut root, "pane-b"));
    }

    #[test]
    fn close_nested_pane_keeps_other_split_branch() {
        let left = split_node(
            "split-left",
            SplitDirection::Horizontal,
            WorkspaceNode::leaf("pane-a"),
            WorkspaceNode::leaf("pane-b"),
        );
        let mut root = split_node(
            "split-root",
            SplitDirection::Horizontal,
            left,
            WorkspaceNode::leaf("pane-c"),
        );

        assert!(close_pane(&mut root, "pane-b"));
        assert_eq!(root.leaves().len(), 2);
        assert_eq!(root.leaves()[0].id, "pane-a");
        assert_eq!(root.leaves()[1].id, "pane-c");
    }

    #[test]
    fn split_keeps_original_and_returns_new_pane() {
        let mut root = WorkspaceNode::leaf("pane-main");
        open_tab(&mut root, "pane-main", chat_tab("a"));

        let new_pane = split_pane(&mut root, "pane-main", SplitDirection::Horizontal).unwrap();

        let leaves = root.leaves();
        assert_eq!(leaves.len(), 2);
        assert_eq!(leaves[0].id, "pane-main");
        assert_eq!(leaves[1].id, new_pane);
        assert!(matches!(root, WorkspaceNode::Split(_)));
    }

    #[test]
    fn move_tab_to_pane_detaches_and_deduplicates() {
        let mut root = split_node(
            "split-1",
            SplitDirection::Horizontal,
            WorkspaceNode::leaf("pane-a"),
            WorkspaceNode::leaf("pane-b"),
        );
        open_tab(&mut root, "pane-a", chat_tab("chat-a"));
        open_tab(&mut root, "pane-b", chat_tab("chat-b"));

        assert_eq!(
            move_tab_to_pane(&mut root, Some("pane-a"), "pane-b", chat_tab("chat-a"),).as_deref(),
            Some("pane-b")
        );
        assert!(root.leaf_mut("pane-a").unwrap().tabs.is_empty());
        let pane_b = root.leaf_mut("pane-b").unwrap();
        assert_eq!(pane_b.tabs.len(), 2);
        assert_eq!(pane_b.active_tab_id.as_deref(), Some("chat:chat-a"));

        move_tab_to_pane(&mut root, Some("pane-b"), "pane-b", chat_tab("chat-a"));
        assert_eq!(root.leaf_mut("pane-b").unwrap().tabs.len(), 2);
    }

    #[test]
    fn move_tab_to_split_detaches_from_source_and_places_left() {
        let mut root = split_node(
            "split-1",
            SplitDirection::Horizontal,
            WorkspaceNode::leaf("pane-a"),
            WorkspaceNode::leaf("pane-b"),
        );
        open_tab(&mut root, "pane-a", chat_tab("chat-a"));
        open_tab(&mut root, "pane-b", chat_tab("chat-b"));

        let new_pane = move_tab_to_split(
            &mut root,
            Some("pane-a"),
            "pane-b",
            chat_tab("chat-a"),
            true,
        )
        .unwrap();

        assert!(root.leaf_mut("pane-a").unwrap().tabs.is_empty());
        let leaves = root.leaves();
        assert_eq!(leaves.iter().filter(|leaf| leaf.id == new_pane).count(), 1);
        let moved = leaves.iter().find(|leaf| leaf.id == new_pane).unwrap();
        assert_eq!(moved.active_tab_id.as_deref(), Some("chat:chat-a"));
        assert_eq!(moved.tabs[0].id(), "chat:chat-a");
    }

    #[test]
    fn move_tab_from_sidebar_does_not_require_a_source_pane() {
        let mut root = WorkspaceNode::leaf("pane-main");
        open_tab(&mut root, "pane-main", chat_tab("existing"));

        let new_pane = move_tab_to_split(
            &mut root,
            None,
            "pane-main",
            chat_tab("from-sidebar"),
            false,
        )
        .unwrap();

        let moved = root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == new_pane)
            .unwrap();
        assert_eq!(moved.tabs[0].id(), "chat:from-sidebar");
    }

    #[test]
    fn active_leaf_falls_back_to_first_or_missing_pane() {
        let mut root = WorkspaceNode::leaf("pane-a");
        open_tab(&mut root, "pane-a", chat_tab("a"));
        // A missing pane id falls back to the first leaf.
        let leaf = active_leaf(&mut root, Some("missing-pane"));
        assert_eq!(leaf.id, "pane-a");
        // No pane id at all also resolves to the first leaf.
        let leaf = active_leaf(&mut root, None);
        assert_eq!(leaf.id, "pane-a");
    }
}
