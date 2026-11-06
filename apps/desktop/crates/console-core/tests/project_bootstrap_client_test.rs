//! Wire-contract tests for the project bootstrap endpoints
//! (`POST /api/projects/init` and `/api/projects/clone`).
//!
//! Two things are pinned here because both are silent failures if they drift:
//! the request body keys, which the server reads with protojson and therefore
//! in camelCase rather than serde's snake_case, and the 409 mapping that lets
//! the dialog tell "connect GitHub in Accounts" apart from a bad URL.

use console_core::{clone_project_body, init_project_body, is_github_not_connected};
use serde_json::json;

#[test]
fn init_body_uses_protojson_camel_case() {
    let body = init_project_body("my-project", "/Users/me/Developer/Projects");
    assert_eq!(
        body,
        json!({ "name": "my-project", "parentDir": "/Users/me/Developer/Projects" })
    );
    // The server would reject `parent_dir`: protojson only accepts the
    // lowerCamelCase name, and unknown fields are discarded rather than
    // matched, so a snake_case key fails as "parentDir is required".
    assert!(body.get("parentDir").is_some());
    assert!(body.get("parent_dir").is_none());
}

#[test]
fn clone_body_omits_name_when_absent() {
    // An absent name is how the server is told to derive the folder from the
    // URL. Sending null instead would decode as an explicitly-set-but-empty
    // optional and produce an empty folder name.
    let body = clone_project_body(
        "https://github.com/octocat/hello.git",
        "/Users/me/Developer/Projects",
        None,
    );
    assert_eq!(
        body,
        json!({
            "url": "https://github.com/octocat/hello.git",
            "parentDir": "/Users/me/Developer/Projects",
        })
    );
    assert!(body.get("name").is_none());
}

#[test]
fn clone_body_carries_an_explicit_name() {
    let body = clone_project_body(
        "git@github.com:octocat/hello.git",
        "/Users/me/Developer/Projects",
        Some("renamed"),
    );
    assert_eq!(body["name"], json!("renamed"));
    // The SSH-style URL is passed through untouched: the rewrite to HTTPS is
    // the server's job, via its injected git config.
    assert_eq!(body["url"], json!("git@github.com:octocat/hello.git"));
}

#[test]
fn clone_body_treats_a_blank_name_as_absent() {
    // A text field left blank should mean "derive it", not "make a folder
    // called nothing".
    let body = clone_project_body("https://github.com/o/r.git", "/tmp", Some("   "));
    assert!(body.get("name").is_none());
}

#[test]
fn a_409_is_reported_as_not_connected() {
    // Stand-in for what clone_project builds from a 409 reply. The branch the
    // dialog takes depends on this being distinguishable from a 400.
    let err = anyhow::anyhow!(console_core::GithubNotConnected {
        message: "GitHub is not connected.".to_string(),
    });
    assert!(is_github_not_connected(&err));
}

#[test]
fn other_failures_are_not_reported_as_not_connected() {
    // A 400 (bad URL, occupied folder) must not send the user to Accounts.
    let err = anyhow::anyhow!("target folder already exists and is not empty");
    assert!(!is_github_not_connected(&err));

    let transport_err = anyhow::anyhow!("Failed to clone repository");
    assert!(!is_github_not_connected(&transport_err));
}

#[test]
fn the_not_connected_marker_keeps_the_server_wording() {
    let err = console_core::GithubNotConnected {
        message: "Add a personal access token in Settings > Accounts".to_string(),
    };
    // The dialog shows this verbatim, so the marker must not swallow it.
    assert_eq!(
        err.to_string(),
        "Add a personal access token in Settings > Accounts"
    );
}

#[test]
fn repo_task_body_omits_name_when_absent() {
    // The from-repo route derives the folder (and the reuse check) from the
    // URL's last segment when no name is sent; an explicit null would decode
    // as an empty optional and break that derivation.
    let body = console_core::repo_task_body("https://github.com/octocat/hello.git", None);
    assert_eq!(body, json!({ "url": "https://github.com/octocat/hello.git" }));
    assert!(body.get("name").is_none());
}

#[test]
fn repo_task_body_carries_an_explicit_name() {
    let body =
        console_core::repo_task_body("https://github.com/octocat/hello.git", Some("renamed"));
    assert_eq!(
        body,
        json!({ "url": "https://github.com/octocat/hello.git", "name": "renamed" })
    );
}

#[test]
fn github_repo_decodes_the_servers_camel_case_json() {
    // The repo list is plain JSON, not the proto schema, so the serde names
    // must match the server's tags exactly or fields silently drop to "".
    let repo: console_core::GithubRepo = serde_json::from_value(json!({
        "fullName": "octocat/hello",
        "name": "hello",
        "private": true,
        "description": "hi",
        "htmlUrl": "https://github.com/octocat/hello",
        "cloneUrl": "https://github.com/octocat/hello.git",
        "pushedAt": "2026-01-02T03:04:05Z",
    }))
    .unwrap();
    assert_eq!(repo.full_name, "octocat/hello");
    assert!(repo.private);
    assert_eq!(repo.clone_url, "https://github.com/octocat/hello.git");
    // The picker clones through this fallback when clone_url is missing.
    assert_eq!(repo.clone_target(), "https://github.com/octocat/hello.git");
    let bare: console_core::GithubRepo =
        serde_json::from_value(json!({ "fullName": "o/r", "name": "r" })).unwrap();
    assert_eq!(bare.clone_target(), "https://github.com/o/r.git");
}

#[test]
fn repo_task_decodes_project_and_session() {
    // The from-repo response nests both protos under data; if these keys
    // drift the picker would open nothing and report success.
    let task: console_core::RepoTask = serde_json::from_value(json!({
        "project": { "id": "p1", "name": "hello", "path": "/home/me/console/projects/hello" },
        "session": { "id": "s1", "title": "Hello", "cwd": "/home/me/console/projects/hello",
                     "projectId": "p1", "modelId": "m", "provider": "p",
                     "approvalMode": "always-ask", "messageCount": 0, "status": "idle" }
    }))
    .unwrap();
    assert_eq!(task.project.id, "p1");
    assert_eq!(task.session.id, "s1");
    assert_eq!(task.session.project_id.as_deref(), Some("p1"));
}
