//! Composer mention bookkeeping: ranges must follow edits and drop when stale.

use console_shared::composer::mentions::{
    ComposerMention, adjust_mentions, mentions_from_context_files, reconcile_mentions,
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
    let mentions = mentions_from_context_files("see b.rs and c.rs", &["/x/b.rs".into(), "/y/c.rs".into()]);
    assert_eq!(mentions.len(), 2);
    assert_eq!(mentions[0].range, 4..8);
    assert_eq!(mentions[1].range, 13..17);
    // A basename that is only a substring of a longer token is not a mention.
    assert!(mentions_from_context_files("abb.rs", &["/x/b.rs".into()]).is_empty());
}
