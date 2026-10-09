//! Workspace keys: a plain project keeps its bare id; a worktree gets its own
//! composite key so its tabs/terminals/files are isolated from the project's.

use console_core::{
    SessionHeader, SessionWorktree, is_worktree_workspace_key, project_id_of_workspace_key,
    split_workspace_key, workspace_key_for_session, worktree_workspace_key,
};

fn session(project_id: Option<&str>, worktree: Option<&str>) -> SessionHeader {
    SessionHeader {
        id: "s1".into(),
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
    let key = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/a"))).unwrap();
    assert_eq!(key, worktree_workspace_key("proj-1", "/wt/a"));
}

#[test]
fn two_worktrees_of_one_project_get_different_workspaces() {
    let a = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/a")));
    let b = workspace_key_for_session(&session(Some("proj-1"), Some("/wt/b")));
    assert_ne!(a, b);
}

#[test]
fn session_without_a_worktree_stays_in_its_project_workspace() {
    assert_eq!(
        workspace_key_for_session(&session(Some("proj-1"), None)),
        Some("proj-1".to_string())
    );
}

#[test]
fn session_without_a_project_has_no_workspace_key() {
    assert_eq!(workspace_key_for_session(&session(None, Some("/wt/a"))), None);
    assert_eq!(workspace_key_for_session(&session(None, None)), None);
}
