use console_core::{ApiResponse, ForwardPortRequest, ForwardedPort};

#[test]
fn test_forwarded_port_serialization() {
    let port = ForwardedPort {
        port: 5173,
        url: "http://192.168.1.10:45173/".to_string(),
    };

    let json = serde_json::to_string(&port).expect("serialize port");
    assert!(json.contains("\"port\":5173"));
    assert!(json.contains("\"url\":\"http://192.168.1.10:45173/\""));

    let decoded: ForwardedPort = serde_json::from_str(&json).expect("deserialize port");
    assert_eq!(decoded, port);
}

#[test]
fn test_forward_port_request_serialization() {
    let req_with_project = ForwardPortRequest {
        port: 3000,
        project_id: Some("proj-123".to_string()),
    };
    let json = serde_json::to_string(&req_with_project).expect("serialize req");
    assert!(json.contains("\"port\":3000"));
    assert!(json.contains("\"projectId\":\"proj-123\""));

    let req_no_project = ForwardPortRequest {
        port: 8080,
        project_id: None,
    };
    let json_no_project = serde_json::to_string(&req_no_project).expect("serialize req");
    assert!(!json_no_project.contains("projectId"));
}

#[test]
fn test_ports_api_response_envelope() {
    let raw = r#"{
        "success": true,
        "data": [
            { "port": 3000, "url": "http://192.168.1.10:45174/" },
            { "port": 5173, "url": "http://192.168.1.10:45173/" }
        ]
    }"#;

    let response: ApiResponse<Vec<ForwardedPort>> =
        serde_json::from_str(raw).expect("parse api response");
    assert!(response.success);
    let ports = response.data.expect("ports data present");
    assert_eq!(ports.len(), 2);
    assert_eq!(ports[0].port, 3000);
    assert_eq!(ports[0].url, "http://192.168.1.10:45174/");
    assert_eq!(ports[1].port, 5173);
    assert_eq!(ports[1].url, "http://192.168.1.10:45173/");
}

#[test]
fn test_ports_error_response_envelope() {
    let raw = r#"{
        "success": false,
        "error": "Port 8080 is not listening on 127.0.0.1."
    }"#;

    let response: ApiResponse<ForwardedPort> =
        serde_json::from_str(raw).expect("parse api error response");
    assert!(!response.success);
    assert_eq!(
        response.error.as_deref(),
        Some("Port 8080 is not listening on 127.0.0.1.")
    );
}
