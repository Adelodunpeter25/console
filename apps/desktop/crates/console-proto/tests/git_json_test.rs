use console_proto::{GitBranchesResponse, GitCheckoutResponse, GitDiffResponse, GitStatusSummary};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/git")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_status_fixture() {
    let msg: GitStatusSummary =
        serde_json::from_str(fixture("status.json").trim()).expect("decode status");
    assert_eq!(msg.branch, "main");
    assert!(!msg.clean);
    assert_eq!(msg.files.len(), 2);
    assert_eq!(msg.files[0].status, "M");
    assert!(msg.files[0].staged);
    assert_eq!(msg.files[0].additions, Some(12));
    assert_eq!(msg.files[1].additions, Some(0));
}

#[test]
fn decodes_golden_branches_fixture() {
    let msg: GitBranchesResponse =
        serde_json::from_str(fixture("branches.json").trim()).expect("decode branches");
    assert_eq!(msg.branches.len(), 2);
    assert!(msg.branches[0].current);
    assert!(!msg.branches[1].current);
    assert!(msg.is_git_repository);
}

#[test]
fn decodes_golden_diff_and_checkout_fixtures() {
    let diff: GitDiffResponse =
        serde_json::from_str(fixture("diff.json").trim()).expect("decode diff");
    assert_eq!(diff.path, "/r/a.go");
    assert!(diff.diff.contains("+++ b/a.go"));
    let checkout: GitCheckoutResponse =
        serde_json::from_str(fixture("checkout.json").trim()).expect("decode checkout");
    assert_eq!(checkout.branch, "dev");
}

#[test]
fn ignores_unknown_fields() {
    let msg: GitStatusSummary = serde_json::from_str(
        r#"{"branch":"b","clean":true,"files":[],"futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(msg.branch, "b");
}

#[test]
fn serializes_counts_as_numbers() {
    // uint32 counts stay JSON numbers (unlike int64 timestamps elsewhere).
    let raw = serde_json::to_string(&GitStatusSummary {
        branch: "main".into(),
        files: vec![console_proto::GitFileEntry {
            path: "/r/a.go".into(),
            status: "M".into(),
            additions: Some(12),
            ..Default::default()
        }],
        ..Default::default()
    })
    .expect("encode status");
    assert!(raw.contains(r#""additions":12"#), "unexpected encoding: {raw}");
    assert!(!raw.contains("clean"), "false bools must omit: {raw}");
}
