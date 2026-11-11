//! The desktop tunnel against the real Go iroh bridge.
//!
//! Needs a binary built with `go build ./cmd/irohecho` in
//! apps/server-go, and its path in IROHECHO. Skipped when unset, since it
//! also needs internet (n0's relays).

use std::io::{BufRead, BufReader};
use std::process::{Command, Stdio};

use console_iroh::Tunnel;

fn start_echo(bin: &str) -> (std::process::Child, String) {
    let mut child = Command::new(bin).stdout(Stdio::piped()).spawn().unwrap();
    let mut lines = BufReader::new(child.stdout.take().unwrap()).lines();
    let id = loop {
        let line = lines.next().expect("irohecho exited").unwrap();
        if let Some(id) = line.strip_prefix("NODE_ID ") {
            break id.to_string();
        }
    };
    (child, id)
}

#[tokio::test(flavor = "multi_thread")]
async fn requests_reach_the_server_by_node_id() {
    let Ok(bin) = std::env::var("IROHECHO") else {
        eprintln!("IROHECHO not set; skipping");
        return;
    };
    let (mut child, node_id) = start_echo(&bin);
    let tunnel = Tunnel::connect(&node_id).await.expect("connect");
    let client = reqwest::Client::new();

    let resp = client
        .post(format!("{}/api/x?y=1", tunnel.base_url()))
        .body("hello")
        .send()
        .await
        .expect("request");
    assert_eq!(resp.status(), 200);
    assert_eq!(resp.headers()["x-seen-path"], "/api/x?y=1");
    assert_eq!(resp.text().await.unwrap(), "POST:hello");

    // Keep-alive: several requests ride the same tunnel concurrently.
    let mut set = tokio::task::JoinSet::new();
    for i in 0..5 {
        let (c, url) = (client.clone(), tunnel.base_url());
        set.spawn(async move {
            c.post(url)
                .body(format!("n{i}"))
                .send()
                .await
                .unwrap()
                .text()
                .await
                .unwrap()
        });
    }
    let mut got = Vec::new();
    while let Some(r) = set.join_next().await {
        got.push(r.unwrap());
    }
    got.sort();
    assert_eq!(got, ["POST:n0", "POST:n1", "POST:n2", "POST:n3", "POST:n4"]);

    child.kill().ok();
}

#[test]
fn rejects_a_bad_node_id() {
    assert!(console_iroh::parse_node_id("not-a-node").is_err());
    // A real ed25519 public key, as printed by the server.
    let real = "3d0588738abdeaf19cf855742d96e0ebbd0aadc13b4021415ab45bc880c2b19f";
    assert!(console_iroh::parse_node_id(real).is_ok());
}

/// Latency of a small request: plain loopback HTTP vs the same request
/// through the iroh tunnel. Run with `--ignored --nocapture`. Both ends are
/// on one machine, so this measures the tunnel's overhead, not the internet.
#[tokio::test(flavor = "multi_thread")]
#[ignore]
async fn latency_through_the_tunnel() {
    let Ok(bin) = std::env::var("IROHECHO") else {
        return;
    };
    let (mut child, node_id) = start_echo(&bin);
    let tunnel = Tunnel::connect(&node_id).await.expect("connect");
    let client = reqwest::Client::new();
    let url = tunnel.base_url();

    let mut times = Vec::new();
    for _ in 0..200 {
        let t = std::time::Instant::now();
        client
            .post(&url)
            .body("x")
            .send()
            .await
            .unwrap()
            .text()
            .await
            .unwrap();
        times.push(t.elapsed());
    }
    times.sort();
    let ms = |d: std::time::Duration| d.as_secs_f64() * 1000.0;
    eprintln!(
        "iroh tunnel, 200 requests: p50 {:.2} ms, p95 {:.2} ms, max {:.2} ms",
        ms(times[100]),
        ms(times[190]),
        ms(times[199])
    );
    child.kill().ok();
}
