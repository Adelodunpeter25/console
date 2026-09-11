# Plan: Customizable Right Sidebar Tabs & Dynamic Tab Management

## 1. Executive Summary & Problem

The Right Inspector Sidebar currently renders all tabs (`All files`, `Changes`, `Browser`, `Subagents`, and upcoming `Devices`) side-by-side in a single static segmented control. As new capabilities are added, this header becomes crowded, reducing readability and wasting horizontal width.

This document outlines the plan to convert the Right Sidebar tab bar into a **Dynamic Tab Strip**:
1. **Primary Fixed Tabs**: `All files` and `Changes` are always present as core workspace anchors.
2. **`+` Add Tab Dropdown**: A compact `+` button opens a popover menu allowing the user to mount auxiliary surfaces (`Browser`, `Subagents`, `Devices`).
3. **Closable Auxiliary Tabs**: Open auxiliary tabs render beside the primary tabs with an **`×` close button on hover** to dismiss them when not in use.

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ Right Inspector Tab Bar                                                                │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ ┌────────────────────────────────────────────────────────────────────────┐ ┌─────────┐ │
│ │ [ All files ] [ Changes (2) ] [ Browser × ]  [ + ▾ ]                   │ │ [ ↻ ]   │ │
│ └────────────────────────────────────────────────────────────────────────┘ └─────────┘ │
│                                                  │                                     │
│                                                  ▼ (Dropdown on click)                 │
│                                           ┌─────────────────────────────┐              │
│                                           │ 🌐  Web Browser             │              │
│                                           │ 🤖  Subagents               │              │
│                                           │ 📱  Device Simulator        │              │
│                                           └─────────────────────────────┘              │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. State & Data Model

### A. Active Tab & Open Auxiliary Tabs
In `apps/desktop/src/state/app.rs`:

```rust
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum PrimaryTab {
    AllFiles,
    Changes,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum AuxiliaryTab {
    Browser,
    Subagents,
    Devices,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum InspectorTab {
    Primary(PrimaryTab),
    Auxiliary(AuxiliaryTab),
}
```

In `ConsoleDesktopApp`:
- `pub inspector_active_tab: InspectorTab` (currently selected tab).
- `pub inspector_open_auxiliary_tabs: Vec<AuxiliaryTab>` (set of auxiliary tabs currently mounted in the tab bar).

### B. Persistence (`apps/desktop/src/persistence/layout.rs`)
The list of `open_auxiliary_tabs` is saved to `layout.json` so the user's custom inspector layout restores seamlessly across app restarts.

---

## 3. UI Component Specification (`crates/console-ui/src/inspector/`)

### Tab Bar Structure

```rust
pub struct RightSidebar {
    width: f32,
    active_tab: InspectorTab,
    open_auxiliary_tabs: Vec<AuxiliaryTab>,
    available_auxiliary_tabs: Vec<AuxiliaryTab>, // Tabs not yet open
    ...
    on_select_tab: Rc<dyn Fn(InspectorTab, &mut Window, &mut App)>,
    on_open_auxiliary_tab: Rc<dyn Fn(AuxiliaryTab, &mut Window, &mut App)>,
    on_close_auxiliary_tab: Rc<dyn Fn(AuxiliaryTab, &mut Window, &mut App)>,
}
```

### Interactions & Lifecycle

1. **Clicking a Tab**:
   - Switches `inspector_active_tab` to that tab and renders its view.
2. **Hovering an Auxiliary Tab**:
   - Reveals an `×` close icon on the right side of the tab chip.
3. **Clicking `×` (Close Tab)**:
   - Removes the auxiliary tab from `open_auxiliary_tabs`.
   - If the closed tab was currently active, automatically selects `InspectorTab::Primary(PrimaryTab::AllFiles)`.
   - The closed tab immediately becomes available again in the `+` dropdown menu.
4. **Clicking `+` (Add Tab)**:
   - Opens a menu listing all unmounted auxiliary surfaces (e.g. `🌐 Web Browser`, `🤖 Subagents`, `📱 Devices`).
   - Selecting an item appends it to `open_auxiliary_tabs` and immediately activates it.

---

## 4. Implementation Steps

1. **Refactor `InspectorTab` Enum (`crates/console-ui/src/inspector/right_sidebar.rs`)**:
   - Support `Primary(AllFiles | Changes)` and `Auxiliary(Browser | Subagents | Devices)`.
2. **Add Closeable Tab Chip Component**:
   - Tab chip renders label, badge count (if any), and an `×` button visible on group hover.
   - Stop click propagation on the `×` button to prevent triggering tab selection before close.
3. **Add `+` Dropdown Menu**:
   - Render dropdown popover next to the last open tab using `console_ui::dropdown_menu`.
4. **Persist Auxiliary Tabs in App State**:
   - Store `open_auxiliary_tabs` in `ConsoleDesktopApp` and serialize into layout persistence.
