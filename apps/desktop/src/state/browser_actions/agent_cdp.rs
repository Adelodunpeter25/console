//! agent-browser (CDP) execution path for browser actions on CEF tabs.
//!
//! On a CEF build, page-level actions (`snapshot`, `click`, `type`, `run_js`,
//! `get_content`, `wait_for`, `screenshot`) run through agent-browser attached
//! to the embedded Chromium, addressed by the tab's CDP target id. Tab
//! lifecycle (`tabs`, `navigate`, `switch_tab`, `close_tab`) stays with the
//! code in the parent module. When agent-browser is unavailable (or the tab is
//! not a CEF tab) the parent falls back to the in-page script path.
//! See `docs/plan/agent-browser-cef.md`.

use std::path::PathBuf;
use std::time::{Duration, Instant};

use console_core::BrowserActionRequest;
use console_ui::browser::BrowserView;
use console_ui::browser::agent_driver::{self, Commands, DriveError, DriveStep};
use gpui::{App, Context, Entity};

use super::{WAIT_FOR_DEFAULT_MS, WAIT_FOR_MAX_MS};
use crate::state::app::ConsoleDesktopApp;

/// Upper bound for one agent-browser call (a `wait_for` adds its own timeout).
const DRIVE_TIMEOUT: Duration = Duration::from_secs(60);

enum Kind {
    Snapshot,
    Interact,
    Script,
    Content,
    Wait,
    Screenshot(PathBuf),
}

struct Plan {
    commands: Commands,
    timeout: Duration,
    kind: Kind,
}

fn non_empty(value: &Option<String>) -> Option<&str> {
    value.as_deref().filter(|s| !s.trim().is_empty())
}

/// Map a Console browser action onto agent-browser commands. `None` means the
/// action is not handled here (or lacks required input): use the old path.
fn plan_for(req: &BrowserActionRequest) -> Option<Plan> {
    let plan = |commands: Commands, kind: Kind| Plan {
        commands,
        timeout: DRIVE_TIMEOUT,
        kind,
    };
    match req.action.as_str() {
        "snapshot" => Some(plan(agent_driver::snapshot_commands(), Kind::Snapshot)),
        "click" => {
            let element_ref = non_empty(&req.element_ref)?;
            Some(plan(agent_driver::click_commands(element_ref), Kind::Interact))
        }
        "type" => {
            let element_ref = non_empty(&req.element_ref)?;
            let text = req.text.as_deref()?;
            Some(plan(
                agent_driver::type_commands(element_ref, text, req.submit.unwrap_or(false)),
                Kind::Interact,
            ))
        }
        "run_js" => {
            let script = non_empty(&req.script)?;
            Some(plan(agent_driver::run_js_commands(script), Kind::Script))
        }
        "get_content" => Some(plan(
            agent_driver::content_commands(req.selector.as_deref()),
            Kind::Content,
        )),
        "wait_for" => {
            let timeout_ms = req
                .timeout_ms
                .unwrap_or(WAIT_FOR_DEFAULT_MS)
                .clamp(100, WAIT_FOR_MAX_MS);
            let commands = agent_driver::wait_commands(
                non_empty(&req.selector),
                non_empty(&req.url_contains),
                non_empty(&req.text),
                timeout_ms,
            );
            if commands.is_empty() {
                return None;
            }
            Some(Plan {
                commands,
                timeout: Duration::from_millis(timeout_ms) + DRIVE_TIMEOUT,
                kind: Kind::Wait,
            })
        }
        "screenshot" => {
            let safe_id: String = req
                .request_id
                .chars()
                .filter(|c| c.is_ascii_alphanumeric() || *c == '-' || *c == '_')
                .collect();
            let path = std::env::temp_dir().join(format!("console-shot-{safe_id}.png"));
            let commands = agent_driver::screenshot_commands(&path.to_string_lossy());
            Some(plan(commands, Kind::Screenshot(path)))
        }
        _ => None,
    }
}

fn tab_header(view: &Entity<BrowserView>, tab_id: &str, cx: &App) -> String {
    let bv = view.read(cx);
    format!(
        "Tab: {}\nTitle: {}\nURL: {}",
        tab_id,
        bv.tab_label().unwrap_or_default(),
        bv.current_url().unwrap_or_default()
    )
}

/// Errors about stale or unknown refs get the same hint the script path gives.
fn with_ref_hint(error: String) -> String {
    let lower = error.to_lowercase();
    if lower.contains("ref") || lower.contains("locate") {
        format!("{error}\n(Take a new snapshot and use a fresh ref.)")
    } else {
        error
    }
}

struct Finished {
    steps: Result<Vec<DriveStep>, DriveError>,
    /// PNG bytes for a screenshot plan, read (and deleted) off the UI thread.
    image: Option<Vec<u8>>,
}

impl ConsoleDesktopApp {
    /// Runs `req` through agent-browser when `view` is a CEF tab that agent-browser
    /// can drive. Returns `true` when it took the request (it resolves it itself),
    /// `false` when the caller should use the in-page script path.
    #[allow(clippy::too_many_arguments)]
    pub(super) fn try_agent_browser(
        &mut self,
        sess_id: &str,
        req: &BrowserActionRequest,
        tab_id: &str,
        view: &Entity<BrowserView>,
        note: Option<String>,
        retries_left: u32,
        cx: &mut Context<Self>,
    ) -> bool {
        if !agent_driver::is_ready() {
            return false;
        }
        let Some(target_id) = view.read(cx).cdp_target_id() else {
            return false;
        };
        let Some(plan) = plan_for(req) else {
            return false;
        };
        let session =
            agent_driver::session_name(std::env::var("CONSOLE_CEF_DATA_DIR").ok().as_deref());

        let sess_id = sess_id.to_string();
        let req = req.clone();
        let tab_id = tab_id.to_string();
        let view = view.clone();
        cx.spawn(async move |this, cx| {
            let started = Instant::now();
            let Plan {
                commands,
                timeout,
                kind,
            } = plan;
            let screenshot_path = match &kind {
                Kind::Screenshot(path) => Some(path.clone()),
                _ => None,
            };
            let finished = cx
                .background_executor()
                .spawn(async move {
                    let steps = agent_driver::drive(&session, &target_id, commands, timeout);
                    let image = screenshot_path.and_then(|path| {
                        let bytes = std::fs::read(&path).ok();
                        let _ = std::fs::remove_file(&path);
                        bytes
                    });
                    Finished { steps, image }
                })
                .await;
            let elapsed_ms = started.elapsed().as_millis();
            let _ = this.update(cx, |app, cx| {
                app.finish_agent_browser(
                    &sess_id,
                    req,
                    &tab_id,
                    &view,
                    note,
                    &kind,
                    finished,
                    elapsed_ms,
                    retries_left,
                    cx,
                );
            });
        })
        .detach();
        true
    }

    #[allow(clippy::too_many_arguments)]
    fn finish_agent_browser(
        &mut self,
        sess_id: &str,
        req: BrowserActionRequest,
        tab_id: &str,
        view: &Entity<BrowserView>,
        note: Option<String>,
        kind: &Kind,
        finished: Finished,
        elapsed_ms: u128,
        retries_left: u32,
        cx: &mut Context<Self>,
    ) {
        let request_id = req.request_id.clone();
        let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
        let fail = |app: &mut Self, message: String, cx: &mut Context<Self>| {
            app.resolve_browser_action_result(
                sess_id,
                request_id.clone(),
                None,
                Some(format!("{note}{message}")),
                cx,
            );
        };

        let steps = match finished.steps {
            Ok(steps) => steps,
            // Could not use agent-browser at all (e.g. it could not be
            // installed): quietly use the in-page script path instead.
            Err(DriveError::Unavailable(reason)) if !agent_driver::is_ready() => {
                log::info!("agent-browser unavailable ({reason}); using in-page scripts");
                self.handle_browser_action_attempt(sess_id, req, retries_left, cx);
                return;
            }
            Err(DriveError::TargetGone(reason)) => {
                fail(
                    self,
                    format!(
                        "[tab {tab_id}] That browser tab is no longer available (closed or replaced): {reason}"
                    ),
                    cx,
                );
                return;
            }
            Err(err) => {
                fail(self, format!("[tab {tab_id}] {err}"), cx);
                return;
            }
        };
        if let Some(error) = steps.iter().find_map(|step| step.error.clone()) {
            let header = tab_header(view, tab_id, cx);
            let message = match kind {
                Kind::Wait => format!("{error}\n{header}"),
                _ => format!("[tab {tab_id}] {error}"),
            };
            fail(self, with_ref_hint(message), cx);
            return;
        }

        let body = steps
            .iter()
            .map(|step| step.output.as_str())
            .collect::<Vec<_>>()
            .join("\n");
        let header = tab_header(view, tab_id, cx);
        match kind {
            Kind::Screenshot(_) => match finished.image {
                Some(png) => {
                    use base64::Engine;
                    let b64 = base64::engine::general_purpose::STANDARD.encode(&png);
                    self.resolve_browser_action_with_image(
                        sess_id,
                        request_id,
                        Some(format!("{note}{header}\nScreenshot attached.")),
                        None,
                        Some(b64),
                        cx,
                    );
                }
                None => fail(
                    self,
                    format!("[tab {tab_id}] Screenshot was not produced"),
                    cx,
                ),
            },
            kind => {
                let text = match kind {
                    Kind::Snapshot | Kind::Content => format!("{note}{header}\n\n{body}"),
                    Kind::Interact => format!(
                        "{note}{header}\n\n{body}\n(Take a new snapshot if the page changed.)"
                    ),
                    Kind::Script => format!("{note}Tab: {tab_id}\n{body}"),
                    Kind::Wait => format!("{note}Condition met after {elapsed_ms}ms\n{header}"),
                    Kind::Screenshot(_) => unreachable!("handled above"),
                };
                self.resolve_browser_action_result(sess_id, request_id, Some(text), None, cx);
            }
        }
    }
}
