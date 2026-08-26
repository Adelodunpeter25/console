use gpui::{
    AppContext, Context, TitlebarOptions, Window, WindowBounds, WindowOptions, point, px, size,
};
use crate::persistence::store::load_settings_window;
use crate::settings_window::SettingsWindow;
use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    pub fn open_settings(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        if let Some(handle) = self.settings_window_handle {
            if cx.update_window(handle, |_root, window, _cx| {
                window.activate_window();
            }).is_ok() {
                return;
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
            let handle = cx.open_window(options, |window, cx| {
                let settings_view = cx.new(|cx| SettingsWindow::new(app_entity, cx));
                cx.new(|cx| gpui_component::Root::new(settings_view, window, cx))
            }).ok();

            if let Some(h) = handle {
                entity.update(cx, |this, _cx| {
                    this.settings_window_handle = Some(h.into());
                });
            }
        });

        // Refresh dynamic status when opening settings
        self.refresh_auth_status(cx);
        self.refresh_deleted_sessions(cx);
        cx.notify();
    }
}
