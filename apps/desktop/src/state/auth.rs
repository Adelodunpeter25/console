use std::io::{Read, Write};
use std::net::TcpListener;
use std::time::Duration;
use gpui::Context;
use console_core::types::OAuthProviderId;
use super::ConsoleDesktopApp;

fn open_browser(url: &str) {
    #[cfg(target_os = "macos")]
    {
        let _ = std::process::Command::new("open").arg(url).spawn();
    }
    #[cfg(target_os = "windows")]
    {
        let _ = std::process::Command::new("cmd").args(["/C", "start", url]).spawn();
    }
    #[cfg(target_os = "linux")]
    {
        let _ = std::process::Command::new("xdg-open").arg(url).spawn();
    }
}

impl ConsoleDesktopApp {
    pub fn refresh_auth_status(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if let Ok(status) = client.auth.status().await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.auth_status = Some(status);
                            cx.notify();
                        });
                    }
                });
            }
        }).detach();
    }

    pub fn login_provider(&mut self, provider_name: String, cx: &mut Context<Self>) {
        if self.auth_logging_in.contains(&provider_name) {
            return;
        }

        self.auth_logging_in.insert(provider_name.clone());
        cx.notify();

        if provider_name == "codebuff" {
            self.login_codebuff(cx);
            return;
        }

        let oauth_id = match provider_name.as_str() {
            "gemini" => OAuthProviderId::Gemini,
            "antigravity" => OAuthProviderId::Antigravity,
            "codex" => OAuthProviderId::Codex,
            _ => {
                self.auth_logging_in.remove(&provider_name);
                cx.notify();
                return;
            }
        };

        self.login_oauth(oauth_id, cx);
    }

    fn login_codebuff(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();

        cx.spawn(async move |entity, cx| {
            let res = client.auth.codebuff_start().await;
            let (login_url, fingerprint_id, fingerprint_hash, expires_at) = match res {
                Ok(data) => (data.login_url, data.fingerprint_id, data.fingerprint_hash, data.expires_at),
                Err(err) => {
                    log::error!("Failed to start Codebuff login: {err}");
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.auth_logging_in.remove("codebuff");
                                cx.notify();
                            });
                        }
                    });
                    return;
                }
            };

            open_browser(&login_url);

            let deadline = std::time::Instant::now() + Duration::from_millis(expires_at.saturating_sub(chrono::Utc::now().timestamp_millis()) as u64).min(Duration::from_secs(300));
            let expires_at_str = expires_at.to_string();

            while std::time::Instant::now() < deadline {
                tokio::time::sleep(Duration::from_secs(2)).await;
                if let Ok(poll) = client.auth.codebuff_poll(&fingerprint_id, &fingerprint_hash, &expires_at_str).await {
                    if poll.completed {
                        break;
                    }
                }
            }

            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.auth_logging_in.remove("codebuff");
                        this.refresh_auth_status(cx);
                        cx.notify();
                    });
                }
            });
        }).detach();
    }

    fn login_oauth(&mut self, provider: OAuthProviderId, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let provider_name = provider.as_str().to_string();

        cx.spawn(async move |entity, cx| {
            let res = client.auth.login_url(provider).await;
            let (auth_url, state, redirect_uri) = match res {
                Ok(resp) => (resp.auth_url, resp.state, resp.redirect_uri),
                Err(err) => {
                    log::error!("Failed to get login URL: {err}");
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.auth_logging_in.remove(&provider_name);
                                cx.notify();
                            });
                        }
                    });
                    return;
                }
            };

            // Parse port from redirect_uri (e.g. http://127.0.0.1:8085/)
            let port: u16 = redirect_uri
                .split(':')
                .nth(2)
                .and_then(|p| p.split('/').next())
                .and_then(|p| p.parse().ok())
                .unwrap_or(8085);

            open_browser(&auth_url);

            // Spawn one-shot blocking loopback listener
            let expected_state = state.clone();
            let callback_res = tokio::task::spawn_blocking(move || -> Option<(String, String)> {
                let listener = TcpListener::bind(("127.0.0.1", port)).ok()?;
                let _ = listener.set_nonblocking(false);

                // Wait up to 120s for one connection
                if let Ok((mut stream, _)) = listener.accept() {
                    let _ = stream.set_read_timeout(Some(Duration::from_secs(5)));
                    let mut buf = [0u8; 4096];
                    let n = stream.read(&mut buf).unwrap_or(0);
                    let request = String::from_utf8_lossy(&buf[..n]);

                    // Parse GET /?code=...&state=...
                    let mut code = None;
                    let mut returned_state = None;

                    if let Some(query_start) = request.find("GET /") {
                        let line = &request[query_start..request[query_start..].find("\r\n").unwrap_or(request.len())];
                        if let Some(q) = line.split('?').nth(1).and_then(|s| s.split_whitespace().next()) {
                            for part in q.split('&') {
                                let mut kv = part.split('=');
                                if let (Some(k), Some(v)) = (kv.next(), kv.next()) {
                                    if k == "code" {
                                        code = Some(urlencoding::decode(v).unwrap_or_default().into_owned());
                                    } else if k == "state" {
                                        returned_state = Some(urlencoding::decode(v).unwrap_or_default().into_owned());
                                    }
                                }
                            }
                        }
                    }

                    // Respond with HTML
                    let html = "<!DOCTYPE html><html><body style='font-family:sans-serif;background:#1a1a1a;color:#fff;display:flex;align-items:center;justify-content:center;height:90vh;'><h2>Authentication successful! You can close this tab and return to Console.</h2></body></html>";
                    let response = format!(
                        "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                        html.len(),
                        html
                    );
                    let _ = stream.write_all(response.as_bytes());

                    if let (Some(c), Some(s)) = (code, returned_state) {
                        if s == expected_state {
                            return Some((c, s));
                        }
                    }
                }
                None
            }).await.ok().flatten();

            if let Some((code, state)) = callback_res {
                let _ = client.auth.handle_callback(provider, &code, Some(&state)).await;
            }

            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.auth_logging_in.remove(&provider_name);
                        this.refresh_auth_status(cx);
                        cx.notify();
                    });
                }
            });
        }).detach();
    }

    pub fn save_gemini_project_id(&mut self, project_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let pid_opt = if project_id.is_empty() { None } else { Some(project_id) };
        cx.spawn(async move |entity, cx| {
            let _ = client.auth.save_project_id(OAuthProviderId::Gemini, pid_opt.as_deref()).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.refresh_auth_status(cx);
                        cx.notify();
                    });
                }
            });
        }).detach();
    }
}
