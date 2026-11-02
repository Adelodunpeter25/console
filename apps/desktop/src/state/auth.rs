use super::ConsoleDesktopApp;
use console_core::types::OAuthProviderId;
use gpui::Context;
use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::time::Duration;

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
        // Same for the previous attempt's error and copyable link.
        self.auth_login_error = None;
        self.auth_login_url = None;
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
                    fail_login(
                        &entity,
                        cx,
                        &provider_name,
                        format!("Couldn't start sign-in: {err:#}"),
                    );
                    return;
                }
            };

            // The URL is published before it is opened: if the system browser
            // opener truncates or drops it, the provider card still offers a
            // copyable link.
            log::info!("Opening {} sign-in URL: {auth_url}", provider_name);
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.auth_login_url = Some((provider_name.clone(), auth_url.clone()));
                        cx.notify();
                    });
                }
            });

            // Parse port from redirect_uri (e.g. http://localhost:8085/ or http://127.0.0.1:8085/)
            let port: u16 = redirect_uri
                .split(':')
                .nth(2)
                .and_then(|p| p.split('/').next())
                .and_then(|p| p.parse().ok())
                .unwrap_or(8085);

            // Bind BEFORE launching the browser, so a fast redirect cannot
            // arrive before the listener exists. RFC 8252 loopback redirects
            // only need the loopback interface, so never bind 0.0.0.0 (on
            // Windows that also exposes the listener to the LAN and shares
            // the port with specific-address binds).
            let listener = match bind_loopback(port).await {
                Ok(l) => l,
                Err(err) => {
                    log::error!("Failed to bind loopback listener on port {port}: {err}");
                    fail_login(
                        &entity,
                        cx,
                        &provider_name,
                        format!(
                            "Sign-in couldn't start because port {port} is already in use. \
                             Close the other Console window (including an older install) \
                             and try again."
                        ),
                    );
                    return;
                }
            };

            // Nonblocking accept: an abandoned sign-in must never pin the
            // port for the rest of the process lifetime.
            if let Err(err) = listener.set_nonblocking(true) {
                log::error!("Failed to set the loopback listener nonblocking: {err}");
                fail_login(
                    &entity,
                    cx,
                    &provider_name,
                    "Sign-in listener failed to start.".to_string(),
                );
                return;
            }

            // Route through the platform opener: on Windows that is
            // ShellExecuteW, so query strings keep their `&` separators
            // instead of being split by `cmd /C start`.
            let _ = cx.update(|cx| {
                cx.open_url(&auth_url);
            });

            // Wait for the callback on a blocking thread with a hard deadline,
            // so an abandoned sign-in releases the port after two minutes.
            let expected_state = state.clone();
            let outcome = tokio::task::spawn_blocking(move || {
                wait_for_callback(listener, &expected_state, Duration::from_secs(120))
            })
            .await
            .ok()
            .flatten();

            match outcome {
                Some(LoopbackResult::Code { code, state }) => {
                    match client
                        .auth
                        .handle_callback(provider, &code, Some(&state))
                        .await
                    {
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
                            // Success: the copyable link has nowhere to go.
                            let _ = cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.auth_login_url = None;
                                        cx.notify();
                                    });
                                }
                            });
                        }
                        Err(e) => {
                            log::error!("Failed to handle OAuth callback on server: {e}");
                            fail_login(
                                &entity,
                                cx,
                                &provider_name,
                                format!("The server rejected the sign-in callback: {e:#}"),
                            );
                        }
                    }
                }
                Some(LoopbackResult::Confirmed(message)) => {
                    fail_login(&entity, cx, &provider_name, message);
                }
                None => {
                    fail_login(
                        &entity,
                        cx,
                        &provider_name,
                        "Sign-in timed out waiting for the browser callback. Try again."
                            .to_string(),
                    );
                }
            }

            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.auth_logging_in.remove(&provider_name);
                        // recent_login_account deliberately stays: it highlights
                        // the account the callback just landed in, and is
                        // cleared only when the NEXT login starts.
                        // auth_login_url deliberately stays with the error, so
                        // the provider card still offers a copyable link.
                        this.refresh_auth_status(cx);
                        cx.notify();
                    });
                }
            });
        })
        .detach();
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

/// Outcome of one loopback callback capture.
enum LoopbackResult {
    Code {
        code: String,
        state: String,
    },
    /// The provider sent an `error=` instead of a code (e.g. access denied).
    Confirmed(String),
}

/// Binds the OAuth loopback listener on the IPv4 loopback interface.
///
/// Retries transient bind failures (an instance just exiting, or TIME_WAIT)
/// with backoff, then falls back to IPv6 loopback, which `localhost` may
/// resolve to first on Windows. Never binds `0.0.0.0`: that wildcard bind is
/// both a LAN exposure and the reason Windows shares the port across
/// specific-address binds, so it hid the real address conflict.
async fn bind_loopback(port: u16) -> std::io::Result<TcpListener> {
    const ATTEMPTS: u32 = 6;
    let mut last_error = None;

    for attempt in 0..ATTEMPTS {
        match TcpListener::bind(("127.0.0.1", port)) {
            Ok(listener) => return Ok(listener),
            Err(error) => {
                last_error = Some(error);
                if attempt + 1 < ATTEMPTS {
                    log::debug!(
                        "Loopback port {port} busy (attempt {}/{ATTEMPTS})",
                        attempt + 1
                    );
                    tokio::time::sleep(Duration::from_millis(250 * u64::from(attempt + 1))).await;
                }
            }
        }
    }

    match TcpListener::bind(("::1", port)) {
        Ok(listener) => Ok(listener),
        Err(_) => Err(last_error
            .unwrap_or_else(|| std::io::Error::other("no loopback address could be bound"))),
    }
}

/// Accept-and-capture loop for one OAuth callback.
///
/// The listener must already be nonblocking, so this loop polls with a short
/// sleep and always respects `timeout`: an abandoned sign-in frees the port
/// instead of pinning it for the process lifetime.
///
/// The success page is only sent when `expected_state` matches, and a wrong
/// state keeps waiting — a stray earlier attempt's redirect should not
/// consume this one.
fn wait_for_callback(
    listener: TcpListener,
    expected_state: &str,
    timeout: Duration,
) -> Option<LoopbackResult> {
    let deadline = std::time::Instant::now() + timeout;

    loop {
        if std::time::Instant::now() >= deadline {
            return None;
        }

        match listener.accept() {
            Ok((mut stream, _)) => {
                let _ = stream.set_read_timeout(Some(Duration::from_secs(4)));
                let mut buffer = [0u8; 8192];
                let read = stream.read(&mut buffer).unwrap_or(0);
                if read == 0 {
                    continue;
                }
                let request = String::from_utf8_lossy(&buffer[..read]);

                // Browsers probe the favicon after a same-tab navigation.
                if request.starts_with("GET /favicon.ico") {
                    respond(&mut stream, "404 Not Found", "");
                    continue;
                }

                match parse_callback(&request) {
                    Some(callback) if callback.error.is_some() => {
                        let message = format!(
                            "The provider refused the sign-in: {}",
                            callback
                                .error
                                .unwrap_or_else(|| "unknown error".to_string())
                        );
                        respond(&mut stream, "200 OK", DENIED_HTML);
                        return Some(LoopbackResult::Confirmed(message));
                    }
                    Some(callback) if callback.code.is_some() && callback.state.is_some() => {
                        let code = callback.code.unwrap_or_default();
                        let state = callback.state.unwrap_or_default();
                        if state == expected_state {
                            respond(&mut stream, "200 OK", SUCCESS_HTML);
                            return Some(LoopbackResult::Code { code, state });
                        }
                        log::warn!("Ignoring OAuth callback with a mismatched state");
                        respond(&mut stream, "400 Bad Request", "");
                    }
                    _ => {
                        // Favicon-adjacent probes and partial requests must
                        // not end the wait.
                        respond(&mut stream, "400 Bad Request", "");
                    }
                }
            }
            Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                std::thread::sleep(Duration::from_millis(50));
            }
            Err(error) => {
                log::debug!("OAuth loopback accept failed: {error}");
                std::thread::sleep(Duration::from_millis(50));
            }
        }
    }
}

/// The callback parameters a single GET can carry.
#[derive(Default)]
struct CallbackParams {
    code: Option<String>,
    state: Option<String>,
    error: Option<String>,
}

fn parse_callback(request: &str) -> Option<CallbackParams> {
    let line = request.lines().next()?;
    let mut fields = line.split_whitespace();
    if fields.next()? != "GET" {
        return None;
    }
    let target = fields.next()?;
    let (_, query) = target.split_once('?')?;

    // `split_once('=')` keeps any later `=` inside the value; a naive split
    // truncates codes that end in padding.
    let mut params = CallbackParams::default();
    for pair in query.split('&') {
        let (key, value) = pair.split_once('=').unwrap_or((pair, ""));
        let value = urlencoding::decode(value).unwrap_or_default().into_owned();
        match key {
            "code" => params.code = Some(value),
            "state" => params.state = Some(value),
            "error" => params.error = Some(value),
            _ => {}
        }
    }
    Some(params)
}

fn respond(stream: &mut TcpStream, status: &str, body: &str) {
    let content_type = if body.is_empty() {
        "text/plain"
    } else {
        "text/html; charset=utf-8"
    };
    let response = format!(
        "HTTP/1.1 {status}\r\nContent-Type: {content_type}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    let _ = stream.write_all(response.as_bytes());
    let _ = stream.flush();
}

/// Records a failed sign-in for its provider row and stops the spinner.
fn fail_login(
    entity: &gpui::WeakEntity<ConsoleDesktopApp>,
    cx: &mut gpui::AsyncApp,
    provider: &str,
    message: String,
) {
    let provider = provider.to_string();
    let _ = cx.update(|cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.auth_logging_in.remove(&provider);
                this.auth_login_error = Some((provider, message));
                this.refresh_auth_status(cx);
                cx.notify();
            });
        }
    });
}

const SUCCESS_HTML: &str = "<!DOCTYPE html><html><body style='font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;'><div style='text-align:center;padding:32px;background:#27272a;border-radius:12px;border:1px solid #3f3f46;'><h2 style='margin:0 0 8px 0;color:#22c55e;'>Authentication successful!</h2><p style='margin:0;color:#a1a1aa;'>You can close this tab and return to Console.</p></div></body></html>";

const DENIED_HTML: &str = "<!DOCTYPE html><html><body style='font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;'><div style='text-align:center;padding:32px;background:#27272a;border-radius:12px;border:1px solid #3f3f46;'><h2 style='margin:0 0 8px 0;color:#ef4444;'>Sign-in was not completed</h2><p style='margin:0;color:#a1a1aa;'>The provider refused the request. You can close this tab.</p></div></body></html>";
