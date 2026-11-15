# Desktop Rust Optimization & Fix Plan

Plan for Rust best practices, clippy fixes, panic prevention, and concurrency improvements across `apps/desktop`.

## Tasks

- [x] **Task 1: Fix Clippy Warnings breaking `-D warnings`** (`crates/console-ui/src/chat/diff_popover.rs`, `crates/console-ui/src/common/input/element.rs`, `crates/console-ui/src/layout/sidebar/session_card.rs`, `crates/console-ui/src/layout/sidebar/session_item.rs`, `crates/console-ui/src/markdown/render.rs`, `crates/console-ui/src/settings/api_key_section.rs`)
- [x] **Task 2: Eliminate Panic Risks from `.unwrap()`** (`crates/console-ui/src/markdown/render.rs`, `src/state/pagination.rs`, `crates/console-ui/src/settings/keybindings_page.rs`, `crates/console-ui/src/terminal/terminal_view/paint.rs`)
- [x] **Task 3: Fix Tunnel Cache Race Condition** (`crates/console-core/src/utils/http_transport.rs`)
- [x] **Task 4: Eliminate Redundant Clones in Loops** (`crates/console-ui/src/common/palette.rs`, `crates/console-ui/src/inspector/file_tree.rs`)
- [x] **Task 5: Replace `Option<String>` with `Option<&str>` in Status Lookups** (`crates/console-ui/src/run/run_panel.rs`)
- [x] **Task 6: Consolidate Terminal Split Locks** (`crates/console-core/src/services/terminal/mod.rs`)
- [x] **Task 7: Add `[workspace.lints]` Configuration** (`apps/desktop/Cargo.toml`)
- [x] **Task 8: Consolidate Global Mutexes in Address Cache** (`crates/console-ui/src/browser/address.rs`)
- [x] **Task 9: Refactor Mutually Exclusive Booleans into Enums** (`crates/console-ui/src/common/floating_composer/mod.rs`)
