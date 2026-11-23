//! Live automation and webhook subscription state fed by SSE streams.

use std::collections::HashSet;

use console_core::{
    Automation, AutomationEvent, AutomationStreamEvent, WebhookEvent, WebhookStreamEvent,
    WebhookSubscription,
};
use gpui::Context;

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    pub fn init_automation_stream(&mut self, cx: &mut Context<Self>) {
        self.automation_stream = None;
        let client = self.client.clone();
        let transport = self.transport;
        let entity = cx.entity().downgrade();
        self.automation_stream = Some(cx.spawn(async move |_, cx| {
            use futures_util::StreamExt;
            loop {
                match transport.watch_automations(&client, None).await {
                    Ok(mut stream) => {
                        while let Some(Ok(item)) = stream.next().await {
                            let _ = cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        match item {
                                            AutomationStreamEvent::Snapshot(list) => {
                                                this.automations = list;
                                            }
                                            AutomationStreamEvent::Event(ev) => {
                                                this.apply_automation_event(*ev);
                                            }
                                        }
                                        cx.notify();
                                    });
                                }
                            });
                        }
                    }
                    Err(error) => log::debug!("Automation stream reconnecting: {error}"),
                }
                tokio::time::sleep(std::time::Duration::from_secs(3)).await;
            }
        }));
    }

    pub fn init_webhook_stream(&mut self, cx: &mut Context<Self>) {
        self.webhook_stream = None;
        let client = self.client.clone();
        let transport = self.transport;
        let entity = cx.entity().downgrade();
        self.webhook_stream = Some(cx.spawn(async move |_, cx| {
            use futures_util::StreamExt;
            loop {
                match transport.watch_webhooks(&client, None).await {
                    Ok(mut stream) => {
                        while let Some(Ok(item)) = stream.next().await {
                            let _ = cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        match item {
                                            WebhookStreamEvent::Snapshot(list) => {
                                                this.webhooks = list;
                                            }
                                            WebhookStreamEvent::Event(ev) => {
                                                this.apply_webhook_event(*ev);
                                            }
                                        }
                                        cx.notify();
                                    });
                                }
                            });
                        }
                    }
                    Err(error) => log::debug!("Webhook stream reconnecting: {error}"),
                }
                tokio::time::sleep(std::time::Duration::from_secs(3)).await;
            }
        }));
    }

    fn apply_automation_event(&mut self, ev: AutomationEvent) {
        if ev.event_type == "deleted"
            || (ev.event_type == "fired"
                && ev.automation.as_ref().is_some_and(|a| a.status == "done"))
        {
            if let Some(id) = ev
                .id
                .as_deref()
                .or(ev.automation.as_ref().map(|a| a.id.as_str()))
            {
                self.automations.retain(|a| a.id != id);
            }
            return;
        }
        if let Some(auto) = ev.automation {
            if let Some(existing) = self.automations.iter_mut().find(|a| a.id == auto.id) {
                *existing = auto;
            } else {
                self.automations.push(auto);
            }
        }
    }

    fn apply_webhook_event(&mut self, ev: WebhookEvent) {
        if ev.event_type == "deleted"
            || (ev.event_type == "fired"
                && ev.subscription.as_ref().is_some_and(|s| s.status == "done"))
        {
            if let Some(id) = ev
                .id
                .as_deref()
                .or(ev.subscription.as_ref().map(|s| s.id.as_str()))
            {
                self.webhooks.retain(|w| w.id != id);
            }
            return;
        }
        if let Some(sub) = ev.subscription {
            if let Some(existing) = self.webhooks.iter_mut().find(|w| w.id == sub.id) {
                *existing = sub;
            } else {
                self.webhooks.push(sub);
            }
        }
    }

    pub fn automations_for_session(&self, session_id: &str) -> Vec<Automation> {
        self.automations
            .iter()
            .filter(|a| a.session_id == session_id && a.status != "done")
            .cloned()
            .collect()
    }

    pub fn webhooks_for_session(&self, session_id: &str) -> Vec<WebhookSubscription> {
        self.webhooks
            .iter()
            .filter(|w| w.session_id == session_id && w.status != "done" && w.status != "expired")
            .cloned()
            .collect()
    }

    pub fn scheduled_session_sets(&self) -> (HashSet<String>, HashSet<String>) {
        let autos = self
            .automations
            .iter()
            .filter(|a| a.status == "active" || a.status == "paused")
            .map(|a| a.session_id.clone())
            .collect();
        let hooks = self
            .webhooks
            .iter()
            .filter(|w| w.status == "active")
            .map(|w| w.session_id.clone())
            .collect();
        (autos, hooks)
    }

    pub fn pause_automation(&mut self, id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            if let Ok(updated) = client.automations.pause(&id).await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.apply_automation_event(AutomationEvent {
                                event_type: "paused".to_string(),
                                automation: Some(updated),
                                id: Some(id),
                                session_id: None,
                            });
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn resume_automation(&mut self, id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            if let Ok(updated) = client.automations.resume(&id).await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.apply_automation_event(AutomationEvent {
                                event_type: "resumed".to_string(),
                                automation: Some(updated),
                                id: Some(id),
                                session_id: None,
                            });
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn delete_automation(&mut self, id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            if client.automations.delete(&id).await.is_ok() {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.automations.retain(|a| a.id != id);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn delete_webhook(&mut self, id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            if client.webhooks.delete(&id).await.is_ok() {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.webhooks.retain(|w| w.id != id);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }
}
