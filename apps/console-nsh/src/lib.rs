//! The crate's native boundary: what `App.tsx` imports from
//! `console-nsh/native`. TypeScript declarations are generated from these
//! types into `.nsh/types/console-nsh/native.d.ts`.

mod native;

use nsh_core::prelude::*;

/// The crate's UI set (`nsh.json` `"rust"`).
#[native_module("console-nsh/native")]
fn ui(registry: &mut Registry) {
    registry.module::<native::Console>();
}
