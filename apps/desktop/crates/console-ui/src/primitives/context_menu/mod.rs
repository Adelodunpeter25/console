//! The app's concrete context menus (sidebar sessions and drafts), built on the
//! shared [`super::menu::context_menu`] surface. That surface owns right-click
//! handling, placement, dismissal and keyboard navigation; this module only
//! decides which entries each menu has and in what order.

pub mod html_file_context_menu;
pub mod session_context_menu;

pub use html_file_context_menu::html_file_context_menu;
pub use session_context_menu::{ReadAction, session_context_menu};
