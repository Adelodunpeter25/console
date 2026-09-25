use console_core::types::*;

#[test]
fn test_git_types_deserialization() {
    let json_data = r#"{
        "branch": "feature/sidebar",
        "clean": false,
        "files": [
            {
                "path": "apps/desktop/src/main.rs",
                "status": "M",
                "staged": false,
                "additions": 12,
                "deletions": 4
            },
            {
                "path": "apps/desktop/src/new_view.rs",
                "status": "A",
                "staged": true,
                "additions": 85,
                "deletions": 0
            }
        ]
    }"#;

    let summary: GitStatusSummary = serde_json::from_str(json_data).expect("deserializes status");
    assert_eq!(summary.branch, "feature/sidebar");
    assert!(!summary.clean);
    assert_eq!(summary.files.len(), 2);
    assert_eq!(summary.files[0].additions, Some(12));
    assert_eq!(summary.files[0].deletions, Some(4));
    assert_eq!(summary.modified_count(), 1);
    assert_eq!(summary.staged_count(), 1);
}

#[test]
fn test_session_file_change_deserialization() {
    let json_data = r#"{
        "path": "crates/console-core/src/lib.rs",
        "status": "modified",
        "additions": 10,
        "deletions": 2,
        "turnIndex": 3,
        "diffText": "--- a/lib.rs\n+++ b/lib.rs\n",
        "reviewed": true,
        "updatedAt": 1700000000000
    }"#;

    let change: SessionFileChange = serde_json::from_str(json_data).expect("deserializes change");
    assert_eq!(change.path, "crates/console-core/src/lib.rs");
    assert_eq!(change.status, "modified");
    assert_eq!(change.additions, 10);
    assert_eq!(change.deletions, 2);
    assert_eq!(change.turn_index, 3);
    assert_eq!(change.diff_text.as_deref(), Some("--- a/lib.rs\n+++ b/lib.rs\n"));
    assert!(change.reviewed);
}

#[test]
fn test_session_file_change_deserialization_defaults() {
    // diffText and reviewed are optional/defaulted for backward compat with
    // any cached payloads predating those fields.
    let json_data = r#"{
        "path": "a.rs",
        "status": "added",
        "additions": 1,
        "deletions": 0,
        "turnIndex": 0,
        "updatedAt": 1700000000000
    }"#;

    let change: SessionFileChange = serde_json::from_str(json_data).expect("deserializes change");
    assert_eq!(change.diff_text, None);
    assert!(!change.reviewed);
}

#[test]
fn test_fs_tree_entry_deserialization() {
    let json_data = r#"{
        "name": "src",
        "path": "/repo/src",
        "isDir": true,
        "children": [
            {
                "name": "main.rs",
                "path": "/repo/src/main.rs",
                "isDir": false,
                "size": 1024
            }
        ]
    }"#;

    let tree: FsTreeEntry = serde_json::from_str(json_data).expect("deserializes tree");
    assert_eq!(tree.name, "src");
    assert!(tree.is_dir);
    assert_eq!(tree.children.as_ref().unwrap().len(), 1);
}

#[test]
fn test_update_session_project_id_null_vs_omit() {
    // Clearing to No project must send explicit null (server scratchpads it);
    // omitting the key would let the server re-infer the old project.
    let clear = UpdateSessionDto {
        cwd: Some("/home/u/.console/scratch/abc".into()),
        project_id: Some(None),
        ..Default::default()
    };
    let v: serde_json::Value = serde_json::to_value(&clear).expect("serializes");
    assert_eq!(v.get("projectId"), Some(&serde_json::Value::Null));

    // Untouched fields stay omitted.
    let rename = UpdateSessionDto {
        title: Some("New title".into()),
        ..Default::default()
    };
    let v: serde_json::Value = serde_json::to_value(&rename).expect("serializes");
    assert!(v.get("projectId").is_none());
    assert!(v.get("cwd").is_none());

    // Linking a project sends the id verbatim.
    let link = UpdateSessionDto {
        project_id: Some(Some("proj-1".into())),
        ..Default::default()
    };
    let v: serde_json::Value = serde_json::to_value(&link).expect("serializes");
    assert_eq!(v.get("projectId"), Some(&serde_json::json!("proj-1")));
}

#[test]
fn test_grep_result_deserialization() {
    // Mirrors the exact camelCase shape returned by GET /api/fs/grep
    // (internal/types/fs.go's GrepResult), including the omitted
    // optional regexError field.
    let json_data = r#"{
        "matches": [
            {
                "relPath": "src/client/frontends/desktop/core/src/conductor/FileAPI.ts",
                "lineNumber": 12,
                "lineContent": "  useQuery,",
                "matchRanges": [{"start": 2, "end": 10}]
            }
        ],
        "totalMatched": 7,
        "filteredFiles": 3,
        "nextCursor": 0,
        "hasMore": false
    }"#;

    let result: GrepResult = serde_json::from_str(json_data).expect("deserializes grep result");
    assert_eq!(result.total_matched, 7);
    assert_eq!(result.filtered_files, 3);
    assert!(!result.has_more);
    assert_eq!(result.regex_error, None);
    assert_eq!(result.matches.len(), 1);
    let m = &result.matches[0];
    assert_eq!(
        m.rel_path,
        "src/client/frontends/desktop/core/src/conductor/FileAPI.ts"
    );
    assert_eq!(m.line_number, 12);
    assert_eq!(m.match_ranges.len(), 1);
    assert_eq!(m.match_ranges[0].start, 2);
    assert_eq!(m.match_ranges[0].end, 10);
}

#[test]
fn test_grep_mode_and_case_mode_query_values() {
    // Query param values must match the server's expected strings exactly
    // (internal/fff.GrepMode / CaseMode parsing).
    assert_eq!(GrepMode::Plain.as_query_value(), "plain");
    assert_eq!(GrepMode::Regex.as_query_value(), "regex");
    assert_eq!(GrepMode::Fuzzy.as_query_value(), "fuzzy");
    assert_eq!(GrepMode::default(), GrepMode::Regex);

    assert_eq!(GrepCaseMode::Smart.as_query_value(), "smart");
    assert_eq!(GrepCaseMode::Sensitive.as_query_value(), "sensitive");
    assert_eq!(GrepCaseMode::Insensitive.as_query_value(), "insensitive");
    assert_eq!(GrepCaseMode::default(), GrepCaseMode::Smart);
}
