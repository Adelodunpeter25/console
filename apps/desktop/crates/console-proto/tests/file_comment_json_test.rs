use console_proto::{FileComment, FileCommentSide};
use std::path::PathBuf;

#[test]
fn decodes_golden_file_comment_fixture() {
    let raw = std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/comments/file_comment.json"),
    )
    .expect("read fixture");
    let comment: FileComment = serde_json::from_str(raw.trim()).expect("decode comment");
    assert_eq!(comment.path, "src/a.rs");
    assert_eq!(comment.side, FileCommentSide::Old as i32);
    assert_eq!((comment.start_line, comment.end_line), (3, 5));
    assert_eq!(comment.created_at, 1_700_000_000_000);
}
