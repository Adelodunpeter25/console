use anyhow::{Context, Result};
use eventsource_stream::Eventsource;
use futures_util::StreamExt;
use reqwest::Response;

use crate::types::events::AgentSessionEvent;

pub struct SseStreamReader;

impl SseStreamReader {
    pub fn parse_stream(
        response: Response,
    ) -> impl futures_util::Stream<Item = Result<AgentSessionEvent>> {
        response
            .bytes_stream()
            .eventsource()
            .map(|item| -> Result<AgentSessionEvent> {
                let event = item.context("SSE stream connection error")?;
                Self::parse_event_data(&event.data)
            })
    }

    /// Parse one server event payload independently from the transport layer.
    /// Keeping this logic separate makes malformed protocol frames easy to
    /// test and keeps SSE connection errors distinct from payload errors.
    pub fn parse_event_data(data: &str) -> Result<AgentSessionEvent> {
        serde_json::from_str(data).with_context(|| format!("Failed to parse SSE payload: {data}"))
    }
}
