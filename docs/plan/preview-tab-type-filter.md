# Fix: Opening a File Tab Should Not Replace a Diff Tab

**Status:** Draft
**Date:** 2026-09-09
**Approach:** Option C — Add type filter to `replace_or_open_tab`

---

## Problem

Opening a file tab can replace an existing diff tab (and vice versa) because `preview_tab` is a single `(tab_id, Instant)` field shared across all tab types. The replacement logic doesn't check whether the target tab matches the type being opened.

### Reproduction

1. Open a diff tab → `preview_tab = ("diff:src/foo.ts", now)`
2. Open a file tab → `preview_tab` still points to the diff
3. `replace_or_open_tab` is called with `replace_tab_id = Some("diff:src/foo.ts")`
4. Diff tab gets replaced by the file tab

---

## Solution

Add a `ReplaceFilter` enum to `replace_or_open_tab` that constrains which tabs can be replaced.

### New Type

```rust
// In console_ui/src/workspace/ops.rs

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ReplaceFilter {
    /// Replace any tab (current behavior).
    Any,
    /// Only replace if the existing tab is the same kind as the new tab.
    SameKind,
    /// Only replace File tabs.
    FileOnly,
    /// Only replace Diff tabs.
    DiffOnly,
}
```

### Updated Signature

```rust
pub fn replace_or_open_tab(
    root: &mut WorkspaceNode,
    pane_id: &str,
    replace_tab_id: Option<&str>,
    new_tab: WorkspaceTabConfig,
    filter: ReplaceFilter,  // NEW
) -> String
```

### Updated Implementation

```rust
pub fn replace_or_open_tab(
    root: &mut WorkspaceNode,
    pane_id: &str,
    replace_tab_id: Option<&str>,
    new_tab: WorkspaceTabConfig,
    filter: ReplaceFilter,
) -> String {
    let leaf = active_leaf(root, Some(pane_id));
    let new_tab_id = new_tab.id();

    if let Some(target_id) = replace_tab_id {
        if let Some(pos) = leaf.tabs.iter().position(|t| t.id() == target_id) {
            let matches = match filter {
                ReplaceFilter::Any => true,
                ReplaceFilter::SameKind => {
                    std::mem::discriminant(&leaf.tabs[pos]) == std::mem::discriminant(&new_tab)
                }
                ReplaceFilter::FileOnly => {
                    matches!(leaf.tabs[pos], WorkspaceTabConfig::File { .. })
                }
                ReplaceFilter::DiffOnly => {
                    matches!(leaf.tabs[pos], WorkspaceTabConfig::Diff { .. })
                }
            };

            if matches {
                leaf.tabs[pos] = new_tab;
                leaf.active_tab_id = Some(new_tab_id.clone());
                return new_tab_id;
            }
        }
    }

    if !leaf.tabs.iter().any(|t| t.id() == new_tab_id) {
        leaf.tabs.push(new_tab);
    }
    leaf.active_tab_id = Some(new_tab_id.clone());
    new_tab_id
}
```

---

## Files to Change

### 1. `apps/desktop/crates/console-ui/src/workspace/ops.rs`

- Add `ReplaceFilter` enum
- Update `replace_or_open_tab` signature and implementation

### 2. `apps/desktop/src/state/workspace_panes.rs`

Update two call sites to pass `ReplaceFilter::SameKind`:

**`open_file_tab_in_pane` (~line 566):**
```rust
let new_tab_id = workspace_ops::replace_or_open_tab(
    &mut self.workspace_root,
    pane_id,
    preview_target.as_deref(),
    tab,
    ReplaceFilter::SameKind,  // Only replace another file preview
);
```

**`open_diff_tab_in_pane` (~line 642):**
```rust
let new_tab_id = workspace_ops::replace_or_open_tab(
    &mut self.workspace_root,
    pane_id,
    preview_target.as_deref(),
    tab,
    ReplaceFilter::SameKind,  // Only replace another diff preview
);
```

### 3. `apps/desktop/crates/console-ui/src/workspace/mod.rs`

- Export `ReplaceFilter` from the module

### 4. `apps/desktop/crates/console-ui/tests/workspace_test.rs`

- Update all `replace_or_open_tab` calls to pass `ReplaceFilter::Any` (preserving existing test behavior)
- Add new tests for `SameKind`, `FileOnly`, `DiffOnly` filters

---

## Test Plan

### Existing Tests
- All existing `replace_or_open_tab` calls get `ReplaceFilter::Any` to maintain current behavior

### New Tests

```rust
#[test]
fn same_kind_replaces_file_with_file() {
    // Open a file preview, then open another file → first is replaced
}

#[test]
fn same_kind_does_not_replace_diff_with_file() {
    // Open a diff preview, then open a file → diff stays, file opens as new tab
}

#[test]
fn same_kind_does_not_replace_file_with_diff() {
    // Open a file preview, then open a diff → file stays, diff opens as new tab
}

#[test]
fn file_only_only_replaces_file_tabs() {
    // With FileOnly filter, diff tab is not replaced
}

#[test]
fn diff_only_only_replaces_diff_tabs() {
    // With DiffOnly filter, file tab is not replaced
}
```

---

## Verification

```bash
cd apps/desktop && cargo test -p console-ui workspace_test
```

---

## Summary

| Aspect | Detail |
|--------|--------|
| Approach | Add `ReplaceFilter` enum to `replace_or_open_tab` |
| Files changed | 4 files (ops.rs, workspace_panes.rs, mod.rs, workspace_test.rs) |
| Risk | Low — all existing callers pass `Any` initially |
| Behavior change | File previews only replace file previews; diff previews only replace diff previews |
