//! Detached background shells: per-session state fed by the run stream's
//! `backgroundWork*` events and the session's work list, plus the Shells
//! inspector tab's selected shell, output polling, and stop action.
//!
//! The open output view subscribes to the server's
//! `/background-work/:id/output/stream` SSE feed: each frame is the new output
//! since the last one, folded into the accumulated text. The feed ends when
//! the shell settles, so there is nothing to poll or stop.

use std::rc::Rc;

use console_core::{BackgroundWork, BackgroundWorkOutput};
use futures_util::StreamExt;
use gpui::Context;

use super::ConsoleDesktopApp;

/// What the Shells tab shows for one session.
#[derive(Clone, Default)]
pub struct ShellOutputState {
    pub work_id: String,
    pub output: Option<BackgroundWorkOutput>,
    pub unavailable: bool,
}

impl ConsoleDesktopApp {
    /// Fold one work record into the session's list (by id).
    pub(crate) fn upsert_background_work(
        &mut self,
        session_id: &str,
        work: BackgroundWork,
        cx: &mut Context<Self>,
    ) {
        let entry = self
            .session_background_work
            .entry(session_id.to_string())
            .or_insert_with(|| Rc::new(Vec::new()));
        let list = Rc::make_mut(entry);
        match list.iter_mut().find(|w| w.id == work.id) {
            Some(existing) => *existing = work,
            None => list.push(work),
        }
        cx.notify();
    }

    /// The session's detached shells that are still running.
    pub(crate) fn running_shells_for_session(&self, session_id: &str) -> Vec<BackgroundWork> {
        self.session_background_work
            .get(session_id)
            .map(|list| list.iter().filter(|w| w.is_running_shell()).cloned().collect())
            .unwrap_or_default()
    }

    /// Load the session's work list so a reopened chat shows shells that were
    /// started before this window attached.
    pub(crate) fn load_background_work(&mut self, session_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let Ok(list) = client.runs.list_background_work(&session_id).await else {
                return;
            };
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.session_background_work
                            .insert(session_id.clone(), Rc::new(list));
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Open the Shells tab on one shell and subscribe to its output.
    pub(crate) fn select_shell(&mut self, session_id: String, work_id: String, cx: &mut Context<Self>) {
        self.shell_output = Some(ShellOutputState {
            work_id: work_id.clone(),
            output: None,
            unavailable: false,
        });
        cx.notify();
        self.stream_shell_output(session_id, work_id, cx);
    }

    /// Back to the shell list.
    pub(crate) fn deselect_shell(&mut self, cx: &mut Context<Self>) {
        self.shell_output = None;
        cx.notify();
    }

    fn stream_shell_output(&mut self, session_id: String, work_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let stream = client
                .runs
                .stream_background_work_output(&session_id, &work_id)
                .await;
            let mut stream = match stream {
                Ok(stream) => stream,
                Err(_) => {
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                if let Some(state) = this.shell_output.as_mut() {
                                    if state.work_id == work_id && state.output.is_none() {
                                        state.unavailable = true;
                                        cx.notify();
                                    }
                                }
                            });
                        }
                    });
                    return;
                }
            };
            while let Some(frame) = stream.next().await {
                let Ok(frame) = frame else { break };
                let still_open = cx.update(|cx| {
                    let Some(app) = entity.upgrade() else {
                        return false;
                    };
                    app.update(cx, |this, cx| {
                        // Stop when the user left this shell or opened another.
                        let Some(state) = this.shell_output.as_mut() else {
                            return false;
                        };
                        if state.work_id != work_id {
                            return false;
                        }
                        state
                            .output
                            .get_or_insert_with(BackgroundWorkOutput::default)
                            .apply_frame(frame);
                        state.unavailable = false;
                        cx.notify();
                        true
                    })
                });
                if !still_open {
                    break;
                }
            }
        })
        .detach();
    }

    /// Stop one detached shell and fold the server's cancelled record back in.
    pub(crate) fn stop_shell(&mut self, session_id: String, work_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let Ok(work) = client.runs.cancel_background_work(&session_id, &work_id).await else {
                return;
            };
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.upsert_background_work(&session_id, work, cx);
                    });
                }
            });
        })
        .detach();
    }
}
