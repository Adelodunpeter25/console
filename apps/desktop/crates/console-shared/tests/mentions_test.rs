//! Composer mention bookkeeping: ranges must follow edits and drop when stale.

use console_shared::composer::mentions::{
    ComposerMention, SKILL_PATH_PREFIX, adjust_mentions, context_file_paths,
    mentions_from_context_files, parse_chat_refs, reconcile_mentions,
};

fn mention(range: std::ops::Range<usize>, path: &str, label: &str) -> ComposerMention {
    ComposerMention {
        range,
        path: path.into(),
        label: label.into(),
    }
}

#[test]
fn edit_before_a_mention_shifts_it() {
    let mut mentions = vec![mention(6..11, "/a/b.rs", "b.rs")];
    adjust_mentions(&mut mentions, &(0..0), 3);
    assert_eq!(mentions[0].range, 9..14);
}

#[test]
fn edit_inside_a_mention_removes_it() {
    let mut mentions = vec![mention(0..5, "/a/b.rs", "b.rs")];
    adjust_mentions(&mut mentions, &(2..3), 1);
    assert!(mentions.is_empty());
}

#[test]
fn edit_after_a_mention_leaves_it() {
    let mut mentions = vec![mention(0..5, "/a/b.rs", "b.rs")];
    adjust_mentions(&mut mentions, &(5..5), 4);
    assert_eq!(mentions[0].range, 0..5);
}

#[test]
fn reconcile_drops_mentions_whose_text_no_longer_matches() {
    let mut mentions = vec![mention(0..4, "/a/b.rs", "b.rs"), mention(5..8, "/c", "zzz")];
    reconcile_mentions(&mut mentions, "b.rs other");
    assert_eq!(mentions.len(), 1);
    assert_eq!(mentions[0].label, "b.rs");
}

#[test]
fn reconcile_rejects_ranges_off_a_char_boundary() {
    let mut mentions = vec![mention(0..1, "/a", "é")];
    reconcile_mentions(&mut mentions, "é");
    assert!(mentions.is_empty());
}

#[test]
fn mentions_are_rebuilt_from_context_files_on_token_boundaries() {
    let mentions =
        mentions_from_context_files("see b.rs and c.rs", &["/x/b.rs".into(), "/y/c.rs".into()]);
    assert_eq!(mentions.len(), 2);
    assert_eq!(mentions[0].range, 4..8);
    assert_eq!(mentions[1].range, 13..17);
    // A basename that is only a substring of a longer token is not a mention.
    assert!(mentions_from_context_files("abb.rs", &["/x/b.rs".into()]).is_empty());
}

#[test]
fn a_skill_chip_is_text_with_a_slash_and_is_recognised() {
    let chip = ComposerMention::skill(5..15, "systematic-debugging");
    assert_eq!(chip.label, "/systematic-debugging");
    assert!(chip.is_skill());
    assert_eq!(chip.skill_name(), Some("systematic-debugging"));
    assert!(chip.path.starts_with(SKILL_PATH_PREFIX));
    assert!(!mention(0..4, "/a/b.rs", "b.rs").is_skill());
}

#[test]
fn a_chat_chip_is_a_dollar_ref_with_a_chat_path() {
    let chip = ComposerMention::chat(5..18, "fix-login-bug");
    assert_eq!(chip.label, "$fix-login-bug");
    assert!(chip.is_chat());
    assert_eq!(chip.chat_slug(), Some("fix-login-bug"));
    assert!(!chip.is_skill());
    assert!(!mention(0..4, "/a/b.rs", "b.rs").is_chat());
}

#[test]
fn skills_are_never_sent_as_context_files() {
    let mentions = vec![
        mention(0..4, "/a/b.rs", "b.rs"),
        ComposerMention::skill(8..14, "plan"),
        mention(20..24, "/a/c.rs", "c.rs"),
    ];
    assert_eq!(context_file_paths(&mentions), vec!["/a/b.rs", "/a/c.rs"]);
}

#[test]
fn a_skill_chip_follows_edits_and_goes_when_its_text_changes() {
    let mut mentions = vec![ComposerMention::skill(3..8, "plan")];
    adjust_mentions(&mut mentions, &(0..0), 2);
    assert_eq!(mentions[0].range, 5..10);
    reconcile_mentions(&mut mentions, "xxxxx/plan");
    assert_eq!(mentions.len(), 1);
    reconcile_mentions(&mut mentions, "xxxxx/plon");
    assert!(mentions.is_empty());
}

#[test]
fn chat_refs_match_the_shared_server_fixture() {
    let raw = std::fs::read_to_string(
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../../proto/testdata/session/chat_refs.json"),
    )
    .expect("read chat_refs.json");
    let cases: Vec<serde_json::Value> = serde_json::from_str(&raw).expect("parse fixture");
    assert!(!cases.is_empty());
    for case in cases {
        let text = case["text"].as_str().unwrap();
        let want: Vec<&str> = case["refs"]
            .as_array()
            .unwrap()
            .iter()
            .map(|r| r.as_str().unwrap())
            .collect();
        let got = parse_chat_refs(text);
        let slugs: Vec<&str> = got.iter().map(|r| r.slug.as_str()).collect();
        assert_eq!(slugs, want, "{text:?}");
    }
}

#[test]
fn chat_ref_range_covers_the_dollar_and_the_token() {
    let refs = parse_chat_refs("use $fix-bug now");
    assert_eq!(refs[0].range, 4..12);
}
