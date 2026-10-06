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
use std::sync::atomic::{AtomicU16, Ordering};

use cef::{args::Args, library_loader::LibraryLoader, CefString};
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
/// ...). Detected by scanning argv for CEF's `--type=...` switch directly:
/// this must not call any CEF API, because the framework is not loaded yet
/// at process entry (`command_line_create` would jump through a null
/// function pointer and segfault).
pub fn is_helper_process() -> bool {
    std::env::args()
        .skip(1)
        .any(|arg| arg == "--type" || arg.starts_with("--type="))
}

/// Run the CEF helper entry point. Returns `Some(exit_code)` when this
/// process is a helper; the caller (`main`) must exit with that code without
/// initializing the app. Returns `None` in the browser process. A helper
/// that cannot load the framework reports exit code 1 instead of falling
/// through into a second app instance.
pub fn run_helper_process() -> Option<i32> {
    if !is_helper_process() {
        return None;
    }
    let exe = std::env::current_exe().ok()?;
    // Helpers live at `Frameworks/<name>.app/Contents/MacOS/<name>`, so the
    // framework resolves three levels up (mirrors `LibraryLoader`'s helper
    // layout). Pre-check before constructing it: the constructor panics on a
    // missing path.
    let framework_dylib = exe
        .parent()?
        .join("../../../Chromium Embedded Framework.framework/Chromium Embedded Framework");
    if !framework_dylib.is_file() {
        return Some(1);
    }
    let library = LibraryLoader::new(&exe, true);
    if !library.load() {
        return Some(1);
    }
    let _ = cef::api_hash(cef::sys::CEF_API_VERSION_LAST, 0);
    let args = Args::new();
    Some(cef::execute_process(
        Some(args.as_main_args()),
        None,
        std::ptr::null_mut(),
    ))
}

/// Bundle locations CEF needs, derived from the running executable. All
/// paths follow the layout `scripts/build.sh --cef` assembles:
/// `<App>.app/Contents/{MacOS/console, Frameworks/...}`.
pub struct BundlePaths {
    /// `.../Frameworks/Chromium Embedded Framework.framework`.
    pub framework_dir: PathBuf,
    /// `.../Frameworks/console Helper.app/Contents/MacOS/console Helper`:
    /// our own binary, re-executed by CEF for subprocesses. The helper base
    /// name tracks the binary stem (not the display name) so dev and prod
    /// bundles resolve identically.
    pub helper_exe: PathBuf,
}

/// Resolve bundle locations for a bundled browser process. `None` when this
/// binary is not inside an app bundle (development `cargo run`).
fn bundle_paths() -> Option<BundlePaths> {
    let exe = std::env::current_exe().ok()?;
    let macos_dir = exe.parent()?;
    let frameworks = macos_dir.join("../Frameworks").canonicalize().ok()?;
    let framework_dir = frameworks.join("Chromium Embedded Framework.framework");
    if !framework_dir.is_dir() {
        return None;
    }
    let stem = exe
        .file_stem()
        .map(|stem| stem.to_string_lossy().into_owned())
        .filter(|stem| !stem.is_empty())?;
    let helper_name = format!("{stem} Helper");
    let helper_exe = frameworks
        .join(format!("{helper_name}.app/Contents/MacOS/{helper_name}"))
        .canonicalize()
        .ok()
        .filter(|path| path.is_file())?;
    Some(BundlePaths {
        framework_dir,
        helper_exe,
    })
}

/// Location of the framework dylib for a bundled browser process, mirroring
/// `LibraryLoader`'s `<exe>/../Frameworks` resolution. `None` when this
/// binary is not inside an app bundle (development `cargo run`).
fn framework_library_path() -> Option<PathBuf> {
    let paths = bundle_paths()?;
    let library = paths
        .framework_dir
        .join("Chromium Embedded Framework")
        .canonicalize()
        .ok()?;
    library.is_file().then_some(library)
}

/// The CDP port handed to CEF, `0` until [`CefRuntime::initialize`] set one.
static DEBUG_PORT: AtomicU16 = AtomicU16::new(0);

/// The loopback port CEF serves the DevTools protocol on, once CEF has been
/// initialized. `None` before that, or if no port could be reserved.
pub fn debug_port() -> Option<u16> {
    match DEBUG_PORT.load(Ordering::Relaxed) {
        0 => None,
        port => Some(port),
    }
}

/// Whether CEF should start at app launch instead of at the first navigating
/// tab. Without this the CDP port does not exist until someone opens a tab,
/// which makes attaching agent-browser (or any CDP client) for debugging a
/// manual chore. On for dev builds (`CONSOLE_ENV=dev`), when a debug port is
/// pinned, or with `CONSOLE_CEF_EAGER=1`; `CONSOLE_CEF_EAGER=0` forces it off.
pub fn should_start_eagerly(
    console_env: Option<&str>,
    debug_port_env: Option<&str>,
    eager_env: Option<&str>,
) -> bool {
    match eager_env.map(str::trim) {
        Some("0") | Some("false") => return false,
        Some("1") | Some("true") => return true,
        _ => {}
    }
    console_env.is_some_and(|env| env.eq_ignore_ascii_case("dev"))
        || debug_port_env.is_some_and(|port| !port.trim().is_empty())
}

/// Start CEF now when [`should_start_eagerly`] says so (reads the env). Call
/// on the main thread once the app is running. Returns whether CEF is up.
pub fn start_eagerly_if_requested() -> bool {
    let wanted = should_start_eagerly(
        std::env::var("CONSOLE_ENV").ok().as_deref(),
        std::env::var("CONSOLE_CEF_DEBUG_PORT").ok().as_deref(),
        std::env::var("CONSOLE_CEF_EAGER").ok().as_deref(),
    );
    if !wanted {
        return false;
    }
    let up = ensure_initialized();
    match (up, debug_port()) {
        (true, Some(port)) => log::info!("CEF: started at launch, CDP on 127.0.0.1:{port}"),
        (true, None) => log::info!("CEF: started at launch (no debug port)"),
        (false, _) => log::warn!("CEF: could not start at launch"),
    }
    up
}

/// Pick the CDP port: a valid `override_port` (1024-65535) wins, otherwise a
/// currently-free port on 127.0.0.1. `None` if neither is available.
pub fn resolve_debug_port(override_port: Option<&str>) -> Option<u16> {
    if let Some(port) = override_port
        .and_then(|raw| raw.trim().parse::<u16>().ok())
        .filter(|port| *port >= 1024)
    {
        return Some(port);
    }
    let listener = std::net::TcpListener::bind(("127.0.0.1", 0)).ok()?;
    let port = listener.local_addr().ok()?.port();
    drop(listener);
    (port >= 1024).then_some(port)
}

impl CefRuntime {
    /// Initialize CEF in the browser process. Must be called on the main
    /// thread before creating any browser. Returns `None` for helper
    /// processes and for unbundled development binaries. No `App` handler is
    /// passed: the message loop is pumped by the `NSTimer` installed in
    /// `ensure_initialized`, which is the documented `CefDoMessageLoopWork`
    /// integration (an `external_message_pump` handler would be a later
    /// efficiency tuning, not a correctness need).
    pub fn initialize() -> Option<Self> {
        if is_helper_process() {
            return None;
        }
        let paths = bundle_paths()?;
        if framework_library_path().is_none() {
            log::debug!("CEF: framework dylib not found next to bundle");
            return None;
        }
        let exe = std::env::current_exe().ok()?;
        let library = LibraryLoader::new(&exe, false);
        if !library.load() {
            log::warn!("CEF: failed to load the Chromium framework");
            return None;
        }
        let _ = cef::api_hash(cef::sys::CEF_API_VERSION_LAST, 0);
        let args = Args::new();
        // Inject CefAppProtocol conformance after the framework is loaded
        // (the protocol object only exists then) and before CefInitialize.
        // Works for GPUI's NSApplication subclass via method injection.
        super::application::conform_ns_application();
        let framework_dir = paths.framework_dir.to_string_lossy().into_owned();
        let helper_exe = paths.helper_exe.to_string_lossy().into_owned();
        // Isolated profile support: point CEF at a scratch user-data dir so
        // concurrent instances (e.g. a test build next to the dev app) do
        // not share the default `~/Library/Application Support/CEF` profile
        // and its process-singleton lock.
        let data_dir = std::env::var("CONSOLE_CEF_DATA_DIR").ok().filter(|dir| {
            std::fs::create_dir_all(dir).is_ok()
        });
        // CDP endpoint (127.0.0.1 only) so agent-browser can attach to this
        // embedded Chromium. Ephemeral by default so a dev build and a test
        // build never fight over a port; `CONSOLE_CEF_DEBUG_PORT` pins one.
        let debug_port = resolve_debug_port(std::env::var("CONSOLE_CEF_DEBUG_PORT").ok().as_deref());
        if let Some(port) = debug_port {
            DEBUG_PORT.store(port, Ordering::Relaxed);
            log::info!("CEF: remote debugging on 127.0.0.1:{port}");
        }
        let settings = cef::Settings {
            // The macOS sandbox needs an endorsed helper plus entitlements;
            // that ships with distribution signing (later). Until then the
            // browser process runs unsandboxed, like our dev builds.
            no_sandbox: 1,
            remote_debugging_port: debug_port.map_or(0, i32::from),
            framework_dir_path: CefString::from(framework_dir.as_str()),
            browser_subprocess_path: CefString::from(helper_exe.as_str()),
            ..Default::default()
        };
        let settings = match data_dir.as_deref() {
            Some(dir) => cef::Settings {
                root_cache_path: CefString::from(dir),
                cache_path: CefString::from(dir),
                ..settings
            },
            None => settings,
        };
        let ok = cef::initialize(
            Some(args.as_main_args()),
            Some(&settings),
            None,
            std::ptr::null_mut(),
        );
        if ok != 1 {
            log::warn!("CEF: cef_initialize failed");
            return None;
        }
        log::debug!("CEF: runtime initialized");
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
        // `conform_ns_application` runs inside `initialize`, after the
        // framework load and before `CefInitialize` (see above).
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
