//! The app's Rust code: what `App.tsx` imports from `nsh-chat/native`. Their
//! TypeScript declarations are generated from these types into
//! `.nsh/types/nsh-chat/native.d.ts`.
//!
//! Spike (phase 0 of the NSH migration): a scripted chat session whose reply
//! streams token by token from Rust, the shape Console's real session
//! service will replace.

use std::time::Duration;

use nsh_core::prelude::*;

/// The crate's UI set (`nsh.json` `"rust"`).
#[native_module("nsh-chat/native")]
fn native(registry: &mut Registry) {
    registry.module::<Session>();
}

/// A scripted chat session: `send` streams a reply token by token through a
/// callback, the shape of a real model reply.
#[module]
pub struct Session;

#[module]
impl Session {
    /// Sends `prompt` and streams a scripted reply token by token; resolves
    /// with the reply's length in characters.
    // An `async` module function runs as a task of the core, so it can
    // `post` to the callback between awaits, where JS is not running.
    pub async fn send(
        prompt: String,
        #[args(token)] on_token: Callback<String>,
    ) -> u32 {
        let reply = scripted_reply(&prompt);
        let mut chars = 0u32;
        for word in reply.split_inclusive(' ') {
            timer(Duration::from_millis(24)).await;
            chars += word.chars().count() as u32;
            on_token.post(word.to_owned());
        }
        chars
    }
}

/// The reply the spike streams: the prompt echoed back, markdown, a list and
/// a code block, ending in a marker the tests wait for.
fn scripted_reply(prompt: &str) -> String {
    format!(
        r#"### Reply to “{prompt}”

This reply is **streamed from Rust** — one token every 24 ms, through a
`Callback<String>` posted from the core's task.

- Same engine as the desktop app
- Same reactive graph as the TSX screens

```rust
fn main() {{
    println!("hello from the Rust core");
}}
```

*End of the scripted reply.*
"#
    )
}
