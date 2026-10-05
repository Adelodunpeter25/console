//! CEF process lifecycle for the desktop app.
//!
//! CEF runs the browser and its helpers (renderer, GPU, ...) as separate
//! processes re-executed from our own binary. Every launch must therefore:
//! 1. Detect a helper invocation (`--type=...`) and run `execute_process`.
//! 2. In the browser process, load `Chromium Embedded Framework.framework`
//!    from the app bundle and call `initialize` on the main thread.
//! 3. Pump `do_message_loop_work` from the host runloop (GPUI owns it).
//! 4. Call `shutdown` after the last browser closes (`Drop` below).
//!
//! `initialize` returns `None` (instead of failing) when the framework is not
//! bundled next to this binary, e.g. `cargo run` during development. Callers
//! fall back to wry in that case; CEF activates only inside a real bundle.

use std::path::PathBuf;

use cef::{args::Args, library_loader::LibraryLoader, CefString, ImplCommandLine};

/// Initialized CEF runtime. Owns the loaded framework and argv storage;
/// `Drop` shuts CEF down.
pub struct CefRuntime {
    _args: Args,
    _library: LibraryLoader,
}

/// True when this process was re-executed as a CEF helper (renderer, GPU,
/// ...). CEF marks helpers with a `--type=...` switch.
pub fn is_helper_process() -> bool {
    let args = Args::new();
    args.as_cmd_line()
        .is_some_and(|cmd| cmd.has_switch(Some(&CefString::from("type"))) == 1)
}

/// Run the CEF helper entry point. Returns `Some(exit_code)` when this
/// process is a helper; the caller (`main`) must exit with that code without
/// initializing the app. Returns `None` in the browser process.
pub fn run_helper_process() -> Option<i32> {
    if !is_helper_process() {
        return None;
    }
    let args = Args::new();
    let _ = cef::api_hash(cef::sys::CEF_API_VERSION_LAST, 0);
    Some(cef::execute_process(
        Some(args.as_main_args()),
        None,
        std::ptr::null_mut(),
    ))
}

/// Location of the framework dylib for a bundled browser process, mirroring
/// `LibraryLoader`'s `<exe>/../Frameworks` resolution. `None` when this
/// binary is not inside an app bundle (development `cargo run`).
fn framework_library_path() -> Option<PathBuf> {
    let dir = std::env::current_exe().ok()?;
    let dir = dir.parent()?;
    dir.join("../Frameworks")
        .join("Chromium Embedded Framework.framework/Chromium Embedded Framework")
        .canonicalize()
        .ok()
        .filter(|path| path.is_file())
}

impl CefRuntime {
    /// Initialize CEF in the browser process. Must be called on the main
    /// thread before creating any browser. Returns `None` for helper
    /// processes and for unbundled development binaries. No `App` handler is
    /// passed yet; the one owning `BrowserProcessHandler` arrives with
    /// browser creation (later slice).
    pub fn initialize() -> Option<Self> {
        if is_helper_process() || framework_library_path().is_none() {
            return None;
        }
        let exe = std::env::current_exe().ok()?;
        let library = LibraryLoader::new(&exe, false);
        if !library.load() {
            return None;
        }
        let _ = cef::api_hash(cef::sys::CEF_API_VERSION_LAST, 0);
        let args = Args::new();
        let settings = cef::Settings {
            // The macOS sandbox needs an endorsed helper plus entitlements;
            // that ships with helper packaging (later slice). Until then the
            // browser process runs unsandboxed, like our dev builds.
            no_sandbox: 1,
            ..Default::default()
        };
        let ok = cef::initialize(
            Some(args.as_main_args()),
            Some(&settings),
            None,
            std::ptr::null_mut(),
        );
        if ok != 1 {
            return None;
        }
        Some(Self {
            _args: args,
            _library: library,
        })
    }

    /// Pump CEF work from the host (GPUI) runloop. Call repeatedly on the
    /// main thread while browsers exist.
    pub fn pump(&self) {
        cef::do_message_loop_work();
    }
}

impl Drop for CefRuntime {
    fn drop(&mut self) {
        cef::shutdown();
    }
}
