use serde::{Deserialize, Serialize};

/// Query params used to spawn a new PTY when the WebSocket connects.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalSpawnParams {
    pub cwd: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub shell: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cols: Option<u16>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rows: Option<u16>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub label: Option<String>,
}

/// Confirmation sent from the server once the PTY has spawned.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalSpawnedEvent {
    #[serde(rename = "type")]
    pub kind: String,
    pub id: String,
    pub pid: u32,
    pub shell: String,
    pub cwd: String,
    pub cols: u16,
    pub rows: u16,
}

/// PTY output pushed from the server to the client.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalOutputEvent {
    #[serde(rename = "type")]
    pub kind: String,
    pub data: String,
}

/// PTY exited (shell process terminated).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalExitEvent {
    #[serde(rename = "type")]
    pub kind: String,
    pub code: Option<i32>,
}

/// Terminal-level failure (spawn error, kill error, unknown frame).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalErrorEvent {
    #[serde(rename = "type")]
    pub kind: String,
    pub message: String,
}

/// Union of all messages the server may send to the client.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum TerminalServerMessage {
    #[serde(rename = "spawned")]
    Spawned {
        id: String,
        pid: u32,
        shell: String,
        cwd: String,
        cols: u16,
        rows: u16,
    },
    #[serde(rename = "output")]
    Output { data: String },
    #[serde(rename = "exit")]
    Exit { code: Option<i32> },
    #[serde(rename = "error")]
    Error { message: String },
}

impl TerminalServerMessage {
    pub fn as_event_payload_by_type(&self) -> (&'static str, serde_json::Value) {
        match self {
            TerminalServerMessage::Spawned {
                id,
                pid,
                shell,
                cwd,
                cols,
                rows,
            } => (
                "spawned",
                serde_json::json!({
                    "type": "spawned",
                    "id": id,
                    "pid": pid,
                    "shell": shell,
                    "cwd": cwd,
                    "cols": cols,
                    "rows": rows,
                }),
            ),
            TerminalServerMessage::Output { data } => {
                ("output", serde_json::json!({ "type": "output", "data": data }))
            }
            TerminalServerMessage::Exit { code } => {
                ("exit", serde_json::json!({ "type": "exit", "code": code }))
            }
            TerminalServerMessage::Error { message } => {
                ("error", serde_json::json!({ "type": "error", "message": message }))
            }
        }
    }
}

/// Client → server: keystrokes / pasted input to feed into the PTY.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalInputMessage {
    #[serde(rename = "type")]
    pub kind: String,
    pub data: String,
}

/// Client → server: terminal viewport resize.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalResizeMessage {
    #[serde(rename = "type")]
    pub kind: String,
    pub cols: u16,
    pub rows: u16,
}

/// Client → server: kill/close the PTY.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalKillMessage {
    #[serde(rename = "type")]
    pub kind: String,
}

/// Union of all messages the client may send to the server.
#[derive(Debug, Clone, Serialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum TerminalClientMessage {
    #[serde(rename = "input")]
    Input { data: String },
    #[serde(rename = "resize")]
    Resize { cols: u16, rows: u16 },
    #[serde(rename = "kill")]
    Kill,
}