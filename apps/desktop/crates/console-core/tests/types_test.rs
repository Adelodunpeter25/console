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
        "updatedAt": 1700000000000
    }"#;

    let change: SessionFileChange = serde_json::from_str(json_data).expect("deserializes change");
    assert_eq!(change.path, "crates/console-core/src/lib.rs");
    assert_eq!(change.status, "modified");
    assert_eq!(change.additions, 10);
    assert_eq!(change.deletions, 2);
    assert_eq!(change.turn_index, 3);
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
