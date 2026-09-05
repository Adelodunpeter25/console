mod app_menu;
mod assets;
mod keybindings;
mod persistence;
mod picker;
mod settings_window;
mod state;
mod types;
mod view;
mod window;

use assets::{Assets, register_fonts};

use console_ui::init_input_keybindings;
use gpui::App;

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
        keybindings::init_handlers(cx);
        app_menu::init(cx);
        console_ui::init_autocomplete_keybindings(cx);
        console_ui::init_session_rename_keybindings(cx);
        console_ui::primitives::menu::init(cx);

        window::open_workspace_window(cx, window::WindowLaunchTarget::RestorePersisted);
        cx.activate(true);
    });
}
