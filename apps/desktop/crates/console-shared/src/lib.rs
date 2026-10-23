//! Pure logic shared by the desktop and Android clients.
//!
//! One implementation, two callers: the desktop imports these modules as a
//! normal Rust dependency; Android reaches them over JNI through
//! `apps/android/rust-wt`. Nothing here may depend on a runtime (`tokio`), a
//! network stack, or a UI toolkit. See `docs/plan/console-shared.md`.

pub mod composer;
pub mod diff;
pub mod file_kind;
pub mod format;
pub mod markdown;
pub mod page_title;
pub mod pairing;
pub mod path;
pub mod throughput;
pub mod tools;
