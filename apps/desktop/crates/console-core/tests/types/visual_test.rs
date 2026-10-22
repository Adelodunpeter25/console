//! Shared visual fixtures (proto/testdata/visual, message/toolresult_visual.json)
//! plus geometry edge cases.
use console_core::{
    AgentMessage, VisualArtifact, VisualBlock, VisualParse, VisualPoint, describe_chart,
    format_compact, looks_numeric, min_max_buckets, parse_visual, value_axis,
};
use std::path::PathBuf;

fn testdata(path: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata")
            .join(path),
    )
    .unwrap_or_else(|e| panic!("read {path}: {e}"))
}

fn artifact(name: &str) -> VisualArtifact {
    VisualArtifact {
        version: 1,
        spec: serde_json::from_str(testdata(&format!("visual/{name}")).trim()).unwrap(),
    }
}

#[test]
fn decodes_tool_result_visual_fixture() {
    let wire: console_proto::AgentMessage =
        serde_json::from_str(testdata("message/toolresult_visual.json").trim()).unwrap();
    let AgentMessage::ToolResult { results, .. } = AgentMessage::from_proto(wire).unwrap() else {
        panic!("expected tool result");
    };
    let visual = results[0].visual.clone().expect("visual present");
    assert_eq!(visual.version, 1);
    let VisualParse::Ok(spec) = parse_visual(&visual) else {
        panic!("spec should parse");
    };
    assert_eq!(spec.title.as_deref(), Some("Weekly signups"));
    let VisualBlock::Chart(chart) = &spec.blocks[0] else {
        panic!("expected chart");
    };
    assert_eq!(chart.kind, "line");
    assert_eq!(chart.series[0].points.len(), 3);
}

#[test]
fn parses_dashboard_blocks() {
    let VisualParse::Ok(spec) = parse_visual(&artifact("dashboard.json")) else {
        panic!("dashboard should parse");
    };
    let kinds: Vec<&str> = spec
        .blocks
        .iter()
        .map(|b| match b {
            VisualBlock::Metric { .. } => "metric",
            VisualBlock::Chart(_) => "chart",
            VisualBlock::Table { .. } => "table",
            VisualBlock::Text { .. } => "text",
        })
        .collect();
    assert_eq!(kinds, ["metric", "metric", "chart", "table", "text"]);
}

#[test]
fn newer_version_and_garbage_fall_back() {
    let newer = VisualArtifact {
        version: 2,
        spec: serde_json::json!({}),
    };
    assert!(matches!(parse_visual(&newer), VisualParse::Unsupported));
    let bad = VisualArtifact {
        version: 1,
        spec: serde_json::json!({"version": 1, "blocks": [{"type": "nope"}]}),
    };
    assert!(matches!(parse_visual(&bad), VisualParse::Invalid));
}

#[test]
fn axis_edge_cases() {
    let flat = value_axis(5.0, 5.0, false, 5);
    assert!(flat.min < 5.0 && flat.max > 5.0);
    let bars = value_axis(40.0, 90.0, true, 5);
    assert_eq!(bars.min, 0.0);
    let neg = value_axis(-30.0, 10.0, true, 5);
    assert!(neg.min <= -30.0 && neg.ticks.contains(&0.0));
    let zero = value_axis(0.0, 0.0, true, 5);
    assert!(zero.ticks.len() >= 2 && zero.min < zero.max);
}

#[test]
fn two_thousand_points_are_bucketed() {
    let pts: Vec<VisualPoint> = (0..2000)
        .map(|i| VisualPoint {
            x: i as f64,
            y: (i % 17) as f64,
            label: None,
        })
        .collect();
    let out = min_max_buckets(&pts, 300);
    assert!(out.len() <= 600);
    assert_eq!(out.iter().map(|p| p.y).fold(f64::INFINITY, f64::min), 0.0);
    assert_eq!(out.iter().map(|p| p.y).fold(f64::NEG_INFINITY, f64::max), 16.0);
    assert_eq!(min_max_buckets(&pts, 1500).len(), 2000);
}

#[test]
fn formats_and_describes() {
    assert_eq!(format_compact(1234.0), "1.2k");
    assert_eq!(format_compact(3.0), "3");
    assert!(looks_numeric("$42.1k") && looks_numeric("49.8%") && !looks_numeric("Organic"));
    let VisualParse::Ok(spec) = parse_visual(&artifact("line.json")) else {
        panic!()
    };
    let VisualBlock::Chart(chart) = &spec.blocks[0] else {
        panic!()
    };
    let d = describe_chart(chart);
    assert!(d.starts_with("Line chart, Signups by week. 1 series, 3 points."), "{d}");
    assert!(d.contains("Last 164"), "{d}");
}
