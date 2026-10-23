//! Markdown helpers that are pure string logic.
//!
//! The Markdown *parser* is deliberately not here: each client parses with its
//! own engine (docs/plan/console-shared.md §3.2).

pub mod file_links;
pub mod mend;
