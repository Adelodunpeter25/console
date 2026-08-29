mod app_menu;
mod assets;
mod keybindings;
mod persistence;
mod picker;
mod settings_window;
mod state;
mod types;
mod view;

use assets::{Assets, register_fonts};
use state::ConsoleDesktopApp;

use console_ui::init_input_keybindings;
use gpui::{App, AppContext, WindowOptions, px};

fn main() {
    let tokio_runtime = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .expect("Failed to initialize Tokio runtime");
    let _guard = tokio_runtime.enter();

    let app = gpui_platform::application().with_assets(Assets);

    app.run(|cx: &mut App| {
        if let Err(error) = register_fonts(cx) {
            log::warn!("Failed to register bundled fonts: {error}");
        }
        // gpui-component: theme/global state/popover plumbing. Required before
        // any of its components (CommandPalette) are used, and its `Root` must
        // wrap the window's top-level view for overlays to render.
        gpui_component::init(cx);
        console_ui::theme::init(cx);
        init_input_keybindings(cx);
        // Global shortcuts last, so context-scoped bindings keep winning ties
        // inside their own contexts.
        keybindings::init(cx);
        console_ui::init_autocomplete_keybindings(cx);
        console_ui::init_session_rename_keybindings(cx);
        console_ui::primitives::menu::init(cx);

        let options = WindowOptions {
            window_bounds: Some(persistence::window::load_window_bounds(cx)),
            titlebar: Some(gpui::TitlebarOptions {
                title: None,
                appears_transparent: true,
                traffic_light_position: Some(gpui::point(px(14.0), px(14.0))),
            }),
            ..Default::default()
        };

        cx.open_window(options, |window, cx| {
            let app_view = cx.new(|cx| ConsoleDesktopApp::new(window, cx));
            // Global shortcut handlers (⌘W/⌘N/⌘⇧O/⌘⇧P/⌘K/…). Must be registered
            // on the App, not the root element: element-level `.on_action` only
            // sees actions along the focus path, which is empty until
            // something is focused. Menu items dispatch the same actions.
            keybindings::init_handlers(app_view.clone(), window.window_handle(), cx);
            // Initialize app menu after all handlers are registered
            app_menu::init(cx);
            // Root hosts overlay surfaces (palette, dialogs, popovers) above the app view.
            cx.new(|cx| gpui_component::Root::new(app_view, window, cx))
        })
        .unwrap();
        cx.activate(true);
    });
}
