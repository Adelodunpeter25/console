//! Streaming markdown repair. These cases mirror the Android fixtures
//! (`apps/android/tests/MarkdownMendFixtureTest.kt`) so both clients keep one
//! contract now that they share the implementation.

use console_shared::markdown::mend::{PENDING_LINK_URL, close_hanging};

#[test]
fn closes_unfinished_emphasis() {
    assert_eq!(close_hanging("**hello").as_deref(), Some("**hello**"));
    assert_eq!(close_hanging("*hello").as_deref(), Some("*hello*"));
    assert_eq!(close_hanging("~~hello").as_deref(), Some("~~hello~~"));
}

#[test]
fn closes_an_unfinished_code_span() {
    assert_eq!(close_hanging("`hello").as_deref(), Some("`hello`"));
}

#[test]
fn unfinished_link_gets_a_stable_pending_destination() {
    assert_eq!(PENDING_LINK_URL, "console:pending-link");
    assert_eq!(
        close_hanging("[hello](https://example").as_deref(),
        Some("[hello](console:pending-link)")
    );
}

#[test]
fn complete_markdown_needs_no_repair() {
    assert_eq!(
        close_hanging("**hello** and [link](https://example.com)"),
        None
    );
}

#[test]
fn guards_a_streaming_setext_underline() {
    assert_eq!(
        close_hanging("Title\n-").as_deref(),
        Some("Title\n-\u{200B}")
    );
}
