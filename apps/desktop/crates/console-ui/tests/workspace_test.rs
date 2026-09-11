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
    let new_pane_id = ops::move_tab_to_horizontal_split(
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

#[test]
fn test_set_tab_project_updates_stored_folder() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "chat-b".into(),
            title: "B".into(),
            project_id: Some("ndi".into()),
        },
    );
    assert!(ops::set_tab_project(
        &mut root,
        "chat:chat-b",
        Some("viewer".into())
    ));
    assert_eq!(
        root.leaves()[0].tabs[0].project_id(),
        Some("viewer")
    );
}

#[test]
fn test_take_tab_leaves_old_workspace() {
    let mut root = WorkspaceNode::leaf("pane-main");
    for (id, proj) in [("chat-a", "ndi"), ("chat-b", "ndi")] {
        ops::open_tab(
            &mut root,
            "pane-main",
            WorkspaceTabConfig::Chat {
                session_id: id.into(),
                title: id.into(),
                project_id: Some(proj.into()),
            },
        );
    }
    let mut taken = ops::take_tab(&mut root, "chat:chat-b").expect("tab removed");
    taken.set_project_id(Some("viewer".into()));
    // Old workspace keeps only its own tab.
    assert_eq!(root.leaves()[0].tabs.len(), 1);
    assert_eq!(root.leaves()[0].tabs[0].id(), "chat:chat-a");
    // Moved tab opens in the new workspace.
    let mut viewer_root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(&mut viewer_root, "pane-main", taken);
    ops::retain_project_tabs(&mut viewer_root, &Some("viewer".into()));
    assert_eq!(viewer_root.leaves()[0].tabs.len(), 1);
    assert_eq!(viewer_root.leaves()[0].tabs[0].id(), "chat:chat-b");
}

#[test]
fn test_retain_project_tabs_drops_foreign_folder() {
    let mut root = WorkspaceNode::leaf("pane-main");
    for (kind, id, proj) in [
        ("chat", "chat-a", "ndi"),
        ("term", "term-a", "ndi"),
        ("chat", "chat-b", "viewer"),
    ] {
        let tab = if kind == "chat" {
            WorkspaceTabConfig::Chat {
                session_id: id.into(),
                title: id.into(),
                project_id: Some(proj.into()),
            }
        } else {
            WorkspaceTabConfig::Terminal {
                terminal_id: id.into(),
                title: id.into(),
                project_id: Some(proj.into()),
            }
        };
        ops::open_tab(&mut root, "pane-main", tab);
    }
    // Restoring the Viewer workspace must not bring Ndi tabs along.
    ops::retain_project_tabs(&mut root, &Some("viewer".into()));
    assert_eq!(root.leaves()[0].tabs.len(), 1);
    assert_eq!(root.leaves()[0].tabs[0].id(), "chat:chat-b");
}

#[test]
fn test_find_tab_returns_clone() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Terminal {
            terminal_id: "term-1".into(),
            title: "Terminal".into(),
            project_id: None,
        },
    );
    let found = ops::find_tab(&root, "term:term-1").expect("tab found");
    assert_eq!(found.title(), "Terminal");
    assert!(ops::find_tab(&root, "chat:missing").is_none());
}

#[test]
fn test_open_file_paths_collects_only_file_tabs() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "s".into(),
            title: "Chat".into(),
            project_id: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::File {
            path: "/a/b.rs".into(),
            title: "b.rs".into(),
            project_id: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Diff {
            path: "/a/c.rs".into(),
            title: "Diff: c.rs".into(),
            project_id: None,
        },
    );
    let mut paths = ops::open_file_paths(&root);
    paths.sort();
    assert_eq!(paths, vec!["/a/b.rs".to_string(), "/a/c.rs".to_string()]);
}

#[test]
fn test_preview_replace_stays_within_kind() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Diff {
            path: "/a/b.rs".into(),
            title: "Diff: b.rs".into(),
            project_id: None,
        },
    );

    // A file preview must not eat the diff tab: it opens alongside instead.
    let file_id = ops::replace_or_open_tab(
        &mut root,
        "pane-main",
        Some("diff:/a/b.rs"),
        WorkspaceTabConfig::File {
            path: "/a/c.rs".into(),
            title: "c.rs".into(),
            project_id: None,
        },
    );
    assert_eq!(file_id, "file:/a/c.rs");
    assert_eq!(root.leaves()[0].tabs.len(), 2);

    // Same-kind preview still replaces.
    let replaced = ops::replace_or_open_tab(
        &mut root,
        "pane-main",
        Some("file:/a/c.rs"),
        WorkspaceTabConfig::File {
            path: "/a/d.rs".into(),
            title: "d.rs".into(),
            project_id: None,
        },
    );
    assert_eq!(replaced, "file:/a/d.rs");
    assert_eq!(root.leaves()[0].tabs.len(), 2);
    assert!(root.leaves()[0].tabs.iter().any(|t| t.id() == "diff:/a/b.rs"));
}

#[test]
fn test_tab_strip_follow_initial_state() {
    let follow = console_ui::workspace::TabStripFollow::new();
    assert_eq!(follow.scroll_handle.offset().x, gpui::px(0.0));
}
