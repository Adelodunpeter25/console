//! Context menus built on `gpui-component`.
//!
//! Right-click handling, popup placement, dismissal, keyboard navigation, and
//! focus restoration are delegated to `gpui_component::menu`; this module only
//! hosts the app's concrete menus on top of it:
//!
//! ```ignore
//! row.context_menu(|menu, _, _| menu.menu("Open", Box::new(OpenFile)))
//! ```

pub mod session_context_menu;

pub use session_context_menu::session_context_menu;
