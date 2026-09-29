//! Composer history recall carries the id of the user message each prompt
//! became, so the owning app can re-stage that message's image attachments
//! from the transcript. The message id lives in the history entry rather than
//! the image bytes so a long session does not hold every screenshot twice.

use console_ui::common::input::PromptHistoryEntry;

fn entry(text: &str, message_id: Option<&str>) -> PromptHistoryEntry {
    PromptHistoryEntry::new(text.to_string(), Vec::new(), message_id.map(str::to_string))
}

#[test]
fn recalled_entry_exposes_its_message_id() {
    let mut history = console_ui::common::input::PromptHistory::default();
    history.set_entries(vec![entry("first", Some("m1")), entry("second", Some("m2"))]);

    let older = history.navigate(false, "draft", &[]).expect("up recalls newest");
    assert_eq!(older.text, "second");
    assert_eq!(older.message_id.as_deref(), Some("m2"));

    let oldest = history.navigate(false, "second", &[]).expect("up recalls older");
    assert_eq!(oldest.text, "first");
    assert_eq!(oldest.message_id.as_deref(), Some("m1"));
}

#[test]
fn optimistic_entries_have_no_message_id() {
    // A prompt recorded at submit time predates the server assigning an id, so
    // it recalls text and mentions but stages no images.
    let mut history = console_ui::common::input::PromptHistory::default();
    history.set_entries(vec![entry("just sent", None)]);

    let recalled = history.navigate(false, "draft", &[]).expect("up recalls entry");
    assert_eq!(recalled.text, "just sent");
    assert_eq!(recalled.message_id, None);
}

#[test]
fn stepping_forward_past_newest_restores_draft_without_id() {
    let mut history = console_ui::common::input::PromptHistory::default();
    history.set_entries(vec![entry("first", Some("m1"))]);

    history.navigate(false, "unsent draft", &[]).expect("up recalls entry");
    let draft = history.navigate(true, "first", &[]).expect("down restores draft");
    assert_eq!(draft.text, "unsent draft");
    assert_eq!(draft.message_id, None);
    // Cleared index is how the composer tells the app it just restored the
    // draft, so the staged chips can be put back.
    assert!(!history.is_navigating());
}

#[test]
fn mentions_survive_a_recall() {
    let mut history = console_ui::common::input::PromptHistory::default();
    history.set_entries(vec![PromptHistoryEntry::new(
        "check @README".to_string(),
        vec!["README.md".to_string()],
        Some("m1".to_string()),
    )]);

    let recalled = history.navigate(false, "draft", &[]).expect("up recalls entry");
    assert_eq!(recalled.context_files, vec!["README.md".to_string()]);
}

#[test]
fn consecutive_duplicates_collapse_but_older_repeats_survive() {
    let mut history = console_ui::common::input::PromptHistory::default();
    history.record("same".to_string(), Vec::new(), Some("m1".to_string()));
    history.record("same".to_string(), Vec::new(), Some("m2".to_string()));
    assert_eq!(history.entries.len(), 1, "consecutive duplicate collapses");

    // A repeat after other work is still a real history entry.
    history.record("other".to_string(), Vec::new(), Some("m3".to_string()));
    history.record("same".to_string(), Vec::new(), Some("m4".to_string()));
    assert_eq!(history.entries.len(), 3);
}
