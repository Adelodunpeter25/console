//! Grouping and lookup of per-turn file changes and diffs across a session transcript.

use console_core::AgentMessage;
use console_core::types::SessionFileChange;
use std::collections::HashMap;
use std::rc::Rc;

use crate::chat::change_chips::{TurnChange, TurnChanges};

/// Group the session's file changes by the user message that started their
/// turn. The server sends rows most recently updated first; each turn's files
/// are reversed so chips read in the order the agent touched them. Rows with
/// no user message id (nothing links them to a turn) are dropped.
pub fn group_turn_changes(changes: &[SessionFileChange]) -> HashMap<String, Rc<TurnChanges>> {
    let mut grouped: HashMap<String, Vec<TurnChange>> = HashMap::new();
    for change in changes.iter().rev() {
        if change.user_message_id.is_empty() {
            continue;
        }
        grouped
            .entry(change.user_message_id.clone())
            .or_default()
            .push(TurnChange::from_change(change));
    }
    grouped
        .into_iter()
        .map(|(user_message_id, files)| {
            (
                user_message_id.clone(),
                Rc::new(TurnChanges {
                    user_message_id,
                    files,
                }),
            )
        })
        .collect()
}

/// File changes of the turn a settled assistant message closes. Only the
/// turn's last visible assistant message carries them: a turn is one user
/// message and everything the agent did until the next one.
pub fn turn_changes_for_assistant(
    messages: &[AgentMessage],
    turn_changes: &HashMap<String, Rc<TurnChanges>>,
    index: usize,
    is_hidden: impl Fn(&AgentMessage) -> bool,
) -> Option<Rc<TurnChanges>> {
    if turn_changes.is_empty() {
        return None;
    }
    let Some(AgentMessage::Assistant { .. }) = messages.get(index) else {
        return None;
    };
    if is_hidden(&messages[index]) {
        return None;
    }
    let closes_turn = messages[index + 1..]
        .iter()
        .take_while(|message| !matches!(message, AgentMessage::User { .. }))
        .all(|message| !matches!(message, AgentMessage::Assistant { .. }) || is_hidden(message));
    if !closes_turn {
        return None;
    }
    let user_id = messages[..index]
        .iter()
        .rev()
        .find_map(|message| match message {
            AgentMessage::User { id, .. } => Some(id.clone()),
            _ => None,
        })
        .flatten();
    turn_changes.get(&user_id?).cloned()
}

/// Indices of messages whose rows carry turn file change chips.
pub fn rows_with_turn_changes(
    messages: &[AgentMessage],
    turn_changes: &HashMap<String, Rc<TurnChanges>>,
    is_hidden: impl Fn(&AgentMessage) -> bool,
) -> Vec<usize> {
    (0..messages.len())
        .filter(|&index| {
            turn_changes_for_assistant(messages, turn_changes, index, &is_hidden).is_some()
        })
        .collect()
}

/// Copy server-assigned ids onto transcript user messages that still lack one.
pub fn adopt_user_ids(messages: &mut [AgentMessage], canonical: &[AgentMessage]) -> bool {
    let mut canonical_ids = canonical.iter().rev().filter_map(|message| match message {
        AgentMessage::User { id, .. } => Some(id.clone()),
        _ => None,
    });
    let mut changed = false;
    for message in messages.iter_mut().rev() {
        let AgentMessage::User { id, .. } = message else {
            continue;
        };
        let Some(canonical_id) = canonical_ids.next() else {
            break;
        };
        if id.is_none() && canonical_id.is_some() {
            *id = canonical_id;
            changed = true;
        }
    }
    changed
}
