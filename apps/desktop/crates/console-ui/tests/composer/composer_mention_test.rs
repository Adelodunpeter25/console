use console_ui::ComposerMention;
use console_ui::{AutocompleteKind, detect_trigger};

#[test]
fn test_slash_command_can_start_after_words() {
    let value = "please run /review";
    let trigger = detect_trigger(value, value.len()).expect("slash command trigger");
    assert_eq!(trigger.kind, AutocompleteKind::Command);
    assert_eq!(trigger.query, "review");
    assert_eq!(&value[trigger.range], "/review");
}

#[test]
fn test_slash_command_does_not_start_inside_word_or_url() {
    assert!(detect_trigger("foo/bar", "foo/bar".len()).is_none());
    assert!(detect_trigger("https://example.com", "https://example.com".len()).is_none());
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
        label: path.to_string(),
    });

    assert_eq!(content, "Hello roadmap.md ");
    assert_eq!(mentions.len(), 1);
    assert_eq!(mentions[0].range, 6..16); // "roadmap.md" (10 chars)
    // Character at index 16 is the trailing space separator, outside the chip
    assert_eq!(&content[mentions[0].range.clone()], "roadmap.md");
    assert_eq!(&content[16..17], " ");
}

#[test]
fn test_mention_atomic_backspace_range_matching() {
    let mentions = vec![ComposerMention {
        range: 6..16, // "roadmap.md"
        path: "roadmap.md".to_string(),
        label: "roadmap.md".to_string(),
    }];

    // Caret at index 17 (after trailing space): NOT at chip end
    let caret_after_space = 17;
    assert!(
        mentions
            .iter()
            .find(|m| m.range.end == caret_after_space)
            .is_none()
    );

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
        label: "roadmap.md".to_string(),
    }];

    // Caret at index 6 (immediately before chip): matches chip start!
    let caret_at_chip_start = 6;
    let target = mentions
        .iter()
        .find(|m| m.range.start == caret_at_chip_start);
    assert!(target.is_some());
    assert_eq!(target.unwrap().range, 6..16);

    // Caret at index 5: does NOT match chip start
    assert!(mentions.iter().find(|m| m.range.start == 5).is_none());
}
