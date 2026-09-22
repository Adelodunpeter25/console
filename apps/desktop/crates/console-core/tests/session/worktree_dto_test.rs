use console_core::{CreateSessionDto, CreateWorktreeSpec};

#[test]
fn worktree_spec_empty_serializes_to_empty_object() {
    let dto = CreateSessionDto {
        cwd: Some("/repo".into()),
        project_id: Some("proj-1".into()),
        model_id: None,
        provider: None,
        title: Some("New Chat".into()),
        approval_mode: None,
        thinking_level: None,
        worktree: Some(CreateWorktreeSpec { branch: None }),
    };
    let json = serde_json::to_value(&dto).expect("serialize create dto");
    assert_eq!(
        json.get("worktree"),
        Some(&serde_json::json!({})),
        "empty spec must send `\"worktree\":{{}}` so the server auto-derives the branch"
    );
}

#[test]
fn worktree_spec_explicit_branch_round_trips() {
    let dto = CreateSessionDto {
        cwd: Some("/repo".into()),
        project_id: None,
        model_id: None,
        provider: None,
        title: None,
        approval_mode: None,
        thinking_level: None,
        worktree: Some(CreateWorktreeSpec {
            branch: Some("my-feature".into()),
        }),
    };
    let json = serde_json::to_value(&dto).expect("serialize create dto");
    assert_eq!(json.get("worktree"), Some(&serde_json::json!({"branch": "my-feature"})));
}

#[test]
fn worktree_omitted_by_default() {
    let dto = CreateSessionDto {
        cwd: None,
        project_id: None,
        model_id: None,
        provider: None,
        title: Some("New Chat".into()),
        approval_mode: None,
        thinking_level: None,
        worktree: None,
    };
    let json = serde_json::to_value(&dto).expect("serialize create dto");
    assert!(
        json.get("worktree").is_none(),
        "plain sessions must not send a worktree key"
    );
}
