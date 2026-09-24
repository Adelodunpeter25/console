use super::ConsoleDesktopApp;
use gpui::Context;
use std::rc::Rc;
use std::time::SystemTime;

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
        })
        .detach();
    }

    /// Fetch usage data for the given provider (identified by session).
    pub fn maybe_fetch_usage(&mut self, session_id: &str, cx: &mut gpui::Context<Self>) {
        // Extract provider from session
        let provider = self
            .sessions
            .iter()
            .find(|s| s.id == session_id)
            .map(|s| s.provider.clone());

        let Some(provider) = provider else {
            return;
        };

        // Skip if already loading or recently fetched
        if self.usage_loading {
            return;
        }

        self.usage_loading = true;
        let client = self.client.clone();
        let entity = cx.entity().downgrade();

        cx.spawn(async move |_, cx| {
            match client.usage.get_provider(&provider).await {
                Ok(report) => {
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, _| {
                                this.usage_loading = false;
                                // Initialize reports map if needed
                                if this.usage_reports.is_none() {
                                    this.usage_reports = Some(std::rc::Rc::new(
                                        std::collections::HashMap::new(),
                                    ));
                                }
                                // Insert the report - create a new map with the update
                                if let Some(existing_rc) = this.usage_reports.take() {
                                    let mut map = match std::rc::Rc::try_unwrap(existing_rc) {
                                        Ok(m) => m,
                                        Err(rc) => (*rc).clone(),
                                    };
                                    map.insert(provider.clone(), report);
                                    this.usage_reports = Some(std::rc::Rc::new(map));
                                }
                            });
                        }
                    });
                }
                Err(_) => {
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, _| {
                                this.usage_loading = false;
                            });
                        }
                    });
                }
            }
        })
        .detach();
    }
}
