use futures_util::StreamExt;
use gpui::Context;

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
                                let title = if event.title.is_empty() {
                                    "Console".to_string()
                                } else {
                                    event.title.clone()
                                };
                                let body = event.body.clone();
                                let _ = cx.update(|cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            // Suppress the banner when the user is already
                                            // viewing this session — silent in-app update,
                                            // matching mobile. Clear any stale delivered
                                            // banner for it.
                                            let viewing = this
                                                .active_pane_id
                                                .as_deref()
                                                .and_then(|pane| this.active_session_for_pane(pane))
                                                .as_deref()
                                                == Some(session_id.as_str());
                                            if viewing {
                                                macos_notifications::clear_for_session(&session_id);
                                            } else if !session_id.is_empty() {
                                                // Same identifier replaces the previous banner
                                                // for this session instead of stacking.
                                                macos_notifications::notify_session(
                                                    &session_id,
                                                    &title,
                                                    &body,
                                                );
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
