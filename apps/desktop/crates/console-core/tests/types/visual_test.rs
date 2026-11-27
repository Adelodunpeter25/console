use console_core::{
    HtmlArtifact, ToolResult, VisualArtifact, VisualBlock, VisualParse, parse_visual,
};
use console_proto::{HtmlPart, ToolResult as ProtoToolResult};

#[test]
fn parses_image_block_and_rejects_legacy_specs() {
    let with_alt = VisualArtifact {
        version: 1,
        spec: serde_json::json!({
            "version": 1,
            "blocks": [{"type": "image", "path": "/workspace/assets/chart.png", "alt": "Chart"}]
        }),
    };
    let VisualParse::Ok(spec) = parse_visual(&with_alt) else {
        panic!("expected Ok");
    };
    assert_eq!(
        spec.blocks,
        vec![VisualBlock::Image {
            path: "/workspace/assets/chart.png".into(),
            alt: Some("Chart".into()),
        }]
    );

    let newer = VisualArtifact {
        version: 2,
        spec: serde_json::json!({}),
    };
    assert!(matches!(parse_visual(&newer), VisualParse::Unsupported));

    let legacy = VisualArtifact {
        version: 1,
        spec: serde_json::json!({"version": 1, "blocks": [{"type": "text", "text": "hi"}]}),
    };
    assert!(matches!(parse_visual(&legacy), VisualParse::Invalid));
}

#[test]
fn decodes_html_part_from_proto() {
    let proto = ProtoToolResult {
        tool_call_id: "c1".into(),
        tool_name: Some("render".into()),
        content: br#"{"ok":true}"#.to_vec(),
        is_error: None,
        visual: None,
        html: Some(HtmlPart {
            path: "/tmp/page.html".into(),
            title: "Sidebar".into(),
            height: 640,
            snapshot: Some(vec![0x89, 0x50, 0x4e, 0x47]),
        }),
    };
    let decoded = ToolResult::from_proto(&proto);
    let html: HtmlArtifact = decoded.html.expect("html artifact");
    assert_eq!(html.path, "/tmp/page.html");
    assert_eq!(html.title, "Sidebar");
    assert_eq!(html.height, 640);
    assert_eq!(html.snapshot, Some(vec![0x89, 0x50, 0x4e, 0x47]));
}
