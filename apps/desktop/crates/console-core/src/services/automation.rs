//! Automation and webhook subscription services + SSE streams.

use crate::types::{ApiResponse, Automation, AutomationEvent, WebhookEvent, WebhookSubscription};
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use futures_util::Stream;
use std::pin::Pin;

#[derive(Clone, Debug)]
pub enum AutomationStreamEvent {
    Snapshot(Vec<Automation>),
    Event(Box<AutomationEvent>),
}

#[derive(Clone, Debug)]
pub enum WebhookStreamEvent {
    Snapshot(Vec<WebhookSubscription>),
    Event(Box<WebhookEvent>),
}

#[derive(Clone)]
pub struct AutomationService {
    transport: HttpTransport,
}

impl AutomationService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(&self, session_id: Option<&str>) -> Result<Vec<Automation>> {
        let endpoint = match session_id {
            Some(sid) => format!("/api/automations?sessionId={}", urlencoding::encode(sid)),
            None => "/api/automations".to_string(),
        };
        let url = self.transport.url(&endpoint).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch automations")?;
        let body: ApiResponse<Vec<Automation>> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse automations list")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch automations".into())
            ))
        }
    }

    pub async fn pause(&self, id: &str) -> Result<Automation> {
        let url = self
            .transport
            .url(&format!(
                "/api/automations/{}/pause",
                urlencoding::encode(id)
            ))
            .await;
        let response = self
            .transport
            .client()
            .post(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to pause automation")?;
        let body: ApiResponse<Automation> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse pause automation response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Pause automation response contained no data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to pause automation".into())
            ))
        }
    }

    pub async fn resume(&self, id: &str) -> Result<Automation> {
        let url = self
            .transport
            .url(&format!(
                "/api/automations/{}/resume",
                urlencoding::encode(id)
            ))
            .await;
        let response = self
            .transport
            .client()
            .post(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to resume automation")?;
        let body: ApiResponse<Automation> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse resume automation response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Resume automation response contained no data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to resume automation".into())
            ))
        }
    }

    pub async fn delete(&self, id: &str) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/automations/{}", urlencoding::encode(id)))
            .await;
        let response = self
            .transport
            .client()
            .delete(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to delete automation")?;
        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse delete automation response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to delete automation".into())
            ))
        }
    }

    pub async fn watch(
        &self,
        session_id: Option<&str>,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<AutomationStreamEvent>> + Send>>> {
        use eventsource_stream::Eventsource;
        use futures_util::StreamExt;

        let endpoint = match session_id {
            Some(sid) => format!(
                "/api/automations/stream?sessionId={}",
                urlencoding::encode(sid)
            ),
            None => "/api/automations/stream".to_string(),
        };
        let url = self.transport.url(&endpoint).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to connect to automations SSE stream")?
            .error_for_status()
            .context("Automations SSE endpoint returned an error")?;

        let stream = response
            .bytes_stream()
            .eventsource()
            .filter_map(|item| async {
                match item {
                    Ok(event) if event.event == "automations" => {
                        serde_json::from_str::<Vec<Automation>>(&event.data)
                            .ok()
                            .map(|list| Ok(AutomationStreamEvent::Snapshot(list)))
                    }
                    Ok(event) if event.event == "automation" => {
                        serde_json::from_str::<AutomationEvent>(&event.data)
                            .ok()
                            .map(|ev| Ok(AutomationStreamEvent::Event(Box::new(ev))))
                    }
                    _ => None,
                }
            });
        Ok(Box::pin(stream))
    }
}

#[derive(Clone)]
pub struct WebhookService {
    transport: HttpTransport,
}

impl WebhookService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(&self, session_id: Option<&str>) -> Result<Vec<WebhookSubscription>> {
        let endpoint = match session_id {
            Some(sid) => format!("/api/webhooks?sessionId={}", urlencoding::encode(sid)),
            None => "/api/webhooks".to_string(),
        };
        let url = self.transport.url(&endpoint).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch webhooks")?;
        let body: ApiResponse<Vec<WebhookSubscription>> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse webhooks list")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch webhooks".into())
            ))
        }
    }

    pub async fn delete(&self, id: &str) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/webhooks/{}", urlencoding::encode(id)))
            .await;
        let response = self
            .transport
            .client()
            .delete(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to delete webhook subscription")?;
        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse delete webhook response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| {
                "Failed to delete webhook subscription".into()
            })))
        }
    }

    pub async fn watch(
        &self,
        session_id: Option<&str>,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<WebhookStreamEvent>> + Send>>> {
        use eventsource_stream::Eventsource;
        use futures_util::StreamExt;

        let endpoint = match session_id {
            Some(sid) => format!(
                "/api/webhooks/stream?sessionId={}",
                urlencoding::encode(sid)
            ),
            None => "/api/webhooks/stream".to_string(),
        };
        let url = self.transport.url(&endpoint).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to connect to webhooks SSE stream")?
            .error_for_status()
            .context("Webhooks SSE endpoint returned an error")?;

        let stream = response
            .bytes_stream()
            .eventsource()
            .filter_map(|item| async {
                match item {
                    Ok(event) if event.event == "webhooks" => {
                        serde_json::from_str::<Vec<WebhookSubscription>>(&event.data)
                            .ok()
                            .map(|list| Ok(WebhookStreamEvent::Snapshot(list)))
                    }
                    Ok(event) if event.event == "webhook" => {
                        serde_json::from_str::<WebhookEvent>(&event.data)
                            .ok()
                            .map(|ev| Ok(WebhookStreamEvent::Event(Box::new(ev))))
                    }
                    _ => None,
                }
            });
        Ok(Box::pin(stream))
    }
}
