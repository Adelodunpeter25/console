use futures_util::StreamExt;
use gpui::Context;

use console_core::types::notification::{
    NotificationDecision, decide_notification, normalize_notification_title,
};

use super::ConsoleDesktopApp;
use super::macos_notifications;

impl ConsoleDesktopApp {
    pub fn init_notifications(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let transport = self.transport.clone();
        let entity = cx.entity().downgrade();

        // Native center delegate + authorization. Clicks arrive as session ids.
        let mut click_rx = macos_notifications::ensure_initialized();
        let click_entity = entity.clone();

        // Banner clicks → focus app + open the session + clear its banner.
        cx.spawn(async move |_entity, cx| {
            while let Some(session_id) = click_rx.recv().await {
                macos_notifications::clear_for_session(&session_id);
                let _ = cx.update(|cx| {
                    if let Some(app) = click_entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.select_and_open_session(session_id, cx);
                        });
                    }
                });
            }
        })
        .detach();

        // SSE event stream → native banner (or silent in-app update).
        cx.spawn(async move |_entity, cx| {
            loop {
                match transport.watch_notifications(&client).await {
                    Ok(mut stream) => {
                        while let Some(event_res) = stream.next().await {
                            if let Ok(event) = event_res {
                                let session_id = event.session_id.clone();
                                if event.kind == "resumed" {
                                    // Silent signal: an idle session woke itself for
                                    // finished background work. Its old run stream has
                                    // ended, so re-attach to the new run. Wait briefly
                                    // so the old run's sessionEnd lands first.
                                    cx.background_executor()
                                        .timer(std::time::Duration::from_millis(400))
                                        .await;
                                    let _ = cx.update(|cx| {
                                        if let Some(app) = entity.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.attach_resumed_session(&session_id, cx);
                                            });
                                        }
                                    });
                                    continue;
                                }
                                let title = normalize_notification_title(&event.title);
                                let subtitle = event.subtitle.clone();
                                let body = event.body.clone();
                                let _ = cx.update(|cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, _cx| {
                                            // Suppress the banner when the user is already
                                            // viewing this session and the window is active —
                                            // silent in-app update, matching mobile. If the window
                                            // is in the background, we notify even for the active tab.
                                            // Same identifier replaces the previous banner instead of stacking.
                                            //
                                            // Deliberately no `cx.notify()` here: the decision's
                                            // effects are all OS-side (UNUserNotificationCenter
                                            // banner + dock badge), so re-rendering the whole
                                            // window per SSE event would be pure waste.
                                            let viewing = if this.is_window_active {
                                                this.active_pane_id.as_deref().and_then(|pane| {
                                                    this.active_session_for_pane(pane)
                                                })
                                            } else {
                                                None
                                            };
                                            match decide_notification(
                                                viewing.as_deref(),
                                                &session_id,
                                            ) {
                                                NotificationDecision::SuppressViewing => {
                                                    macos_notifications::clear_for_session(
                                                        &session_id,
                                                    );
                                                }
                                                NotificationDecision::Notify => {
                                                    macos_notifications::notify_session(
                                                        &session_id,
                                                        &title,
                                                        &subtitle,
                                                        &body,
                                                    );
                                                }
                                                NotificationDecision::SkipEmpty => {}
                                            }
                                        });
                                    }
                                });
                            }
                        }
                    }
                    Err(err) => {
                        log::debug!("Notification stream reconnecting: {err}");
                    }
                }
                tokio::time::sleep(std::time::Duration::from_secs(3)).await;
            }
        })
        .detach();
    }

    /// Re-attach every pane showing `session_id` to its freshly woken run.
    fn attach_resumed_session(&mut self, session_id: &str, cx: &mut Context<Self>) {
        if session_id.is_empty() || self.is_session_running(session_id) {
            return;
        }
        let pane = self
            .workspace_root
            .leaves()
            .into_iter()
            .map(|leaf| leaf.id.clone())
            .find(|id| self.active_session_for_pane(id).as_deref() == Some(session_id));
        if let Some(pane_id) = pane {
            self.attach_session_run_for_pane(pane_id, session_id.to_string(), cx);
        }
    }
}
