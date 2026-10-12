use super::ConsoleDesktopApp;
use crate::new_project_dialog::NewProjectDialog;
use crate::persistence::store::load_settings_window;
use crate::settings_window::SettingsWindow;
use console_core::ConsoleSettings;
use console_ui::common::FileBrowser;
use gpui::{
    App, AppContext, Context, TitlebarOptions, Window, WindowBounds, WindowOptions, point, px, size,
};
use std::rc::Rc;

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
                size: size(px(620.0), px(490.0)),
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
            let dialog_ref = std::cell::RefCell::new(None);
            let handle = cx
                .open_window(options, |window, cx| {
                    let view = cx.new(|cx| NewProjectDialog::new(app_entity, window, cx));
                    *dialog_ref.borrow_mut() = Some(view.downgrade());
                    cx.new(|cx| gpui_component::Root::new(view, window, cx))
                })
                .ok();

            if let Some(h) = handle {
                // The weak view lets the folder chooser report its choice back
                // without the dialog having to hold a window handle.
                let dialog_view = dialog_ref.borrow().clone();
                entity.update(cx, |this, _cx| {
                    this.new_project_dialog_handle = Some(h.into());
                    this.new_project_dialog_view = dialog_view;
                });
            }
        });
    }

    /// Close the new-project dialog. Safe to call when it is not open, so the
    /// success path can end with it unconditionally.
    pub fn close_new_project_dialog(&mut self, cx: &mut Context<Self>) {
        if let Some(handle) = self.new_project_dialog_handle.take() {
            self.new_project_dialog_view = None;
            let _ = cx.update_window(handle, |_root, window, _cx| {
                window.remove_window();
            });
        }
    }

    /// Open the folder chooser on behalf of the New Project dialog.
    ///
    /// Its own window rather than an overlay, and the dialog is explicitly
    /// re-activated afterwards so the chooser cannot end up buried behind it —
    /// the bug an in-window overlay had. The New Project dialog stays open
    /// behind; only the chooser closes.
    pub fn open_file_browser(
        &mut self,
        start_path: Option<String>,
        _window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        // Already open: just bring it forward rather than stacking copies.
        if let Some(handle) = self.file_browser_window_handle
            && cx
                .update_window(handle, |_root, window, _cx| window.activate_window())
                .is_ok()
        {
            return;
        }

        let Some(dialog) = self.new_project_dialog_view.clone() else {
            // No dialog to report back to; opening a chooser would be a dead end.
            return;
        };

        let app_entity = cx.entity().downgrade();
        let client = self.client.clone();

        let options = WindowOptions {
            window_bounds: Some(WindowBounds::Windowed(gpui::Bounds {
                origin: point(px(280.0), px(240.0)),
                size: size(px(520.0), px(540.0)),
            })),
            titlebar: Some(TitlebarOptions {
                title: Some("Choose a folder".into()),
                appears_transparent: true,
                traffic_light_position: Some(point(px(14.0), px(14.0))),
            }),
            ..Default::default()
        };

        let this_entity = cx.entity().clone();

        cx.defer(move |cx| {
            let handle = cx
                .open_window(options, |window, cx| {
                    let on_choose: Rc<dyn Fn(String, &mut Window, &mut App)> = {
                        let dialog = dialog.clone();
                        let app = app_entity.clone();
                        Rc::new(move |path, window, cx| {
                            if let Some(dialog) = dialog.upgrade() {
                                dialog.update(cx, |this, cx| this.set_parent_dir(path, cx));
                            }
                            // Close this window directly instead of routing
                            // through the stored handle: removing a window via
                            // `update_window` from inside its own event
                            // dispatch never lands — the settings window
                            // closes itself this way for the same reason.
                            window.remove_window();
                            if let Some(app) = app.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.file_browser_window_handle = None;
                                    // Put the dialog back in front so the folder
                                    // it now shows is what the user is left
                                    // looking at. A different window, so
                                    // reaching it through its handle is fine.
                                    if let Some(handle) = this.new_project_dialog_handle {
                                        let _ = cx.update_window(handle, |_r, window, _c| {
                                            window.activate_window()
                                        });
                                    }
                                });
                            }
                        })
                    };

                    let on_cancel: Rc<dyn Fn(&mut Window, &mut App)> = {
                        let app = app_entity.clone();
                        Rc::new(move |window, cx| {
                            window.remove_window();
                            // Leave the New Project dialog exactly as it was.
                            if let Some(app) = app.upgrade() {
                                app.update(cx, |this, _cx| {
                                    this.file_browser_window_handle = None;
                                });
                            }
                        })
                    };

                    let view = cx.new(|cx| {
                        let mut browser =
                            FileBrowser::new(client.clone(), on_choose, on_cancel, window, cx);
                        browser.open(start_path.clone(), cx);
                        browser
                    });
                    cx.new(|cx| gpui_component::Root::new(view, window, cx))
                })
                .ok();

            if let Some(h) = handle {
                this_entity.update(cx, |this, _cx| {
                    this.file_browser_window_handle = Some(h.into());
                });
            }
        });
    }

    /// Close the folder chooser, leaving the New Project dialog untouched.
    pub fn close_file_browser(&mut self, cx: &mut Context<Self>) {
        if let Some(handle) = self.file_browser_window_handle.take() {
            let _ = cx.update_window(handle, |_root, window, _cx| {
                window.remove_window();
            });
        }
    }
}
