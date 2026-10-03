//! MCP settings tab: server list, add/edit modal, connect (including the
//! client-loopback OAuth flow) and the advertised-tools accordion. Owned by
//! `SettingsWindow` as a single `mcp` field; rendering delegates here so
//! this flow lives next to its state instead of inside the window file.
use std::rc::Rc;

use console_ui::input::ComposerInput;
use console_ui::settings::{McpModalMode, McpPage};
use console_core::types::mcp::{
    McpAuthType, McpConnectionStatus, McpServerConfig, McpTransportType,
};
use gpui::{App, AppContext, Context, Entity, IntoElement, Window};

use crate::settings_window::SettingsWindow;
use crate::state::ConsoleDesktopApp;

pub struct McpTabState {
    app: gpui::WeakEntity<ConsoleDesktopApp>,
    pub servers: Vec<McpServerConfig>,
    pub is_modal_open: bool,
    pub modal_mode: McpModalMode,
    pub selected_transport: McpTransportType,
    pub selected_auth: McpAuthType,
    pub name_input: Entity<ComposerInput>,
    pub url_input: Entity<ComposerInput>,
    pub command_input: Entity<ComposerInput>,
    pub args_input: Entity<ComposerInput>,
    pub env_input: Entity<ComposerInput>,
    pub expanded_server_id: Option<String>,
    pub action_error: Option<String>,
    pub is_submitting: bool,
}

impl McpTabState {
    pub fn new(
        app: gpui::WeakEntity<ConsoleDesktopApp>,
        window: &mut Window,
        cx: &mut Context<SettingsWindow>,
    ) -> Self {
        let name_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. atlassian, filesystem", cx);
            input
        });
        let url_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("https://mcp.atlassian.com/v2/mcp", cx);
            input
        });
        let command_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("npx, uvx, docker, or path to binary", cx);
            input
        });
        let args_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder(
                "-y @modelcontextprotocol/server-filesystem /path/to/folder",
                cx,
            );
            input
        });
        let env_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("API_KEY=xyz, DEBUG=true", cx);
            input
        });
        Self {
            app,
            servers: Vec::new(),
            is_modal_open: false,
            modal_mode: McpModalMode::Add,
            selected_transport: McpTransportType::Stdio,
            selected_auth: McpAuthType::None,
            name_input,
            url_input,
            command_input,
            args_input,
            env_input,
            expanded_server_id: None,
            action_error: None,
            is_submitting: false,
        }
    }

    pub fn fetch_mcp_servers(&mut self, cx: &mut Context<SettingsWindow>) {
        let Some(app_entity) = self.app.upgrade() else {
            return;
        };
        let client = app_entity.read(cx).client.clone();
        cx.spawn(async move |entity, cx| {
            if let Ok(servers) = client.mcp.list_servers().await {
                let _ = cx.update(|cx| {
                    let _ = entity.update(cx, |window: &mut SettingsWindow, cx| {
                        window.mcp.servers = servers;
                        cx.notify();
                    });
                });
            }
        })
        .detach();
    }
}

impl McpTabState {
    pub fn render_mcp_tab(
        &self,
        window: &mut Window,
        cx: &mut Context<SettingsWindow>,
    ) -> gpui::AnyElement {
        let _ = window;
        let on_open_add: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let name_input = self.name_input.clone();
            let url_input = self.url_input.clone();
            let command_input = self.command_input.clone();
            let args_input = self.args_input.clone();
            let env_input = self.env_input.clone();
            Rc::new(move |_w: &mut Window, cx: &mut App| {
                name_input.update(cx, |input, cx| input.clear(cx));
                url_input.update(cx, |input, cx| input.clear(cx));
                command_input.update(cx, |input, cx| input.clear(cx));
                args_input.update(cx, |input, cx| input.clear(cx));
                env_input.update(cx, |input, cx| input.clear(cx));
                entity.update(cx, |this, cx| {
                    this.mcp.is_modal_open = true;
                    this.mcp.modal_mode = McpModalMode::Add;
                    this.mcp.selected_transport = McpTransportType::Stdio;
                    this.mcp.selected_auth = McpAuthType::None;
                    this.mcp.action_error = None;
                    cx.notify();
                });
            })
        };

        let on_open_edit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let name_input = self.name_input.clone();
            let url_input = self.url_input.clone();
            let command_input = self.command_input.clone();
            let args_input = self.args_input.clone();
            let env_input = self.env_input.clone();
            Rc::new(move |server_id: String, _w: &mut Window, cx: &mut App| {
                entity.update(cx, |this, cx| {
                    if let Some(srv) = this.mcp.servers.iter().find(|s| s.id == server_id) {
                        name_input.update(cx, |input, cx| input.set_content(srv.name.clone(), cx));
                        url_input.update(cx, |input, cx| input.set_content(srv.url.clone().unwrap_or_default(), cx));
                        command_input.update(cx, |input, cx| input.set_content(srv.command.clone().unwrap_or_default(), cx));
                        args_input.update(cx, |input, cx| input.set_content(srv.args.join(" "), cx));
                        let env_str = srv.env.iter().map(|(k, v)| format!("{}={}", k, v)).collect::<Vec<_>>().join(", ");
                        env_input.update(cx, |input, cx| input.set_content(env_str, cx));

                        this.mcp.is_modal_open = true;
                        this.mcp.modal_mode = McpModalMode::Edit(server_id.clone());
                        this.mcp.selected_transport = srv.transport.clone();
                        this.mcp.selected_auth = srv.auth_type.clone();
                        this.mcp.action_error = None;
                        cx.notify();
                    }
                });
            })
        };

        let on_close_modal: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            Rc::new(move |_w: &mut Window, cx: &mut App| {
                entity.update(cx, |this, cx| {
                    this.mcp.is_modal_open = false;
                    this.mcp.action_error = None;
                    cx.notify();
                });
            })
        };

        let on_select_transport: Rc<dyn Fn(McpTransportType, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            Rc::new(move |transport, _w: &mut Window, cx: &mut App| {
                entity.update(cx, |this, cx| {
                    this.mcp.selected_transport = transport;
                    cx.notify();
                });
            })
        };

        let on_select_auth: Rc<dyn Fn(McpAuthType, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            Rc::new(move |auth, _w: &mut Window, cx: &mut App| {
                entity.update(cx, |this, cx| {
                    this.mcp.selected_auth = auth;
                    cx.notify();
                });
            })
        };

        let on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let app_handle = self.app.clone();
            let name_input = self.name_input.clone();
            let url_input = self.url_input.clone();
            let command_input = self.command_input.clone();
            let args_input = self.args_input.clone();
            let env_input = self.env_input.clone();

            Rc::new(move |_w: &mut Window, cx: &mut App| {
                let Some(app) = app_handle.upgrade() else { return };
                let client = app.read(cx).client.clone();
                let name = name_input.read(cx).content().trim().to_string();
                let url = url_input.read(cx).content().trim().to_string();
                let cmd = command_input.read(cx).content().trim().to_string();
                let args_str = args_input.read(cx).content().trim().to_string();
                let env_str = env_input.read(cx).content().trim().to_string();

                if name.is_empty() {
                    entity.update(cx, |this, cx| {
                        this.mcp.action_error = Some("Server name is required".to_string());
                        cx.notify();
                    });
                    return;
                }

                let args = if args_str.is_empty() {
                    Vec::new()
                } else {
                    args_str.split_whitespace().map(String::from).collect()
                };

                let env = if env_str.is_empty() {
                    Vec::new()
                } else {
                    env_str.split(',')
                        .filter_map(|pair| {
                            let mut parts = pair.splitn(2, '=');
                            let k = parts.next()?.trim().to_string();
                            let v = parts.next().unwrap_or("").trim().to_string();
                            if k.is_empty() { None } else { Some((k, v)) }
                        })
                        .collect()
                };

                let (transport, auth, id) = {
                    let w = entity.read(cx);
                    let t = w.mcp.selected_transport.clone();
                    let a = w.mcp.selected_auth.clone();
                    let srv_id = match &w.mcp.modal_mode {
                        McpModalMode::Add => {
                            name.to_lowercase().replace(' ', "-").replace(|c: char| !c.is_alphanumeric() && c != '-', "")
                        }
                        McpModalMode::Edit(id) => id.clone(),
                    };
                    (t, a, srv_id)
                };

                let server = McpServerConfig {
                    id: id.clone(),
                    name,
                    transport: transport.clone(),
                    url: if transport == McpTransportType::Http { Some(url) } else { None },
                    auth_type: auth,
                    command: if transport == McpTransportType::Stdio { Some(cmd) } else { None },
                    args,
                    env,
                    status: McpConnectionStatus::Disconnected,
                    auth_url: None,
                    tools: Vec::new(),
                };

                entity.update(cx, |this, cx| {
                    this.mcp.is_submitting = true;
                    this.mcp.action_error = None;
                    cx.notify();
                });

                let entity_clone = entity.clone();
                cx.spawn(async move |cx| {
                    match client.mcp.save_server(&server).await {
                        Ok(_) => {
                            let updated_list = client.mcp.list_servers().await.unwrap_or_default();
                            let _ = cx.update(|cx| {
                                entity_clone.update(cx, |this, cx| {
                                    this.mcp.servers = updated_list;
                                    this.mcp.is_modal_open = false;
                                    this.mcp.is_submitting = false;
                                    cx.notify();
                                });
                            });
                        }
                        Err(e) => {
                            let _ = cx.update(|cx| {
                                entity_clone.update(cx, |this, cx| {
                                    this.mcp.action_error = Some(format!("Failed to save server: {}", e));
                                    this.mcp.is_submitting = false;
                                    cx.notify();
                                });
                            });
                        }
                    }
                })
                .detach();
            })
        };

        let on_delete: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let app_handle = self.app.clone();
            Rc::new(move |id: String, _w: &mut Window, cx: &mut App| {
                let Some(app) = app_handle.upgrade() else { return };
                let client = app.read(cx).client.clone();
                let entity_clone = entity.clone();
                cx.spawn(async move |cx| {
                    if let Ok(_) = client.mcp.delete_server(&id).await {
                        let updated_list = client.mcp.list_servers().await.unwrap_or_default();
                        let _ = cx.update(|cx| {
                            entity_clone.update(cx, |this, cx| {
                                this.mcp.servers = updated_list;
                                cx.notify();
                            });
                        });
                    }
                })
                .detach();
            })
        };

        let on_connect: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let app_handle = self.app.clone();
            Rc::new(move |id: String, _w: &mut Window, cx: &mut App| {
                let Some(app) = app_handle.upgrade() else { return };
                let client = app.read(cx).client.clone();
                let entity_clone = entity.clone();
                // Optimistically set to Connecting
                entity.update(cx, |this, cx| {
                    if let Some(s) = this.mcp.servers.iter_mut().find(|s| s.id == id) {
                        s.status = McpConnectionStatus::Connecting;
                        cx.notify();
                    }
                });
                cx.spawn(async move |cx| {
                    // Client-owned loopback, bound before connect
                    // so the redirect URI is known up front. Falls
                    // back to server loopback when the bind fails.
                    let listener =
                        std::net::TcpListener::bind(("127.0.0.1", 0)).ok();
                    let redirect_uri: Option<String> = listener
                        .as_ref()
                        .and_then(|l| l.local_addr().ok())
                        .map(|a| {
                            format!("http://127.0.0.1:{}/callback", a.port())
                        });
                    if let Err(e) = client
                        .mcp
                        .start_connect(&id, redirect_uri.as_deref())
                        .await
                    {
                        let _ = cx.update(|cx| {
                            entity_clone.update(cx, |this, cx| {
                                if let Some(s) = this.mcp.servers.iter_mut().find(|s| s.id == id) {
                                    s.status = McpConnectionStatus::Error(e.to_string());
                                    cx.notify();
                                }
                            });
                        });
                        return;
                    }
                    // Poll until the server leaves `connecting` —
                    // connected, failed, or waiting on the browser.
                    let mut auth_url: Option<String> = None;
                    let mut done = false;
                    for _ in 0..240 {
                        match client.mcp.list_servers().await {
                            Ok(list) => {
                                let terminal = list.iter().find(|s| s.id == id).map(|s| {
                                    match &s.status {
                                        McpConnectionStatus::NeedsAuth => {
                                            auth_url = s.auth_url.clone();
                                            true
                                        }
                                        McpConnectionStatus::Connecting => false,
                                        _ => {
                                            done = true;
                                            true
                                        }
                                    }
                                });
                                let _ = cx.update(|cx| {
                                    entity_clone.update(cx, |this, cx| {
                                        this.mcp.servers = list;
                                        cx.notify();
                                    });
                                });
                                if terminal == Some(true) {
                                    break;
                                }
                            }
                            Err(e) => {
                                let _ = cx.update(|cx| {
                                    entity_clone.update(cx, |this, cx| {
                                        if let Some(s) = this.mcp.servers.iter_mut().find(|s| s.id == id) {
                                            s.status = McpConnectionStatus::Error(e.to_string());
                                            cx.notify();
                                        }
                                    });
                                });
                                return;
                            }
                        }
                        tokio::time::sleep(std::time::Duration::from_millis(250)).await;
                    }
                    if done {
                        return;
                    }
                    let Some(auth_url) = auth_url else {
                        return;
                    };
                    // Without a client redirect the server owns the
                    // loopback: just open the URL and let the
                    // server-side flow finish on its own box.
                    if listener.is_none() {
                        let _ = cx.update(|cx| {
                            cx.open_url(&auth_url);
                        });
                        return;
                    }
                    let _ = cx.update(|cx| {
                        cx.open_url(&auth_url);
                    });
                    // Serve our own loopback, then forward the
                    // captured code+state into the pending server
                    // flow.
                    let forwarded = match listener {
                        Some(listener) => {
                            let captured = tokio::task::spawn_blocking(move || {
                                serve_mcp_callback_once(listener)
                            })
                            .await
                            .ok()
                            .flatten();
                            match captured {
                                Some((state, code, err_msg)) => {
                                    client
                                        .mcp
                                        .forward_oauth_callback(
                                            &id,
                                            &state,
                                            code.as_deref(),
                                            err_msg.as_deref(),
                                            None,
                                        )
                                        .await
                                        .is_ok()
                                }
                                None => false,
                            }
                        }
                        None => false,
                    };
                    if !forwarded {
                        let _ = cx.update(|cx| {
                            entity_clone.update(cx, |this, cx| {
                                if let Some(s) = this.mcp.servers.iter_mut().find(|s| s.id == id) {
                                    s.status = McpConnectionStatus::Error(
                                        "browser sign-in timed out".to_string(),
                                    );
                                    cx.notify();
                                }
                            });
                        });
                        return;
                    }
                    // The exchange runs server-side now; poll for
                    // the terminal state.
                    for _ in 0..360 {
                        match client.mcp.list_servers().await {
                            Ok(list) => {
                                let terminal = list.iter().find(|s| s.id == id).map(|s| {
                                    !matches!(
                                        s.status,
                                        McpConnectionStatus::Connecting
                                            | McpConnectionStatus::NeedsAuth
                                    )
                                });
                                let _ = cx.update(|cx| {
                                    entity_clone.update(cx, |this, cx| {
                                        this.mcp.servers = list;
                                        cx.notify();
                                    });
                                });
                                if terminal == Some(true) {
                                    break;
                                }
                            }
                            Err(_) => break,
                        }
                        tokio::time::sleep(std::time::Duration::from_millis(250)).await;
                    }
                })
                .detach();
            })
        };

        let on_disconnect: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let app_handle = self.app.clone();
            Rc::new(move |id: String, _w: &mut Window, cx: &mut App| {
                let Some(app) = app_handle.upgrade() else { return };
                let client = app.read(cx).client.clone();
                let entity_clone = entity.clone();
                cx.spawn(async move |cx| {
                    if let Ok(_) = client.mcp.disconnect_server(&id).await {
                        let _ = cx.update(|cx| {
                            entity_clone.update(cx, |this, cx| {
                                if let Some(s) = this.mcp.servers.iter_mut().find(|s| s.id == id) {
                                    s.status = McpConnectionStatus::Disconnected;
                                    cx.notify();
                                }
                            });
                        });
                    }
                })
                .detach();
            })
        };

        let on_toggle_expand: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            Rc::new(move |id: String, _w: &mut Window, cx: &mut App| {
                entity.update(cx, |this, cx| {
                    if this.mcp.expanded_server_id.as_deref() == Some(&id) {
                        this.mcp.expanded_server_id = None;
                    } else {
                        this.mcp.expanded_server_id = Some(id);
                    }
                    cx.notify();
                });
            })
        };

        McpPage {
            servers: self.servers.clone(),
            is_modal_open: self.is_modal_open,
            modal_mode: self.modal_mode.clone(),
            selected_transport: self.selected_transport.clone(),
            selected_auth: self.selected_auth.clone(),
            name_input: Some(self.name_input.clone()),
            url_input: Some(self.url_input.clone()),
            command_input: Some(self.command_input.clone()),
            args_input: Some(self.args_input.clone()),
            env_input: Some(self.env_input.clone()),
            expanded_server_id: self.expanded_server_id.clone(),
            action_error: self.action_error.clone(),
            is_submitting: self.is_submitting,
            on_open_add,
            on_open_edit,
            on_close_modal,
            on_select_transport,
            on_select_auth,
            on_save,
            on_delete,
            on_connect,
            on_disconnect,
            on_toggle_expand,
        }
        .into_any_element()
    }
}

/// Serves a single OAuth callback on an already-bound loopback listener and
/// returns (state, code, provider-error). Blocking with the server's 5-minute
/// auth window; meant for `spawn_blocking`. Mirrors the provider-login
/// listener in `state/auth.rs`.
fn serve_mcp_callback_once(
    listener: std::net::TcpListener,
) -> Option<(String, Option<String>, Option<String>)> {
    use std::io::{Read, Write};

    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(300);
    while std::time::Instant::now() < deadline {
        let (mut stream, _) = match listener.accept() {
            Ok(pair) => pair,
            Err(_) => {
                std::thread::sleep(std::time::Duration::from_millis(100));
                continue;
            }
        };
        let _ = stream.set_read_timeout(Some(std::time::Duration::from_secs(4)));
        let mut buf = [0u8; 4096];
        let n = stream.read(&mut buf).unwrap_or(0);
        if n == 0 {
            continue;
        }
        let request = String::from_utf8_lossy(&buf[..n]);
        if request.contains("GET /favicon.ico") {
            let response =
                "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n";
            let _ = stream.write_all(response.as_bytes());
            continue;
        }
        let mut code = None;
        let mut state = None;
        let mut err_msg = None;
        for line in request.lines() {
            if line.starts_with("GET ") {
                if let Some(path_and_query) = line.split_whitespace().nth(1) {
                    if let Some(query) = path_and_query.split('?').nth(1) {
                        for part in query.split('&') {
                            let mut kv = part.splitn(2, '=');
                            match (kv.next(), kv.next()) {
                                (Some("code"), Some(v)) => {
                                    code = Some(
                                        urlencoding::decode(v).unwrap_or_default().into_owned(),
                                    )
                                }
                                (Some("state"), Some(v)) => {
                                    state = Some(
                                        urlencoding::decode(v).unwrap_or_default().into_owned(),
                                    )
                                }
                                (Some("error"), Some(v)) => {
                                    err_msg = Some(
                                        urlencoding::decode(v).unwrap_or_default().into_owned(),
                                    )
                                }
                                _ => {}
                            }
                        }
                    }
                }
                break;
            }
        }
        let ok = state.is_some() && (code.is_some() || err_msg.is_some());
        let html = if ok {
            "<!DOCTYPE html><html><body style='font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;'><div style='text-align:center;padding:32px;background:#27272a;border-radius:12px;border:1px solid #3f3f46;'><h2 style='margin:0 0 8px 0;color:#22c55e;'>Authentication successful!</h2><p style='margin:0;color:#a1a1aa;'>You can close this tab and return to Console.</p></div></body></html>"
        } else {
            "<!DOCTYPE html><html><body style='font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;'><div style='text-align:center;padding:32px;background:#27272a;border-radius:12px;border:1px solid #3f3f46;'><h2 style='margin:0 0 8px 0;color:#ef4444;'>Sign-in failed</h2><p style='margin:0;color:#a1a1aa;'>Return to Console and try again.</p></div></body></html>"
        };
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            html.len(),
            html
        );
        let _ = stream.write_all(response.as_bytes());
        let _ = stream.flush();
        if let Some(s) = state {
            if code.is_some() || err_msg.is_some() {
                return Some((s, code, err_msg));
            }
        }
    }
    None
}
