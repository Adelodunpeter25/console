//! Detached background shells: per-session state fed by the run stream's
//! `backgroundWork*` events and the session's work list, plus the stop action
//! behind the chat's shells pill.
//!
//! Live output streaming for a selected shell went away with the Shells
//! inspector tab; the pill popover lists running shells and stops them.

use std::rc::Rc;

use console_core::BackgroundWork;
use gpui::Context;

use super::ConsoleDesktopApp;

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
            .map(|list| {
                list.iter()
                    .filter(|w| w.is_running_shell())
                    .cloned()
                    .collect()
            })
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

    /// Stop one detached shell and fold the server's cancelled record back in.
    pub(crate) fn stop_shell(
        &mut self,
        session_id: String,
        work_id: String,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let Ok(work) = client
                .runs
                .cancel_background_work(&session_id, &work_id)
                .await
            else {
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
