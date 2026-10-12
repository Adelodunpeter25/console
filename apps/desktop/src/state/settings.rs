use super::ConsoleDesktopApp;
use crate::new_project_dialog::NewProjectDialog;
use crate::persistence::store::load_settings_window;
use crate::settings_window::SettingsWindow;
use console_core::ConsoleSettings;
use gpui::{
    AppContext, Context, TitlebarOptions, Window, WindowBounds, WindowOptions, point, px, size,
};

impl ConsoleDesktopApp {
    /// Reload all server-backed settings-window data in this workspace.
    pub fn refresh_settings(&mut self, cx: &mut Context<Self>) {
        if let Some(persisted) = crate::persistence::store::load_environments() {
            self.init_environments(Some(persisted), self.active_env_id.clone(), cx);
        }
        self.refresh_auth_status(cx);
        self.refresh_deleted_sessions(cx);
        self.load_projects(cx);
        self.load_sessions(cx);
        self.load_providers(cx);
        self.fetch_usage(cx);

        let client = self.client.clone();
        cx.spawn(async move |this, cx| {
            let Ok(settings) = client.settings.get().await else {
                return;
            };
            cx.update(|cx| {
                let _ = this.update(cx, |app, cx| {
                    app.apply_settings(settings, cx);
                });
            });
        })
        .detach();
    }

    fn apply_settings(&mut self, settings: ConsoleSettings, cx: &mut Context<Self>) {
        if let Some(settings_view) = self
            .settings_window_view
            .as_ref()
            .and_then(|view| view.upgrade())
        {
            settings_view.update(cx, |view, cx| view.apply_model_settings(&settings, cx));
        }
        cx.notify();
    }

    pub fn save_model_settings(&mut self, settings: ConsoleSettings, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let Some(settings_view) = self
            .settings_window_view
            .as_ref()
            .and_then(|view| view.upgrade())
        else {
            return;
        };

        settings_view.update(cx, |view, cx| {
            view.model_saving = true;
            view.model_error = None;
            cx.notify();
        });

        cx.spawn(async move |_, cx| {
            let result = client.settings.update(&settings).await;
            cx.update(|cx| match result {
                Ok(saved_settings) => {
                    settings_view.update(cx, |view, cx| {
                        view.model_saving = false;
                        view.apply_model_settings(&saved_settings, cx);
                        view.model_error = None;
                        cx.notify();
                    });
                    crate::window::broadcast_settings_refresh(cx);
                }
                Err(error) => {
                    settings_view.update(cx, |view, cx| {
                        view.model_saving = false;
                        view.model_error = Some(error.to_string());
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    pub fn open_settings(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.open_settings_tab(console_ui::settings::SettingsTab::Providers, window, cx);
    }

    pub fn open_settings_tab(
        &mut self,
        tab: console_ui::settings::SettingsTab,
        _window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if let Some(handle) = self.settings_window_handle {
            if let Some(view) = self.settings_window_view.as_ref().and_then(|v| v.upgrade()) {
                if cx
                    .update_window(handle, |_root, window, cx| {
                        window.activate_window();
                        view.update(cx, |settings, cx| {
                            settings.set_tab(tab, cx);
                        });
                    })
                    .is_ok()
                {
                    return;
                }
            }
        }

        let bounds = load_settings_window()
            .map(|w| w.bounds())
            .unwrap_or_else(|| gpui::Bounds {
                origin: point(px(120.0), px(120.0)),
                size: size(px(780.0), px(560.0)),
            });

        let options = WindowOptions {
            window_bounds: Some(WindowBounds::Windowed(bounds)),
            titlebar: Some(TitlebarOptions {
                title: Some("Settings".into()),
                appears_transparent: true,
                traffic_light_position: Some(point(px(14.0), px(14.0))),
            }),
            ..Default::default()
        };

        let entity = cx.entity().clone();
        let app_entity = cx.entity().downgrade();

        cx.defer(move |cx| {
            let mut settings_weak = None;
            let handle = cx
                .open_window(options, |window, cx| {
                    let settings_view =
                        cx.new(|cx| SettingsWindow::new(app_entity, tab, window, cx));
                    settings_weak = Some(settings_view.downgrade());
                    cx.new(|cx| gpui_component::Root::new(settings_view, window, cx))
                })
                .ok();

            if let Some(h) = handle {
                entity.update(cx, |this, _cx| {
                    this.settings_window_handle = Some(h.into());
                    this.settings_window_view = settings_weak;
                });
            }
        });

        // Refresh dynamic status when opening settings
        self.refresh_auth_status(cx);
        self.refresh_deleted_sessions(cx);
        cx.notify();
    }

    pub fn close_settings(&mut self, cx: &mut Context<Self>) {
        if let Some(handle) = self.settings_window_handle.take() {
            self.settings_window_view = None;
            let _ = cx.update_window(handle, |_root, window, _cx| {
                window.remove_window();
            });
        }
    }

    /// File ▸ New Project… — open (or focus) the new-project dialog.
    ///
    /// Opens as its own window rather than an overlay: the server call is
    /// synchronous and a slow clone must not block the main window. Reopening
    /// focuses the existing dialog so repeated menu clicks cannot stack
    /// copies of it.
    pub fn open_new_project_dialog(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        if let Some(handle) = self.new_project_dialog_handle
            && cx
                .update_window(handle, |_root, window, _cx| window.activate_window())
                .is_ok()
        {
            return;
        }

        let options = WindowOptions {
            window_bounds: Some(WindowBounds::Windowed(gpui::Bounds {
                origin: point(px(180.0), px(160.0)),
                size: size(px(620.0), px(660.0)),
            })),
            titlebar: Some(TitlebarOptions {
                title: Some("New Project".into()),
                appears_transparent: true,
                traffic_light_position: Some(point(px(14.0), px(14.0))),
            }),
            ..Default::default()
        };

        let entity = cx.entity().clone();
        let app_entity = cx.entity().downgrade();

        cx.defer(move |cx| {
            let handle = cx
                .open_window(options, |window, cx| {
                    let view = cx.new(|cx| NewProjectDialog::new(app_entity, window, cx));
                    cx.new(|cx| gpui_component::Root::new(view, window, cx))
                })
                .ok();

            if let Some(h) = handle {
                entity.update(cx, |this, _cx| {
                    this.new_project_dialog_handle = Some(h.into());
                });
            }
        });
    }

    /// Close the new-project dialog. Safe to call when it is not open, so the
    /// success path can end with it unconditionally.
    pub fn close_new_project_dialog(&mut self, cx: &mut Context<Self>) {
        if let Some(handle) = self.new_project_dialog_handle.take() {
            let _ = cx.update_window(handle, |_root, window, _cx| {
                window.remove_window();
            });
        }
    }
}
