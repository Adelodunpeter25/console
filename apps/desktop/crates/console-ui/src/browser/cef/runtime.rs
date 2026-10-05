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
use objc2::rc::Retained;
use objc2_foundation::NSTimer;

/// How often CEF's work is pumped from the main runloop. Matches the ~10ms
/// cadence `cefclient` uses for `CefDoMessageLoopWork` without an external
/// pump.
const PUMP_INTERVAL_SECS: f64 = 0.01;

/// Initialized CEF runtime. Owns the loaded framework and argv storage;
/// `Drop` stops the pump timer and shuts CEF down.
pub struct CefRuntime {
    _args: Args,
    _library: LibraryLoader,
    pump_timer: Option<Retained<NSTimer>>,
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
            pump_timer: None,
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
        if let Some(timer) = self.pump_timer.take() {
            timer.invalidate();
        }
        cef::shutdown();
    }
}

std::thread_local! {
    /// The live runtime, if CEF initialized on this (main) thread. All CEF
    /// calls happen on the main thread, so thread-local storage is
    /// sufficient and avoids `Sync` bounds on the framework handles.
    static CEF: std::cell::RefCell<Option<CefRuntime>> = std::cell::RefCell::new(None);
}

/// Pump CEF once if a runtime lives on this thread. Invoked by the
/// runloop timer installed in `ensure_initialized`; never call in a tight
/// loop.
fn pump_global() {
    CEF.with(|slot| {
        if let Some(runtime) = slot.borrow().as_ref() {
            runtime.pump();
        }
    });
}

/// Initialize CEF once on the main thread and install the message-loop pump.
/// Returns false when CEF is unavailable (helper process or an unbundled
/// development binary); the caller surfaces a host error in that case. Safe
/// to call repeatedly.
pub fn ensure_initialized() -> bool {
    debug_assert!(
        objc2_foundation::MainThreadMarker::new().is_some(),
        "CEF must initialize on the main thread"
    );
    CEF.with(|slot| {
        if slot.borrow().is_some() {
            return true;
        }
        let Some(mut runtime) = CefRuntime::initialize() else {
            return false;
        };
        // GPUI owns the main runloop, so an `NSTimer` is the least invasive
        // pump driver: no executor involvement, no signature changes. The
        // block captures nothing, satisfying the timer's sendability
        // contract, and the timer copies it (released on `invalidate`).
        let block = block2::RcBlock::new(|_: std::ptr::NonNull<NSTimer>| pump_global());
        let timer = unsafe {
            NSTimer::scheduledTimerWithTimeInterval_repeats_block(
                PUMP_INTERVAL_SECS,
                true,
                &block,
            )
        };
        runtime.pump_timer = Some(timer);
        slot.borrow_mut().replace(runtime);
        true
    })
}
