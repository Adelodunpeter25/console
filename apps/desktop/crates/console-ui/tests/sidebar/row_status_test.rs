//! The sidebar status slot: running, then needs attention, then unread, then
//! nothing. The server's `unread` flag is the only unread input.

use console_core::SessionHeader;
use console_ui::layout::sidebar_loading::{RowStatus, SidebarLoadingState, row_status};

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
fn only_the_server_flag_makes_a_finished_chat_unread() {
    // The same finished chat is the dot or the check depending on one flag.
    assert_eq!(row_status(&header("done", true), None), RowStatus::Unread);
    assert_eq!(row_status(&header("done", false), None), RowStatus::Done);
}
