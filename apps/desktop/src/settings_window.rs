use crate::state::ConsoleDesktopApp;
use console_ui::input::ComposerInput;
use console_ui::settings::{
    DeletedChatsPage, IntegrationsPage, KeybindingsPage, ModelsPage, ProbeState, ProjectsPage,
    ProvidersPage, ServersPage, SettingsShell, SettingsTab, UsagePage,
};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, AppContext, Context, Entity, FocusHandle, Focusable, InteractiveElement, IntoElement,
    KeyDownEvent, ParentElement, Render, Styled, WeakEntity, Window, div,
};
use std::rc::Rc;
use std::time::Duration;

pub struct SettingsWindow {
    app: WeakEntity<ConsoleDesktopApp>,
    active_tab: SettingsTab,
    focus_handle: FocusHandle,
    is_adding_env: bool,
    editing_env_id: Option<String>,
    new_env_name_input: Entity<ComposerInput>,
    new_env_url_input: Entity<ComposerInput>,
    /// The pairing code, redeemed for this device's own credential.
    new_env_code_input: Entity<ComposerInput>,
    /// True while the "pair a phone" modal is open.
    pair_modal_open: bool,
    new_env_probe: ProbeState,
    model_vision_input: Entity<ComposerInput>,
    model_smol_input: Entity<ComposerInput>,
    model_menus: [console_ui::ContextMenuHandle; 2],
    model_searches: [Entity<ComposerInput>; 2],
    model_tabs: [console_ui::PickerTab; 2],
    keybindings_search: Entity<ComposerInput>,
    pub(crate) model_saving: bool,
    pub(crate) model_error: Option<String>,
    pub(crate) mcp: crate::settings::mcp::McpTabState,
    github_token_input: Entity<ComposerInput>,
    rename_input: Entity<ComposerInput>,
    /// One key field per key-taking provider, created lazily: the catalog
    /// arrives from the server, so the set is not known when the window is
    /// built. A field per provider rather than one shared, because a key typed
    /// for one row must never be submittable against another.
    api_key_inputs: std::collections::HashMap<String, Entity<ComposerInput>>,
    _subscriptions: Vec<gpui::Subscription>,
}

impl SettingsWindow {
    pub fn new(
        app: WeakEntity<ConsoleDesktopApp>,
        client: console_core::ConsoleClient,
        initial_tab: SettingsTab,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let subscription = app
            .upgrade()
            .map(|app_entity| cx.observe(&app_entity, |_this, _app, cx| cx.notify()));

        let new_env_name_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. Production Daemon", cx);
            input
        });

        let new_env_url_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. 192.168.1.50:3000", cx);
            input
        });

        // The pairing code field. Created up front like the other settings
        // inputs so its entity identity is stable across renders — rebuilding
        // it each render would drop focus and any half-typed code.
        let new_env_code_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. abc-123-xyz", cx);
            input
        });

        let model_vision_input = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .placeholder("provider/model or leave blank for chat model")
        });
        let model_smol_input = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .placeholder("provider/model or leave blank for chat model")
        });

        let vision_for_load = model_vision_input.clone();
        let smol_for_load = model_smol_input.clone();
        let client_for_load = client.clone();
        cx.spawn(async move |_, cx| {
            if let Ok(settings) = client_for_load.settings.get().await {
                cx.update(|cx| {
                    vision_for_load.update(cx, |input, cx| {
                        input.set_content(
                            settings
                                .model_roles
                                .as_ref()
                                .and_then(|roles| roles.vision.clone())
                                .unwrap_or_default(),
                            cx,
                        )
                    });
                    smol_for_load.update(cx, |input, cx| {
                        input.set_content(
                            settings
                                .model_roles
                                .as_ref()
                                .and_then(|roles| roles.smol.clone())
                                .unwrap_or_default(),
                            cx,
                        )
                    });
                });
            }
        })
        .detach();

        let model_menus = [
            console_ui::ContextMenuHandle::new(cx),
            console_ui::ContextMenuHandle::new(cx),
        ];
        let model_searches = [
            cx.new(|cx| {
                ComposerInput::new(window, cx)
                    .search_field()
                    .placeholder("Search models...")
            }),
            cx.new(|cx| {
                ComposerInput::new(window, cx)
                    .search_field()
                    .placeholder("Search models...")
            }),
        ];
        let focus_handle = cx.focus_handle();

        let keybindings_search = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Filter shortcuts (e.g. \"browser\", \"composer\", \"cmd+r\")...")
        });

        let mut subscriptions = Vec::new();
        subscriptions.extend(subscription);
        // A pasted pairing line is the whole connection: the host and the
        // code travel together, separated by a "#". Splitting it here means
        // the user pastes one line and the two fields fill themselves.
        {
            let code_input = new_env_code_input.clone();
            let url_input = new_env_url_input.clone();
            subscriptions.push(cx.subscribe(
                &url_input,
                move |_this, input, event: &console_ui::input::ComposerEvent, cx| match event {
                    console_ui::input::ComposerEvent::Edited
                    | console_ui::input::ComposerEvent::Focus => {
                        let text = input.read(cx).content();
                        let (address, code) = console_core::utils::split_pairing_line(&text);
                        if code.is_empty() {
                            return;
                        }
                        input.update(cx, |input, cx| input.set_content(address, cx));
                        code_input.update(cx, |input, cx| input.set_content(code, cx));
                    }
                    _ => {}
                },
            ));
        }
        for search in &model_searches {
            subscriptions.push(cx.subscribe(
                search,
                |_this, _input, event: &console_ui::input::ComposerEvent, cx| match event {
                    console_ui::input::ComposerEvent::Edited
                    | console_ui::input::ComposerEvent::Focus => cx.notify(),
                    _ => {}
                },
            ));
        }
        subscriptions.push(cx.subscribe(
            &keybindings_search,
            |_this, _input, event: &console_ui::input::ComposerEvent, cx| match event {
                console_ui::input::ComposerEvent::Edited
                | console_ui::input::ComposerEvent::Focus => cx.notify(),
                _ => {}
            },
        ));

        let mcp = crate::settings::mcp::McpTabState::new(app.clone(), window, cx);

        // GitHub PAT field. Created up front (like the other settings inputs)
        // so its entity identity is stable across renders — rebuilding it each
        // render would drop focus and any half-typed token on every keystroke.
        //
        // Editable on purpose: this is where the token is entered. It is
        // cleared the moment it is submitted (see on_github_submit), which is
        // what keeps the credential from lingering client-side — a read-only
        // field would block pasting entirely.
        let github_token_input =
            cx.new(|cx| ComposerInput::new(window, cx).placeholder("ghp_… or github_pat_…"));
        subscriptions.push(cx.subscribe(
            &github_token_input,
            |_this, _input, event: &console_ui::input::ComposerEvent, cx| match event {
                console_ui::input::ComposerEvent::Edited
                | console_ui::input::ComposerEvent::Focus => cx.notify(),
                _ => {}
            },
        ));

        // Account rename field. Created up front for the same reason as the
        // others: a stable entity keeps focus and half-typed names across
        // renders. Seeded with the account's display name when the form opens.
        let rename_input =
            cx.new(|cx| ComposerInput::new(window, cx).placeholder("Account name"));
        subscriptions.push(cx.subscribe(
            &rename_input,
            |_this, _input, event: &console_ui::input::ComposerEvent, cx| match event {
                console_ui::input::ComposerEvent::Edited
                | console_ui::input::ComposerEvent::Focus => cx.notify(),
                _ => {}
            },
        ));

        Self {
            app,
            active_tab: initial_tab,
            focus_handle,
            is_adding_env: false,
            editing_env_id: None,
            new_env_name_input,
            github_token_input,
            rename_input,
            api_key_inputs: std::collections::HashMap::new(),
            new_env_url_input,
            new_env_code_input,
            new_env_probe: ProbeState::Unknown,
            pair_modal_open: false,
            model_vision_input,
            model_smol_input,
            model_menus,
            model_searches,
            model_tabs: [
                console_ui::PickerTab::Favorites,
                console_ui::PickerTab::Favorites,
            ],
            keybindings_search,
            model_saving: false,
            model_error: None,
            mcp,
            _subscriptions: subscriptions,
        }
    }

    pub fn apply_model_settings(
        &mut self,
        settings: &console_core::ConsoleSettings,
        cx: &mut Context<Self>,
    ) {
        self.model_vision_input.update(cx, |input, cx| {
            input.set_content(
                settings
                    .model_roles
                    .as_ref()
                    .and_then(|roles| roles.vision.clone())
                    .unwrap_or_default(),
                cx,
            );
        });
        self.model_smol_input.update(cx, |input, cx| {
            input.set_content(
                settings
                    .model_roles
                    .as_ref()
                    .and_then(|roles| roles.smol.clone())
                    .unwrap_or_default(),
                cx,
            );
        });
    }

    pub fn set_tab(&mut self, tab: SettingsTab, cx: &mut Context<Self>) {
        self.active_tab = tab;
        if tab == SettingsTab::Mcp {
            self.mcp.fetch_mcp_servers(cx);
        }
        cx.notify();
    }

    /// The tab the settings are showing.
    pub fn active_tab(&self) -> SettingsTab {
        self.active_tab
    }

    /// The focus handle the settings hold while they own the window.
    ///
    /// The workspace is unmounted while the settings are open, so the
    /// composer that had the focus goes with it. The settings take the focus
    /// instead, or the window root would swallow every keystroke.
    pub fn settings_focus_handle(&self) -> FocusHandle {
        self.focus_handle.clone()
    }

    /// True while a modal is open on top of the settings.
    ///
    /// The window root uses this to tell "close the modal" from "close the
    /// settings": one Escape must only close one thing.
    pub fn modal_open(&self, cx: &App) -> bool {
        if self.is_adding_env || self.pair_modal_open || self.mcp.is_modal_open {
            return true;
        }
        // The rename dialog's state lives on the app.
        self.app
            .upgrade()
            .map(|app| app.read(cx).rename_form.is_some())
            .unwrap_or(false)
    }

    /// Close any modal open on top of the settings.
    pub fn close_modals(&mut self, cx: &mut Context<Self>) {
        self.is_adding_env = false;
        self.pair_modal_open = false;
        self.editing_env_id = None;
        // The rename dialog belongs to the app state.
        if let Some(app) = self.app.upgrade() {
            app.update(cx, |app_state, cx| {
                if app_state.rename_form.take().is_some() {
                    cx.notify();
                }
            });
        }
        cx.notify();
    }

    /// Build one row per key-taking provider, creating each row's key field on
    /// first render.
    ///
    /// The fields are lazy because the provider catalog is fetched from the
    /// server, so which providers take a key is not known when the window is
    /// built — and a field per provider (rather than one shared) keeps a key
    /// typed against one row from being submittable against another.
    fn build_api_key_sections(
        &mut self,
        app_entity: &Entity<ConsoleDesktopApp>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Vec<console_ui::settings::ApiKeyProviderSection> {
        // Snapshot what the rows need out of the app entity first, so the read
        // guard is dropped before this method creates entities via `cx`.
        let rows: Vec<(String, String, String, Option<console_core::types::ProviderCredentialStatus>, bool, bool, Option<String>)> = {
            let app = app_entity.read(cx);
            app.providers
                .iter()
                .filter(|p| p.auth_method == "api-key")
                .map(|p| {
                    (
                        p.name.clone(),
                        p.display_name.clone(),
                        p.description.clone(),
                        app.provider_credentials.get(&p.name).cloned(),
                        app.provider_credential_busy.contains(&p.name),
                        app.provider_credential_forms_open.contains(&p.name),
                        app.provider_credential_error.clone(),
                    )
                })
                .collect()
        };

        let mut sections = Vec::with_capacity(rows.len());
        for (provider_id, display_name, description, status, busy, form_open, error) in
            rows
        {
            let input = self
                .api_key_inputs
                .entry(provider_id.clone())
                .or_insert_with(|| cx.new(|cx| ComposerInput::new(window, cx).placeholder("sk-…")));

            let on_toggle: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                let app_handle = self.app.clone();
                let provider_id = provider_id.clone();
                Rc::new(move |_w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            let next = !app_state
                                .provider_credential_forms_open
                                .contains(&provider_id);
                            app_state.set_provider_credential_form_open(
                                provider_id.clone(),
                                next,
                                cx,
                            );
                        });
                    }
                })
            };
            let on_submit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                let app_handle = self.app.clone();
                let provider_id = provider_id.clone();
                let input = input.clone();
                Rc::new(move |key: String, _w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        // Clear the field as soon as the key is handed off: it
                        // is a credential, and nothing should keep a copy
                        // client-side.
                        input.update(cx, |input, cx| input.set_content("", cx));
                        app.update(cx, |app_state, cx| {
                            app_state.set_provider_credential(
                                provider_id.clone(),
                                key,
                                cx,
                            );
                            app_state.set_provider_credential_form_open(
                                provider_id.clone(),
                                false,
                                cx,
                            );
                        });
                    }
                })
            };
            let on_remove: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                let app_handle = self.app.clone();
                let provider_id = provider_id.clone();
                Rc::new(move |_w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.clear_provider_credential(provider_id.clone(), cx);
                        });
                    }
                })
            };

            sections.push(console_ui::settings::ApiKeyProviderSection {
                provider_id,
                display_name,
                description,
                status,
                busy,
                form_open,
                error,
                key_input: Some(input.clone()),
                on_toggle_form: on_toggle,
                on_submit,
                on_remove,
            });
        }
        sections
    }

    pub fn fetch_mcp_servers(&mut self, cx: &mut Context<Self>) {
        self.mcp.fetch_mcp_servers(cx);
    }
}

impl Focusable for SettingsWindow {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for SettingsWindow {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let Some(app_entity) = self.app.upgrade() else {
            return div()
                .size_full()
                .child("Application closed")
                .into_any_element();
        };

        let active_tab = self.active_tab;

        // Built before the `app` read guard below: creating each field needs
        // `&mut self`, which a live Ref would forbid. Doing it here also keeps
        // the guard from being held across the whole tab's render.
        let api_keys = if active_tab == SettingsTab::Providers {
            self.build_api_key_sections(&app_entity, window, cx)
        } else {
            Vec::new()
        };

        let app = app_entity.read(cx);

        let on_select_tab: Rc<dyn Fn(SettingsTab, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let app_handle = self.app.clone();
            Rc::new(move |tab: SettingsTab, _w: &mut Window, cx: &mut App| {
                if tab == SettingsTab::Usage {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.fetch_usage(cx);
                        });
                    }
                }
                if tab == SettingsTab::Mcp {
                    entity.update(cx, |this, cx| {
                        this.fetch_mcp_servers(cx);
                    });
                }
                // Sidebar clicks bypass set_tab, and without this the GitHub
                // row never re-reads: one failed load stuck it on
                // "Not connected" until the whole window reopened.
                if tab == SettingsTab::Providers {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.refresh_github_status(cx);
                        });
                    }
                }
                if tab == SettingsTab::Integrations {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.refresh_integrations(cx);
                        });
                    }
                }
                entity.update(cx, |this, cx| {
                    this.active_tab = tab;
                    // The modals belong to the servers tab. Leaving it
                    // closes them, so they do not reappear out of nowhere
                    // when the tab comes back.
                    if tab != SettingsTab::Servers {
                        this.is_adding_env = false;
                        this.editing_env_id = None;
                        this.pair_modal_open = false;
                    }
                    // The rename dialog belongs to the providers tab the
                    // same way.
                    if tab != SettingsTab::Providers {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                if app_state.rename_form.take().is_some() {
                                    cx.notify();
                                }
                            });
                        }
                    }
                    cx.notify();
                });
            })
        };

        let on_probe_new: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            let url_input = self.new_env_url_input.clone();
            Rc::new(move |_w: &mut Window, cx: &mut App| {
                let url = url_input.read(cx).content().trim().to_string();
                if url.is_empty() {
                    return;
                }
                entity.update(cx, |this, cx| {
                    this.new_env_probe = ProbeState::Probing;
                    cx.notify();
                });
                let ent = entity.clone();
                cx.spawn(async move |cx| {
                    let ok =
                        console_core::utils::probe_backend(&url, Duration::from_secs(3))
                            .await;
                    let _ = cx.update(|cx| {
                        ent.update(cx, |this, cx| {
                            this.new_env_probe = if ok.is_ok() {
                                ProbeState::Ok
                            } else {
                                ProbeState::Failed
                            };
                            cx.notify();
                        });
                    });
                })
                .detach();
            })
        };

        let on_save_new: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let app_handle = self.app.clone();
            let entity = cx.entity().clone();
            let name_input = self.new_env_name_input.clone();
            let url_input = self.new_env_url_input.clone();
            let code_input = self.new_env_code_input.clone();
            Rc::new(move |_w: &mut Window, cx: &mut App| {
                let name = name_input.read(cx).content().trim().to_string();
                let url = url_input.read(cx).content().trim().to_string();
                let code = code_input.read(cx).content().trim().to_string();
                if url.is_empty() {
                    return;
                }

                // A pairing code means the server wants this device
                // to pair: exchange it for a credential, and only then
                // point the app at that address. The host field may
                // also carry the whole pairing URL, in which case the
                // app splits it into the two.
                if !code.is_empty() {
                    let (host, split_code) = console_core::utils::split_pairing_line(&url);
                    let code = if split_code.is_empty() {
                        code
                    } else {
                        split_code
                    };
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.pair_with_code(
                                host,
                                code,
                                "Desktop".to_string(),
                                cx,
                            );
                        });
                    }
                    entity.update(cx, |this, cx| {
                        this.is_adding_env = false;
                        this.editing_env_id = None;
                        this.new_env_probe = ProbeState::Unknown;
                        cx.notify();
                    });
                    return;
                }

                let final_name = if name.is_empty() {
                    // The server names itself when this device pairs with
                    // it. Without a code there is nobody to ask, so the
                    // address stands in — the same fallback the phone
                    // app uses.
                    console_core::utils::host_port(&url)
                } else {
                    name
                };

                let editing_id = entity.read(cx).editing_env_id.clone();

                if let Some(app) = app_handle.upgrade() {
                    app.update(cx, |app_state, cx| {
                        if let Some(id) = editing_id {
                            app_state.update_environment(id, final_name, url, cx);
                        } else {
                            app_state.add_environment(final_name, url, cx);
                        }
                    });
                }

                entity.update(cx, |this, cx| {
                    this.is_adding_env = false;
                    this.editing_env_id = None;
                    this.new_env_probe = ProbeState::Unknown;
                    cx.notify();
                });
            })
        };

        // Open the "pair a phone" modal, and ask the server for a
        // one-time code to show in it.
        let on_pair_device: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let app_handle = self.app.clone();
            let entity = cx.entity().clone();
            Rc::new(move |_w: &mut Window, cx: &mut App| {
                entity.update(cx, |this, cx| {
                    this.pair_modal_open = true;
                    cx.notify();
                });
                if let Some(app) = app_handle.upgrade() {
                    app.update(cx, |app_state, cx| {
                        // The old grant is spent by now, and showing it
                        // next to a fresh one would invite a scan of a code
                        // that no longer works.
                        app_state.server_pairing_grant = None;
                        app_state.create_pairing_grant("My phone".to_string(), cx);
                    });
                }
            })
        };
        // The read guard below cannot outlive the match (one arm needs
        // &mut cx), so the pairing state the modals show is copied out
        // first.
        let pairing_grant = app.server_pairing_grant.clone();
        let pairing_error = app.server_auth_error.clone();
        let this_device = app.server_device.clone();
        let paired_devices = app.server_devices.clone();

        let content = match active_tab {
            SettingsTab::Providers => {
                let on_login: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |provider: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.login_provider(provider, cx);
                            });
                        }
                    })
                };

                let on_github_toggle: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                let next = !app_state.github_form_open;
                                app_state.set_github_form_open(next, cx);
                            });
                        }
                    })
                };
                let on_github_submit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    let input = self.github_token_input.clone();
                    Rc::new(move |token: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            // Clear the field as soon as the token is handed
                            // off: it is a credential, and nothing should keep
                            // it on screen after submission.
                            input.update(cx, |input, cx| input.set_content("", cx));
                            app.update(cx, |app_state, cx| {
                                app_state.connect_github(token, cx);
                                app_state.set_github_form_open(false, cx);
                            });
                        }
                    })
                };
                let on_github_disconnect: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| app_state.disconnect_github(cx));
                        }
                    })
                };

                ProvidersPage {
                    providers: app.providers.clone(),
                    auth_status: app.auth_status.clone(),
                    logging_in: app.auth_logging_in.clone(),
                    on_login,
                    account_busy: app.account_busy.clone(),
                    recent_login_account: app.recent_login_account.clone(),
                    on_account_use: {
                        let app_handle = self.app.clone();
                        Rc::new(move |provider: String, id: String, _w: &mut Window, cx: &mut App| {
                            if let Some(app) = app_handle.upgrade() {
                                app.update(cx, |app_state, cx| {
                                    app_state.use_account(provider, id, cx);
                                });
                            }
                        })
                    },
                    on_account_remove: {
                        let app_handle = self.app.clone();
                        // Removing is destructive but v1 keeps it a one-click
                        // action: the server response refreshes the row and a
                        // deliberate second click on a missing id answers
                        // "Unknown account." instead of removing the wrong one.
                        Rc::new(move |provider: String, id: String, _w: &mut Window, cx: &mut App| {
                            if let Some(app) = app_handle.upgrade() {
                                app.update(cx, |app_state, cx| {
                                    app_state.remove_account(provider, id, cx);
                                });
                            }
                        })
                    },
                    on_account_rename: {
                        let app_handle = self.app.clone();
                        let input = self.rename_input.clone();
                        Rc::new(
                            move |provider: String,
                                  id: String,
                                  current: String,
                                  _w: &mut Window,
                                  cx: &mut App| {
                                if let Some(app) = app_handle.upgrade() {
                                    input.update(cx, |input, cx| {
                                        input.set_content(current, cx)
                                    });
                                    app.update(cx, |app_state, cx| {
                                        app_state.account_error = None;
                                        app_state.rename_form = Some((provider, id));
                                        cx.notify();
                                    });
                                }
                            },
                        )
                    },
                    account_error: app.account_error.clone(),
                    api_keys,
                    github: Some(console_ui::settings::GitHubSection {
                        status: app.github_status.clone(),
                        busy: app.github_busy,
                        form_open: app.github_form_open,
                        error: app.github_error.clone(),
                        token_input: Some(self.github_token_input.clone()),
                        on_toggle_form: on_github_toggle,
                        on_submit: on_github_submit,
                        on_disconnect: on_github_disconnect,
                    }),
                }
                .into_any_element()
            }
            SettingsTab::Servers => {
                let on_activate: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |env_id: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.activate_environment(env_id, cx);
                            });
                        }
                    })
                };

                let on_probe: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |env_id: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.probe_environment(env_id, cx);
                            });
                        }
                    })
                };

                let on_remove: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |env_id: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.remove_environment(env_id, cx);
                            });
                        }
                    })
                };

                let on_toggle_add: Rc<dyn Fn(bool, &mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    let name_input = self.new_env_name_input.clone();
                    let url_input = self.new_env_url_input.clone();
                    let code_input = self.new_env_code_input.clone();
                    Rc::new(move |is_adding: bool, _w: &mut Window, cx: &mut App| {
                        if is_adding {
                            name_input.update(cx, |input, cx| {
                                input.clear(cx);
                            });
                            url_input.update(cx, |input, cx| {
                                input.clear(cx);
                            });
                            // A code left over from an earlier pairing would
                            // send this server somewhere else entirely.
                            code_input.update(cx, |input, cx| {
                                input.clear(cx);
                            });
                        }
                        entity.update(cx, |this, cx| {
                            this.is_adding_env = is_adding;
                            this.editing_env_id = None;
                            this.new_env_probe = ProbeState::Unknown;
                            cx.notify();
                        });
                    })
                };

                let on_edit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    let app_handle = self.app.clone();
                    let name_input = self.new_env_name_input.clone();
                    let url_input = self.new_env_url_input.clone();
                    let code_input = self.new_env_code_input.clone();
                    Rc::new(move |env_id: String, _w: &mut Window, cx: &mut App| {
                        let (name, url, probe) = if let Some(app) = app_handle.upgrade() {
                            let app_state = app.read(cx);
                            if let Some(env) =
                                app_state.environments.iter().find(|e| e.id == env_id)
                            {
                                let probe = app_state
                                    .env_probes
                                    .get(&env.id)
                                    .copied()
                                    .unwrap_or(ProbeState::Unknown);
                                (env.name.clone(), env.url.clone(), probe)
                            } else {
                                return;
                            }
                        } else {
                            return;
                        };

                        name_input.update(cx, |input, cx| {
                            input.set_content(name, cx);
                        });
                        url_input.update(cx, |input, cx| {
                            input.set_content(url, cx);
                        });
                        // The code is single-use, and one left over from an
                        // earlier pairing would send this server somewhere
                        // else entirely.
                        code_input.update(cx, |input, cx| {
                            input.clear(cx);
                        });

                        entity.update(cx, |this, cx| {
                            this.is_adding_env = true;
                            this.editing_env_id = Some(env_id);
                            this.new_env_probe = probe;
                            cx.notify();
                        });
                    })
                };

                ServersPage {
                    environments: app.environment_rows(),
                    on_activate,
                    on_probe,
                    on_edit,
                    on_remove,
                    on_toggle_add,
                    on_pair_device,
                }
                .into_any_element()
            }
            SettingsTab::Models => {
                let do_save = {
                    let app_handle = self.app.clone();
                    let vision_input = self.model_vision_input.clone();
                    let smol_input = self.model_smol_input.clone();
                    Rc::new(move |cx: &mut App| {
                        let Some(app) = app_handle.upgrade() else {
                            return;
                        };
                        let settings = console_core::ConsoleSettings {
                            model_roles: Some(console_core::ModelRoleMapping {
                                vision: Some(vision_input.read(cx).content().trim().to_string())
                                    .filter(|v| !v.is_empty()),
                                smol: Some(smol_input.read(cx).content().trim().to_string())
                                    .filter(|v| !v.is_empty()),
                            }),
                        };
                        app.update(cx, |app_state, cx| {
                            app_state.save_model_settings(settings, cx)
                        });
                    })
                };
                let on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let do_save = do_save.clone();
                    Rc::new(move |_window: &mut Window, cx: &mut App| {
                        do_save(cx);
                    })
                };
                let on_select: Rc<
                    dyn Fn(String, String, String, Entity<ComposerInput>, &mut Window, &mut App)
                        + 'static,
                > = Rc::new(move |_role, provider, model, input, _window, cx| {
                    input.update(cx, |input, cx| {
                        input.set_content(format!("{provider}/{model}"), cx)
                    });
                    do_save(cx);
                });
                let on_clear: Rc<dyn Fn(Entity<ComposerInput>, &mut Window, &mut App) + 'static> =
                    Rc::new(move |input, _window, cx| {
                        input.update(cx, |input, cx| {
                            input.clear(cx);
                        });
                    });
                let self_entity = cx.entity().downgrade();
                let on_tab: Rc<
                    dyn Fn(usize, console_ui::PickerTab, &mut Window, &mut App) + 'static,
                > = Rc::new(move |idx, tab, _window, cx| {
                    if let Some(settings) = self_entity.upgrade() {
                        settings.update(cx, |this, cx| {
                            if idx < 2 {
                                this.model_tabs[idx] = tab;
                                cx.notify();
                            }
                        });
                    }
                });
                let app_for_fav = self.app.clone();
                let on_favorite: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static> =
                    Rc::new(move |provider, model_id, _window, cx| {
                        if let Some(app) = app_for_fav.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.toggle_model_favorite(provider, model_id, cx);
                            });
                        }
                    });
                ModelsPage {
                    providers: app.providers.clone(),
                    models_by_provider: app.models_by_provider.clone(),
                    tabs: self.model_tabs.clone(),
                    favorites: app.favorites.clone(),
                    vision_input: self.model_vision_input.clone(),
                    smol_input: self.model_smol_input.clone(),
                    menus: self.model_menus.clone(),
                    searches: self.model_searches.clone(),
                    on_select,
                    on_clear,
                    on_tab,
                    on_favorite,
                    on_save,
                    saving: self.model_saving,
                    error: self.model_error.clone(),
                }
                .into_any_element()
            }
            SettingsTab::Integrations => {
                let on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.refresh_integrations(cx);
                            });
                        }
                    })
                };
                let on_install: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.install_gh(cx);
                            });
                        }
                    })
                };
                IntegrationsPage {
                    integrations: app.integrations.clone(),
                    loading: app.integrations_loading,
                    error: app.integrations_error.clone(),
                    on_refresh,
                    on_install,
                }
                .into_any_element()
            }
            SettingsTab::Usage => {
                let on_refresh: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.fetch_usage(cx);
                            });
                        }
                    })
                };

                let on_login: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |provider: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.login_provider(provider, cx);
                            });
                        }
                    })
                };

                UsagePage {
                    reports: app.usage_reports.clone(),
                    providers: app.providers.clone(),
                    auth_status: app.auth_status.clone(),
                    credentials: std::rc::Rc::new(app.provider_credentials.clone()),
                    loading: app.usage_loading,
                    on_refresh,
                    on_login,
                }
                .into_any_element()
            }
            SettingsTab::Projects => {
                let on_add_project: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |window: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                // The palette belongs to the workspace, so the
                                // settings step aside for it.
                                app_state.open_project_browse(window, cx);
                            });
                        }
                    })
                };

                let on_remove_project: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |proj_id: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.remove_project(proj_id, cx);
                            });
                        }
                    })
                };

                ProjectsPage {
                    projects: app.projects.clone(),
                    on_add_project,
                    on_remove_project,
                }
                .into_any_element()
            }
            SettingsTab::Mcp => self.mcp.render_mcp_tab(window, cx),
            SettingsTab::DeletedChats => {
                let on_restore: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |session_id: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.restore_deleted_session(session_id, cx);
                            });
                        }
                    })
                };

                let on_permanent_delete: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |session_id: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.permanent_delete_session(session_id, cx);
                            });
                        }
                    })
                };

                DeletedChatsPage {
                    deleted_sessions: app.deleted_sessions.clone(),
                    on_restore,
                    on_permanent_delete,
                }
                .into_any_element()
            }
            SettingsTab::Keybindings => KeybindingsPage {
                filter_query: self.keybindings_search.read(cx).content().to_string(),
                search_input: Some(self.keybindings_search.clone()),
            }
            .into_any_element(),
        };

        // The modals. They are built here rather than inside the tab so
        // they can sit above the whole window.
        let show_server_modal = active_tab == SettingsTab::Servers && self.is_adding_env;
        let show_pair_modal = active_tab == SettingsTab::Servers && self.pair_modal_open;
        let show_mcp_modal = active_tab == SettingsTab::Mcp && self.mcp.is_modal_open;
        let mcp_modal = self.mcp.render_mcp_modal(window, cx);

        // The account rename dialog: same shared shell. Its state lives on
        // the app, and it must be snapshotted here because the MCP tab's
        // `&mut cx` calls above conflict with holding the app's read guard
        // across the build. Busy and error piggyback on the account-mutation
        // state that rename_account writes.
        let (rename_target, rename_busy_src, rename_error_src) = self
            .app
            .upgrade()
            .map(|app| {
                let app = app.read(cx);
                (
                    app.rename_form.clone(),
                    app.account_busy.clone(),
                    app.account_error.clone(),
                )
            })
            .unwrap_or_default();
        let show_rename_modal = rename_target.is_some();
        let rename_busy =
            show_rename_modal && rename_busy_src.as_ref() == rename_target.as_ref();
        let rename_error = match rename_target.as_ref() {
            Some((rp, ri)) => rename_error_src
                .iter()
                .filter(|(p, i)| p == rp && i == ri)
                .map(|(_, text)| text.clone())
                .next(),
            None => None,
        };
        let rename_modal = console_ui::settings::RenameAccountModal {
            input: Some(self.rename_input.clone()),
            busy: rename_busy,
            error: rename_error,
            on_close: {
                let app_handle = self.app.clone();
                Rc::new(move |_w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.rename_form = None;
                            cx.notify();
                        });
                    }
                })
            },
            on_save: {
                let app_handle = self.app.clone();
                let input = self.rename_input.clone();
                Rc::new(move |_w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        let name = input.read(cx).content().trim().to_string();
                        if name.is_empty() {
                            return;
                        }
                        app.update(cx, |app_state, cx| {
                            if let Some((provider, id)) = app_state.rename_form.clone() {
                                app_state.rename_account(provider, id, name, cx);
                            } else {
                                // Nothing to apply (the dialog closed under
                                // us); just put the field away.
                                app_state.rename_form = None;
                            }
                        });
                        input.update(cx, |input, cx| input.set_content("", cx));
                    }
                })
            },
        };

        let on_modal_close: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            Rc::new(move |_w: &mut Window, cx: &mut App| {
                entity.update(cx, |this, cx| {
                    this.is_adding_env = false;
                    this.editing_env_id = None;
                    this.new_env_probe = ProbeState::Unknown;
                    cx.notify();
                });
            })
        };

        let server_modal = console_ui::settings::ServerModal {
            is_edit: self.editing_env_id.is_some(),
            name_input: Some(self.new_env_name_input.clone()),
            host_input: Some(self.new_env_url_input.clone()),
            code_input: Some(self.new_env_code_input.clone()),
            probe_state: self.new_env_probe,
            error: pairing_error.clone(),
            on_close: on_modal_close.clone(),
            on_probe: on_probe_new.clone(),
            on_save: on_save_new.clone(),
        };

        let pair_modal = console_ui::settings::PairDeviceModal {
            grant: pairing_grant,
            this_device,
            devices: paired_devices,
            error: pairing_error,
            on_close: {
                let entity = cx.entity().clone();
                Rc::new(move |_w: &mut Window, cx: &mut App| {
                    entity.update(cx, |this, cx| {
                        this.pair_modal_open = false;
                        cx.notify();
                    });
                })
            },
            on_revoke: {
                let app_handle = self.app.clone();
                Rc::new(move |device_id: String, _w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.revoke_server_device(device_id, cx);
                        });
                    }
                })
            },
            on_unpair: {
                let app_handle = self.app.clone();
                Rc::new(move |_w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.unpair_server(cx);
                        });
                    }
                })
            },
            on_refresh: {
                let app_handle = self.app.clone();
                Rc::new(move |_w: &mut Window, cx: &mut App| {
                    if let Some(app) = app_handle.upgrade() {
                        app.update(cx, |app_state, cx| {
                            app_state.server_pairing_grant = None;
                            app_state.create_pairing_grant("My phone".to_string(), cx);
                        });
                    }
                })
            },
        };

        div()
            .size_full()
            // The modals are positioned against this box, so they cover
            // the whole window.
            .relative()
            .track_focus(&self.focus_handle)
            .on_key_down(cx.listener(|this, event: &KeyDownEvent, _window, cx| {
                if event.keystroke.key == "escape" {
                    // The rename dialog's state lives on the app, not on
                    // this window, so it is checked first the same way.
                    if this
                        .app
                        .upgrade()
                        .map(|app| app.read(cx).rename_form.is_some())
                        .unwrap_or(false)
                    {
                        if let Some(app) = this.app.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.rename_form = None;
                                cx.notify();
                            });
                        }
                        cx.stop_propagation();
                        cx.notify();
                        return;
                    }
                    // A modal on top of the settings closes first, and the
                    // event stops there: one Escape must not close the modal
                    // *and* the settings behind it.
                    if this.pair_modal_open || this.is_adding_env || this.mcp.is_modal_open {
                        this.pair_modal_open = false;
                        this.is_adding_env = false;
                        this.mcp.is_modal_open = false;
                        this.editing_env_id = None;
                        cx.stop_propagation();
                        cx.notify();
                        return;
                    }
                    // Otherwise the event is left to bubble: the window root
                    // closes the settings and gives the workspace back.
                }
            }))
            .child(SettingsShell::new(active_tab, on_select_tab, content))
            // The modals sit above the whole window, not inside the
            // scrolling content, so they read as dialogs.
            .when(show_server_modal, |el| el.child(server_modal))
            .when(show_pair_modal, |el| el.child(pair_modal))
            .when(show_mcp_modal, |el| el.child(mcp_modal))
            .when(show_rename_modal, |el| el.child(rename_modal))
            .into_any_element()
    }
}
