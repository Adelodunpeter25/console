//! The settings, taking over the whole window.
//!
//! The settings are not a tab. They replace the sidebar, the workspace and
//! the inspector, so a settings page is not something you read next to a
//! chat: opening them is going somewhere, and closing them comes back.
//!
//! The title bar stays — the window keeps its chrome and its traffic lights
//! — but its sidebar toggle becomes a back arrow, because there is no
//! sidebar to toggle while the settings own the window.

use std::rc::Rc;

use console_ui::settings::SETTINGS_NAV_WIDTH;
use console_ui::{Theme, TitleBar};
use gpui::{
    AnyElement, App, Context, InteractiveElement, IntoElement, KeyDownEvent, ParentElement, Styled,
    Window, div,
};

use crate::state::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// The settings, rendered as the whole window.
    pub(crate) fn render_settings_takeover(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> AnyElement {
        let theme = Theme::current(cx);
        let entity = cx.entity().downgrade();

        // The way back: the title bar's arrow, which stands in for the
        // sidebar toggle it replaces.
        let on_back: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.close_settings(window, cx));
                }
            })
        };
        // Unused in this mode: the title bar always wants a sidebar toggle,
        // and the settings replace it with the back arrow.
        let on_toggle_sidebar: Rc<dyn Fn(&mut Window, &mut App) + 'static> = Rc::new(|_w, _cx| {});

        // The same entity the settings tab used, so a half-filled form keeps
        // what was typed into it.
        let view = match &self.settings_view {
            Some(view) => view.clone(),
            None => self.get_or_create_settings_view(
                console_ui::settings::SettingsTab::Providers,
                window,
                cx,
            ),
        };
        let title = view.read(cx).active_tab().title().to_string();

        div()
            .size_full()
            .bg(theme.canvas)
            .flex()
            .flex_col()
            // Escape gives the workspace back. A modal on top of the settings
            // closes first and stops the event there, so this never sees it.
            .on_key_down(cx.listener(|this, event: &KeyDownEvent, window, cx| {
                if event.keystroke.key == "escape" {
                    let modal_open = this
                        .settings_view
                        .as_ref()
                        .map(|view| view.read(cx).modal_open(cx))
                        .unwrap_or(false);
                    if !modal_open {
                        this.close_settings(window, cx);
                    }
                }
            }))
            .child(
                TitleBar::new(Some(title), SETTINGS_NAV_WIDTH, on_toggle_sidebar)
                    .with_back_action(on_back),
            )
            .child(div().flex_1().min_h_0().child(view))
            .into_any_element()
    }
}
