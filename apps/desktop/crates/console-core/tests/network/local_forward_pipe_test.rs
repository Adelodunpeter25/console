//! End-to-end pipe test for the real LocalForwardManager.
//!
//! Spins a fake remote TCP target + a WS tunnel gateway speaking the same
//! raw-TCP-over-WS protocol as the Bun server, then drives the real
//! `ensure()` listener and sends plain HTTP through it like a browser would.

use console_core::{HttpTransport, LocalForwardManager};
use futures_util::{SinkExt, StreamExt};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tokio_tungstenite::tungstenite::handshake::server::{Request, Response};
use tokio_tungstenite::tungstenite::protocol::Message;

fn tunnel_target(path: &str) -> Option<u16> {
    // Expect /api/ports/<port>/tunnel
    let mut parts = path.split('/').filter(|part| !part.is_empty());
    if parts.next()? != "api" {
        return None;
    }
    if parts.next()? != "ports" {
        return None;
    }
    let port: u16 = parts.next()?.parse().ok()?;
    if parts.next()? != "tunnel" {
        return None;
    }
    Some(port)
}

async fn bridge(ws: tokio_tungstenite::WebSocketStream<TcpStream>, target_port: u16) {
    let (mut sink, mut stream) = ws.split();
    let upstream = match TcpStream::connect(format!("127.0.0.1:{target_port}")).await {
        Ok(socket) => socket,
        Err(_) => return,
    };
    let (mut up_read, mut up_write) = upstream.into_split();
    let to_upstream = async move {
        while let Some(message) = stream.next().await {
            let message = match message {
                Ok(message) => message,
                Err(_) => break,
            };
            match message {
                Message::Binary(data) => {
                    if up_write.write_all(&data).await.is_err() {
                        break;
                    }
                }
                Message::Text(text) => {
                    if up_write.write_all(text.as_bytes()).await.is_err() {
                        break;
                    }
                }
                Message::Close(_) => break,
                _ => {}
            }
        }
    };
    let to_ws = async move {
        let mut buf = vec![0u8; 8192];
        loop {
            match up_read.read(&mut buf).await {
                Ok(0) => {
                    let _ = sink.send(Message::Close(None)).await;
                    break;
                }
                Ok(n) => {
                    if sink.send(Message::Binary(buf[..n].to_vec().into())).await.is_err() {
                        break;
                    }
                }
                Err(_) => break,
            }
        }
    };
    tokio::select! {
        _ = to_upstream => {},
        _ = to_ws => {},
    }
}

async fn read_http_response(stream: &mut TcpStream) -> String {
    let mut out = Vec::new();
    let mut buf = vec![0u8; 8192];
    loop {
        match stream.read(&mut buf).await {
            Ok(0) => break,
            Ok(n) => out.extend_from_slice(&buf[..n]),
            Err(_) => break,
        }
    }
    String::from_utf8_lossy(&out).into_owned()
}

/// Outbound LAN IP without sending packets (UDP connect only queries routing).
/// Lets the test address a same-machine gateway via a non-loopback IP, so the
/// manager takes the real remote-forward path.
fn local_lan_ip() -> std::net::IpAddr {
    let socket = std::net::UdpSocket::bind("0.0.0.0:0").expect("udp bind");
    socket
        .connect("8.8.8.8:80")
        .expect("udp connect (no packets sent)");
    socket.local_addr().expect("udp local addr").ip()
}

#[tokio::test]
async fn browser_http_survives_real_manager_forward() {
    // Fake dev server: answers HTTP on 127.0.0.1 with React-like routes.
    let target = TcpListener::bind("127.0.0.1:0").await.expect("bind target");
    let target_port = target.local_addr().expect("target addr").port();
    tokio::spawn(async move {
        loop {
            let (mut socket, _) = match target.accept().await {
                Ok(pair) => pair,
                Err(_) => break,
            };
            tokio::spawn(async move {
                let mut buf = vec![0u8; 8192];
                let n = socket.read(&mut buf).await.unwrap_or(0);
                let request = String::from_utf8_lossy(&buf[..n]).into_owned();
                let path = request
                    .lines()
                    .next()
                    .and_then(|line| line.split_whitespace().nth(1))
                    .unwrap_or("/");
                let body = if path == "/" {
                    "<div id=root>app</div>".to_string()
                } else {
                    format!("target:{path}")
                };
                let response = format!(
                    "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body.len(),
                    body
                );
                let _ = socket.write_all(response.as_bytes()).await;
            });
        }
    });

    // Tunnel gateway routing /api/ports/<port>/tunnel to 127.0.0.1:<port>.
    // Bound on all interfaces and addressed via LAN IP so the manager takes
    // the real remote-backend path (a loopback base would take the direct
    // local shortcut instead).
    let lan = local_lan_ip();
    let gateway = TcpListener::bind("0.0.0.0:0").await.expect("bind gateway");
    let gateway_port = gateway.local_addr().expect("gateway addr").port();
    tokio::spawn(async move {
        while let Ok((stream, _)) = gateway.accept().await {
            tokio::spawn(async move {
                let path = std::sync::Arc::new(std::sync::Mutex::new(String::new()));
                let path_clone = path.clone();
                let callback = move |request: &Request, response: Response| {
                    *path_clone.lock().expect("path lock") = request.uri().path().to_string();
                    Ok(response)
                };
                let ws = match tokio_tungstenite::accept_hdr_async(stream, callback).await {
                    Ok(ws) => ws,
                    Err(_) => return,
                };
                let target = tunnel_target(&path.lock().expect("path lock").clone());
                match target {
                    Some(port) => bridge(ws, port).await,
                    None => {}
                }
            });
        }
    });

    // The real manager: base points at the gateway, remote is the target.
    // Preferred local bind (target_port) is taken by the target itself, so the
    // manager must fall back to an OS-picked ephemeral port — no hardcoding.
    let transport = HttpTransport::new(Some(format!("http://{lan}:{gateway_port}")));
    let manager = LocalForwardManager::new(transport);
    let forwards = manager.ensure(&[target_port]).await;
    assert_eq!(forwards.len(), 1, "manager should forward the remote port");
    assert_eq!(forwards[0].remote_port, target_port);
    assert_ne!(forwards[0].local_port, 0);
    assert_ne!(
        forwards[0].local_port, target_port,
        "preferred port is taken, must fall back to ephemeral"
    );
    let local_port = forwards[0].local_port;

    // Browser-equivalent: plain HTTP GET through the real local listener.
    let mut browser = TcpStream::connect(format!("127.0.0.1:{local_port}"))
        .await
        .expect("browser connects to local forward");
    browser
        .write_all(b"GET /assets/index-abc123.js HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")
        .await
        .expect("browser sends request");
    let response = read_http_response(&mut browser).await;
    assert!(response.contains("200 OK"), "expected HTTP 200, got: {response}");
    assert!(
        response.contains("target:/assets/index-abc123.js"),
        "expected path-preserving proxy, got: {response}"
    );

    let mut browser = TcpStream::connect(format!("127.0.0.1:{local_port}"))
        .await
        .expect("browser reconnects (keep-alive per connection)");
    browser
        .write_all(b"GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")
        .await
        .expect("browser sends root request");
    let root = read_http_response(&mut browser).await;
    assert!(root.contains("<div id=root>app</div>"), "expected React root, got: {root}");

    manager.clear().await;
}
