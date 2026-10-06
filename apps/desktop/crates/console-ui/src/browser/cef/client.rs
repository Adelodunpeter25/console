//! CEF client and handlers bridging page events to [`HostCallbacks`].
//!
//! Page-to-host messaging uses the console bridge: `on_load_end` installs a
//! `window.ipc.postMessage` shim that logs with a magic prefix, and
//! `on_console_message` routes prefixed logs back to the `on_ipc` callback.
//! This needs no renderer-process code, so helper processes stay stock. A
//! future slice may upgrade to `CefMessageRouter` or CDP
//! `Runtime.consoleAPICalled`.

use std::cell::Cell;
use std::rc::Rc;

use cef::{
    Browser, BrowserSettings, CefString, Client, ContextMenuHandler, ContextMenuParams,
    DictionaryValue, DisplayHandler, Errorcode, Frame, ImplBrowser, ImplClient,
    ImplContextMenuHandler, ImplDisplayHandler, ImplFrame, ImplLifeSpanHandler, ImplLoadHandler,
    ImplMenuModel, LifeSpanHandler, LoadHandler, LogSeverity, MenuModel, PopupFeatures, WindowInfo,
    WindowOpenDisposition, WrapClient, WrapContextMenuHandler, WrapDisplayHandler,
    WrapLifeSpanHandler, WrapLoadHandler,
};
use cef::{wrap_client, wrap_context_menu_handler, wrap_display_handler, wrap_life_span_handler, wrap_load_handler};
use cef::rc::Rc as CefRc;

use crate::browser::host::NativeNavigationError;

/// Prefix marking bridge logs in `on_console_message`. Pages must not log
/// lines starting with this except through the installed shim.
const IPC_CONSOLE_PREFIX: &str = "[console-ipc]";

/// Installed on every main-frame load: defines `window.ipc.postMessage`
/// (the same surface wry injects natively) on top of `console.log`.
const IPC_SHIM_JS: &str = r#"(function() {
  if (window.__consoleIpcInstalled) return;
  window.__consoleIpcInstalled = true;
  window.ipc = { postMessage: function(message) {
    console.log("[console-ipc]" + message);
  } };
})();"#;

/// State shared between the handlers and the host. All CEF UI-thread
/// callbacks run on the main thread, so `Rc`/`Cell` suffice.
pub(super) struct Shared {
    pub(super) nav: NavCallbacks,
    pub(super) on_title: Option<Box<dyn Fn(String)>>,
    pub(super) on_ipc: Option<Box<dyn Fn(String)>>,
    pub(super) on_new_window_url: Option<Box<dyn Fn(String)>>,
    pub(super) init_script: Option<String>,
    pub(super) progress: Cell<f64>,
    /// Set when the host starts tearing down: callbacks must not run page
    /// script anymore (the browser may be half-closed).
    pub(super) closing: Cell<bool>,
    /// The native child view, filled in by `create` once CEF hands it over.
    /// Detached in `on_before_close`, when CEF is done with it — never
    /// earlier, so CEF's async teardown never meets a view we pulled away.
    pub(super) native_view: Cell<Option<objc2::rc::Retained<objc2_app_kit::NSView>>>,
}

/// The navigation subset of [`HostCallbacks`]. Responder tracking stays with
/// the host (via `firstResponder` KVO); only these cross into CEF handlers.
pub(super) struct NavCallbacks {
    pub(super) on_start: Box<dyn Fn()>,
    pub(super) on_finish: Box<dyn Fn()>,
    pub(super) on_error: Box<dyn Fn(NativeNavigationError)>,
}

pub(super) type SharedRef = Rc<Shared>;

fn is_main_frame(frame: &Option<&mut Frame>) -> bool {
    frame.as_ref().is_some_and(|frame| frame.is_main() != 0)
}

/// Map a Chromium load error onto our (WK-shaped) navigation error so the
/// existing error pages, titles, and hints are reused unchanged.
fn map_load_error(
    error_code: Errorcode,
    error_text: Option<&CefString>,
    failed_url: Option<&CefString>,
) -> NativeNavigationError {
    let code = cef::sys::cef_errorcode_t::from(error_code);
    let wk_code = match code {
        cef::sys::cef_errorcode_t::ERR_CONNECTION_REFUSED => -1004,
        cef::sys::cef_errorcode_t::ERR_NAME_NOT_RESOLVED => -1003,
        cef::sys::cef_errorcode_t::ERR_TIMED_OUT => -1001,
        cef::sys::cef_errorcode_t::ERR_INTERNET_DISCONNECTED => -1009,
        _ => -1,
    };
    let text = error_text.map(CefString::to_string).unwrap_or_default();
    let url = failed_url.map(CefString::to_string).unwrap_or_default();
    let description = if text.is_empty() {
        format!("Chromium could not load this page ({url}).")
    } else if url.is_empty() {
        text
    } else {
        format!("{text} ({url})")
    };
    NativeNavigationError::new(wk_code, "CEF".to_string(), description)
}

wrap_client! {
    struct ConsoleClient {
        shared: SharedRef,
    }

    impl Client {
        fn context_menu_handler(&self) -> Option<ContextMenuHandler> {
            Some(ConsoleContextMenuHandler::new(self.shared.clone()))
        }

        fn display_handler(&self) -> Option<DisplayHandler> {
            Some(ConsoleDisplayHandler::new(self.shared.clone()))
        }

        fn life_span_handler(&self) -> Option<LifeSpanHandler> {
            Some(ConsoleLifeSpanHandler::new(self.shared.clone()))
        }

        fn load_handler(&self) -> Option<LoadHandler> {
            Some(ConsoleLoadHandler::new(self.shared.clone()))
        }
    }
}

wrap_context_menu_handler! {
    struct ConsoleContextMenuHandler {
        shared: SharedRef,
    }

    impl ContextMenuHandler {
        fn on_before_context_menu(
            &self,
            _browser: Option<&mut Browser>,
            _frame: Option<&mut Frame>,
            _params: Option<&mut ContextMenuParams>,
            model: Option<&mut MenuModel>,
        ) {
            // Interim: suppress the native context menu. Showing it crashes
            // inside CEF on right-click (unrecognized selector on a dead
            // object during menu handling); clearing the model avoids the
            // native menu path entirely. A custom menu is a later slice.
            let _ = &self.shared;
            if let Some(model) = model {
                model.clear();
            }
            log::debug!("CEF: context menu suppressed");
        }
    }
}

wrap_display_handler! {
    struct ConsoleDisplayHandler {
        shared: SharedRef,
    }

    impl DisplayHandler {
        fn on_title_change(&self, _browser: Option<&mut Browser>, title: Option<&CefString>) {
            let Some(title) = title else {
                return;
            };
            if let Some(on_title) = self.shared.on_title.as_ref() {
                on_title(title.to_string());
            }
        }

        fn on_loading_progress_change(&self, _browser: Option<&mut Browser>, progress: f64) {
            self.shared.progress.set(progress);
        }

        fn on_console_message(
            &self,
            _browser: Option<&mut Browser>,
            _level: LogSeverity,
            message: Option<&CefString>,
            _source: Option<&CefString>,
            _line: ::std::os::raw::c_int,
        ) -> ::std::os::raw::c_int {
            let Some(message) = message else {
                return 0;
            };
            let text = message.to_string();
            let Some(payload) = text.strip_prefix(IPC_CONSOLE_PREFIX) else {
                return 0;
            };
            if let Some(on_ipc) = self.shared.on_ipc.as_ref() {
                on_ipc(payload.to_owned());
            }
            // Handled: keep bridge traffic out of DevTools.
            1
        }
    }
}

wrap_load_handler! {
    struct ConsoleLoadHandler {
        shared: SharedRef,
    }

    impl LoadHandler {
        fn on_load_start(
            &self,
            _browser: Option<&mut Browser>,
            frame: Option<&mut Frame>,
            _transition_type: cef::TransitionType,
        ) {
            if !is_main_frame(&frame) {
                return;
            }
            log::debug!("CEF: load started");
            self.shared.progress.set(0.0);
            (self.shared.nav.on_start)();
        }

        fn on_load_end(
            &self,
            browser: Option<&mut Browser>,
            frame: Option<&mut Frame>,
            _http_status_code: ::std::os::raw::c_int,
        ) {
            if !is_main_frame(&frame) {
                return;
            }
            let url = frame
                .as_ref()
                .map(|frame| CefString::from(&frame.url()).to_string())
                .unwrap_or_default();
            log::debug!("CEF: load finished ({url})");
            self.shared.progress.set(1.0);
            // Never run page script during teardown: the browser may be
            // half-closed and V8 re-entrancy there corrupts CEF state
            // (observed as delayed retain crashes in the message pump).
            if self.shared.closing.get() {
                (self.shared.nav.on_finish)();
                return;
            }
            // The page's own scripts run before this handler, but every
            // `window.ipc` call site in our pages is either guarded
            // (`inspector.js`, `player.rs`) or runs post-load
            // (`agent_script.js`), so late shim installation is sufficient.
            if let Some(browser) = browser {
                if let Some(main) = browser.main_frame() {
                    main.execute_java_script(
                        Some(&CefString::from(IPC_SHIM_JS)),
                        Some(&CefString::from("console://ipc-shim")),
                        0,
                    );
                    if let Some(script) = self.shared.init_script.as_deref() {
                        main.execute_java_script(
                            Some(&CefString::from(script)),
                            Some(&CefString::from("console://init-script")),
                            0,
                        );
                    }
                }
            }
            (self.shared.nav.on_finish)();
        }

        fn on_load_error(
            &self,
            _browser: Option<&mut Browser>,
            frame: Option<&mut Frame>,
            error_code: Errorcode,
            error_text: Option<&CefString>,
            failed_url: Option<&CefString>,
        ) {
            if !is_main_frame(&frame) {
                return;
            }
            // Aborted loads (e.g. superseded navigations, downloads) are not
            // failures; showing an error page for them would be wrong.
            if cef::sys::cef_errorcode_t::from(error_code)
                == cef::sys::cef_errorcode_t::ERR_ABORTED
            {
                return;
            }
            let failed = failed_url
                .map(CefString::to_string)
                .unwrap_or_default();
            log::warn!("CEF: load error {error_code:?} for {failed}");
            let error = map_load_error(error_code, error_text, failed_url);
            (self.shared.nav.on_error)(error);
        }
    }
}

wrap_life_span_handler! {
    struct ConsoleLifeSpanHandler {
        shared: SharedRef,
    }

    impl LifeSpanHandler {
        fn do_close(&self, _browser: Option<&mut Browser>) -> ::std::os::raw::c_int {
            // The browser is a child view of the app window. Returning false
            // would make CEF send performClose: to that top-level window and
            // close the whole app window, so claim the close ourselves.
            1
        }

        fn on_before_close(&self, _browser: Option<&mut Browser>) {
            // CEF is done with the browser: now (and only now) detach the
            // native view. Detaching earlier races CEF's async teardown.
            if let Some(view) = self.shared.native_view.take() {
                view.removeFromSuperview();
                log::debug!("CEF: native view detached on before-close");
            }
        }

        fn on_before_popup(
            &self,
            _browser: Option<&mut Browser>,
            _frame: Option<&mut Frame>,
            _popup_id: ::std::os::raw::c_int,
            target_url: Option<&CefString>,
            _target_frame_name: Option<&CefString>,
            _target_disposition: WindowOpenDisposition,
            _user_gesture: ::std::os::raw::c_int,
            _popup_features: Option<&PopupFeatures>,
            _window_info: Option<&mut WindowInfo>,
            _client: Option<&mut Option<Client>>,
            _settings: Option<&mut BrowserSettings>,
            _extra_info: Option<&mut Option<DictionaryValue>>,
            _no_javascript_access: Option<&mut ::std::os::raw::c_int>,
        ) -> ::std::os::raw::c_int {
            // Mirror the wry backend: popups never open a new native window;
            // the URL navigates in this view instead.
            if let Some(url) = target_url {
                if let Some(on_new_window_url) = self.shared.on_new_window_url.as_ref() {
                    on_new_window_url(url.to_string());
                }
            }
            1
        }
    }
}

/// Built client plus the shared state the host keeps for progress reads.
pub(super) struct ClientBundle {
    pub(super) client: Client,
    pub(super) shared: SharedRef,
}

pub(super) fn build_client(
    nav: NavCallbacks,
    on_title: Option<Box<dyn Fn(String)>>,
    on_ipc: Option<Box<dyn Fn(String)>>,
    on_new_window_url: Option<Box<dyn Fn(String)>>,
    init_script: Option<String>,
) -> ClientBundle {
    let shared = Rc::new(Shared {
        nav,
        on_title,
        on_ipc,
        on_new_window_url,
        init_script,
        progress: Cell::new(0.0),
        closing: Cell::new(false),
        native_view: Cell::new(None),
    });
    ClientBundle {
        client: ConsoleClient::new(shared.clone()),
        shared,
    }
}

/// Read the loading progress last reported by
/// `on_loading_progress_change` (0.0 until the first report).
pub(super) fn progress_of(shared: &SharedRef) -> f64 {
    shared.progress.get()
}
