use console_ui::ComposerMention;
use std::ops::Range;

fn adjust_mentions(
    mentions: &mut Vec<ComposerMention>,
    content: &str,
    edit_range: &Range<usize>,
    new_text_len: usize,
) {
    let old_len = edit_range.len();
    let delta = (new_text_len as isize) - (old_len as isize);
    let mut kept = Vec::with_capacity(mentions.len());

    for mut mention in mentions.drain(..) {
        if edit_range.end <= mention.range.start {
            let start = ((mention.range.start as isize) + delta).max(0) as usize;
            let end = ((mention.range.end as isize) + delta).max(0) as usize;
            mention.range = start..end;
            kept.push(mention);
        } else if edit_range.start >= mention.range.end {
            kept.push(mention);
        }
    }
    *mentions = kept;

    // Reconcile pass
    mentions.retain(|m| {
        if m.range.end > content.len() || m.range.start >= m.range.end {
            return false;
        }
        if !content.is_char_boundary(m.range.start) || !content.is_char_boundary(m.range.end) {
            return false;
        }
        content.get(m.range.clone()) == Some(m.path.as_str())
    });
    mentions.sort_by_key(|m| m.range.start);
}

#[test]
fn test_mention_creation_and_separator_outside_range() {
    let mut mentions = Vec::new();
    let mut content = String::from("Hello ");
    let path = "roadmap.md";
    let insert_text = format!("{} ", path);
    let mention_len = path.len();

    let start = 6;
    content.push_str(&insert_text);
    let mention_range = start..(start + mention_len);

    mentions.push(ComposerMention {
        range: mention_range.clone(),
        path: path.to_string(),
    });

    assert_eq!(content, "Hello roadmap.md ");
    assert_eq!(mentions.len(), 1);
    assert_eq!(mentions[0].range, 6..16); // "roadmap.md" (10 chars)
    // Character at index 16 is the trailing space separator, outside the chip
    assert_eq!(&content[mentions[0].range.clone()], "roadmap.md");
    assert_eq!(&content[16..17], " ");
}

#[test]
fn test_multiple_mentions_shifting_on_edits() {
    let mut mentions = vec![
        ComposerMention {
            range: 0..6, // "foo.rs"
            path: "foo.rs".to_string(),
        },
        ComposerMention {
            range: 7..13, // "bar.rs"
            path: "bar.rs".to_string(),
        },
    ];
    let mut content = String::from("foo.rs bar.rs ");

    // 1. Insert text before first mention: "Look at " (8 chars)
    let edit_range = 0..0;
    let insert = "Look at ";
    content.insert_str(0, insert);
    adjust_mentions(&mut mentions, &content, &edit_range, insert.len());

    assert_eq!(content, "Look at foo.rs bar.rs ");
    assert_eq!(mentions[0].range, 8..14);
    assert_eq!(mentions[1].range, 15..21);
    assert_eq!(&content[mentions[0].range.clone()], "foo.rs");
    assert_eq!(&content[mentions[1].range.clone()], "bar.rs");

    // 2. Insert text between mentions at index 14: " and" (4 chars)
    let edit_range = 14..14;
    let insert = " and";
    content.insert_str(14, insert);
    adjust_mentions(&mut mentions, &content, &edit_range, insert.len());

    assert_eq!(content, "Look at foo.rs and bar.rs ");
    assert_eq!(mentions[0].range, 8..14);
    assert_eq!(mentions[1].range, 19..25);
    assert_eq!(&content[mentions[0].range.clone()], "foo.rs");
    assert_eq!(&content[mentions[1].range.clone()], "bar.rs");
}

#[test]
fn test_mention_atomic_backspace_range_matching() {
    let mentions = vec![ComposerMention {
        range: 6..16, // "roadmap.md"
        path: "roadmap.md".to_string(),
    }];

    // Caret at index 17 (after trailing space): NOT at chip end
    let caret_after_space = 17;
    assert!(mentions.iter().find(|m| m.range.end == caret_after_space).is_none());

    // Caret at index 16 (immediately after chip): matches chip end!
    let caret_at_chip_end = 16;
    let target = mentions.iter().find(|m| m.range.end == caret_at_chip_end);
    assert!(target.is_some());
    assert_eq!(target.unwrap().range, 6..16);
}

#[test]
fn test_mention_atomic_delete_range_matching() {
    let mentions = vec![ComposerMention {
        range: 6..16, // "roadmap.md"
        path: "roadmap.md".to_string(),
    }];

    // Caret at index 6 (immediately before chip): matches chip start!
    let caret_at_chip_start = 6;
    let target = mentions.iter().find(|m| m.range.start == caret_at_chip_start);
    assert!(target.is_some());
    assert_eq!(target.unwrap().range, 6..16);

    // Caret at index 5: does NOT match chip start
    assert!(mentions.iter().find(|m| m.range.start == 5).is_none());
}

#[test]
fn test_mention_invalidation_on_interior_edit() {
    let mut mentions = vec![ComposerMention {
        range: 0..6, // "foo.rs"
        path: "foo.rs".to_string(),
    }];
    let mut content = String::from("foo.rs ");

    // Mutate characters inside the mention (replace "oo" with "xx")
    let edit_range = 1..3;
    content.replace_range(edit_range.clone(), "xx");
    adjust_mentions(&mut mentions, &content, &edit_range, 2);

    assert_eq!(content, "fxx.rs ");
    assert_eq!(mentions.len(), 0);
}
