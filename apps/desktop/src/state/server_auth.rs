//! Server device pairing for the desktop app.
//!
//! Separate from state/auth.rs, which is the *provider* OAuth flow (Codex /
//! Claude / Antigravity). Pairing authenticates this app to a Console
//! server: the server issues a device token only in exchange for a one-time
//! pairing grant, and the token is what every later request carries.
//!
//! No UI lives here yet — this is the core logic the pairing screen will
//! call. The token itself never enters app state as a separate field: it
//! goes straight into the shared transport, and only its *presence* is
//! reported, so no widget can accidentally render it.

// The pairing UI lands in a later change. Until then these handlers and the
// state they write are unreachable, so the dead-code lint would fire on
// every one of them. The allow is scoped to this module (and to the pairing
// fields in `state::types`) and carries its reason, so it is removed along
// with the UI that makes it unnecessary.
#![allow(dead_code)]

use super::ConsoleDesktopApp;
use console_core::services::{DeviceInfo, PairingData, PairingGrantData, PairingGrantRequest, PairingRequest, ServerDevice};
use gpui::Context;

impl ConsoleDesktopApp {
    /// Pair this app with a server using a pasted pairing URL.
    ///
    /// The URL carries the server's address and the one-time grant, so one
    /// paste is the whole connection — no separate "which server" step. On
    /// success the device token is stored against that server's URL and
    /// installed on the shared transport, so every subsequent request — and
    /// the WebTransport handshake — is authenticated.
    pub fn pair_with_url(&mut self, url: String, device_name: String, cx: &mut Context<Self>) {
        if url.trim().is_empty() {
            self.server_auth_error = Some("Paste the pairing URL from the server.".to_string());
            cx.notify();
            return;
        }
        self.server_auth_busy = true;
        self.server_auth_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client
                .server_auth
                .pair_with_url(&url, &device_name, "desktop")
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.server_auth_busy = false;
                        match result {
                            Ok((data, base_url)) => {
                                this.server_auth_error = None;
                                let device = data.device.as_ref().cloned();
                                this.server_device = device.as_ref().map(|device| ServerDevice {
                                    id: device.id.clone(),
                                    name: device.name.clone(),
                                    platform: device.platform.clone(),
                                    created_at: None,
                                    last_seen_at: None,
                                    revoked_at: None,
                                });
                                this.apply_paired_device(base_url.clone(), &data, device, cx);
                                // The pairing URL named the server, so the app
                                // has to point at it: the environment it was on
                                // before may be a different machine, and a
                                // token for one server is not valid on
                                // another.
                                let server_name = data
                                    .server
                                    .as_ref()
                                    .map(|server| server.name.clone())
                                    .filter(|name| !name.trim().is_empty())
                                    .unwrap_or_else(|| "Paired Server".to_string());
                                this.adopt_paired_server(server_name, base_url, cx);
                            }
                            Err(error) => {
                                this.server_auth_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Pair this app with the active server using a one-time grant.
    ///
    /// On success the device token is stored against that server's URL and
    /// installed on the shared transport, so every subsequent request — and
    /// the WebTransport handshake — is authenticated. The grant is one-time:
    /// a second call with the same grant fails on the server.
    pub fn pair_with_server(
        &mut self,
        server_url: String,
        grant: String,
        device_name: String,
        cx: &mut Context<Self>,
    ) {
        if grant.trim().is_empty() {
            self.server_auth_error = Some("Enter the pairing code from the server.".to_string());
            cx.notify();
            return;
        }
        self.server_auth_busy = true;
        self.server_auth_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client
                .server_auth
                .pair(PairingRequest {
                    grant: grant.clone(),
                    device_name: device_name.clone(),
                    platform: "desktop".to_string(),
                })
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.server_auth_busy = false;
                        match result {
                            Ok(data) => {
                                this.server_auth_error = None;
                                // The paired device and the token live in one
                                // response; the device is optional in the
                                // schema, so it is read defensively.
                                let device = data.device.as_ref().cloned();
                                this.server_device = device.as_ref().map(|device| ServerDevice {
                                    id: device.id.clone(),
                                    name: device.name.clone(),
                                    platform: device.platform.clone(),
                                    created_at: None,
                                    last_seen_at: None,
                                    revoked_at: None,
                                });
                                this.apply_paired_device(server_url, &data, device, cx);
                            }
                            Err(error) => {
                                this.server_auth_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Create a short-lived pairing grant for another device (the QR flow).
    ///
    /// The grant carries no reusable credential: it is single-use and short
    /// lived, so handing it to a phone is safe even if the phone is later
    /// lost. The desktop's own token is never included.
    pub fn create_pairing_grant(
        &mut self,
        device_name: String,
        cx: &mut Context<Self>,
    ) {
        if device_name.trim().is_empty() {
            self.server_auth_error = Some("Name the device you are pairing.".to_string());
            cx.notify();
            return;
        }
        self.server_auth_busy = true;
        self.server_auth_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client
                .server_auth
                .create_pairing_grant(PairingGrantRequest {
                    device_name: device_name.clone(),
                    platform: "android".to_string(),
                })
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.server_auth_busy = false;
                        match result {
                            Ok(grant) => {
                                this.server_auth_error = None;
                                this.server_pairing_grant = Some(grant);
                            }
                            Err(error) => {
                                this.server_auth_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// List the devices paired with the active server.
    pub fn refresh_server_devices(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.server_auth.devices().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(devices) => {
                                this.server_devices = devices;
                                this.server_auth_error = None;
                            }
                            Err(error) => {
                                this.server_auth_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Revoke a paired device. Revoking the current device also drops the
    /// local credential, so the app stops authenticating as it.
    pub fn revoke_server_device(&mut self, device_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let current = self.server_device.as_ref().map(|device| device.id.clone());
        cx.spawn(async move |entity, cx| {
            let result = client.server_auth.revoke_device(&device_id).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(()) => {
                                this.server_auth_error = None;
                                if current.as_deref() == Some(device_id.as_str()) {
                                    this.server_device = None;
                                    let url = this.active_environment_url();
                                    let _ = crate::persistence::clear_server_auth(&url);
                                    this.clear_paired_device(cx);
                                }
                            }
                            Err(error) => {
                                this.server_auth_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Install a freshly issued device token: persist it for that server and
    /// put it on the shared transport.
    fn apply_paired_device(
        &mut self,
        server_url: String,
        data: &PairingData,
        device: Option<DeviceInfo>,
        cx: &mut Context<Self>,
    ) {
        let _ = crate::persistence::save_server_auth(
            &server_url,
            crate::persistence::StoredServerAuth {
                token: data.token.clone(),
                device_id: device.as_ref().map(|d| d.id.clone()).unwrap_or_default(),
                device_name: device.as_ref().map(|d| d.name.clone()).unwrap_or_default(),
                expires_at: data.expires_at,
            },
        );
        let client = self.client.clone();
        let token = data.token.clone();
        cx.spawn(async move |_entity, _cx| {
            client.set_auth_token(Some(token)).await;
        })
        .detach();
    }

    /// Point the app at the server it just paired with.
    ///
    /// A pairing URL names its server, so pairing with one means "connect to
    /// this machine" — the environment the app was on before may be a
    /// different one, and a device token is only valid on the server that
    /// issued it. An environment for that address is reused if it already
    /// exists, so pairing twice does not litter the list.
    fn adopt_paired_server(&mut self, name: String, url: String, cx: &mut Context<Self>) {
        let existing = self
            .environments
            .iter()
            .find(|env| env.url.trim_end_matches('/') == url.trim_end_matches('/'))
            .map(|env| env.id.clone());
        let env_id = match existing {
            Some(id) => id,
            None => {
                self.add_environment(name, url, cx);
                match self.environments.last() {
                    Some(env) => env.id.clone(),
                    None => return,
                }
            }
        };
        self.activate_environment(env_id, cx);
    }

    /// Drop the device credential from the transport and from disk.
    pub fn unpair_server(&mut self, cx: &mut Context<Self>) {
        let url = self.active_environment_url();
        let _ = crate::persistence::clear_server_auth(&url);
        self.server_device = None;
        self.clear_paired_device(cx);
        cx.notify();
    }

    fn clear_paired_device(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |_entity, _cx| {
            client.set_auth_token(None).await;
        })
        .detach();
    }
}

/// Convenience: the device rows the settings screen will render.
pub fn device_label(device: &ServerDevice) -> String {
    if device.name.trim().is_empty() {
        device.platform.clone()
    } else {
        device.name.clone()
    }
}

/// The grant payload a QR code would carry.
pub fn pairing_grant_payload(grant: &PairingGrantData, server_url: &str) -> String {
    format!(
        "{{\"v\":1,\"server\":\"{}\",\"grant\":\"{}\",\"expiresAt\":{}}}",
        server_url,
        grant.grant,
        grant.expires_at
    )
}
