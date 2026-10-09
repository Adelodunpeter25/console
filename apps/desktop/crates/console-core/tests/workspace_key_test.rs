//! Workspace keys: a plain project keeps its bare id; a worktree gets its own
//! composite key so its tabs/terminals/files are isolated from the project's.

use console_core::{
    SessionHeader, SessionWorktree, is_worktree_workspace_key, project_id_of_workspace_key,
    split_workspace_key, workspace_key_for_session, worktree_workspace_key,
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
    assert_eq!(split_workspace_key(&key), ("proj-1", Some("/odd/::wt::/dir")));
}

#[test]
fn session_in_a_worktree_gets_the_worktree_workspace() {
    let key = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/a")), Some("/repo")).unwrap();
    assert_eq!(key, worktree_workspace_key("proj-1", "/wt/a"));
}

#[test]
fn two_worktrees_of_one_project_get_different_workspaces() {
    let a = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/a")), Some("/repo"));
    let b = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/b")), Some("/repo"));
    assert_ne!(a, b);
}

#[test]
fn session_without_a_worktree_stays_in_its_project_workspace() {
    assert_eq!(
        workspace_key_for_session(&session(Some("proj-1"), None), Some("/repo")),
        Some("proj-1".to_string())
    );
}

#[test]
fn session_without_a_project_has_no_workspace_key() {
    assert_eq!(workspace_key_for_session(&session(None, Some("/wt/a")), Some("/repo")), None);
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
fn trailing_slash_on_the_project_folder_does_not_make_a_new_workspace() {
    let mut s = session(Some("proj-1"), None);
    s.cwd = "/repo/".into();
    assert_eq!(workspace_key_for_session(&s, Some("/repo")), Some("proj-1".to_string()));
}

#[test]
fn unknown_project_folder_falls_back_to_the_project_workspace() {
    let mut s = session(Some("proj-1"), None);
    s.cwd = "/wt/a".into();
    assert_eq!(workspace_key_for_session(&s, None), Some("proj-1".to_string()));
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
