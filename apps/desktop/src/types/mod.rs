//! App-owned type definitions, kept out of the state/logic modules.
//!
//! Domain and transport types live in `console-core`; this module only holds
//! structures specific to the desktop shell.

pub(crate) mod workspace;

pub(crate) use workspace::WorkspacePaneState;
