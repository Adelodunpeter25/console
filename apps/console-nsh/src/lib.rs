//! The app's Rust code: what `App.tsx` imports from `console-nsh/native`. Their
//! TypeScript declarations are generated from these types into
//! `.nsh/types/console-nsh/native.d.ts`.
//!
//! Phase 1 of the NSH migration: the crate links Console's existing
//! `console-core` (UI-free) by path, so services move over without forking.

use std::time::Duration;

use console_core::HttpTransport;
use nsh_core::prelude::*;

/// The crate's UI set (`nsh.json` `"rust"`).
#[native_module("console-nsh/native")]
fn native(registry: &mut Registry) {
    registry.module::<Console>();
}

/// The Console backend, reached through `console-core`'s `HttpTransport`.
#[module]
pub struct Console;

#[module]
impl Console {
    /// Fetches `{base_url}/health` through console-core and returns the
    /// response body, or an error message if the server is unreachable.
    pub async fn health(base_url: String) -> String {
        let transport = HttpTransport::new(Some(base_url));
        let url = transport.url("/health").await;
        match tokio::time::timeout(Duration::from_secs(5), transport.client().get(url).send()).await
        {
            Ok(Ok(response)) => match response.text().await {
                Ok(body) => body,
                Err(err) => format!("bad response: {err}"),
            },
            Ok(Err(err)) => format!("unreachable: {err}"),
            Err(_) => "unreachable: timed out".to_owned(),
        }
    }
}
