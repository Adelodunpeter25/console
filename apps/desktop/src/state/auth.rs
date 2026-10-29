use super::ConsoleDesktopApp;
use console_core::types::OAuthProviderId;
use gpui::Context;
use std::io::{Read, Write};
use std::net::TcpListener;
use std::time::Duration;

fn open_browser(url: &str) {
    #[cfg(target_os = "macos")]
    {
        let _ = std::process::Command::new("open").arg(url).spawn();
    }
    #[cfg(target_os = "windows")]
    {
        let _ = std::process::Command::new("cmd")
            .args(["/C", "start", url])
            .spawn();
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
        })
        .detach();
    }

    pub fn login_provider(&mut self, provider_name: String, cx: &mut Context<Self>) {
        if self.auth_logging_in.contains(&provider_name) {
            return;
        }

        self.auth_logging_in.insert(provider_name.clone());
        // A new login is starting: the previous callback's highlight no
        // longer describes anything the user is looking at.
        self.recent_login_account = None;
        cx.notify();

        let oauth_id = match provider_name.as_str() {
            "antigravity" => OAuthProviderId::Antigravity,
            "codex" | "openai" => OAuthProviderId::Codex,
            "claude" | "anthropic" => OAuthProviderId::Claude,
            _ => {
                self.auth_logging_in.remove(&provider_name);
                cx.notify();
                return;
            }
        };

        self.login_oauth(oauth_id, cx);
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

            // Parse port from redirect_uri (e.g. http://localhost:8085/ or http://127.0.0.1:8085/)
            let port: u16 = redirect_uri
                .split(':')
                .nth(2)
                .and_then(|p| p.split('/').next())
                .and_then(|p| p.parse().ok())
                .unwrap_or(8085);

            // Bind listener BEFORE launching browser to avoid race conditions with fast redirects
            let listener = match TcpListener::bind(("127.0.0.1", port))
                .or_else(|_| TcpListener::bind(("0.0.0.0", port)))
                .or_else(|_| TcpListener::bind(("localhost", port)))
            {
                Ok(l) => l,
                Err(err) => {
                    log::error!("Failed to bind loopback listener on port {port}: {err}");
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

            let _ = listener.set_nonblocking(false);

            // Now open browser safely
            open_browser(&auth_url);

            // Spawn loopback callback handler
            let expected_state = state.clone();
            let callback_res = tokio::task::spawn_blocking(move || -> Option<(String, String)> {
                let start = std::time::Instant::now();
                let deadline = Duration::from_secs(120);

                while start.elapsed() < deadline {
                    if let Ok((mut stream, _)) = listener.accept() {
                        let _ = stream.set_read_timeout(Some(Duration::from_secs(4)));
                        let mut buf = [0u8; 4096];
                        let n = stream.read(&mut buf).unwrap_or(0);
                        if n == 0 {
                            continue;
                        }
                        let request = String::from_utf8_lossy(&buf[..n]);

                        // Handle browser favicon probes gracefully
                        if request.contains("GET /favicon.ico") {
                            let response = "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n";
                            let _ = stream.write_all(response.as_bytes());
                            continue;
                        }

                        let mut code = None;
                        let mut returned_state = None;

                        for line in request.lines() {
                            if line.starts_with("GET ") {
                                if let Some(path_and_query) = line.split_whitespace().nth(1) {
                                    if let Some(query) = path_and_query.split('?').nth(1) {
                                        for part in query.split('&') {
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
                                break;
                            }
                        }

                        if let (Some(c), Some(s)) = (code, returned_state) {
                            let html = "<!DOCTYPE html><html><body style='font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;'><div style='text-align:center;padding:32px;background:#27272a;border-radius:12px;border:1px solid #3f3f46;'><h2 style='margin:0 0 8px 0;color:#22c55e;'>Authentication successful!</h2><p style='margin:0;color:#a1a1aa;'>You can close this tab and return to Console.</p></div></body></html>";
                            let response = format!(
                                "HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                                html.len(),
                                html
                            );
                            let _ = stream.write_all(response.as_bytes());
                            let _ = stream.flush();

                            if s == expected_state {
                                return Some((c, s));
                            }
                        } else {
                            let response = "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n";
                            let _ = stream.write_all(response.as_bytes());
                        }
                    }
                }
                None
            }).await.ok().flatten();

            if let Some((code, state)) = callback_res {
                match client.auth.handle_callback(provider, &code, Some(&state)).await {
                    Ok(resp) => {
                        // Remember which account the login landed in so the
                        // providers page highlights it. `account_id` is
                        // optional on the wire; with it missing, a full
                        // status refresh shows the (possibly new) active
                        // account anyway.
                        if let Some(id) = resp.account_id {
                            let _ = cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, _cx| {
                                        this.recent_login_account =
                                            Some((provider_name.clone(), id));
                                    });
                                }
                            });
                        }
                    }
                    Err(e) => {
                        log::error!("Failed to handle OAuth callback on server: {e}");
                    }
                }
            }

            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.auth_logging_in.remove(&provider_name);
                        // recent_login_account deliberately stays: it highlights
                        // the account the callback just landed in, and is
                        // cleared only when the NEXT login starts.
                        this.refresh_auth_status(cx);
                        cx.notify();
                    });
                }
            });
        }).detach();
    }

    /// Activates one stored account (decision 2: explicit switching). The
    /// mutation response already carries the provider's post-mutation
    /// status, so the update applies it without a second status read.
    pub fn use_account(&mut self, provider: String, account_id: String, cx: &mut Context<Self>) {
        if self
            .account_busy
            .as_ref()
            .is_some_and(|(p, i)| *p == provider && *i == account_id)
        {
            return;
        }
        self.account_busy = Some((provider.clone(), account_id.clone()));
        self.account_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let res = client
                .auth
                .patch_account(&provider, &account_id, Some(true), None)
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, _cx| match res {
                        Ok(mutated) => {
                            if let Some(status) = this.auth_status.as_mut() {
                                console_core::types::merge_provider_status(
                                    status,
                                    &mutated.provider,
                                    mutated.status.unwrap_or_default(),
                                );
                            }
                            this.account_busy = None;
                        }
                        Err(err) => {
                            this.account_busy = None;
                            this.account_error = Some((provider.clone(), format!("{err:#}")));
                        }
                    });
                }
            });
        })
        .detach();
    }

    /// Gives an account a display label. Same flow as `use_account`: the
    /// mutation response carries the provider's post-mutation status and the
    /// caller updates its account list from it alone. While the request is in
    /// flight the rename form's Save button reads busy; the settings window
    /// closes the form when submissions start.
    pub fn rename_account(
        &mut self,
        provider: String,
        account_id: String,
        label: String,
        cx: &mut Context<Self>,
    ) {
        if self
            .account_busy
            .as_ref()
            .is_some_and(|(p, i)| *p == provider && *i == account_id)
        {
            return;
        }
        self.account_busy = Some((provider.clone(), account_id.clone()));
        self.account_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let res = client
                .auth
                .patch_account(&provider, &account_id, None, Some(&label))
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, _cx| match res {
                        Ok(mutated) => {
                            if let Some(status) = this.auth_status.as_mut() {
                                console_core::types::merge_provider_status(
                                    status,
                                    &mutated.provider,
                                    mutated.status.unwrap_or_default(),
                                );
                            }
                            this.account_busy = None;
                            this.rename_form = None;
                        }
                        Err(err) => {
                            this.account_busy = None;
                            this.account_error = Some((provider.clone(), format!("{err:#}")));
                            // The rename dialog stays open: it shows the
                            // error next to the name that failed, so the
                            // user fixes rather than retypes.
                        }
                    });
                }
            });
        })
        .detach();
    }

    /// Removes one stored account (decision 4: deleting the active one
    /// promotes the next-oldest account server-side). Removing a provider's
    /// last account also clears its legacy file, so the row's status may
    /// drop to "Not connected" from the same response.
    pub fn remove_account(&mut self, provider: String, account_id: String, cx: &mut Context<Self>) {
        if self
            .account_busy
            .as_ref()
            .is_some_and(|(p, i)| *p == provider && *i == account_id)
        {
            return;
        }
        self.account_busy = Some((provider.clone(), account_id.clone()));
        self.account_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let res = client.auth.delete_account(&provider, &account_id).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, _cx| match res {
                        Ok(mutated) => {
                            if let Some(status) = this.auth_status.as_mut() {
                                console_core::types::merge_provider_status(
                                    status,
                                    &mutated.provider,
                                    mutated.status.unwrap_or_default(),
                                );
                            }
                            if this
                                .recent_login_account
                                .as_ref()
                                .is_some_and(|(p, id)| *p == provider && id == &account_id)
                            {
                                this.recent_login_account = None;
                            }
                            this.account_busy = None;
                        }
                        Err(err) => {
                            this.account_busy = None;
                            this.account_error = Some((provider.clone(), format!("{err:#}")));
                        }
                    });
                }
            });
        })
        .detach();
    }
}
