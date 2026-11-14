//! The sidebar status slot: running, then needs attention, then unread, then
//! nothing. The server's `unread` flag is the only unread input.

use console_core::SessionHeader;
use console_ui::ReadAction;
use console_ui::layout::sidebar_loading::{
    RowStatus, SidebarLoadingState, read_action, row_status,
};

fn header(status: &str, unread: bool) -> SessionHeader {
    SessionHeader {
        status: status.to_string(),
        unread,
        ..Default::default()
    }
}

#[test]
fn a_finished_chat_you_have_not_opened_shows_the_dot() {
    assert_eq!(row_status(&header("done", true), None), RowStatus::Unread);
}

#[test]
fn a_finished_chat_you_have_read_shows_the_check() {
    assert_eq!(row_status(&header("done", false), None), RowStatus::Done);
}

#[test]
fn a_chat_that_never_ran_shows_nothing() {
    assert_eq!(row_status(&header("idle", false), None), RowStatus::Quiet);
}

#[test]
fn needs_attention_outranks_unread() {
    // An errored run or a pending question is both unread and needing the
    // user; the alert is the stronger signal.
    assert_eq!(
        row_status(&header("needs_attention", true), None),
        RowStatus::NeedsAttention
    );
    assert_eq!(
        row_status(&header("done", true), Some(SidebarLoadingState::Waiting)),
        RowStatus::NeedsAttention
    );
}

#[test]
fn running_outranks_everything() {
    assert_eq!(
        row_status(&header("working", true), None),
        RowStatus::Running
    );
    assert_eq!(
        row_status(
            &header("needs_attention", true),
            Some(SidebarLoadingState::Working { started_at: 100 })
        ),
        RowStatus::Running
    );
}

#[test]
fn an_unread_chat_offers_mark_as_read() {
    let chat = header("done", true);
    assert_eq!(
        read_action(&chat, row_status(&chat, None)),
        Some(ReadAction::MarkRead)
    );
}

#[test]
fn a_read_idle_chat_offers_mark_as_unread() {
    let chat = header("done", false);
    assert_eq!(
        read_action(&chat, row_status(&chat, None)),
        Some(ReadAction::MarkUnread)
    );
}

#[test]
fn running_and_settled_chats_offer_no_read_entry() {
    let running = header("working", false);
    assert_eq!(read_action(&running, row_status(&running, None)), None);
    let mut settled = header("done", false);
    settled.settled_at = Some(5);
    assert_eq!(read_action(&settled, row_status(&settled, None)), None);
}
