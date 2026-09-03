use super::ConsoleDesktopApp;
use gpui::Context;

impl ConsoleDesktopApp {
    pub fn refresh_deleted_sessions(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if let Ok(list) = client.sessions.list_deleted(None).await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.deleted_sessions = list;
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn restore_deleted_session(&mut self, session_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if client.sessions.restore(&session_id).await.is_ok() {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.refresh_deleted_sessions(cx);
                            this.load_sessions(cx);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn permanent_delete_session(&mut self, session_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if client.sessions.permanent_delete(&session_id).await.is_ok() {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.refresh_deleted_sessions(cx);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }
}
