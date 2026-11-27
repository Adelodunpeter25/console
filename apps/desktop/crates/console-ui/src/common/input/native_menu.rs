//! Native macOS context menu for text input fields.
//!
//! Replaces the GPUI-rendered context menu on macOS with a real AppKit `NSMenu`
//! that integrates with standard system features (Look Up, Google Search, Speech,
//! Services, and clipboard actions).
//!
//! DO NOT simplify this back to a single blocking call inside the mouse-down
//! handler. `NSMenu::popUpContextMenu:withEvent:forView:` spins a nested AppKit
//! run loop on the main thread, and that loop drains the main dispatch queue,
//! which re-enters GPUI and polls its own queued tasks — including background
//! timers such as `schedule_drafts_save`, which call `AsyncApp::update` and so
//! `AppCell::borrow_mut`. The mouse-down handler runs inside `entity.update`,
//! which holds that same `RefCell` borrow for its whole body, so the re-entrant
//! borrow panics; under the release profile's `panic = "abort"` that surfaces as
//! `EXC_BAD_ACCESS`, not a readable panic. It only reproduces when a task happens
//! to be pending, so it looks intermittent.
//!
//! The call is therefore SPLIT, exactly as `picker.rs` splits its AppKit panel:
//!   - [`prepare`] is SYNCHRONOUS and cheap. It captures the current `NSEvent`
//!     and the hosting view while they are still valid, so calling it inside an
//!     `entity.update` holds no borrow across any modal loop.
//!   - [`Prepared::show`] is the blocking half, and MUST be called off the UI's
//!     current stack — from a spawned task, whose first poll runs outside
//!     `App::update` and so holds no borrow while AppKit's loop runs.

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum NativeContextMenuAction {
    Cut,
    Copy,
    Paste,
    SelectAll,
    LookUp(String),
    SearchGoogle(String),
    Speak(String),
    StopSpeaking,
}

#[derive(Clone, Debug)]
pub struct NativeContextMenuParams {
    pub has_selection: bool,
    pub selected_text: Option<String>,
    pub can_cut: bool,
    pub can_copy: bool,
    pub can_paste: bool,
    pub can_select_all: bool,
    pub total_text: String,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum NativeContextMenuResult {
    Action(NativeContextMenuAction),
    Dismissed,
    NotSupported,
}

pub use imp::{Prepared, execute_native_action, prepare};

#[cfg(not(target_os = "macos"))]
mod imp {
    use super::{NativeContextMenuAction, NativeContextMenuParams, NativeContextMenuResult};

    /// Opaque stand-in so callers need no `cfg` of their own.
    pub struct Prepared;

    pub fn prepare(_params: NativeContextMenuParams) -> Option<Prepared> {
        None
    }

    impl Prepared {
        pub fn show(self) -> NativeContextMenuResult {
            NativeContextMenuResult::NotSupported
        }
    }

    pub fn execute_native_action(_action: &NativeContextMenuAction) {}
}

#[cfg(target_os = "macos")]
mod imp {
    use std::cell::RefCell;

    use objc2::rc::Retained;
    use objc2::runtime::{AnyObject, Sel};
    use objc2::{AnyThread, ClassType, DefinedClass, MainThreadOnly, define_class, msg_send, sel};
    use objc2_app_kit::{NSApplication, NSEvent, NSMenu, NSMenuItem, NSView};
    use objc2_foundation::{MainThreadMarker, NSObjectProtocol, NSString, NSURL};

    use super::{NativeContextMenuAction, NativeContextMenuParams, NativeContextMenuResult};

    struct NativeMenuTargetIvars {
        selected_action: RefCell<Option<NativeContextMenuAction>>,
        actions: RefCell<Vec<NativeContextMenuAction>>,
    }

    define_class!(
        #[unsafe(super(objc2::runtime::NSObject))]
        #[ivars = NativeMenuTargetIvars]
        struct NativeMenuTarget;

        impl NativeMenuTarget {
            #[unsafe(method(menuItemClicked:))]
            fn menu_item_clicked(&self, sender: &NSMenuItem) {
                let tag = sender.tag() as usize;
                if let Some(action) = self.ivars().actions.borrow().get(tag).cloned() {
                    *self.ivars().selected_action.borrow_mut() = Some(action);
                }
            }
        }

        unsafe impl NSObjectProtocol for NativeMenuTarget {}
    );

    impl NativeMenuTarget {
        fn new() -> Retained<Self> {
            let this = Self::alloc().set_ivars(NativeMenuTargetIvars {
                selected_action: RefCell::new(None),
                actions: RefCell::new(Vec::new()),
            });
            unsafe { msg_send![super(this), init] }
        }

        fn register_action(&self, action: NativeContextMenuAction) -> isize {
            let mut actions = self.ivars().actions.borrow_mut();
            let tag = actions.len() as isize;
            actions.push(action);
            tag
        }

        fn selected(&self) -> Option<NativeContextMenuAction> {
            self.ivars().selected_action.borrow_mut().take()
        }
    }

    /// The menu presentation, captured while the triggering event is current.
    ///
    /// Holding the `NSEvent` and the hosting `NSView` here is what lets [`show`]
    /// run later, on a stack with no `App` borrow held: AppKit needs both, and
    /// neither is retrievable once the mouse event has finished dispatching.
    ///
    /// [`show`]: Prepared::show
    pub struct Prepared {
        event: Retained<NSEvent>,
        view: Option<Retained<NSView>>,
        params: NativeContextMenuParams,
    }

    pub fn prepare(params: NativeContextMenuParams) -> Option<Prepared> {
        let mtm = MainThreadMarker::new()?;
        let app = NSApplication::sharedApplication(mtm);
        let event = app.currentEvent()?;
        let view = app.keyWindow().and_then(|window| window.contentView());
        Some(Prepared {
            event,
            view,
            params,
        })
    }

    fn open_system_url(url_str: &str) {
        let ns_str = NSString::from_str(url_str);
        let url: Option<Retained<NSURL>> =
            unsafe { msg_send![NSURL::class(), URLWithString: &*ns_str] };
        if let Some(url) = url {
            unsafe {
                let ws: *mut AnyObject = msg_send![objc2::class!(NSWorkspace), sharedWorkspace];
                if !ws.is_null() {
                    let _: bool = msg_send![ws, openURL: &*url];
                }
            }
        }
    }

    fn speak_text(text: &str) {
        let ns_str = NSString::from_str(text);
        unsafe {
            let synth: *mut AnyObject = msg_send![objc2::class!(NSSpeechSynthesizer), new];
            if !synth.is_null() {
                let _: bool = msg_send![synth, startSpeakingString: &*ns_str];
            }
        }
    }

    fn stop_speaking() {
        unsafe {
            let synth: *mut AnyObject = msg_send![objc2::class!(NSSpeechSynthesizer), new];
            if !synth.is_null() {
                let _: () = msg_send![synth, stopSpeaking];
            }
        }
    }

    pub fn execute_native_action(action: &NativeContextMenuAction) {
        match action {
            NativeContextMenuAction::LookUp(word) => {
                let query = urlencoding::encode(word.trim());
                open_system_url(&format!("dict://{}", query));
            }
            NativeContextMenuAction::SearchGoogle(query) => {
                let encoded = urlencoding::encode(query.trim());
                open_system_url(&format!("https://www.google.com/search?q={}", encoded));
            }
            NativeContextMenuAction::Speak(text) => {
                speak_text(text);
            }
            NativeContextMenuAction::StopSpeaking => {
                stop_speaking();
            }
            _ => {}
        }
    }

    impl Prepared {
        /// Blocks until the menu closes. Must not be called while the `App`
        /// borrow is held — see this module's header.
        pub fn show(self) -> NativeContextMenuResult {
            let Some(mtm) = MainThreadMarker::new() else {
                return NativeContextMenuResult::NotSupported;
            };

            let Prepared {
                event,
                view,
                params,
            } = self;

            let target = NativeMenuTarget::new();

            let menu: Retained<NSMenu> =
                unsafe { msg_send![NSMenu::alloc(mtm), initWithTitle: &*NSString::from_str("")] };
            unsafe {
                let _: () = msg_send![&*menu, setAllowsContextMenuPlugIns: true];
            }

            let add_item = |menu: &NSMenu,
                            title: &str,
                            key_equiv: &str,
                            action: Option<NativeContextMenuAction>,
                            enabled: bool| {
                let item: Retained<NSMenuItem> = unsafe {
                    msg_send![
                        NSMenuItem::alloc(mtm),
                        initWithTitle: &*NSString::from_str(title),
                        action: sel!(menuItemClicked:),
                        keyEquivalent: &*NSString::from_str(key_equiv)
                    ]
                };
                if let Some(action) = action {
                    let tag = target.register_action(action);
                    unsafe {
                        let _: () = msg_send![&*item, setTag: tag];
                        let _: () = msg_send![&*item, setTarget: Some(&*target)];
                    }
                }
                unsafe {
                    let _: () = msg_send![&*item, setEnabled: enabled];
                    let _: () = msg_send![menu, addItem: &*item];
                }
            };

            let add_separator = |menu: &NSMenu| unsafe {
                let sep: Retained<NSMenuItem> = msg_send![NSMenuItem::class(), separatorItem];
                let _: () = msg_send![menu, addItem: &*sep];
            };

            // 1. Look Up & Search (if text is selected)
            if let Some(selected) = &params.selected_text {
                let trimmed = selected.trim();
                if !trimmed.is_empty() {
                    let display_term = if trimmed.chars().count() > 20 {
                        let mut s: String = trimmed.chars().take(17).collect();
                        s.push_str("...");
                        s
                    } else {
                        trimmed.to_string()
                    };

                    add_item(
                        &menu,
                        &format!("Look Up “{}”", display_term),
                        "",
                        Some(NativeContextMenuAction::LookUp(trimmed.to_string())),
                        true,
                    );
                    add_item(
                        &menu,
                        "Search with Google",
                        "",
                        Some(NativeContextMenuAction::SearchGoogle(trimmed.to_string())),
                        true,
                    );
                    add_separator(&menu);
                }
            }

            // 2. Clipboard actions
            add_item(
                &menu,
                "Cut",
                "x",
                Some(NativeContextMenuAction::Cut),
                params.can_cut,
            );
            add_item(
                &menu,
                "Copy",
                "c",
                Some(NativeContextMenuAction::Copy),
                params.can_copy,
            );
            add_item(
                &menu,
                "Paste",
                "v",
                Some(NativeContextMenuAction::Paste),
                params.can_paste,
            );
            add_separator(&menu);

            // 3. Selection
            add_item(
                &menu,
                "Select All",
                "a",
                Some(NativeContextMenuAction::SelectAll),
                params.can_select_all,
            );
            add_separator(&menu);

            // 4. Speech submenu
            let speech_menu: Retained<NSMenu> = unsafe {
                msg_send![NSMenu::alloc(mtm), initWithTitle: &*NSString::from_str("Speech")]
            };
            let speech_text = params
                .selected_text
                .as_deref()
                .unwrap_or(&params.total_text);
            let can_speak = !speech_text.trim().is_empty();

            add_item(
                &speech_menu,
                "Start Speaking",
                "",
                Some(NativeContextMenuAction::Speak(speech_text.to_string())),
                can_speak,
            );
            add_item(
                &speech_menu,
                "Stop Speaking",
                "",
                Some(NativeContextMenuAction::StopSpeaking),
                true,
            );

            let speech_item: Retained<NSMenuItem> = unsafe {
                msg_send![
                    NSMenuItem::alloc(mtm),
                    initWithTitle: &*NSString::from_str("Speech"),
                    action: None::<Sel>,
                    keyEquivalent: &*NSString::from_str("")
                ]
            };
            unsafe {
                let _: () = msg_send![&*speech_item, setSubmenu: &*speech_menu];
                let _: () = msg_send![&*menu, addItem: &*speech_item];
            }

            // Show the menu at the current mouse event location
            let view_ptr = view.as_deref();

            unsafe {
                let _: () = msg_send![
                    NSMenu::class(),
                    popUpContextMenu: &*menu,
                    withEvent: &*event,
                    forView: view_ptr
                ];
            }

            if let Some(action) = target.selected() {
                NativeContextMenuResult::Action(action)
            } else {
                NativeContextMenuResult::Dismissed
            }
        }
    }
}
