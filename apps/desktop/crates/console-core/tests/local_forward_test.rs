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
