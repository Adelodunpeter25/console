use tauri::AppHandle;

use crate::api::terminal as terminal_api;
use crate::error::AppResult;
use crate::models::terminal::{TerminalSpawnParams, TerminalSpawnedEvent};

#[tauri::command]
pub async fn terminal_open(
    app: AppHandle,
    cwd: String,
    shell: Option<String>,
    cols: Option<u16>,
    rows: Option<u16>,
    label: Option<String>,
) -> AppResult<TerminalSpawnedEvent> {
    let params = TerminalSpawnParams {
        cwd,
        shell,
        cols,
        rows,
        label,
    };
    terminal_api::open_terminal(app, &params).await
}

#[tauri::command]
pub async fn terminal_input(id: String, data: String) -> AppResult<()> {
    terminal_api::write_input(&id, &data).await
}

#[tauri::command]
pub async fn terminal_resize(id: String, cols: u16, rows: u16) -> AppResult<()> {
    terminal_api::resize(&id, cols, rows).await
}

#[tauri::command]
pub async fn terminal_kill(id: String) -> AppResult<()> {
    terminal_api::kill(&id).await
}