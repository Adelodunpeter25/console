//! Workspace keys: every chat gets its own desk — plain chats keyed by
//! session, worktree chats by checkout — while the project id stays the
//! prefix so grouping and lookups keep working.

use console_core::{
    SessionHeader, SessionWorktree, desk_leaf_id, desk_pane_prefix, is_session_workspace_key,
    is_worktree_workspace_key, namespace_pane_ids, project_id_of_workspace_key,
    session_workspace_key, split_workspace_key, workspace_key_for_session,
    worktree_workspace_key,
};

fn session(project_id: Option<&str>, worktree: Option<&str>) -> SessionHeader {
    SessionHeader {
        id: "s1".into(),
        cwd: worktree.unwrap_or("/repo").into(),
        project_id: project_id.map(str::to_owned),
        worktree: worktree.map(|path| SessionWorktree {
            path: path.into(),
            branch: "feature".into(),
            repo: "/repo".into(),
        }),
        ..Default::default()
    }
}

#[test]
fn plain_project_key_is_its_bare_id() {
    assert_eq!(split_workspace_key("proj-1"), ("proj-1", None));
    assert_eq!(project_id_of_workspace_key("proj-1"), "proj-1");
    assert!(!is_worktree_workspace_key("proj-1"));
}

#[test]
fn worktree_key_round_trips_the_project_and_path() {
    let key = worktree_workspace_key("proj-1", "/Users/me/console/worktrees/warm-austin");
    assert_ne!(key, "proj-1");
    assert!(is_worktree_workspace_key(&key));
    assert_eq!(project_id_of_workspace_key(&key), "proj-1");
    assert_eq!(
        split_workspace_key(&key),
        ("proj-1", Some("/Users/me/console/worktrees/warm-austin"))
    );
}

#[test]
fn a_path_that_itself_looks_like_a_key_still_splits_on_the_first_separator() {
    let key = worktree_workspace_key("proj-1", "/odd/::wt::/dir");
    assert_eq!(
        split_workspace_key(&key),
        ("proj-1", Some("/odd/::wt::/dir"))
    );
}

#[test]
fn session_in_a_worktree_gets_the_worktree_workspace() {
    let key =
        workspace_key_for_session(&session(Some("proj-1"), Some("/wt/a")), Some("/repo")).unwrap();
    assert_eq!(key, worktree_workspace_key("proj-1", "/wt/a"));
}

#[test]
fn two_worktrees_of_one_project_get_different_workspaces() {
    let a = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/a")), Some("/repo"));
    let b = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/b")), Some("/repo"));
    assert_ne!(a, b);
}

#[test]
fn session_without_a_worktree_gets_its_own_workspace() {
    let key = workspace_key_for_session(&session(Some("proj-1"), None), Some("/repo")).unwrap();
    assert_eq!(key, session_workspace_key("proj-1", "s1"));
    // The chat still belongs to the project: the id stays the prefix and the
    // desk sits in the project folder.
    assert_eq!(project_id_of_workspace_key(&key), "proj-1");
    assert_eq!(split_workspace_key(&key), ("proj-1", None));
    assert!(is_session_workspace_key(&key));
    assert!(!is_worktree_workspace_key(&key));
}

#[test]
fn two_plain_chats_of_one_project_get_different_workspaces() {
    let mut a = session(Some("proj-1"), None);
    let mut b = session(Some("proj-1"), None);
    a.id = "a".into();
    b.id = "b".into();
    assert_ne!(
        workspace_key_for_session(&a, Some("/repo")),
        workspace_key_for_session(&b, Some("/repo"))
    );
}

#[test]
fn session_without_a_project_has_no_workspace_key() {
    assert_eq!(
        workspace_key_for_session(&session(None, Some("/wt/a")), Some("/repo")),
        None
    );
    assert_eq!(workspace_key_for_session(&session(None, None), None), None);
}

// A chat started inside an existing worktree has no worktree record from the
// server, only a cwd that differs from the project folder — still its own
// workspace.
#[test]
fn chat_in_an_existing_worktree_is_keyed_by_its_cwd() {
    let mut s = session(Some("proj-1"), None);
    s.cwd = "/wt/a/".into();
    assert_eq!(
        workspace_key_for_session(&s, Some("/repo")),
        Some(worktree_workspace_key("proj-1", "/wt/a"))
    );
}

#[test]
fn trailing_slash_on_the_project_folder_does_not_change_the_desk() {
    let mut s = session(Some("proj-1"), None);
    s.cwd = "/repo/".into();
    assert_eq!(
        workspace_key_for_session(&s, Some("/repo")),
        Some(session_workspace_key("proj-1", "s1"))
    );
}

#[test]
fn unknown_project_folder_still_gets_the_chats_own_workspace() {
    let mut s = session(Some("proj-1"), None);
    s.cwd = "/wt/a".into();
    assert_eq!(
        workspace_key_for_session(&s, None),
        Some(session_workspace_key("proj-1", "s1"))
    );
}

#[test]
fn dead_worktree_workspaces_are_those_no_live_chat_maps_to() {
    use console_core::dead_worktree_workspace_keys;
    use std::collections::HashSet;

    let alive = worktree_workspace_key("proj-1", "/wt/alive");
    let gone = worktree_workspace_key("proj-1", "/wt/gone");
    let live: HashSet<String> = [alive.clone(), "proj-1".to_string()].into_iter().collect();

    let dead = dead_worktree_workspace_keys(["proj-1", alive.as_str(), gone.as_str()], &live);
    assert_eq!(dead, vec![gone]);
}

#[test]
fn plain_project_workspaces_are_never_dead_even_with_no_chats() {
    use console_core::dead_worktree_workspace_keys;
    use std::collections::HashSet;

    assert!(dead_worktree_workspace_keys(["proj-1", "proj-2"], &HashSet::new()).is_empty());
}

#[test]
fn each_desk_starts_on_a_pane_of_its_own() {
    use console_core::WorkspaceNode;

    let key_a = Some(session_workspace_key("proj-1", "s1"));
    let key_b = Some(session_workspace_key("proj-1", "s2"));
    assert_ne!(desk_leaf_id(&key_a), desk_leaf_id(&key_b));

    // Two chats of the same project must not share a pane id: a pane's draft
    // and staged comments are filed under it.
    let a = WorkspaceNode::leaf(desk_leaf_id(&key_a));
    let b = WorkspaceNode::leaf(desk_leaf_id(&key_b));
    assert_ne!(a.leaves()[0].id, b.leaves()[0].id);

    // The project-less desk keeps the window's shared main pane.
    assert_eq!(desk_leaf_id(&None), "pane-main");
}

#[test]
fn pane_namespacing_is_idempotent() {
    use console_core::WorkspaceNode;

    let key = Some(session_workspace_key("proj-1", "s1"));
    let mut root = WorkspaceNode::leaf("pane-28c247c8");

    let renamed = namespace_pane_ids(&mut root, &key);
    assert_eq!(renamed.len(), 1);
    let namespaced = root.leaves()[0].id.clone();
    assert!(namespaced.starts_with(&desk_pane_prefix(&key).unwrap()));
    assert!(namespaced.ends_with("pane-28c247c8"));

    // Saving and restoring again keeps the id instead of nesting a new prefix.
    let renamed_again = namespace_pane_ids(&mut root, &key);
    assert!(renamed_again.is_empty());
    assert_eq!(root.leaves()[0].id, namespaced);
}
