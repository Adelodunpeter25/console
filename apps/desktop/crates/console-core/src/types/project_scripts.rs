//! Project run-script models mirroring the server project-scripts API.
//!
//! Scripts are defined by the project's `console.toml`; the server owns
//! parsing, validation, and execution. The desktop only ever references
//! scripts by id — it never sends a raw command.

use std::collections::{HashMap, HashSet};

use serde::{Deserialize, Serialize};

/// A normalized script definition from `GET /api/projects/:projectId/scripts`.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProjectScript {
    pub id: String,
    pub label: String,
    pub command: String,
    pub shortcut: Option<String>,
    pub persistent: bool,
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

/// Script list result, including where the definitions came from.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProjectScriptsResult {
    #[serde(rename = "projectId")]
    pub project_id: String,
    pub scripts: Vec<ProjectScript>,
    /// `"console.toml"` when parsed from file, `"missing"` when absent.
    pub source: String,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ScriptRunStatus {
    Running,
    Succeeded,
    Failed,
    Stopped,
}

impl ScriptRunStatus {
    pub fn is_terminal(self) -> bool {
        !matches!(self, Self::Running)
    }
}

/// A managed script execution record.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct ScriptRun {
    #[serde(rename = "runId")]
    pub run_id: String,
    #[serde(rename = "projectId")]
    pub project_id: String,
    #[serde(rename = "scriptId")]
    pub script_id: String,
    pub label: String,
    pub persistent: bool,
    pub status: ScriptRunStatus,
    #[serde(rename = "startedAt")]
    pub started_at: String,
    #[serde(rename = "endedAt")]
    pub ended_at: Option<String>,
    #[serde(rename = "exitCode")]
    pub exit_code: Option<i32>,
    pub stdout: String,
    pub stderr: String,
}

/// Live events from `GET .../runs/:runId/stream`.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum ScriptRunEvent {
    Status {
        status: ScriptRunStatus,
    },
    Output {
        stream: ScriptOutputStream,
        text: String,
    },
    Exit {
        status: ScriptRunStatus,
        #[serde(rename = "exitCode")]
        exit_code: Option<i32>,
    },
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ScriptOutputStream {
    Stdout,
    Stderr,
}
