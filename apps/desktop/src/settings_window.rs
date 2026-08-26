use std::rc::Rc;
use gpui::{
    App, Context, FocusHandle, Focusable, IntoElement, ParentElement, Render, Styled, WeakEntity,
    Window, div,
};
use console_ui::settings::{
    AccountsPage, ConnectionPage, DeletedChatsPage, ProjectsPage, SettingsShell, SettingsTab,
};
use crate::state::ConsoleDesktopApp;

pub struct SettingsWindow {
    app: WeakEntity<ConsoleDesktopApp>,
    active_tab: SettingsTab,
    focus_handle: FocusHandle,
    _subscription: Option<gpui::Subscription>,
}

impl SettingsWindow {
    pub fn new(app: WeakEntity<ConsoleDesktopApp>, cx: &mut Context<Self>) -> Self {
        let subscription = app.upgrade().map(|app_entity| {
            cx.observe(&app_entity, |_this, _app, cx| cx.notify())
        });
        Self {
            app,
            active_tab: SettingsTab::Accounts,
            focus_handle: cx.focus_handle(),
            _subscription: subscription,
        }
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

        let app = app_entity.read(cx);
        let active_tab = self.active_tab;

        let on_select_tab: Rc<dyn Fn(SettingsTab, &mut Window, &mut App) + 'static> = {
            let entity = cx.entity().clone();
            Rc::new(move |tab: SettingsTab, _w: &mut Window, cx: &mut App| {
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

                let on_save_project: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |project_id: String, _w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.save_gemini_project_id(project_id, cx);
                            });
                        }
                    })
                };

                AccountsPage {
                    providers: app.providers.clone(),
                    auth_status: app.auth_status.clone(),
                    logging_in: app.auth_logging_in.clone(),
                    gemini_project_id: String::new(),
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

                let on_add_new: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |_w: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.add_environment("Remote Daemon".to_string(), "http://127.0.0.1:4040".to_string(), cx);
                            });
                        }
                    })
                };

                ConnectionPage {
                    environments: app.environment_rows(),
                    on_activate,
                    on_probe,
                    on_remove,
                    on_add_new,
                }.into_any_element()
            }
            SettingsTab::Projects => {
                let on_add_project: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                    let app_handle = self.app.clone();
                    Rc::new(move |_window: &mut Window, cx: &mut App| {
                        if let Some(app) = app_handle.upgrade() {
                            app.update(cx, |app_state, cx| {
                                app_state.add_project(cx);
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

        SettingsShell::new(active_tab, on_select_tab, content).into_any_element()
    }
}
