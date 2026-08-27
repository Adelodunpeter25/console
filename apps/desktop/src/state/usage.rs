use std::rc::Rc;
use std::time::SystemTime;
use gpui::Context;
use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    pub fn fetch_usage(&mut self, cx: &mut Context<Self>) {
        if self.usage_loading {
            return;
        }

        self.usage_loading = true;
        cx.notify();

        let client = self.client.clone();

        cx.spawn(async move |entity, cx| {
            let res = client.usage.get_all().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.usage_loading = false;
                        match res {
                            Ok(reports) => {
                                this.usage_reports = Some(Rc::new(reports));
                                this.usage_last_fetched = Some(SystemTime::now());
                            }
                            Err(err) => {
                                log::error!("Failed to fetch usage quota: {err}");
                            }
                        }
                        cx.notify();
                    });
                }
            });
        }).detach();
    }
}
