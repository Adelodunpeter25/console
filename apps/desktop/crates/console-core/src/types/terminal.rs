use serde::{Deserialize, Serialize};

/// Opaque terminal id issued by the server after `spawned`.
pub type TerminalId = String;

/// Spawn params — query string for `GET /api/terminals?cwd=&cols=&rows=`.
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
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

impl Default for TerminalSpawnParams {
    fn default() -> Self {
        Self {
            cwd: String::new(),
            shell: None,
            cols: Some(80),
            rows: Some(24),
            label: None,
        }
    }
}

/// Logical size of a terminal viewport.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct TerminalSize {
    pub cols: u16,
    pub rows: u16,
}

impl TerminalSize {
    pub fn new(cols: u16, rows: u16) -> Self {
        Self { cols, rows }
    }
}

impl Default for TerminalSize {
    fn default() -> Self {
        Self { cols: 80, rows: 24 }
    }
}

/// Lifecycle of a PTY on the server.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum TerminalStatus {
    Spawning,
    Running,
    Exited,
    Error,
}

/// Persisted-ish view of a terminal session (mirrors mobile `TerminalRecord`).
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct TerminalRecord {
    pub id: TerminalId,
    pub project_id: Option<String>,
    pub status: TerminalStatus,
    pub pid: Option<u32>,
    pub shell: Option<String>,
    pub cwd: Option<String>,
    pub cols: u16,
    pub rows: u16,
    pub error: Option<String>,
    /// Monotonic nonce bumped on every server frame — useful for UI revision tracking.
    pub revision: u64,
}

/// Server → client frames (mirrors `@console/types` `TerminalServerMessage`).
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum TerminalServerMessage {
    #[serde(rename = "spawned")]
    Spawned {
        id: TerminalId,
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

/// Client → server frames.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum TerminalClientMessage {
    #[serde(rename = "input")]
    Input { data: String },
    #[serde(rename = "resize")]
    Resize { cols: u16, rows: u16 },
    #[serde(rename = "kill")]
    Kill,
}

// ── Grid snapshot — backend-agnostic view for the UI ─────────────────────

/// One cell in a rendered grid snapshot.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct TerminalCell {
    pub c: char,
    pub fg: Option<TerminalColor>,
    pub bg: Option<TerminalColor>,
    pub flags: TerminalCellFlags,
}

/// Simple RGB color (mirrors alacritty's `Rgb` but backend-agnostic).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct TerminalColor {
    pub r: u8,
    pub g: u8,
    pub b: u8,
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct TerminalCellFlags {
    pub bold: bool,
    pub dim: bool,
    pub italic: bool,
    pub underline: bool,
    pub inverse: bool,
    pub hidden: bool,
    pub strike: bool,
    pub blink: bool,
    pub wrapline: bool,
}

/// One row of cells.
pub type TerminalRow = Vec<TerminalCell>;

/// Full viewport snapshot, top-to-bottom lines.
#[derive(Clone, Debug)]
pub struct TerminalGridSnapshot {
    pub rows: Vec<TerminalRow>,
    pub cursor: CursorPosition,
    pub size: TerminalSize,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct CursorPosition {
    pub col: u16,
    pub row: u16,
    pub visible: bool,
}

// ── Backend trait — VT processing abstraction ─────────────────────────────

/// Backend-agnostic VT handler. `alacritty` and a future `ghostty` impl can both satisfy this.
///
/// Implementors own the grid state: `advance` feeds server output bytes (ANSI escapes included),
/// `resize` updates the viewport, `snapshot` returns the current rendered rows for the UI.
pub trait TerminalBackend: Send {
    fn new(size: TerminalSize) -> Self
    where
        Self: Sized;
    fn resize(&mut self, size: TerminalSize);
    fn advance(&mut self, data: &str);
    fn snapshot(&self) -> TerminalGridSnapshot;
    fn size(&self) -> TerminalSize;
    /// Serialize input for the wire (identity for now, but ghostty may need transforms).
    fn encode_input(data: &str) -> String
    where
        Self: Sized,
    {
        data.to_string()
    }
}
