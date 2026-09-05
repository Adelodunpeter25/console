use console_core::{WorkspaceNode, WorkspaceTabConfig};
use console_ui::workspace::ops;

#[test]
fn test_workspace_node_serialization_roundtrip() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "session-123".into(),
            title: "First Chat".into(),
            project_id: Some("proj-abc".into()),
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::File {
            path: "/path/to/file.rs".into(),
            title: "file.rs".into(),
            project_id: Some("proj-abc".into()),
        },
    );

    let json = serde_json::to_string_pretty(&root).expect("serialize workspace node");
    let deserialized: WorkspaceNode = serde_json::from_str(&json).expect("deserialize workspace node");

    assert_eq!(root, deserialized);
    assert_eq!(deserialized.leaves().len(), 1);
    assert_eq!(deserialized.leaves()[0].tabs.len(), 2);
    assert_eq!(
        deserialized.leaves()[0].active_tab_id.as_deref(),
        Some("file:/path/to/file.rs")
    );
}

#[test]
fn test_split_and_deduplication_across_panes() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "chat-alpha".into(),
            title: "Alpha".into(),
            project_id: None,
        },
    );

    // Split off a new pane with a second chat
    let new_pane_id = ops::move_tab_to_split(
        &mut root,
        None,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "chat-beta".into(),
            title: "Beta".into(),
            project_id: None,
        },
        false,
    )
    .expect("split creates new pane");

    assert_eq!(root.leaves().len(), 2);

    // Simulate deduplication check:
    // When opening "chat-alpha" while focused on new_pane_id, find if it's already open
    let target_tab_id = "chat:chat-alpha";
    let found_pane = root.leaves().iter().find_map(|leaf| {
        if leaf.tabs.iter().any(|t| t.id() == target_tab_id) {
            Some(leaf.id.clone())
        } else {
            None
        }
    });

    assert_eq!(found_pane, Some("pane-main".to_string()));
    assert_ne!(found_pane, Some(new_pane_id));

    // Closing a tab removes it from the pane
    let next_tab = ops::close_tab(&mut root, "pane-main", target_tab_id);
    assert_eq!(next_tab, None);
    assert!(root.leaves()[0].tabs.is_empty());
}
