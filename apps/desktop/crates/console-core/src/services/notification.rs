use crate::types::*;
use crate::utils::{HttpTransport, SseStreamReader};
use anyhow::{Context, Result};
use futures_util::Stream;
use std::pin::Pin;

#[derive(Clone)]
pub struct NotificationService {
    transport: HttpTransport,
}

impl NotificationService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn stream(
        &self,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<NotificationEvent>> + Send>>> {
        let url = self.transport.url("/api/notifications/stream").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to connect to notifications stream")?;

        Ok(Box::pin(SseStreamReader::parse_typed_stream::<
            NotificationEvent,
        >(resp)))
    }
}
