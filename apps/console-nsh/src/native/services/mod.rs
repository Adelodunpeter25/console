//! One module per console-core service, in the shape the GPUI app's state
//! uses them; each maps the proto types to the plain structs of
//! [`crate::native::types`].

pub mod auth;
pub mod chat;
pub mod projects;
pub mod sessions;
pub mod workspace;
