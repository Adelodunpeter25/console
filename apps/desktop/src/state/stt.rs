//! Speech-to-text credential state for the Providers settings page.
//!
//! Separate from state/provider_credentials.rs, which is the single pasted
//! key for LLM providers. Speech-to-text needs a pair (Cloudflare account ID
//! + API token) for a server-side Whisper model — not chat credentials — so
//! it gets its own section and its own state, modeled on GitHub's section
//! (state/github.rs).
//!
//! Neither half is retained in app state. The pair goes from the two text
//! inputs into the request body and is dropped when the request completes;
//! what is cached is only the token-free status.

use super::ConsoleDesktopApp;
use gpui::Context;

impl ConsoleDesktopApp {
    /// Ask the server for the stored speech-to-text credential state.
    ///
    /// Reported, not silent, for GitHub's reason: "Not connected" must mean
    /// "no credential", and a failed fetch rendering the same text made an
    /// unreachable backend indistinguishable from a disconnected account.
    pub fn refresh_stt_status(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.stt.status().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(status) => {
                                this.stt_status = Some(status);
                                this.stt_error = None;
                            }
                            Err(error) => {
                                this.stt_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Store the Cloudflare account pair on the server.
    ///
    /// Both halves are required on every save: the token is never returned,
    /// so a partial update cannot preserve it. The pair is written into the
    /// request body and never retained — the caller drops it as soon as
    /// this returns. On success the cached status replaces it.
    pub fn save_stt_config(&mut self, account_id: String, api_token: String, cx: &mut Context<Self>) {
        let account_id = account_id.trim().to_string();
        let api_token = api_token.trim().to_string();
        if account_id.is_empty() || api_token.is_empty() {
            self.stt_error = Some("A Cloudflare account ID and API token are both required.".to_string());
            cx.notify();
            return;
        }
        if self.stt_busy {
            return;
        }

        self.stt_busy = true;
        self.stt_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.stt.set_config(&account_id, &api_token).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(response) => {
                                if let Some(status) = response.status {
                                    this.stt_status = Some(status);
                                }
                                this.stt_error = None;
                            }
                            // The server's wording is already written to be
                            // shown as-is, so pass it through rather than
                            // rewriting it.
                            Err(error) => {
                                this.stt_error = Some(error.to_string());
                            }
                        }
                        this.stt_busy = false;
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Remove the stored credential. Safe to call when already disconnected.
    pub fn disconnect_stt(&mut self, cx: &mut Context<Self>) {
        if self.stt_busy {
            return;
        }
        self.stt_busy = true;
        self.stt_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.stt.clear_config().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.stt_busy = false;
                        // Drop the cached status either way: if the remove
                        // call failed but the credential is gone, a stale
                        // "Connected" row would be worse than showing
                        // disconnected.
                        this.stt_status = None;
                        if let Err(error) = result {
                            this.stt_error = Some(error.to_string());
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Show or hide the credentials form.
    pub fn set_stt_form_open(&mut self, open: bool, cx: &mut Context<Self>) {
        self.stt_form_open = open;
        // Closing the form must not leave the previous failure on screen.
        if !open {
            self.stt_error = None;
        }
        cx.notify();
    }
}
