use anyhow::{Context, Result};
use eventsource_stream::Eventsource;
use futures_util::StreamExt;
use reqwest::Response;
use serde::de::DeserializeOwned;

use crate::types::events::AgentSessionEvent;

pub struct SseStreamReader;

impl SseStreamReader {
    pub fn parse_stream(
        response: Response,
    ) -> impl futures_util::Stream<Item = Result<AgentSessionEvent>> {
        Self::parse_typed_stream::<AgentSessionEvent>(response)
    }

    pub fn parse_typed_stream<T: DeserializeOwned + Send + 'static>(
        response: Response,
    ) -> impl futures_util::Stream<Item = Result<T>> {
        response
            .bytes_stream()
            .eventsource()
            .map(|item| -> Result<T> {
                let event = item.context("SSE stream connection error")?;
                serde_json::from_str(&event.data)
                    .with_context(|| format!("Failed to parse SSE payload: {}", event.data))
            })
    }

    /// Parse one server event payload independently from the transport layer.
    /// Keeping this logic separate makes malformed protocol frames easy to
    /// test and keeps SSE connection errors distinct from payload errors.
    pub fn parse_event_data(data: &str) -> Result<AgentSessionEvent> {
        serde_json::from_str(data).with_context(|| format!("Failed to parse SSE payload: {data}"))
    }
}
