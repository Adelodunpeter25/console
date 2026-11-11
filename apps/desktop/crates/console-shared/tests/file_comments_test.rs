use console_shared::composer::file_comments::{
    FileCommentRef, format_file_comment, split_file_comments,
};

#[test]
fn round_trips_comments_after_text() {
    let content = format!(
        "fix these\n\n{}\n\n{}",
        format_file_comment("src/a.rs", "3", "rename"),
        format_file_comment("src/b.rs", "10–12", "line one\nline two"),
    );
    let (text, comments) = split_file_comments(&content);
    assert_eq!(text, "fix these");
    assert_eq!(
        comments,
        vec![
            FileCommentRef {
                path: "src/a.rs".into(),
                lines: "3".into(),
                text: "rename".into()
            },
            FileCommentRef {
                path: "src/b.rs".into(),
                lines: "10–12".into(),
                text: "line one\nline two".into()
            },
        ]
    );
}

#[test]
fn plain_text_and_broken_tags_pass_through() {
    assert_eq!(split_file_comments("hello"), ("hello".to_string(), vec![]));
    let broken = "hi <file_comment path=\"x";
    assert_eq!(split_file_comments(broken).0, broken);
}
