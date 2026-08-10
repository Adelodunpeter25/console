use std::collections::HashMap;

use futures_util::{SinkExt, StreamExt};
use tauri::{AppHandle, Emitter};
use tokio::sync::Mutex;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::{connect_async, MaybeTlsStream, WebSocketStream};

use once_cell::sync::Lazy;

use crate::error::{AppError, AppResult};
use crate::models::terminal::{
    TerminalClientMessage, TerminalServerMessage, TerminalSpawnParams, TerminalSpawnedEvent,
};

type WsStream = WebSocketStream<MaybeTlsStream<tokio::net::TcpStream>>;
type WsSink = futures_util::stream::SplitSink<WsStream, Message>;

/// Registry of live terminal sockets keyed by terminal id, so the input /
/// resize / kill commands can write into the right connection.
static TERMINALS: Lazy<Mutex<HashMap<String, WsSink>>> = Lazy::new(|| Mutex::new(HashMap::new()));

fn ws_terminal_url(params: &TerminalSpawnParams) -> String {
    // Server URL like http://localhost:3000 → ws://localhost:3000.
    let base = crate::config::get_server_url().replace("http", "ws");
    let cwd = url::form_urlencoded::byte_serialize(params.cwd.as_bytes()).collect::<String>();
    let shell = params
        .shell
        .as_deref()
        .map(|s| format!("&shell={}", url::form_urlencoded::byte_serialize(s.as_bytes()).collect::<String>()))
        .unwrap_or_default();
    format!(
        "{base}/api/terminals?cwd={cwd}&cols={}&rows={}{shell}",
        params.cols.unwrap_or(80),
        params.rows.unwrap_or(24),
    )
}

/// Connect to the server's terminal WebSocket, spawn a PTY in `cwd`, and await
/// the `spawned` confirmation. Returns the terminal id plus pid/shell metadata.
pub async fn open_terminal(
    app: AppHandle,
    params: &TerminalSpawnParams,
) -> AppResult<TerminalSpawnedEvent> {
    let url = ws_terminal_url(params);
    let (ws, _resp) = connect_async(url.as_str())
        .await
        .map_err(|e| AppError::Sse(format!("terminal ws connect failed: {e}")))?;

    let (sink, mut stream) = ws.split();

    // Await the spawn confirmation (the server sends "spawned" first).
    let spawned: TerminalSpawnedEvent = loop {
        match stream.next().await {
            Some(Ok(Message::Text(text))) => {
                match serde_json::from_str::<TerminalServerMessage>(&text) {
                    Ok(TerminalServerMessage::Spawned {
                        id,
                        pid,
                        shell,
                        cwd,
                        cols,
                        rows,
                    }) => {
                        break TerminalSpawnedEvent {
                            kind: "spawned".to_string(),
                            id,
                            pid,
                            shell,
                            cwd,
                            cols,
                            rows,
                        };
                    }
                    Ok(TerminalServerMessage::Error { message }) => {
                        return Err(AppError::Sse(format!("terminal spawn error: {message}")));
                    }
                    // Ignore stray output/exit frames before spawn confirms.
                    _ => continue,
                }
            }
            Some(Ok(_)) => continue,
            Some(Err(e)) => return Err(AppError::Sse(format!("terminal ws error: {e}"))),
            None => return Err(AppError::Sse("terminal ws closed before spawn".to_string())),
        }
    };

    TERMINALS.lock().await.insert(spawned.id.clone(), sink);

    // Drain the socket: classify frames and hand them to the frontend via a
    // per-terminal Tauri event channel (terminal-events:{id}).
    let id = spawned.id.clone();
    let _reader = tokio::spawn(async move {
        let channel = format!("terminal-events:{id}");
        while let Some(msg) = stream.next().await {
            let text = match msg {
                Ok(Message::Text(t)) => t,
                Ok(Message::Binary(b)) => String::from_utf8_lossy(&b).to_string(),
                _ => continue,
            };
            let parsed = serde_json::from_str::<TerminalServerMessage>(&text);
            match parsed {
                Ok(event) => {
                    let (_event_name, payload) = event.as_event_payload_by_type();
                    let _ = app.emit(&channel, payload);
                    if matches!(event, TerminalServerMessage::Exit { .. }) {
                        break;
                    }
                }
                Err(e) => {
                    eprintln!("terminal frame parse error: {e}; raw={text}");
                }
            }
        }
        TERMINALS.lock().await.remove(&id);
    });

    Ok(spawned)
}

/// Send keystrokes / pasted input into the PTY.
pub async fn write_input(id: &str, data: &str) -> AppResult<()> {
    let mut registry = TERMINALS.lock().await;
    let sink = registry
        .get_mut(id)
        .ok_or_else(|| AppError::Sse(format!("no active terminal: {id}")))?;
    let frame = serde_json::to_string(&TerminalClientMessage::Input { data: data.to_string() })?;
    sink.send(Message::Text(frame.into()))
        .await
        .map_err(|e| AppError::Sse(format!("terminal write failed: {e}")))?;
    Ok(())
}

/// Resize the PTY viewport.
pub async fn resize(id: &str, cols: u16, rows: u16) -> AppResult<()> {
    let mut registry = TERMINALS.lock().await;
    let sink = registry
        .get_mut(id)
        .ok_or_else(|| AppError::Sse(format!("no active terminal: {id}")))?;
    let frame = serde_json::to_string(&TerminalClientMessage::Resize { cols, rows })?;
    sink.send(Message::Text(frame.into()))
        .await
        .map_err(|e| AppError::Sse(format!("terminal resize failed: {e}")))?;
    Ok(())
}

/// Kill the PTY and drop the socket. Idempotent.
pub async fn kill(id: &str) -> AppResult<()> {
    let mut registry = TERMINALS.lock().await;
    if let Some(mut sink) = registry.remove(id) {
        let frame = serde_json::to_string(&TerminalClientMessage::Kill)?;
        let _ = sink
            .send(Message::Text(frame.into()))
            .await
            .map_err(|e| AppError::Sse(format!("terminal kill failed: {e}")));
        let _ = sink.close().await;
    }
    Ok(())
}

/// Remove a terminal from the registry without sending frames (used on
/// uncontrolled teardown paths).
pub async fn forget(id: &str) {
    TERMINALS.lock().await.remove(id);
}