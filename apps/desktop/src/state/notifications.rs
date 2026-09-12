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
                match client.notifications.stream().await {
                    Ok(mut stream) => {
                        while let Some(event_res) = stream.next().await {
                            if let Ok(event) = event_res {
                                let session_id = event.session_id.clone();
                                let title = normalize_notification_title(&event.title);
                                let body = event.body.clone();
                                let _ = cx.update(|cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            // Suppress the banner when the user is already
                                            // viewing this session and the window is active —
                                            // silent in-app update, matching mobile. If the window
                                            // is in the background, we notify even for the active tab.
                                            // Same identifier replaces the previous banner instead of stacking.
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
                                                        &body,
                                                    );
                                                }
                                                NotificationDecision::SkipEmpty => {}
                                            }
                                            cx.notify();
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
}
