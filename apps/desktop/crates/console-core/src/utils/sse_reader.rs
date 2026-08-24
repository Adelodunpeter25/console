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

#[cfg(test)]
mod tests {
    use super::SseStreamReader;
    use crate::types::AgentSessionEvent;

    #[test]
    fn parses_camel_case_lifecycle_fields() {
        let start =
            SseStreamReader::parse_event_data(r#"{"type":"modelStreamStart","turnId":"turn-1"}"#)
                .expect("model stream start should parse");
        assert!(matches!(
            start,
            AgentSessionEvent::ModelStreamStart { turn_id } if turn_id == "turn-1"
        ));

        let end = SseStreamReader::parse_event_data(r#"{"type":"turnEnd","turnId":"turn-1"}"#)
            .expect("turn end should parse");
        assert!(matches!(
            end,
            AgentSessionEvent::TurnEnd { turn_id } if turn_id == "turn-1"
        ));
    }

    #[test]
    fn parses_tool_call_preview_without_arguments() {
        let event = SseStreamReader::parse_event_data(
            r#"{"type":"modelStreamPart","part":{"toolCall":{"id":"call-1","name":"readFile"}}}"#,
        )
        .expect("tool-call preview should be valid without streamed arguments");

        match event {
            AgentSessionEvent::ModelStreamPart { part } => {
                let preview = part.tool_call.expect("tool-call preview");
                assert_eq!(preview.id, "call-1");
                assert_eq!(preview.name, "readFile");
                assert!(preview.arguments.is_none());
            }
            other => panic!("expected model stream part, got {other:?}"),
        }
    }
}
