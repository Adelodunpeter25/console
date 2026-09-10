//! Embedded native browser surface for Console desktop.
//!
//! Provides a complete native browser tab with navigation toolbar,
//! omnibox address resolution, focus management, geometry synchronization,
//! and overlay snapshot protection.

mod actions;
pub mod address;
pub mod host;
pub mod view;

pub use actions::{
    BROWSER_ADDRESS_KEY_CONTEXT, BROWSER_KEY_CONTEXT, BrowserAddressCancel, BrowserBack,
    BrowserDevtools, BrowserForward, BrowserHardReload, BrowserReload, BrowserStop,
    FocusBrowserAddress, WebviewCopy, WebviewCut, WebviewPaste, WebviewSelectAll,
    init_browser_keybindings,
};
pub use address::*;
pub use host::*;
pub use view::*;
