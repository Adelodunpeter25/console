use console_proto::{
    DirCreateResponse, FileDeleteResponse, FileSearchResult, FileWriteResponse, FsBrowseResult,
    FsChangeEvent, FsDirectoryTree, FsFileContent, FsTreeEntry, GrepResult,
};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/fs")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_browse_fixture() {
    let msg: FsBrowseResult =
        serde_json::from_str(fixture("browse.json").trim()).expect("decode browse");
    assert_eq!(msg.current_path, "/tmp/proj");
    assert_eq!(msg.parent_path.as_deref(), Some("/tmp"));
    assert_eq!(msg.entries.len(), 2);
    assert!(msg.entries[0].is_dir);
    // Sizes arrive as protojson strings, not numbers.
    assert_eq!(msg.entries[1].size, Some(1234));
}

#[test]
fn decodes_golden_entries_fixture() {
    let items: Vec<FsTreeEntry> =
        serde_json::from_str(fixture("entries.json").trim()).expect("decode entries");
    assert_eq!(items.len(), 2);
    assert_eq!(items[0].children.len(), 1);
    assert_eq!(items[0].children[0].name, "a.go");
}

#[test]
fn decodes_golden_search_fixture() {
    let items: Vec<FileSearchResult> =
        serde_json::from_str(fixture("search.json").trim()).expect("decode search");
    assert_eq!(items.len(), 1);
    assert_eq!(items[0].relative_path, "src/a.go");
    assert_eq!(items[0].score, 0.95);
}

#[test]
fn decodes_golden_grep_fixture() {
    let msg: GrepResult =
        serde_json::from_str(fixture("grep.json").trim()).expect("decode grep");
    assert_eq!(msg.total_matched, 3);
    assert_eq!(msg.next_cursor, 200);
    assert!(msg.has_more);
    let m = &msg.matches[0];
    assert_eq!(m.line_number, 12);
    assert_eq!(m.match_ranges.len(), 1);
    assert_eq!(m.match_ranges[0].start, 4);
}

#[test]
fn decodes_golden_small_fixtures() {
    let tree: FsDirectoryTree =
        serde_json::from_str(fixture("tree.json").trim()).expect("tree");
    assert!(tree.tree_formatted.contains("a.go"));
    let content: FsFileContent =
        serde_json::from_str(fixture("file_content.json").trim()).expect("content");
    assert_eq!(content.content, "package main\n");
    let wrote: FileWriteResponse =
        serde_json::from_str(fixture("file_write.json").trim()).expect("write");
    assert!(wrote.message.contains("12 bytes"));
    let deleted: FileDeleteResponse =
        serde_json::from_str(fixture("file_delete.json").trim()).expect("delete");
    assert!(deleted.deleted);
    let created: DirCreateResponse =
        serde_json::from_str(fixture("dir_create.json").trim()).expect("mkdir");
    assert!(created.created);
    let watch: FsChangeEvent =
        serde_json::from_str(fixture("watch.json").trim()).expect("watch");
    assert_eq!(watch.event_path.as_deref(), Some("/p/a.go"));
}

#[test]
fn ignores_unknown_fields() {
    // Dropped shapes (git status, modified_at, is_binary, duration, notes,
    // metadata, raw) must be ignored, not fatal.
    let msg: FsTreeEntry = serde_json::from_str(
        r#"{"name":"a","path":"/a","gitStatus":"M","modifiedAt":1,"isBinary":false}"#,
    )
    .expect("dropped fields must be ignored");
    assert_eq!(msg.name, "a");
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&FsTreeEntry {
        name: "a.go".into(),
        path: "/p/a.go".into(),
        size: Some(42),
        ..Default::default()
    })
    .expect("encode entry");
    assert!(raw.contains(r#""size":"42""#), "unexpected encoding: {raw}");
    assert!(!raw.contains("isDir"), "false bools must omit: {raw}");
}
