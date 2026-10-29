//! Server device pairing for the desktop app.
//!
//! Separate from state/auth.rs, which is the *provider* OAuth flow (Codex /
//! Claude / Antigravity). Pairing authenticates this app to a Console
//! server: the server issues a device token only in exchange for a one-time
//! pairing grant, and the token is what every later request carries.
//!
//! The token itself never enters app state as a separate field: it goes
//! straight into the shared transport, and only its *presence* is reported,
//! so no widget can accidentally render it.

use super::ConsoleDesktopApp;
use console_core::services::{DeviceInfo, PairingData, PairingGrantRequest, ServerDevice};
use gpui::Context;

impl ConsoleDesktopApp {
    /// Pair this app with a server using a host and a pairing code.
    ///
    /// This is the flow the settings sheet drives: the operator copies a
    /// host and a short code off the server (or scans the QR code the
    /// server prints), and the desktop exchanges the code for a credential.
    pub fn pair_with_code(
        &mut self,
        host: String,
        code: String,
        device_name: String,
        cx: &mut Context<Self>,
    ) {
        let address = host.trim().trim_end_matches('/').to_string();
        if address.is_empty() || code.trim().is_empty() {
            self.server_auth_error =
                Some("Enter the host and pairing code from the server.".to_string());
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
                .pair_with_code(&address, &code, &device_name)
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

    /// Create a short-lived pairing grant for another device (the QR flow).
    ///
    /// The grant carries no reusable credential: it is single-use and short
    /// lived, so handing it to a phone is safe even if the phone is later
    /// lost. The desktop's own token is never included.
    ///
    /// The server cannot know the address the phone should use — it may be
    /// behind a proxy or a tunnel — so this app says which address it is
    /// reaching the server at, and the server builds the QR code from that.
    pub fn create_pairing_grant(&mut self, device_name: String, cx: &mut Context<Self>) {
        if device_name.trim().is_empty() {
            self.server_auth_error = Some("Name the device you are pairing.".to_string());
            cx.notify();
            return;
        }
        let base_url = self.active_environment_url();
        // Only a paired device may mint a grant, so say so plainly instead
        // of letting the server answer "Authentication required." to a
        // question the user did not ask.
        if crate::persistence::server_auth_token(&base_url).is_none() {
            self.server_auth_error = Some(
                "Pair this device with the server first: add the server with its host and pairing code."
                    .to_string(),
            );
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
                    platform: "desktop".to_string(),
                    base_url: base_url.clone(),
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
                                // The list is now out of date: the revoked
                                // device is still on it.
                                this.refresh_server_devices(cx);
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
            Some(id) => {
                // The server names itself, and a paired environment is
                // named after the server it talks to. Without this the
                // name stays whatever placeholder the user typed before
                // the server had a chance to answer.
                self.update_environment(id.clone(), name, url, cx);
                id
            }
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
        self.server_devices.clear();
        self.server_pairing_grant = None;
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
