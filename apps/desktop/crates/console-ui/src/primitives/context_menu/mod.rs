//! The app's concrete context menus (sidebar sessions and drafts), built on the
//! shared [`super::menu::context_menu`] surface. That surface owns right-click
//! handling, placement, dismissal and keyboard navigation; this module only
//! decides which entries each menu has and in what order.

pub mod session_context_menu;

pub use session_context_menu::{draft_context_menu, session_context_menu};
