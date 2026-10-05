//! Project run-script models. Canonical wire types come from the shared
//! protobuf schema (proto/console/v1/scripts.proto); run statuses and output
//! streams stay plain strings, matching the usage-domain decision.
//!
//! The shortcut helpers are behavior, not schema: they operate on the
//! generated ProjectScript exactly as they did on the hand-written struct.

use std::collections::{HashMap, HashSet};

pub use console_proto::{
    ProjectScript, ProjectScriptsResult, ScriptExitEvent, ScriptOutputEvent, ScriptRun,
    ScriptRunEvent, ScriptStatusEvent, StopScriptRunResponse,
};
pub use console_proto::script_run_event::Event as ScriptRunEventKind;

/// A run status other than "running" is terminal: the stream has delivered
/// (or will never deliver) its final event.
pub fn script_run_is_terminal(status: &str) -> bool {
    status != "running"
}

/// Canonicalize a `console.toml` shortcut (`shift-cmd-R`, `cmd-shift-r`)
/// into one comparable form: modifiers in ctrl-alt-cmd-shift order, lowercase
/// key (`cmd-shift-r`). Returns `None` for empty input, bare keys without a
/// modifier, or unknown segments. The desktop normalizes live keystrokes into
/// this same form, so author order and letter case never matter.
pub fn canonicalize_shortcut(shortcut: &str) -> Option<String> {
    let mut parts: Vec<&str> = shortcut.split('-').collect();
    let key = parts.pop()?.to_lowercase();
    if key.is_empty() {
        return None;
    }
    let mut out = String::new();
    let mut modifiers = 0;
    for modifier in ["ctrl", "alt", "cmd", "shift"] {
        if parts.iter().any(|part| part.eq_ignore_ascii_case(modifier)) {
            if !out.is_empty() {
                out.push('-');
            }
            out.push_str(modifier);
            modifiers += 1;
        }
    }
    if modifiers == 0 || modifiers != parts.len() {
        return None;
    }
    out.push('-');
    out.push_str(&key);
    Some(out)
}

/// Shortcut dispatch state for one script list. Unique shortcuts stay live
/// in `bindings`; only the duplicated ones land in `conflicts` and stay
/// disabled — one bad pair never kills the rest.
#[derive(Clone, Debug, Default)]
pub struct ShortcutState {
    /// Canonical shortcut → script id, unique shortcuts only.
    pub bindings: HashMap<String, String>,
    /// Canonical shortcut → claiming script ids, duplicates only.
    pub conflicts: HashMap<String, HashSet<String>>,
}

/// Derive dispatch state from a script list in a single pass so both halves
/// stay consistent. Unparseable shortcuts are ignored.
pub fn compute_shortcut_state(scripts: &[ProjectScript]) -> ShortcutState {
    let mut groups: HashMap<String, HashSet<String>> = HashMap::new();
    for script in scripts {
        if let Some(shortcut) = script.shortcut.as_deref()
            && let Some(canonical) = canonicalize_shortcut(shortcut)
        {
            groups
                .entry(canonical)
                .or_default()
                .insert(script.id.clone());
        }
    }
    let mut state = ShortcutState::default();
    for (shortcut, ids) in groups {
        if ids.len() > 1 {
            state.conflicts.insert(shortcut, ids);
        } else {
            // Unwrap is safe: `ids` has exactly one entry when len == 1.
            state
                .bindings
                .insert(shortcut, ids.into_iter().next().unwrap());
        }
    }
    state
}
