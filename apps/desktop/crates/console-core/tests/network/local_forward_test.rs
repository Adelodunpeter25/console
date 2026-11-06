use console_core::{HttpTransport, LocalForwardManager};

#[tokio::test]
async fn tunnel_url_derives_from_base() {
    let transport = HttpTransport::new(Some("http://80.190.80.2:3000".to_string()));
    let manager = LocalForwardManager::new(transport);
    assert_eq!(
        manager.tunnel_url(8000).await,
        "ws://80.190.80.2:3000/api/ports/8000/tunnel"
    );

    let secure = HttpTransport::new(Some("https://example.test:3000/".to_string()));
    let secure_manager = LocalForwardManager::new(secure);
    assert_eq!(
        secure_manager.tunnel_url(8000).await,
        "wss://example.test:3000/api/ports/8000/tunnel"
    );
}

#[tokio::test]
async fn local_backend_needs_no_listener() {
    let transport = HttpTransport::new(Some("http://localhost:3000".to_string()));
    let manager = LocalForwardManager::new(transport);
    assert!(manager.is_local_backend().await);

    let forwards = manager.ensure(&[8000]).await;
    assert_eq!(forwards.len(), 1);
    assert_eq!(forwards[0].remote_port, 8000);
    assert_eq!(forwards[0].local_port, 8000);
    assert_eq!(forwards[0].local_url, "http://localhost:8000/");
    manager.clear().await;
}

#[tokio::test]
async fn remote_backend_binds_localhost_ephemeral() {
    let transport = HttpTransport::new(Some("http://80.190.80.2:3000".to_string()));
    let manager = LocalForwardManager::new(transport);
    assert!(!manager.is_local_backend().await);

    // Preferred 0 guarantees an OS-picked ephemeral port (no hardcoded port).
    let forwards = manager.ensure(&[0]).await;
    assert_eq!(forwards.len(), 1);
    assert_ne!(forwards[0].local_port, 0);
    assert!(forwards[0].local_url.starts_with("http://localhost:"));
    // Reconciling with an empty list drops the listener.
    let empty = manager.ensure(&[]).await;
    assert!(empty.is_empty());
    manager.clear().await;
}

#[tokio::test]
async fn remove_drops_single_forward() {
    let transport = HttpTransport::new(Some("http://80.190.80.2:3000".to_string()));
    let manager = LocalForwardManager::new(transport);
    let _ = manager.ensure(&[0]).await;
    manager.remove(0).await;
    let remaining = manager.ensure(&[]).await;
    assert!(remaining.is_empty());
    manager.clear().await;
}

// Regression test: a local server on the preferred port (e.g. a wildcard dev
// server a specific 127.0.0.1 bind silently shadows on macOS) must keep the
// port; the forward moves to an ephemeral one.
#[tokio::test]
async fn occupied_preferred_port_is_not_shadowed() {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    let transport = HttpTransport::new(Some("http://80.190.80.2:3000".to_string()));
    let manager = LocalForwardManager::new(transport);

    let forwards = manager.ensure(&[port]).await;
    assert_eq!(forwards.len(), 1);
    assert_ne!(forwards[0].local_port, port);
    assert!(forwards[0].local_url.starts_with("http://localhost:"));
    manager.clear().await;
    drop(listener);
}

#[tokio::test]
async fn free_preferred_port_is_kept() {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    drop(listener);

    let transport = HttpTransport::new(Some("http://80.190.80.2:3000".to_string()));
    let manager = LocalForwardManager::new(transport);
    let forwards = manager.ensure(&[port]).await;
    assert_eq!(forwards.len(), 1);
    assert_eq!(forwards[0].local_port, port);
    assert_eq!(forwards[0].local_url, format!("http://localhost:{port}/"));
    manager.clear().await;
}

#[tokio::test]
async fn loopback_backend_urls_are_local() {
    for base in [
        "http://localhost:3000",
        "http://127.0.0.1:3000",
        "http://[::1]:3000",
    ] {
        let manager = LocalForwardManager::new(HttpTransport::new(Some(base.to_string())));
        assert!(manager.is_local_backend().await, "{base} should be local");
    }
}

// Non-loopback literals take the parse path (no DNS) and stay remote; a
// non-canonical IPv6 loopback literal skips the fast path but parses local.
#[tokio::test]
async fn backend_classification_uses_the_parsed_host() {
    let local = LocalForwardManager::new(HttpTransport::new(Some(
        "http://[0:0:0:0:0:0:0:1]:3000".to_string(),
    )));
    assert!(local.is_local_backend().await);

    let remote = LocalForwardManager::new(HttpTransport::new(Some(
        "http://192.168.1.50:3000".to_string(),
    )));
    assert!(!remote.is_local_backend().await);

    let remote_public = LocalForwardManager::new(HttpTransport::new(Some(
        "http://80.190.80.2:3000".to_string(),
    )));
    assert!(!remote_public.is_local_backend().await);
}
