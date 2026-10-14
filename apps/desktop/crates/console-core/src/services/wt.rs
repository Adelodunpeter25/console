//! WebTransport service methods (QUIC migration): chat runs, terminals and
//! watches over one QUIC connection.
//!
//! [`WtService`] owns a single [`WtSession`] (one
//! `CONNECT /wt/v1/connect?token=...` handshake) and multiplexes streams on
//! it. Every method here is the WebTransport twin of an existing SSE/WS
//! method and returns the same item type with the same stream shape, so the
//! transport swap at a call site is a one-line change — only framing differs,
//! since WT payloads are the legacy bodies byte for byte.
//!
//! Unary control ops (abort/answer/approve/queue/steer) stay on REST for now:
//! they have no stream to ride.

use crate::net::{WtClient, WtSession, WtStreamKind, WtStreamReader, WtStreamWriter, WtTls};
use crate::services::auth::AuthService;
use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use futures_util::Stream;
use serde::de::DeserializeOwned;
use std::pin::Pin;
use std::sync::Arc;

/// Binary terminal output frames carry this tag byte before the raw PTY
/// bytes, so a binary stream never collides with the protojson text frames.
const TERMINAL_OUTPUT_TAG: u8 = 0x01;

/// Streams shared by one device over one QUIC connection. Cheap to clone —
/// every clone shares the same connection, which outlives the individual
/// streams.
#[derive(Clone)]
pub struct WtService {
    session: Arc<WtSession>,
}

impl WtService {
    /// Mint a handshake token and connect one WT session.
    ///
    /// `origin` defaults to the HTTP base URL with the scheme upgraded to
    /// `https` (production serves HTTP/3 on the API's own origin);
    /// `make dev-server-wt` listens on its own port, so dev passes
    /// `https://127.0.0.1:14443` explicitly. `tls` decides how the server
    /// certificate is trusted — platform roots, or pinned hashes for the
    /// self-signed dev certificate.
    pub async fn connect(
        transport: &HttpTransport,
        origin: Option<&str>,
        tls: WtTls,
    ) -> Result<Self> {
        let token = AuthService::new(transport.clone())
            .mint_api_token(&device_id())
            .await?;
        let base_url = match origin {
            Some(origin) => origin.to_string(),
            None => upgrade_to_https(&transport.base_url().await),
        };
        let session = WtClient::connect(&base_url, &token, tls)
            .await
            .with_context(|| format!("Failed to open WebTransport session at {base_url}"))?;
        Ok(Self {
            session: Arc::new(session),
        })
    }

    /// Close the connection. Streams unwind on the same cause.
    pub fn close(&self, reason: &str) {
        self.session.close(reason.as_bytes());
    }

    /// Start an agent run and stream its events — the WebTransport twin of
    /// [`RunService::stream_prompt`].
    ///
    /// The first item may be an error: the server answers a rejected start
    /// (unknown session, bad params) as an `error` frame on the accepted
    /// stream, since HTTP statuses are gone by then.
    pub async fn start_run(
        &self,
        session_id: &str,
        prompt: &RunPromptDto,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<AgentSessionEvent>> + Send>>> {
        let request = console_proto::WtChatRunRequest {
            session_id: session_id.into(),
            request: Some(console_proto::wt_chat_run_request::Request::Start(
                chat_run_start(prompt),
            )),
        };
        self.chat_stream(session_id, "start", &request).await
    }

    /// Attach to a run already in flight — the WebTransport twin of
    /// [`RunService::attach_run_stream`]. `since` replays frames newer than
    /// that hub sequence; `None` replays nothing and waits for live frames.
    pub async fn attach_run(
        &self,
        session_id: &str,
        since: Option<u64>,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<AgentSessionEvent>> + Send>>> {
        let request = console_proto::WtChatRunRequest {
            session_id: session_id.into(),
            request: Some(console_proto::wt_chat_run_request::Request::Attach(
                console_proto::WtChatRunAttach {
                    since: since.map(|seq| seq as i64),
                },
            )),
        };
        self.chat_stream(session_id, "attach", &request).await
    }

    /// Open a chat run stream and send the request frame that selects
    /// start vs attach.
    async fn chat_stream(
        &self,
        session_id: &str,
        request_type: &str,
        request: &console_proto::WtChatRunRequest,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<AgentSessionEvent>> + Send>>> {
        let payload =
            serde_json::to_vec(request).context("Failed to encode WT chat run request")?;
        let (_writer, reader) = self
            .session
            .open_stream_with_request(WtStreamKind::ChatRun, session_id, request_type, payload)
            .await?;
        // Chat frames arrive in SSE body form: a tagged `AgentSessionEvent`
        // whose `type` is the frame name, so the payload decodes directly
        // and only error frames need special handling.
        Ok(wt_events(reader, FrameFilter::Any))
    }

    /// Spawn a terminal — the WebTransport twin of [`TerminalService::spawn`].
    ///
    /// Every stream spawns fresh (a PTY cannot outlive its session), which
    /// matches the WS upgrade. `binary` selects the raw-bytes output path;
    /// the default text mode is decoded server-side so multibyte sequences
    /// split across coalesced frames survive.
    pub async fn spawn_terminal(
        &self,
        params: &TerminalSpawnParams,
        binary: bool,
    ) -> Result<WtTerminalHandle> {
        let request = console_proto::WtTerminalRequest {
            cwd: params.cwd.clone(),
            shell: params.shell.clone(),
            label: params.label.clone(),
            cols: params.cols.map(i32::from),
            rows: params.rows.map(i32::from),
            binary,
        };
        let payload =
            serde_json::to_vec(&request).context("Failed to encode WT terminal request")?;
        let (writer, reader) = self
            .session
            .open_stream_with_request(WtStreamKind::Terminal, "", "spawn", payload)
            .await?;
        Ok(WtTerminalHandle {
            writer,
            reader,
            binary,
        })
    }

    /// Watch a filesystem path — the WebTransport twin of
    /// [`FsService::watch_events`].
    pub async fn watch_fs(
        &self,
        path: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<FsChangeEvent>> + Send>>> {
        self.watch(
            console_proto::WtWatchRequest {
                target: watch_target(console_proto::WtWatchTarget::Fs),
                path: Some(path.into()),
                ..Default::default()
            },
            "fsChange",
        )
        .await
    }

    /// Watch git status — the WebTransport twin of
    /// [`GitService::watch_status`]. Mirrors the SSE route's leading-edge
    /// debounce, so continuous build/push events fold into one refresh.
    pub async fn watch_git_status(
        &self,
        path: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<GitStatusSummary>> + Send>>> {
        self.watch(
            console_proto::WtWatchRequest {
                target: watch_target(console_proto::WtWatchTarget::GitStatus),
                path: Some(path.into()),
                ..Default::default()
            },
            "gitStatus",
        )
        .await
    }

    /// Watch forwarded ports — the WebTransport twin of
    /// [`PortService::watch`]. Each item is a full snapshot, exactly like the
    /// SSE route.
    pub async fn watch_ports(
        &self,
        project_id: Option<&str>,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<Vec<ForwardedPort>>> + Send>>> {
        self.watch(
            console_proto::WtWatchRequest {
                target: watch_target(console_proto::WtWatchTarget::Ports),
                project_id: project_id.map(Into::into),
                ..Default::default()
            },
            "ports",
        )
        .await
    }

    /// Watch notification banners — the WebTransport twin of
    /// [`NotificationService::stream`].
    pub async fn watch_notifications(
        &self,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<NotificationEvent>> + Send>>> {
        self.watch(
            console_proto::WtWatchRequest {
                target: watch_target(console_proto::WtWatchTarget::Notifications),
                ..Default::default()
            },
            "notification",
        )
        .await
    }

    /// Subscribe to a script run's live events — the WebTransport twin of
    /// [`ProjectScriptsService::stream_run`]. The server ends the stream
    /// after the final exit event.
    pub async fn watch_script_run(
        &self,
        project_id: &str,
        run_id: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<ScriptRunEvent>> + Send>>> {
        self.watch(
            console_proto::WtWatchRequest {
                target: watch_target(console_proto::WtWatchTarget::ScriptRun),
                project_id: Some(project_id.into()),
                run_id: Some(run_id.into()),
                ..Default::default()
            },
            "",
        )
        .await
    }

    /// Open a watch stream and pump `event`-named frames as `T`. An empty
    /// `event` accepts every name (the script run stream reuses one message
    /// type across status/output/exit frames).
    async fn watch<T>(
        &self,
        request: console_proto::WtWatchRequest,
        event: &str,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<T>> + Send>>>
    where
        T: DeserializeOwned + Send + 'static,
    {
        let payload = serde_json::to_vec(&request).context("Failed to encode WT watch request")?;
        let (_writer, reader) = self
            .session
            .open_stream_with_request(WtStreamKind::Watch, "", "watch", payload)
            .await?;
        let filter = if event.is_empty() {
            FrameFilter::Any
        } else {
            FrameFilter::Named(event.to_string())
        };
        Ok(wt_events(reader, filter))
    }
}

/// A terminal event on a WT terminal stream: the WS server messages under
/// their own names, so the desktop terminal backend can consume either
/// transport's frames with the same match.
#[derive(Debug)]
pub enum WtTerminalEvent {
    Spawned(console_proto::TerminalSpawned),
    /// Text mode: the server already decoded the PTY bytes to UTF-8, so
    /// multibyte sequences split across coalesced frames survive.
    Output(console_proto::TerminalOutput),
    /// Binary mode: raw PTY bytes, still needing the terminal emulator's own
    /// UTF-8 decoding.
    OutputBytes(Vec<u8>),
    Exit(console_proto::TerminalExit),
    Error(String),
}

/// A spawned terminal on its own QUIC bidi stream. Both directions are live,
/// so the halves are split: `send_*` from anywhere (they are `&self`, backed
/// by a mutex around the one send stream), `next_event` from the reader.
pub struct WtTerminalHandle {
    writer: WtStreamWriter,
    reader: WtStreamReader,
    binary: bool,
}

impl WtTerminalHandle {
    /// Next server message, or `None` once the terminal is gone (the server
    /// kills the PTY and closes the stream when either side goes away).
    ///
    /// Terminal frames carry the WS shapes under their oneof field names, so
    /// every protojson frame decodes as a `TerminalServerMessage`, while a
    /// binary stream's `output` frames hold raw bytes instead. See
    /// [`terminal_event`].
    pub async fn next_event(&mut self) -> Result<Option<WtTerminalEvent>> {
        let Some(frame) = self.reader.next_frame().await? else {
            return Ok(None);
        };
        terminal_event(self.binary, &frame).map(Some)
    }

    /// Write raw PTY bytes. Binary mode is not required — input is always
    /// raw bytes, just like the WS path.
    pub async fn send_input(&self, data: &[u8]) -> Result<()> {
        self.send("input", data.to_vec()).await
    }

    /// Resize the PTY. Rides the stream in v1 rather than a parallel unary
    /// channel.
    pub async fn resize(&self, cols: u16, rows: u16) -> Result<()> {
        let payload = serde_json::to_vec(&console_proto::TerminalResize {
            cols: i32::from(cols),
            rows: i32::from(rows),
        })
        .context("Failed to encode WT terminal resize")?;
        self.send("resize", payload).await
    }

    /// Kill the terminal. The server reaps the PTY and closes the stream.
    pub async fn kill(&self) -> Result<()> {
        self.send("kill", Vec::new()).await
    }

    async fn send(&self, frame_type: &str, payload: Vec<u8>) -> Result<()> {
        self.writer
            .send_frame(&console_proto::WtFrame {
                seq: 0,
                r#type: frame_type.into(),
                payload,
            })
            .await
    }
}

/// Decode one terminal frame. `binary` is the mode the stream was spawned
/// in, which is the only thing distinguishing the two output encodings.
///
/// Every protojson frame is a whole [`console_proto::TerminalServerMessage`]
/// under its oneof field name. Decoding an `output` payload straight into the
/// inner `TerminalOutput` would quietly succeed with an empty string (the
/// pbjson impl ignores unknown fields), so the wrapper is always unwrapped
/// here.
pub fn terminal_event(binary: bool, frame: &console_proto::WtFrame) -> Result<WtTerminalEvent> {
    match frame.r#type.as_str() {
        "output" if binary => {
            let bytes = frame
                .payload
                .strip_prefix(&[TERMINAL_OUTPUT_TAG])
                .ok_or_else(|| anyhow!("WT binary output frame is missing its tag byte."))?;
            Ok(WtTerminalEvent::OutputBytes(bytes.to_vec()))
        }
        name @ ("output" | "spawned" | "exit" | "error") => {
            let message: console_proto::TerminalServerMessage =
                serde_json::from_slice(&frame.payload)
                    .with_context(|| format!("Failed to decode WT terminal {name} frame"))?;
            match message.event {
                Some(console_proto::terminal_server_message::Event::Spawned(spawned)) => {
                    Ok(WtTerminalEvent::Spawned(spawned))
                }
                Some(console_proto::terminal_server_message::Event::Output(output)) => {
                    Ok(WtTerminalEvent::Output(output))
                }
                Some(console_proto::terminal_server_message::Event::Exit(exit)) => {
                    Ok(WtTerminalEvent::Exit(exit))
                }
                Some(console_proto::terminal_server_message::Event::Error(error)) => {
                    Ok(WtTerminalEvent::Error(error.message))
                }
                None => anyhow::bail!("WT terminal {name} frame carried no event."),
            }
        }
        other => anyhow::bail!("Unknown WT terminal frame type: {other}"),
    }
}

/// Which frames a watch/chat pump accepts. Mirrors the SSE consumers'
/// event-name filter; `Any` takes every name, which is what chat runs (one
/// message type, many event names) and script runs (one type reused across
/// status/output/exit) need.
enum FrameFilter {
    Any,
    Named(String),
}

/// Turn a pull-loop over the read half into a `Stream`, so the WT methods
/// hand back the same item shape as their SSE siblings. The stream ends at
/// end-of-stream or on the first error (an error frame, a truncated frame,
/// or a failed decode) — the error is yielded, then the stream stops, rather
/// than repeating a failure that cannot recover.
fn wt_events<T>(
    reader: WtStreamReader,
    filter: FrameFilter,
) -> Pin<Box<dyn Stream<Item = Result<T>> + Send>>
where
    T: DeserializeOwned + Send + 'static,
{
    Box::pin(futures_util::stream::unfold(
        Some((reader, filter)),
        |state| async move {
            let (mut reader, filter) = state?;
            let next = match &filter {
                FrameFilter::Any => reader.next_payload::<T>().await,
                FrameFilter::Named(name) => reader.next_event::<T>(name).await,
            };
            match next {
                Ok(Some(item)) => Some((Ok(item), Some((reader, filter)))),
                Ok(None) => None,
                Err(error) => Some((Err(error), None)),
            }
        },
    ))
}

/// Map a run prompt onto the WT start payload field for field, matching the
/// REST `/run` body the server reads.
pub fn chat_run_start(prompt: &RunPromptDto) -> console_proto::WtChatRunStart {
    console_proto::WtChatRunStart {
        text: prompt.prompt.clone(),
        context_files: prompt.context_files.clone().unwrap_or_default(),
        model_id: prompt.model_id.clone(),
        provider: prompt.provider.clone(),
        approval_mode: prompt.approval_mode.clone(),
        thinking_level: prompt
            .thinking_level
            .map(|level| level.as_str().to_string()),
        attachments: prompt
            .attachments
            .iter()
            .flatten()
            .map(|attachment| console_proto::ImageAttachment {
                data: attachment.data.clone(),
                mime_type: attachment.mime_type.clone(),
            })
            .collect(),
        annotations: Vec::new(),
    }
}

fn watch_target(target: console_proto::WtWatchTarget) -> i32 {
    target as i32
}

/// HTTP base URL as a WebTransport origin: the same host and port over TLS,
/// which is where production serves HTTP/3.
pub fn upgrade_to_https(base_url: &str) -> String {
    match base_url.strip_prefix("http://") {
        Some(rest) => format!("https://{rest}"),
        None => base_url.to_string(),
    }
}

/// Stable device id for the handshake token, so repeated connects in one app
/// run share one server-side identity. Process-scoped by design: the token
/// itself is short-lived, and the server binds the id to the QUIC connection
/// rather than persisting it.
fn device_id() -> String {
    static DEVICE_ID: std::sync::OnceLock<String> = std::sync::OnceLock::new();
    DEVICE_ID
        .get_or_init(|| {
            let nanos = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|elapsed| elapsed.as_nanos())
                .unwrap_or_default();
            format!("desktop-{}-{nanos}", std::process::id())
        })
        .clone()
}
