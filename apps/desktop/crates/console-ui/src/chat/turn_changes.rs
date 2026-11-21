//! Grouping and lookup of per-turn file changes and diffs across a session transcript.

use console_core::types::SessionFileChange;
use console_core::{AgentMessage, Turn};
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

/// File changes of the turn a settled assistant response closes. Only the
/// last turn with responses before the next user turn carries them.
pub fn turn_changes_for_turn(
    turns: &[Turn],
    turn_changes: &HashMap<String, Rc<TurnChanges>>,
    turn_index: usize,
) -> Option<Rc<TurnChanges>> {
    if turn_changes.is_empty() {
        return None;
    }
    let turn = turns.get(turn_index)?;
    if turn.responses.is_empty() {
        return None;
    }
    let closes_user_turn = turns[turn_index + 1..]
        .iter()
        .take_while(|next| next.user.is_none())
        .all(|next| next.responses.is_empty());
    if !closes_user_turn {
        return None;
    }
    let user_id = turns[..=turn_index]
        .iter()
        .rev()
        .find_map(|t| match t.user.as_ref() {
            Some(AgentMessage::User { id, .. }) => id.clone(),
            _ => None,
        })?;
    turn_changes.get(&user_id).cloned()
}

/// Indices of turns whose rows carry turn file change chips.
pub fn rows_with_turn_changes(
    turns: &[Turn],
    turn_changes: &HashMap<String, Rc<TurnChanges>>,
) -> Vec<usize> {
    (0..turns.len())
        .filter(|&index| turn_changes_for_turn(turns, turn_changes, index).is_some())
        .collect()
}

/// Copy server-assigned ids onto optimistic user turns that still lack one.
pub fn adopt_user_ids(turns: &mut [Turn], canonical: &[Turn]) -> bool {
    let mut canonical_ids = canonical.iter().rev().filter_map(|turn| {
        let Some(AgentMessage::User { id, .. }) = turn.user.as_ref() else {
            return None;
        };
        Some((turn.id.clone(), id.clone()))
    });
    let mut changed = false;
    for turn in turns.iter_mut().rev() {
        let Some(AgentMessage::User { id, .. }) = turn.user.as_mut() else {
            continue;
        };
        let Some((canonical_turn_id, canonical_user_id)) = canonical_ids.next() else {
            break;
        };
        if turn.id.is_empty() && !canonical_turn_id.is_empty() {
            turn.id = canonical_turn_id;
            changed = true;
        }
        if id.is_none() && canonical_user_id.is_some() {
            *id = canonical_user_id;
            changed = true;
        }
    }
    changed
}
