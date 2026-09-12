//! Project run-script models mirroring the server project-scripts API.
//!
//! Scripts are defined by the project's `console.toml`; the server owns
//! parsing, validation, and execution. The desktop only ever references
//! scripts by id — it never sends a raw command.

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
