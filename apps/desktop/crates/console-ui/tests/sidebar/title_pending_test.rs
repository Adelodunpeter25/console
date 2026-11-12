use console_core::SessionHeader;
use console_ui::layout::sidebar_loading::{
    SidebarLoadingState, is_generic_title, title_is_pending,
};

fn header(title: &str, message_count: u64) -> SessionHeader {
    // Only title + message_count feed the predicate; the rest are defaults.
    console_core::SessionHeader {
        id: String::new(),
        title: title.to_string(),
        cwd: String::new(),
        project_id: None,
        model_id: String::new(),
        provider: String::new(),
        approval_mode: String::new(),
        thinking_level: None,
        created_at: 0,
        updated_at: 0,
        message_count: Some(message_count.try_into().unwrap()),
        status: "done".to_string(),
        deleted_at: None,
        pinned_at: None,
        settled_at: None,
        unread: false,
        worktree: None,
    }
}

#[test]
fn placeholder_remain_but_not_pending() {
    // Placeholder title with no messages yet: empty new chat, nothing running.
    assert!(!title_is_pending(&header("New Session", 0), None));
}

#[test]
fn placeholder_while_working_is_pending() {
    // Session is working immediately on prompt submit even before message count reflects.
    assert!(title_is_pending(
        &header("New Session", 0),
        Some(SidebarLoadingState::Working { started_at: 100 })
    ));
}

#[test]
fn placeholder_after_first_prompt_is_pending() {
    assert!(title_is_pending(&header("New Session", 1), None));
    assert!(title_is_pending(&header("New Chat", 2), None));
}

#[test]
fn generated_titles_are_never_pending() {
    assert!(!title_is_pending(&header("Fix flaky spec", 1), None));
    assert!(!title_is_pending(
        &header("Fix flaky spec", 1),
        Some(SidebarLoadingState::Working { started_at: 100 })
    ));
    // Rough check that generated-titles-specific placeholders are not generic.
    assert!(!is_generic_title("Rename all over"));
}

#[test]
fn generic_placeholder_list_matches_server() {
    assert!(is_generic_title(""));
    assert!(is_generic_title("  New Chat  "));
    assert!(is_generic_title("Untitled"));
    assert!(is_generic_title("New mobile session"));
    assert!(!is_generic_title("manual"));
}
