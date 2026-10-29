mod app_menu;
mod assets;
mod keybindings;
mod new_project_dialog;
mod persistence;
mod picker;
mod settings;
mod settings_window;
mod state;
mod types;
mod view;
mod window;

use assets::{Assets, register_core_fonts};

use console_ui::init_input_keybindings;
use gpui::App;

/// Install a panic hook that appends the message plus full backtrace to
/// ~/Library/Logs/console/panic.log (and stderr). Never fails loudly.
fn log_panics_to_file() {
    std::panic::set_hook(Box::new(|info| {
        let backtrace = std::backtrace::Backtrace::force_capture();
        let thread = std::thread::current();
        let thread_name = thread.name().unwrap_or("<unnamed>").to_string();
        let text = format!(
            "==== panic in thread '{thread_name}' ====\n{info}\nbacktrace:\n{backtrace}\n\n"
        );
        eprint!("{text}");
        if let Some(home) = std::env::var_os("HOME") {
            let dir = std::path::Path::new(&home).join("Library/Logs/console");
            if std::fs::create_dir_all(&dir).is_ok() {
                let _ = std::fs::OpenOptions::new()
                    .create(true)
                    .append(true)
                    .open(dir.join("panic.log"))
                    .and_then(|mut f| std::io::Write::write_all(&mut f, text.as_bytes()));
            }
        }
    }));
}

fn main() {
    // Route log:: macros to stderr (see scripts/dev.sh: RUST_LOG controls the
    // level, reload.sh captures output to dist/.dev_console.log). Without this
    // every log:: call in the codebase is silently discarded.
    env_logger::init();

    // Record panics even when launched from Finder/launchd (stderr invisible):
    // GPUI re-raises view-handler panics as `panic_cannot_unwind` and macOS
    // crash reports strip the original message, so this file is the only
    // place the real "panicked at file:line" survives.
    log_panics_to_file();

    let tokio_runtime = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .expect("Failed to initialize Tokio runtime");
    let _guard = tokio_runtime.enter();

    let app = gpui_platform::application().with_assets(Assets);

    app.run(|cx: &mut App| {
        // Core weights only: the remaining 7 register after first paint
        // (see ConsoleDesktopApp::new) so ~1.7MB of TTF parsing stays off
        // the startup path.
        if let Err(error) = register_core_fonts(cx) {
            log::warn!("Failed to register bundled fonts: {error}");
        }
        // gpui-component: theme/global state/popover plumbing. Required before
        // any of its components (CommandPalette) are used, and its `Root` must
        // wrap the window's top-level view for overlays to render.
        gpui_component::init(cx);
        editor_ui::init(cx);
        console_ui::theme::init(cx);
        init_input_keybindings(cx);
        // Global shortcuts last, so context-scoped bindings keep winning ties
        // inside their own contexts.
        keybindings::init(cx);
        keybindings::init_handlers(cx);
        app_menu::init(cx);
        console_ui::init_autocomplete_keybindings(cx);
        console_ui::init_floating_composer_keybindings(cx);
        console_ui::init_session_rename_keybindings(cx);
        console_ui::init_code_viewer_keybindings(cx);
        console_ui::terminal::init_terminal_keybindings(cx);
        console_ui::init_browser_keybindings(cx);
        console_ui::primitives::menu::init(cx);

        // Only the main window is persisted and restored. Extra windows
        // opened in-session (New Window) are session-only by design.
        window::open_workspace_window(cx, window::WindowLaunchTarget::RestorePersisted);
        cx.activate(true);
    });
}
