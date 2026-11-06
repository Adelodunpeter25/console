# console-nsh

Console's NSH app (phase 1 of the NSH migration, `docs/plan/migration/`). A TSX app with a Rust
crate of its own: the screens are TSX (`App.tsx`), and the directory is also a crate (`Cargo.toml`,
`src/lib.rs`; `nsh.json` `"rust": "."`) whose `#[native_module("console-nsh/native")]` exports what
`App.tsx` imports from `console-nsh/native`. The crate links Console's existing `console-core`
(`apps/desktop/crates/console-core`) by path — shared, not forked.

```sh
nsh run                      # this machine's backend; --target ios, android, web, macos, linux, windows or gpui
nsh check                    # cargo check of the crate, then type checks App.tsx against the declarations generated from it
nsh test                     # cargo test, then App.test.tsx, headless; --target to run it on a platform
nsh package --target web     # or ios, android, macos, linux, windows
```

## Identity

`nsh.json` carries the release id `com.console.desktop`, the same bundle id the GPUI app packages
with (`apps/desktop/scripts/package.sh`). NSH has no dev/prod split in `nsh.json`; a development
build overrides the id on the command line, the way the GPUI dev script does:

```sh
nsh package --target macos --id com.console.desktop.dev
```

Keeping the dev id distinct keeps the app's storage (`<app id>/…` per platform) separate from a
release install.

The `nsh` commands build the app's hosts from the NSH crates `Cargo.toml` depends on: crates.io's at
the version of the `nsh` you run (keep the `nsh-*` dependencies at that version when you upgrade
`nsh`), or an NSH checkout's by path.

NSH-generated files stay in `.nsh/`: runner sources in `.nsh/rust/` and default packages in
`.nsh/out/`. Cargo keeps its normal `target/` directory and respects your existing
`CARGO_TARGET_DIR`.

## Known gaps

- The web target does not build yet: `console-core` pulls in tokio's net stack (`mio`),
  `tokio-tungstenite` and `termy`, none of which compile for `wasm32-unknown-unknown`.
  `docs/plan/migration/implementation-notes.md`, finding 5, has the options.
