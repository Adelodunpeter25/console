//! GitHub git-credential state for the Providers settings page.
//!
//! Separate from state/auth.rs, which is the OAuth flow for the LLM chat
//! providers (Codex / Claude / Antigravity). GitHub is not a chat provider —
//! its token exists so the git CLI on the *server host* can clone private
//! repositories — so it authenticates by pasted token, not by browser.
//!
//! That difference is why this cannot reuse `login_provider`: that path opens
//! a browser and waits on a loopback listener bound to this machine, which
//! never completes when the backend is a remote box.

use super::ConsoleDesktopApp;
use gpui::Context;

impl ConsoleDesktopApp {
    /// Ask the server for the stored GitHub credential state.
    ///
    /// A failure is silent by design: this runs on page open, and a backend
    /// that is merely offline should not surface as a GitHub error. The row
    /// shows "Not connected" in that case, which is the safe reading.
    pub fn refresh_github_status(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if let Ok(status) = client.auth.github_status().await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.github_status = Some(status);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    /// Validate and store a personal access token.
    ///
    /// The token is written straight into the request body and never retained
    /// in app state — the caller drops it as soon as this returns. On success
    /// the cached username/scopes replace the typed token, so the UI can clear
    /// the field without losing the row's state.
    pub fn connect_github(&mut self, token: String, cx: &mut Context<Self>) {
        let trimmed = token.trim().to_string();
        if trimmed.is_empty() {
            self.github_error = Some("Enter a personal access token.".to_string());
            cx.notify();
            return;
        }
        if self.github_busy {
            return;
        }

        self.github_busy = true;
        self.github_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.auth.connect_github(&trimmed).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(status) => {
                                this.github_status = Some(status);
                                this.github_error = None;
                            }
                            // The server's wording is already written to be
                            // shown as-is (it separates "bad token" from
                            // "GitHub is down"), so pass it through rather than
                            // rewriting it.
                            Err(error) => {
                                this.github_error = Some(error.to_string());
                            }
                        }
                        this.github_busy = false;
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Remove the stored token. Safe to call when already disconnected.
    pub fn disconnect_github(&mut self, cx: &mut Context<Self>) {
        if self.github_busy {
            return;
        }
        self.github_busy = true;
        self.github_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.auth.disconnect_github().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.github_busy = false;
                        // Drop the cached status either way: if the remove call
                        // failed but the token is gone, a stale "connected" row
                        // would be worse than showing disconnected.
                        this.github_status = None;
                        if let Err(error) = result {
                            this.github_error = Some(error.to_string());
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Whether the token sheet / field is showing.
    pub fn github_connect_form_open(&self) -> bool {
        self.github_form_open
    }

    pub fn set_github_form_open(&mut self, open: bool, cx: &mut Context<Self>) {
        self.github_form_open = open;
        // Closing the sheet must not leave the previous failure on screen.
        if !open {
            self.github_error = None;
        }
        cx.notify();
    }
}
