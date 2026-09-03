use super::ConsoleDesktopApp;
use crate::persistence::store::load_settings_window;
use crate::settings_window::SettingsWindow;
use gpui::{
    AppContext, Context, TitlebarOptions, Window, WindowBounds, WindowOptions, point, px, size,
};

impl ConsoleDesktopApp {
    pub fn open_settings(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.open_settings_tab(console_ui::settings::SettingsTab::Accounts, window, cx);
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
}
