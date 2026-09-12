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

/// Derive the active shortcut map and conflict set from a script list.
///
/// - `Ok(map)`: shortcut string → script id, with one entry per *unique*
///   shortcut. Conflicts are excluded.
/// - `Err(conflicts)`: shortcut string → set of conflicting script ids,
///   populated when two or more scripts claim the same shortcut.
///
/// Both halves come from a single pass so they stay consistent.
pub fn compute_shortcut_state(
    scripts: &[ProjectScript],
) -> Result<HashMap<String, String>, HashMap<String, HashSet<String>>> {
    let mut groups: HashMap<String, HashSet<String>> = HashMap::new();
    for script in scripts {
        if let Some(shortcut) = script.shortcut.as_deref() {
            groups
                .entry(shortcut.to_string())
                .or_default()
                .insert(script.id.clone());
        }
    }
    let mut map = HashMap::new();
    let mut conflicts = HashMap::new();
    for (shortcut, ids) in groups {
        if ids.len() > 1 {
            conflicts.insert(shortcut, ids);
        } else {
            // Unwrap is safe: `ids` has exactly one entry when len == 1.
            map.insert(shortcut, ids.into_iter().next().unwrap());
        }
    }
    if conflicts.is_empty() {
        Ok(map)
    } else {
        Err(conflicts)
    }
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
