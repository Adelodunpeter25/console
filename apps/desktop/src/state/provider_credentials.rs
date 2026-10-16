//! Pasted provider API keys for the Providers settings page.
//!
//! Separate from state/auth.rs, which is the OAuth flow for the LLM chat
//! providers (Codex / Claude / Antigravity). A key provider authenticates by a
//! secret the user types once and nobody renews, so — like GitHub's token — it
//! cannot reuse `login_provider`.
//!
//! The key never lands in app state. It goes from the text input into the
//! request body and is dropped when the request completes; what is cached is
//! only *whether* one exists, which is all the server will tell us anyway.

use super::ConsoleDesktopApp;
use gpui::Context;

impl ConsoleDesktopApp {
    /// Ask the server which providers have a key stored.
    ///
    /// Reported, not silent, for GitHub's reason: "Using the free tier" must
    /// mean "no key", and a failed fetch rendering the same text made an
    /// unreachable backend indistinguishable from an unconfigured provider.
    pub fn refresh_provider_credentials(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.auth.provider_credentials().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(statuses) => {
                                this.provider_credentials = statuses.providers;
                                this.provider_credential_error = None;
                            }
                            Err(error) => {
                                this.provider_credential_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Store a pasted API key on the server for one provider.
    ///
    /// The key goes into the request body and is never retained — the caller
    /// drops it as soon as this returns. On success the server's status for
    /// that provider replaces whatever was cached, so the row updates without
    /// a second round trip.
    pub fn set_provider_credential(
        &mut self,
        provider: String,
        api_key: String,
        cx: &mut Context<Self>,
    ) {
        let trimmed = api_key.trim().to_string();
        if trimmed.is_empty() {
            self.provider_credential_error = Some("Enter an API key.".to_string());
            cx.notify();
            return;
        }
        if self.provider_credential_busy.contains(&provider) {
            return;
        }

        self.provider_credential_busy.insert(provider.clone());
        self.provider_credential_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client
                .auth
                .set_provider_credential(&provider, &trimmed)
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(response) => {
                                this.provider_credentials.insert(
                                    provider.clone(),
                                    console_core::types::ProviderCredentialStatus {
                                        configured: response.configured,
                                        source: response.source,
                                        updated_at: None,
                                    },
                                );
                                this.provider_credential_error = None;
                            }
                            // The server's wording is written to be shown as-is.
                            Err(error) => {
                                this.provider_credential_error = Some(error.to_string());
                            }
                        }
                        this.provider_credential_busy.remove(&provider);
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Remove a provider's stored key, falling back to its free tier.
    ///
    /// Cannot remove a key supplied by the server's environment variable, so
    /// the UI hides the action rather than offering one that does nothing.
    pub fn clear_provider_credential(&mut self, provider: String, cx: &mut Context<Self>) {
        if self.provider_credential_busy.contains(&provider) {
            return;
        }
        self.provider_credential_busy.insert(provider.clone());
        self.provider_credential_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.auth.clear_provider_credential(&provider).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // Drop the cached status either way: if the call failed
                        // but the key is gone, a stale "Using your API key" row
                        // would be worse than showing the free tier.
                        this.provider_credentials.insert(
                            provider.clone(),
                            console_core::types::ProviderCredentialStatus {
                                configured: None,
                                source: None,
                                updated_at: None,
                            },
                        );
                        if let Err(error) = result {
                            this.provider_credential_error = Some(error.to_string());
                        }
                        this.provider_credential_busy.remove(&provider);
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Show or hide one provider's key field.
    pub fn set_provider_credential_form_open(
        &mut self,
        provider: String,
        open: bool,
        cx: &mut Context<Self>,
    ) {
        if open {
            self.provider_credential_forms_open.insert(provider);
        } else {
            self.provider_credential_forms_open.remove(&provider);
        }
        // Closing the form must not leave a previous failure on screen.
        if !open {
            self.provider_credential_error = None;
        }
        cx.notify();
    }
}