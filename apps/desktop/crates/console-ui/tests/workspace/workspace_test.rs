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
            provider: Some("claude".into()),
            project_id: Some("proj-abc".into()),
            last_active_at_ms: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::File {
            path: "/path/to/file.rs".into(),
            title: "file.rs".into(),
            project_id: Some("proj-abc".into()),
            last_active_at_ms: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Settings {
            title: "Settings".into(),
            active_tab: Some("providers".into()),
            last_active_at_ms: None,
        },
    );

    let json = serde_json::to_string_pretty(&root).expect("serialize workspace node");
    let deserialized: WorkspaceNode =
        serde_json::from_str(&json).expect("deserialize workspace node");

    assert_eq!(root, deserialized);
    assert_eq!(deserialized.leaves().len(), 1);
    assert_eq!(deserialized.leaves()[0].tabs.len(), 3);
    assert_eq!(
        deserialized.leaves()[0].active_tab_id.as_deref(),
        Some("settings")
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
            provider: None,
            project_id: None,
            last_active_at_ms: None,
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
            provider: None,
            project_id: None,
            last_active_at_ms: None,
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
            provider: None,
            project_id: Some("ndi".into()),
            last_active_at_ms: None,
        },
    );
    assert!(ops::set_tab_project(
        &mut root,
        "chat:chat-b",
        Some("viewer".into())
    ));
    assert_eq!(root.leaves()[0].tabs[0].project_id(), Some("viewer"));
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
                provider: None,
                project_id: Some(proj.into()),
                last_active_at_ms: None,
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
                provider: None,
                project_id: Some(proj.into()),
                last_active_at_ms: None,
            }
        } else {
            WorkspaceTabConfig::Terminal {
                terminal_id: id.into(),
                title: id.into(),
                project_id: Some(proj.into()),
                last_active_at_ms: None,
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
            last_active_at_ms: None,
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
            provider: None,
            project_id: None,
            last_active_at_ms: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::File {
            path: "/a/b.rs".into(),
            title: "b.rs".into(),
            project_id: None,
            last_active_at_ms: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Diff {
            path: "/a/c.rs".into(),
            title: "Diff: c.rs".into(),
            project_id: None,
            last_active_at_ms: None,
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
            last_active_at_ms: None,
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
            last_active_at_ms: None,
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
            last_active_at_ms: None,
        },
    );
    assert_eq!(replaced, "file:/a/d.rs");
    assert_eq!(root.leaves()[0].tabs.len(), 2);
    assert!(
        root.leaves()[0]
            .tabs
            .iter()
            .any(|t| t.id() == "diff:/a/b.rs")
    );
}

#[test]
fn test_tab_strip_follow_initial_state() {
    let follow = console_ui::workspace::TabStripFollow::new();
    assert_eq!(follow.scroll_handle.offset().x, gpui::px(0.0));
}

#[test]
fn test_browser_ids_lists_only_browser_tabs() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Browser {
            browser_id: "browser-1".into(),
            url: String::new(),
            title: "One".into(),
            session_id: None,
            project_id: None,
            last_active_at_ms: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "s1".into(),
            title: "Chat".into(),
            provider: None,
            project_id: None,
            last_active_at_ms: None,
        },
    );
    assert_eq!(ops::browser_ids(&root), vec!["browser-1".to_string()]);
    assert!(ops::browser_ids(&WorkspaceNode::leaf("empty")).is_empty());
}

// The settings are not a tab any more: they take over the window. A
// workspace saved by an older build can still name a Settings tab, and
// restoring it must not bring that tab back.
#[test]
fn test_settings_tabs_are_dropped_from_a_restored_workspace() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "session-123".into(),
            title: "First Chat".into(),
            provider: None,
            project_id: None,
            last_active_at_ms: None,
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Settings {
            title: "Settings".into(),
            active_tab: Some("providers".into()),
            last_active_at_ms: None,
        },
    );

    ops::retain_non_settings_tabs(&mut root);

    let leaves = root.leaves();
    assert_eq!(leaves.len(), 1);
    assert_eq!(leaves[0].tabs.len(), 1);
    assert!(matches!(leaves[0].tabs[0], WorkspaceTabConfig::Chat { .. }));
    // The chat is the active tab again, not the settings that were.
    assert_eq!(leaves[0].active_tab_id.as_deref(), Some("chat:session-123"));
}

#[test]
fn test_workspace_is_empty_only_when_every_pane_is_empty() {
    let mut root = WorkspaceNode::leaf("pane-main");
    assert!(ops::is_workspace_empty(&root));

    let right = ops::split_pane(
        &mut root,
        "pane-main",
        console_core::SplitDirection::Vertical,
    )
    .expect("split");
    assert!(
        ops::is_workspace_empty(&root),
        "two empty panes are still an empty workspace"
    );

    let chat = WorkspaceTabConfig::Chat {
        session_id: "session-1".into(),
        title: "Chat".into(),
        provider: None,
        project_id: Some("proj-a".into()),
        last_active_at_ms: None,
    };
    ops::open_tab(&mut root, &right, chat);
    assert!(
        !ops::is_workspace_empty(&root),
        "a tab in the second pane keeps the workspace non-empty"
    );

    ops::close_tab(&mut root, &right, "chat:session-1");
    assert!(ops::is_workspace_empty(&root));
}

#[test]
fn test_close_tab_prefers_most_recently_active_tab() {
    let mut root = WorkspaceNode::leaf("pane-main");
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "chat-1".into(),
            title: "Chat 1".into(),
            provider: None,
            project_id: None,
            last_active_at_ms: Some(100),
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "chat-2".into(),
            title: "Chat 2".into(),
            provider: None,
            project_id: None,
            last_active_at_ms: Some(500),
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::Chat {
            session_id: "chat-3".into(),
            title: "Chat 3".into(),
            provider: None,
            project_id: None,
            last_active_at_ms: Some(200),
        },
    );
    ops::open_tab(
        &mut root,
        "pane-main",
        WorkspaceTabConfig::File {
            path: "/a/file.rs".into(),
            title: "file.rs".into(),
            project_id: None,
            last_active_at_ms: Some(1000),
        },
    );

    let next = ops::close_tab(&mut root, "pane-main", "file:/a/file.rs");
    assert_eq!(next, Some("chat:chat-2".to_string()));
}

// Leaving a chat for another and coming back must return the desk exactly as
// it was left: the split and every side pane tab (browser, file, terminal)
// survive. The old per-chat "subtab sidecar" used to close the side pane
// whenever it had no chat tab, which silently destroyed those tabs.
#[test]
fn test_desk_round_trip_keeps_the_side_pane_tabs() {
    use std::collections::HashMap;

    let key_a = Some("proj-a::session::chat-a".to_string());
    let key_b = Some("proj-a::session::chat-b".to_string());
    let mut desks = HashMap::new();

    let mut desk_a = WorkspaceNode::leaf(console_core::desk_leaf_id(&key_a));
    let chat_pane = desk_a.first_leaf().expect("leaf").id.clone();
    ops::open_tab(
        &mut desk_a,
        &chat_pane,
        WorkspaceTabConfig::Chat {
            session_id: "chat-a".into(),
            title: "Chat A".into(),
            provider: None,
            project_id: Some(key_a.clone().unwrap()),
            last_active_at_ms: None,
        },
    );
    let side = ops::split_pane(
        &mut desk_a,
        &chat_pane,
        console_core::SplitDirection::Horizontal,
    )
    .expect("split");
    ops::open_tab(
        &mut desk_a,
        &side,
        WorkspaceTabConfig::Browser {
            browser_id: "browser-1".into(),
            url: "https://waku.sh".into(),
            title: "Waku".into(),
            session_id: None,
            project_id: Some(key_a.clone().unwrap()),
            last_active_at_ms: None,
        },
    );
    ops::open_tab(
        &mut desk_a,
        &side,
        WorkspaceTabConfig::File {
            path: "/a/.gitignore".into(),
            title: ".gitignore".into(),
            project_id: Some(key_a.clone().unwrap()),
            last_active_at_ms: None,
        },
    );

    let desk_b = ops::switch_desk(&mut desks, &desk_a, &key_a, &key_b);
    let desk_a_again = ops::switch_desk(&mut desks, &desk_b, &key_b, &key_a);

    assert_eq!(desk_a_again, desk_a, "the desk comes back unchanged");
    assert_eq!(desk_a_again.leaves().len(), 2, "the split survives");
    let side_tabs: Vec<String> = desk_a_again.leaves()[1]
        .tabs
        .iter()
        .map(|tab| tab.id())
        .collect();
    assert!(side_tabs.contains(&"browser:browser-1".to_string()));
    assert!(side_tabs.contains(&"file:/a/.gitignore".to_string()));
}

// A desk that has never been visited starts on a pane of its own, so its
// state never shares with another desk's.
#[test]
fn test_a_new_desk_starts_on_its_own_pane() {
    use std::collections::HashMap;

    let key_a = Some("proj-a::session::chat-a".to_string());
    let key_b = Some("proj-a::session::chat-b".to_string());
    let mut desks = HashMap::new();

    let desk_a = WorkspaceNode::leaf(console_core::desk_leaf_id(&key_a));
    let desk_b = ops::switch_desk(&mut desks, &desk_a, &key_a, &key_b);

    assert_eq!(desk_b.leaves().len(), 1);
    assert_ne!(
        desk_a.leaves()[0].id, desk_b.leaves()[0].id,
        "two desks must not share a pane id"
    );
    assert!(desk_b.leaves()[0].tabs.is_empty());
}
