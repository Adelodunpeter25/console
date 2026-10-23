//! Detached background shells: per-session state fed by the run stream's
//! `backgroundWork*` events and the session's work list, plus the Shells
//! inspector tab's selected shell, output polling, and stop action.
//!
//! The server has no output stream for a detached shell, so the open output
//! view polls `/background-work/:id/output` once a second while the shell
//! runs and stops as soon as it settles.

use std::rc::Rc;
use std::time::Duration;

use console_core::{BackgroundWork, BackgroundWorkOutput};
use gpui::Context;

use super::ConsoleDesktopApp;

const OUTPUT_POLL: Duration = Duration::from_millis(1000);

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

    /// Open the Shells tab on one shell and start polling its output.
    pub(crate) fn select_shell(&mut self, session_id: String, work_id: String, cx: &mut Context<Self>) {
        self.shell_output = Some(ShellOutputState {
            work_id: work_id.clone(),
            output: None,
            unavailable: false,
        });
        cx.notify();
        self.poll_shell_output(session_id, work_id, cx);
    }

    /// Back to the shell list.
    pub(crate) fn deselect_shell(&mut self, cx: &mut Context<Self>) {
        self.shell_output = None;
        cx.notify();
    }

    fn poll_shell_output(&mut self, session_id: String, work_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            loop {
                let result = client
                    .runs
                    .background_work_output(&session_id, &work_id)
                    .await;
                let mut keep_polling = false;
                let alive = cx
                    .update(|cx| {
                        let Some(app) = entity.upgrade() else {
                            return false;
                        };
                        app.update(cx, |this, cx| {
                            // Stop when the user left this shell.
                            let still_open = this
                                .shell_output
                                .as_ref()
                                .is_some_and(|s| s.work_id == work_id);
                            if !still_open {
                                return false;
                            }
                            match &result {
                                Ok(out) => {
                                    keep_polling = out.is_running();
                                    if let Some(state) = this.shell_output.as_mut() {
                                        state.output = Some(out.clone());
                                        state.unavailable = false;
                                    }
                                }
                                Err(_) => {
                                    if let Some(state) = this.shell_output.as_mut() {
                                        if state.output.is_none() {
                                            state.unavailable = true;
                                        }
                                    }
                                }
                            }
                            cx.notify();
                            true
                        })
                    });
                if !alive || !keep_polling {
                    break;
                }
                cx.background_executor().timer(OUTPUT_POLL).await;
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
