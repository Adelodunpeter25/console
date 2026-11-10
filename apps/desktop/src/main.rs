// Release builds are GUI-only on Windows; without this a console window opens
// behind the app.
#![cfg_attr(
    all(target_os = "windows", not(debug_assertions)),
    windows_subsystem = "windows"
)]
#![allow(
    clippy::type_complexity,
    clippy::let_unit_value,
    clippy::too_many_arguments
)]

mod app_menu;
mod assets;
mod keybindings;
mod log_filter;
mod menubar;
mod new_project_dialog;
mod persistence;
mod picker;
mod recording;
mod settings;
mod settings_window;
mod single_instance;
mod state;
mod types;
mod view;
mod window;

use assets::{Assets, register_core_fonts};

use console_ui::init_input_keybindings;
use gpui::App;

/// Directory that receives `panic.log`: `~/Library/Logs/<App>` on macOS,
/// `%LOCALAPPDATA%\<App>\logs` on Windows, `$XDG_STATE_HOME/<app>` on Linux.
fn panic_log_dir() -> Option<std::path::PathBuf> {
    console_shared::path::log_directory()
}

/// Install a panic hook that appends the message plus full backtrace to
/// `panic.log` (and stderr). Never fails loudly.
fn log_panics_to_file() {
    std::panic::set_hook(Box::new(|info| {
        let backtrace = std::backtrace::Backtrace::force_capture();
        let thread = std::thread::current();
        let thread_name = thread.name().unwrap_or("<unnamed>").to_string();
        let text = format!(
            "==== panic in thread '{thread_name}' ====\n{info}\nbacktrace:\n{backtrace}\n\n"
        );
        eprint!("{text}");
        if let Some(dir) = panic_log_dir()
            && std::fs::create_dir_all(&dir).is_ok() {
                let _ = std::fs::OpenOptions::new()
                    .create(true)
                    .append(true)
                    .open(dir.join("panic.log"))
                    .and_then(|mut f| std::io::Write::write_all(&mut f, text.as_bytes()));
            }
    }));
}

/// Route `log::` output to stderr, or to a file on Windows where a GUI
/// process has no console attached.
fn init_logging() {
    #[cfg(target_os = "windows")]
    {
        if let Some(dir) = panic_log_dir()
            && std::fs::create_dir_all(&dir).is_ok()
            && let Ok(file) = std::fs::OpenOptions::new()
                .create(true)
                .append(true)
                .open(dir.join("console.log"))
        {
            let logger = env_logger::Builder::from_default_env()
                .target(env_logger::Target::Pipe(Box::new(file)))
                .build();
            log_filter::install(logger);
            return;
        }
    }

    log_filter::install(env_logger::Builder::from_default_env().build());
}

/// Log the version, commit and exe path so a log from an installed copy can be
/// matched to the build that produced it.
fn log_build_identity() {
    let exe = std::env::current_exe()
        .map(|path| path.display().to_string())
        .unwrap_or_else(|_| "<unknown>".to_string());
    log::info!(
        "Console {} ({}) starting from {}",
        env!("CARGO_PKG_VERSION"),
        option_env!("CONSOLE_GIT_SHA").unwrap_or("unknown"),
        exe
    );
}

fn main() {
    // Route log:: macros to stderr (see scripts/dev.sh: RUST_LOG controls the
    // level, reload.sh captures output to dist/.dev_console.log). Without this
    // every log:: call in the codebase is silently discarded.
    init_logging();
    log_build_identity();

    // One client per session. A second launch restores the existing window and
    // exits before Tokio/GPUI start, so it cannot hold the OAuth loopback port
    // or add another window-teardown cycle. No-op outside Windows.
    let Some(_instance_guard) = single_instance::acquire() else {
        return;
    };

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
