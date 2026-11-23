# Desktop Clippy Performance & Lint Cleanup

Resolve all warnings and test compilation issues surfaced by running `cargo clippy --workspace --all-targets` with performance-oriented lints (`clippy::perf`, `clippy::redundant_clone`, `clippy::needless_collect`, `clippy::large_enum_variant`, `clippy::box_collection`, `clippy::vec_box`, `clippy::rc_buffer`, `clippy::rc_mutex`).

---

## 1. Inventory

1. **Test compile blocker (`console-ui`)**
   - `crates/console-ui/tests/browser/visibility_test.rs:1` imports `console_ui::browser::view::should_show_native`, which broke when `browser::view` became private.
   - Fix: re-export `should_show_native` from `console_ui::browser` and update the test import.

2. **Auto-fixable lints (`cargo clippy --fix`)**
   - `redundant_clone` (~165 across `console-app`, ~58 across `console-ui`, plus tests)
   - `collapsible_if` (3 in `console-ui`, 1 in `console-app`)
   - `let_and_return` (1 in `console-app`)
   - `useless_vec`, `useless_format`, `single_match` in workspace tests

3. **`rc_buffer` (`Rc<Vec<T>>` / `Arc<Vec<T>>` -> `Rc<[T]>` / `Arc<[T]>`)**
   - `console-ui`: `transcript_view/mod.rs`, `composer_view.rs`, `global_search_panel.rs`, `model_picker.rs`, `palette.rs`, `pill_popovers.rs`, `workspace_footer.rs`, `devices/view.rs`, `inspector/file_tree.rs`, `layout/ports_popover.rs`, `primitives/menu.rs`, `settings/keybindings_page.rs` (`Arc<[T]>`), `viewer/code_viewer.rs`, `viewer/logs_viewer.rs`, `viewer/review_tab.rs`.
   - `console-app`: `state/types.rs`, `types/workspace.rs`, `state/workspace_snapshot.rs`, `state/browser_annotations.rs`, `state/execution.rs`, `state/file_comments.rs`, `state/port_forward.rs`, `state/workspace_tabs/chat.rs`.
   - Note: `Rc::make_mut` call sites (e.g. `sessions`, `changes_review_data`, `working_changes`) cannot mutate an unsized `Rc<[T]>` in place; either convert those specific fields to `Rc<[T]>` with copy-on-write helper or keep `Vec` only where `Rc::make_mut` is required, or replace `Rc::make_mut` with explicit clone-to-Vec + `Rc::from`.

4. **`large_enum_variant` (`console-core`)**
   - `crates/console-core/src/services/automation.rs`: `AutomationStreamEvent::Event(Box<AutomationEvent>)` and `WebhookStreamEvent::Event(Box<WebhookEvent>)`.

5. **Remaining manual lints**
   - `if_same_then_else` in `console-ui/src/common/project_browse_palette.rs:180` and `console-ui/src/computer/view.rs:224` (inspect logic carefully before simplifying).
   - `result_large_err` in `console-core/tests/network/local_forward_pipe_test.rs:157`.
   - `type_complexity` in `console-ui/tests/composer/floating_composer_test.rs`.
   - `needless_update` in `console-proto/tests/event_parts_test.rs:70`.

---

## 2. Execution Order

1. Fix `browser/visibility_test.rs` compile error so `--all-targets` builds cleanly.
2. Run `cargo clippy --fix` across `console-core`, `console-proto`, `console-shared`, `console-ui`, and `console-app` (with `--allow-dirty --allow-staged`) to apply all safe auto-fixes.
3. Fix `large_enum_variant` in `console-core` and update callers.
4. Convert `Rc<Vec<T>>` / `Arc<Vec<T>>` -> `Rc<[T]>` / `Arc<[T]>` across `console-ui` and `console-app`.
5. Resolve remaining manual warnings (`if_same_then_else`, test lints).
6. Re-run full `cargo clippy --workspace --all-targets` with the same flags to verify zero warnings, run relevant tests, and commit per logical task.
