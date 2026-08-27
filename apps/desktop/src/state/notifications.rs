use futures_util::StreamExt;
use gpui::Context;

use super::ConsoleDesktopApp;

pub fn show_native_notification(title: &str, body: &str) {
    #[cfg(target_os = "macos")]
    {
        let script = format!(
            "display notification {:?} with title {:?}",
            body, title
        );
        let _ = std::process::Command::new("osascript")
            .arg("-e")
            .arg(script)
            .spawn();
    }
}

impl ConsoleDesktopApp {
    pub fn init_notifications(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let entity = cx.entity().downgrade();

        cx.spawn(async move |_entity, cx| {
            loop {
                match client.notifications.stream().await {
                    Ok(mut stream) => {
                        while let Some(event_res) = stream.next().await {
                            if let Ok(event) = event_res {
                                show_native_notification(&event.title, &event.body);
                                cx.update(|cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |_this, cx| {
                                            // Trigger UI notification / refresh on new notification
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
