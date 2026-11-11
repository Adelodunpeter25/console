//! The parts of the desktop app that can be tested without a window.
//!
//! The app itself is a binary; this library carries the persisted state
//! format so it can be tested the way every other crate in the workspace is.
//! The binary uses it through `use console_app::persistence;`.

pub mod persistence;
