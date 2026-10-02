use crate::state::ConsoleDesktopApp;
use console_ui::input::ComposerInput;
use console_ui::settings::{
    AccountsPage, ConnectionPage, DeletedChatsPage, KeybindingsPage, ModelsPage, ProbeState,
    ProjectsPage, SettingsShell, SettingsTab, UsagePage,
};
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
    new_env_probe: ProbeState,
    model_vision_input: Entity<ComposerInput>,
    model_smol_input: Entity<ComposerInput>,
    model_menus: [console_ui::ContextMenuHandle; 2],
    model_searches: [Entity<ComposerInput>; 2],
    model_tabs: [console_ui::PickerTab; 2],
    keybindings_search: Entity<ComposerInput>,
    pub(crate) model_saving: bool,
    pub(crate) model_error: Option<String>,
    // MCP State
    mcp_servers: Vec<console_core::types::mcp::McpServerConfig>,
    mcp_is_modal_open: bool,
    mcp_modal_mode: console_ui::settings::McpModalMode,
    mcp_selected_transport: console_core::types::mcp::McpTransportType,
    mcp_selected_auth: console_core::types::mcp::McpAuthType,
    mcp_name_input: Entity<ComposerInput>,
    mcp_url_input: Entity<ComposerInput>,
    mcp_command_input: Entity<ComposerInput>,
    mcp_args_input: Entity<ComposerInput>,
    mcp_env_input: Entity<ComposerInput>,
    mcp_expanded_server_id: Option<String>,
    mcp_action_error: Option<String>,
    mcp_is_submitting: bool,
    _subscriptions: Vec<gpui::Subscription>,
}

impl SettingsWindow {
    pub fn new(
        app: WeakEntity<ConsoleDesktopApp>,
        initial_tab: SettingsTab,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let subscription = app
            .upgrade()
            .map(|app_entity| cx.observe(&app_entity, |_this, _app, cx| cx.notify()));

        if initial_tab == SettingsTab::Usage {
            if let Some(app_entity) = app.upgrade() {
                app_entity.update(cx, |app_state, cx| {
                    app_state.fetch_usage(cx);
                });
            }
        }

        let new_env_name_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. Production Daemon", cx);
            input
        });

        let new_env_url_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. http://localhost:3000", cx);
            input
        });

        let model_vision_input = cx.new(|cx| {
            ComposerInput::new(window, cx).placeholder("provider/model or leave blank for chat model")
        });
        let model_smol_input = cx.new(|cx| {
            ComposerInput::new(window, cx).placeholder("provider/model or leave blank for chat model")
        });

        let settings_client = app.upgrade().map(|entity| entity.read(cx).client.clone());
        let vision_for_load = model_vision_input.clone();
        let smol_for_load = model_smol_input.clone();
        if let Some(client) = settings_client {
            cx.spawn(async move |_, cx| {
                if let Ok(settings) = client.settings.get().await {
                    cx.update(|cx| {
                        vision_for_load.update(cx, |input, cx| {
                            input.set_content(settings.model_roles.vision.unwrap_or_default(), cx)
                        });
                        smol_for_load.update(cx, |input, cx| {
                            input.set_content(settings.model_roles.smol.unwrap_or_default(), cx)
                        });
                    });
                }
            })
            .detach();
        }

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
        window.focus(&focus_handle, cx);

        let keybindings_search = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Filter shortcuts (e.g. \"browser\", \"composer\", \"cmd+r\")...")
        });

        let mut subscriptions = Vec::new();
        subscriptions.extend(subscription);
        for search in &model_searches {
            subscriptions.push(cx.subscribe(search, |_this, _input, event: &console_ui::input::ComposerEvent, cx| match event {
                console_ui::input::ComposerEvent::Edited | console_ui::input::ComposerEvent::Focus => cx.notify(),
                _ => {}
            }));
        }
        subscriptions.push(cx.subscribe(&keybindings_search, |_this, _input, event: &console_ui::input::ComposerEvent, cx| match event {
            console_ui::input::ComposerEvent::Edited | console_ui::input::ComposerEvent::Focus => cx.notify(),
            _ => {}
        }));

        let mcp_name_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. atlassian, filesystem", cx);
            input
        });
        let mcp_url_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("https://mcp.atlassian.com/v2/mcp", cx);
            input
        });
        let mcp_command_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("npx, uvx, docker, or path to binary", cx);
            input
        });
        let mcp_args_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("-y @modelcontextprotocol/server-filesystem /path/to/folder", cx);
            input
        });
        let mcp_env_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("API_KEY=xyz, DEBUG=true", cx);
            input
        });

        Self {
            app,
            active_tab: initial_tab,
            focus_handle,
            is_adding_env: false,
            editing_env_id: None,
            new_env_name_input,
            new_env_url_input,
            new_env_probe: ProbeState::Unknown,
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
            mcp_servers: Vec::new(),
            mcp_is_modal_open: false,
            mcp_modal_mode: console_ui::settings::McpModalMode::Add,
            mcp_selected_transport: console_core::types::mcp::McpTransportType::Stdio,
            mcp_selected_auth: console_core::types::mcp::McpAuthType::None,
            mcp_name_input,
            mcp_url_input,
            mcp_command_input,
            mcp_args_input,
            mcp_env_input,
            mcp_expanded_server_id: None,
            mcp_action_error: None,
            mcp_is_submitting: false,
            _subscriptions: subscriptions,
        }
    }

    pub fn apply_model_settings(
        &mut self,
        settings: &console_core::ConsoleSettings,
        cx: &mut Context<Self>,
    ) {
        self.model_vision_input.update(cx, |input, cx| {
            input.set_content(settings.model_roles.vision.clone().unwrap_or_default(), cx);
        });
        self.model_smol_input.update(cx, |input, cx| {
            input.set_content(settings.model_roles.smol.clone().unwrap_or_default(), cx);
        });
    }

    pub fn set_tab(&mut self, tab: SettingsTab, cx: &mut Context<Self>) {
        self.active_tab = tab;
        if tab == SettingsTab::Usage {
            if let Some(app_entity) = self.app.upgrade() {
                app_entity.update(cx, |app_state, cx| {
                    app_state.fetch_usage(cx);
                });
            }
        }
        if tab == SettingsTab::Mcp {
            self.fetch_mcp_servers(cx);
        }
        cx.notify();
    }

    pub fn fetch_mcp_servers(&mut self, cx: &mut Context<Self>) {
        let Some(app_entity) = self.app.upgrade() else { return };
        let client = app_entity.read(cx).client.clone();
        cx.spawn(async move |entity, cx| {
            if let Ok(servers) = client.mcp.list_servers().await {
                let _ = cx.update(|cx| {
                    let _ = entity.update(cx, |window, cx| {
                        window.mcp_servers = servers;
                        cx.notify();
                    });
                });
            }
        })
        .detach();
    }
}

impl Focusable for SettingsWindow {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for SettingsWindow {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if let Some(state) = crate::persistence::window::capture(window) {
            crate::persistence::store::save_settings_window(state);
        }

        let Some(app_entity) = self.app.upgrade() else {
            return div()
                .size_full()
                .child("Application closed")
                .into_any_element();
        };

        let app = app_entity.read(cx);

        let active_tab = self.active_tab;

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
                entity.update(cx, |this, cx| {
                    this.active_tab = tab;
                    cx.notify();
                });
            })
        };

        let content = match active_tab {
            SettingsTab::Accounts => {
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

                AccountsPage {
                    providers: app.providers.clone(),
                    auth_status: app.auth_status.clone(),
                    logging_in: app.auth_logging_in.clone(),
                    on_login,
                }
                .into_any_element()
            }
            SettingsTab::Connection => {
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
                    Rc::new(move |is_adding: bool, _w: &mut Window, cx: &mut App| {
                        if is_adding {
                            name_input.update(cx, |input, cx| {
                                input.clear(cx);
                            });
                            url_input.update(cx, |input, cx| {
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

                        entity.update(cx, |this, cx| {
                            this.is_adding_env = true;
                            this.editing_env_id = Some(env_id);
                            this.new_env_probe = probe;
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
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        let name = name_input.read(cx).content().trim().to_string();
                        let url = url_input.read(cx).content().trim().to_string();
                        if url.is_empty() {
                            return;
                        }
                        let final_name = if name.is_empty() {
                            "Custom Server".to_string()
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

                ConnectionPage {
                    environments: app.environment_rows(),
                    is_adding: self.is_adding_env,
                    is_editing: self.editing_env_id.is_some(),
                    name_input: Some(self.new_env_name_input.clone()),
                    url_input: Some(self.new_env_url_input.clone()),
                    new_probe_state: self.new_env_probe,
                    on_activate,
                    on_probe,
                    on_edit,
                    on_remove,
                    on_toggle_add,
                    on_probe_new,
                    on_save_new,
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
                            model_roles: console_core::ModelRoleMapping {
                                vision: Some(vision_input.read(cx).content().trim().to_string())
                                    .filter(|v| !v.is_empty()),
                                smol: Some(smol_input.read(cx).content().trim().to_string())
                                    .filter(|v| !v.is_empty()),
                            },
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
                let on_tab: Rc<dyn Fn(usize, console_ui::PickerTab, &mut Window, &mut App) + 'static> =
                    Rc::new(move |idx, tab, _window, cx| {
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
                        window.remove_window();
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.settings_window_handle = None;
                                app_state.settings_window_view = None;
                                if let Some(main_handle) = app_state.main_window_handle {
                                    let app_entity = cx.entity().clone();
                                    cx.defer(move |cx| {
                                        let _ = main_handle.update(cx, |_root, main_window, cx| {
                                            main_window.activate_window();
                                            app_entity.update(cx, |this, cx| {
                                                this.open_project_browse(main_window, cx);
                                            });
                                        });
                                    });
                                }
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
            SettingsTab::Mcp => {
                let on_open_add: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    let name_input = self.mcp_name_input.clone();
                    let url_input = self.mcp_url_input.clone();
                    let command_input = self.mcp_command_input.clone();
                    let args_input = self.mcp_args_input.clone();
                    let env_input = self.mcp_env_input.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        name_input.update(cx, |input, cx| input.clear(cx));
                        url_input.update(cx, |input, cx| input.clear(cx));
                        command_input.update(cx, |input, cx| input.clear(cx));
                        args_input.update(cx, |input, cx| input.clear(cx));
                        env_input.update(cx, |input, cx| input.clear(cx));
                        entity.update(cx, |this, cx| {
                            this.mcp_is_modal_open = true;
                            this.mcp_modal_mode = console_ui::settings::McpModalMode::Add;
                            this.mcp_selected_transport = console_core::types::mcp::McpTransportType::Stdio;
                            this.mcp_selected_auth = console_core::types::mcp::McpAuthType::None;
                            this.mcp_action_error = None;
                            cx.notify();
                        });
                    })
                };

                let on_open_edit: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    let name_input = self.mcp_name_input.clone();
                    let url_input = self.mcp_url_input.clone();
                    let command_input = self.mcp_command_input.clone();
                    let args_input = self.mcp_args_input.clone();
                    let env_input = self.mcp_env_input.clone();
                    Rc::new(move |server_id: String, _w: &mut Window, cx: &mut App| {
                        entity.update(cx, |this, cx| {
                            if let Some(srv) = this.mcp_servers.iter().find(|s| s.id == server_id) {
                                name_input.update(cx, |input, cx| input.set_content(srv.name.clone(), cx));
                                url_input.update(cx, |input, cx| input.set_content(srv.url.clone().unwrap_or_default(), cx));
                                command_input.update(cx, |input, cx| input.set_content(srv.command.clone().unwrap_or_default(), cx));
                                args_input.update(cx, |input, cx| input.set_content(srv.args.join(" "), cx));
                                let env_str = srv.env.iter().map(|(k, v)| format!("{}={}", k, v)).collect::<Vec<_>>().join(", ");
                                env_input.update(cx, |input, cx| input.set_content(env_str, cx));

                                this.mcp_is_modal_open = true;
                                this.mcp_modal_mode = console_ui::settings::McpModalMode::Edit(server_id.clone());
                                this.mcp_selected_transport = srv.transport.clone();
                                this.mcp_selected_auth = srv.auth_type.clone();
                                this.mcp_action_error = None;
                                cx.notify();
                            }
                        });
                    })
                };

                let on_close_modal: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        entity.update(cx, |this, cx| {
                            this.mcp_is_modal_open = false;
                            this.mcp_action_error = None;
                            cx.notify();
                        });
                    })
                };

                let on_select_transport: Rc<dyn Fn(console_core::types::mcp::McpTransportType, &mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    Rc::new(move |transport, _w: &mut Window, cx: &mut App| {
                        entity.update(cx, |this, cx| {
                            this.mcp_selected_transport = transport;
                            cx.notify();
                        });
                    })
                };

                let on_select_auth: Rc<dyn Fn(console_core::types::mcp::McpAuthType, &mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    Rc::new(move |auth, _w: &mut Window, cx: &mut App| {
                        entity.update(cx, |this, cx| {
                            this.mcp_selected_auth = auth;
                            cx.notify();
                        });
                    })
                };

                let on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let entity = cx.entity().clone();
                    let app_handle = self.app.clone();
                    let name_input = self.mcp_name_input.clone();
                    let url_input = self.mcp_url_input.clone();
                    let command_input = self.mcp_command_input.clone();
                    let args_input = self.mcp_args_input.clone();
                    let env_input = self.mcp_env_input.clone();

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
                                this.mcp_action_error = Some("Server name is required".to_string());
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
                            let t = w.mcp_selected_transport.clone();
                            let a = w.mcp_selected_auth.clone();
                            let srv_id = match &w.mcp_modal_mode {
                                console_ui::settings::McpModalMode::Add => {
                                    name.to_lowercase().replace(' ', "-").replace(|c: char| !c.is_alphanumeric() && c != '-', "")
                                }
                                console_ui::settings::McpModalMode::Edit(id) => id.clone(),
                            };
                            (t, a, srv_id)
                        };

                        let server = console_core::types::mcp::McpServerConfig {
                            id: id.clone(),
                            name,
                            transport: transport.clone(),
                            url: if transport == console_core::types::mcp::McpTransportType::Http { Some(url) } else { None },
                            auth_type: auth,
                            command: if transport == console_core::types::mcp::McpTransportType::Stdio { Some(cmd) } else { None },
                            args,
                            env,
                            status: console_core::types::mcp::McpConnectionStatus::Disconnected,
                            auth_url: None,
                            tools: Vec::new(),
                        };

                        entity.update(cx, |this, cx| {
                            this.mcp_is_submitting = true;
                            this.mcp_action_error = None;
                            cx.notify();
                        });

                        let entity_clone = entity.clone();
                        cx.spawn(async move |cx| {
                            match client.mcp.save_server(&server).await {
                                Ok(_) => {
                                    let updated_list = client.mcp.list_servers().await.unwrap_or_default();
                                    let _ = cx.update(|cx| {
                                        entity_clone.update(cx, |this, cx| {
                                            this.mcp_servers = updated_list;
                                            this.mcp_is_modal_open = false;
                                            this.mcp_is_submitting = false;
                                            cx.notify();
                                        });
                                    });
                                }
                                Err(e) => {
                                    let _ = cx.update(|cx| {
                                        entity_clone.update(cx, |this, cx| {
                                            this.mcp_action_error = Some(format!("Failed to save server: {}", e));
                                            this.mcp_is_submitting = false;
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
                                        this.mcp_servers = updated_list;
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
                            if let Some(s) = this.mcp_servers.iter_mut().find(|s| s.id == id) {
                                s.status = console_core::types::mcp::McpConnectionStatus::Connecting;
                                cx.notify();
                            }
                        });
                        cx.spawn(async move |cx| {
                            match client.mcp.connect_server(&id).await {
                                // The service polls until the server leaves
                                // `connecting`, so this list carries the real
                                // status rather than the optimistic one.
                                Ok(updated_list) => {
                                    let auth_url = updated_list
                                        .iter()
                                        .find(|s| s.id == id)
                                        .and_then(|s| {
                                            if matches!(s.status, console_core::types::mcp::McpConnectionStatus::NeedsAuth) {
                                                s.auth_url.clone()
                                            } else {
                                                None
                                            }
                                        });

                                    let _ = cx.update(|cx| {
                                        if let Some(url) = auth_url {
                                            cx.open_url(&url);
                                        }
                                        entity_clone.update(cx, |this, cx| {
                                            this.mcp_servers = updated_list;
                                            cx.notify();
                                        });
                                    });
                                }
                                Err(e) => {
                                    let _ = cx.update(|cx| {
                                        entity_clone.update(cx, |this, cx| {
                                            if let Some(s) = this.mcp_servers.iter_mut().find(|s| s.id == id) {
                                                s.status = console_core::types::mcp::McpConnectionStatus::Error(e.to_string());
                                                cx.notify();
                                            }
                                        });
                                    });
                                }
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
                                        if let Some(s) = this.mcp_servers.iter_mut().find(|s| s.id == id) {
                                            s.status = console_core::types::mcp::McpConnectionStatus::Disconnected;
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
                            if this.mcp_expanded_server_id.as_deref() == Some(&id) {
                                this.mcp_expanded_server_id = None;
                            } else {
                                this.mcp_expanded_server_id = Some(id);
                            }
                            cx.notify();
                        });
                    })
                };

                console_ui::settings::McpPage {
                    servers: self.mcp_servers.clone(),
                    is_modal_open: self.mcp_is_modal_open,
                    modal_mode: self.mcp_modal_mode.clone(),
                    selected_transport: self.mcp_selected_transport.clone(),
                    selected_auth: self.mcp_selected_auth.clone(),
                    name_input: Some(self.mcp_name_input.clone()),
                    url_input: Some(self.mcp_url_input.clone()),
                    command_input: Some(self.mcp_command_input.clone()),
                    args_input: Some(self.mcp_args_input.clone()),
                    env_input: Some(self.mcp_env_input.clone()),
                    expanded_server_id: self.mcp_expanded_server_id.clone(),
                    action_error: self.mcp_action_error.clone(),
                    is_submitting: self.mcp_is_submitting,
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

        div()
            .size_full()
            .track_focus(&self.focus_handle)
            .on_key_down(cx.listener(|this, event: &KeyDownEvent, window, cx| {
                if event.keystroke.key == "escape" {
                    if let Some(app) = this.app.upgrade() {
                        app.update(cx, |app_state, _cx| {
                            app_state.settings_window_handle = None;
                            app_state.settings_window_view = None;
                        });
                    }
                    window.remove_window();
                }
            }))
            .child(SettingsShell::new(active_tab, on_select_tab, content))
            .into_any_element()
    }
}
