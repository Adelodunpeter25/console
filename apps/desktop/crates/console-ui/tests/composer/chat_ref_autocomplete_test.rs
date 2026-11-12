//! `$` chat-ref autocomplete: trigger detection, title slugging, and
//! suggestion filtering (desktop parity with the server's `$` parsing and
//! the Android composer's chat-ref trigger).

use console_core::SessionHeader;
use console_ui::{AutocompleteKind, detect_trigger, filter_items, slugify_chat_title};

fn header(id: &str, title: &str) -> SessionHeader {
    SessionHeader {
        id: id.to_string(),
        title: title.to_string(),
        cwd: String::new(),
        project_id: None,
        model_id: String::new(),
        provider: String::new(),
        approval_mode: String::new(),
        thinking_level: None,
        created_at: 0,
        updated_at: 0,
        message_count: None,
        status: "done".to_string(),
        deleted_at: None,
        pinned_at: None,
        settled_at: None,
        unread: false,
        worktree: None,
    }
}

#[test]
fn test_chat_ref_triggers_after_boundary() {
    let value = "hello $fix-login";
    let trigger = detect_trigger(value, value.len()).expect("chat ref trigger");
    assert_eq!(trigger.kind, AutocompleteKind::Chat);
    assert_eq!(trigger.query, "fix-login");
    assert_eq!(&value[trigger.range], "$fix-login");

    let bare = detect_trigger("$", 1).expect("bare trigger shows all chats");
    assert_eq!(bare.kind, AutocompleteKind::Chat);
    assert_eq!(bare.query, "");

    let paren = detect_trigger("hello ($fix-log", "hello ($fix-log".len())
        .expect("opening punctuation is a boundary");
    assert_eq!(paren.kind, AutocompleteKind::Chat);
    assert_eq!(paren.query, "fix-log");
}

#[test]
fn test_chat_ref_skips_prices_and_mid_word_dollar() {
    assert!(detect_trigger("it costs $5", "it costs $5".len()).is_none());
    assert!(detect_trigger("it costs $500", "it costs $500".len()).is_none());
    assert!(detect_trigger("a$b", "a$b".len()).is_none());
    assert!(detect_trigger("hello $foo bar", "hello $foo bar".len()).is_none());
}

#[test]
fn test_slugify_matches_server() {
    assert_eq!(slugify_chat_title("Fix Login Bug"), "fix-login-bug");
    assert_eq!(slugify_chat_title("  What's -- this!! "), "what-s-this");
    assert_eq!(slugify_chat_title("!!!"), "");
}

#[test]
fn test_chat_ref_filter_ranks_slug_prefix_first() {
    let chats = vec![
        header("id-other", "Other Work"),
        header("id-fix", "Fix Login Bug"),
        header("id-sub", "Login page tweaks"),
    ];
    let trigger = console_ui::AutocompleteTrigger {
        kind: AutocompleteKind::Chat,
        range: 0..4,
        query: "fix".to_string(),
    };
    let items = filter_items(&[], &[], &chats, &trigger);
    assert_eq!(items.len(), 1);
    match &items[0] {
        console_ui::AutocompleteItem::Chat(session) => assert_eq!(session.id, "id-fix"),
        other => panic!("unexpected item: {other:?}"),
    }
    assert_eq!(items[0].insert_text(), "$fix-login-bug ");
}
