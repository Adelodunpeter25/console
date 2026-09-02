use std::rc::Rc;
use std::time::Duration;
use gpui::{
    App, AppContext, Context, Entity, FocusHandle, Focusable, InteractiveElement, IntoElement,
    KeyDownEvent, ParentElement, Render, Styled, WeakEntity, Window, div,
};
use console_ui::input::ComposerInput;
use console_ui::settings::{
    AccountsPage, ConnectionPage, DeletedChatsPage, ProbeState, ProjectsPage, SettingsShell,
    SettingsTab, UsagePage,
};
use crate::state::ConsoleDesktopApp;

pub struct SettingsWindow {
    app: WeakEntity<ConsoleDesktopApp>,
    active_tab: SettingsTab,
    focus_handle: FocusHandle,
    gemini_project_input: Entity<ComposerInput>,
    gemini_project_seeded: bool,
    is_adding_env: bool,
    editing_env_id: Option<String>,
    new_env_name_input: Entity<ComposerInput>,
    new_env_url_input: Entity<ComposerInput>,
    new_env_probe: ProbeState,
    _subscription: Option<gpui::Subscription>,
}

impl SettingsWindow {
    pub fn new(
        app: WeakEntity<ConsoleDesktopApp>,
        initial_tab: SettingsTab,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let subscription = app.upgrade().map(|app_entity| {
            cx.observe(&app_entity, |_this, _app, cx| cx.notify())
        });

        if initial_tab == SettingsTab::Usage {
            if let Some(app_entity) = app.upgrade() {
                app_entity.update(cx, |app_state, cx| {
                    app_state.fetch_usage(cx);
                });
            }
        }

        let gemini_project_input = cx.new(|cx| {
            let mut input = ComposerInput::new(window, cx);
            input.set_placeholder("e.g. my-gcp-project-123", cx);
            input
        });

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

        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle, cx);

        Self {
            app,
            active_tab: initial_tab,
            focus_handle,
            gemini_project_input,
            gemini_project_seeded: false,
            is_adding_env: false,
            editing_env_id: None,
            new_env_name_input,
            new_env_url_input,
            new_env_probe: ProbeState::Unknown,
            _subscription: subscription,
        }
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
        cx.notify();
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
            return div().size_full().child("Application closed").into_any_element();
        };

        let initial_pid = if !self.gemini_project_seeded {
            app_entity.read(cx).auth_status.as_ref().and_then(|st| {
                st.gemini.configured_project_id.as_ref().or(st.gemini.project_id.as_ref()).cloned()
            })
        } else {
            None
        };

        if let Some(pid) = initial_pid {
            self.gemini_project_input.update(cx, |input, cx| {
                input.set_content(pid, cx);
            });
            self.gemini_project_seeded = true;
        }

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

                let on_save_project: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    let input_entity = self.gemini_project_input.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        let text = input_entity.read(cx).content().trim().to_string();
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.save_gemini_project_id(text, cx);
                            });
                        }
                    })
                };

                AccountsPage {
                    providers: app.providers.clone(),
                    auth_status: app.auth_status.clone(),
                    logging_in: app.auth_logging_in.clone(),
                    gemini_project_input: Some(self.gemini_project_input.clone()),
                    on_login,
                    on_save_gemini_project_id: on_save_project,
                }.into_any_element()
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
                            if let Some(env) = app_state.environments.iter().find(|e| e.id == env_id) {
                                let probe = app_state.env_probes.get(&env.id).copied().unwrap_or(ProbeState::Unknown);
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
                        if url.is_empty() { return; }
                        entity.update(cx, |this, cx| {
                            this.new_env_probe = ProbeState::Probing;
                            cx.notify();
                        });
                        let ent = entity.clone();
                        cx.spawn(async move |cx| {
                            let ok = console_core::utils::probe_backend(&url, Duration::from_secs(3)).await;
                            let _ = cx.update(|cx| {
                                ent.update(cx, |this, cx| {
                                    this.new_env_probe = if ok.is_ok() { ProbeState::Ok } else { ProbeState::Failed };
                                    cx.notify();
                                });
                            });
                        }).detach();
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
                        if url.is_empty() { return; }
                        let final_name = if name.is_empty() { "Custom Server".to_string() } else { name };

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
                }.into_any_element()
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
                }.into_any_element()
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
                }.into_any_element()
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
                }.into_any_element()
            }
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
