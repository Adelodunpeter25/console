# Command Palette, Remote File Browser & Fuzzy Search Specification

**Status**: Draft  
**Applies to**: Desktop (`apps/desktop`), Mobile (`apps/mobile`), Server API (`apps/server`)  
**Target Capabilities**: Remote File Navigation, Project Onboarding, Fuzzy File Search (`Cmd+P`), Project Directory Picker (`Cmd+O`)

---

## 1. Overview & Problem Statement

### 1.1 The Remote Environment Challenge (`Cmd+O`)
Currently, opening a project directory or browsing files relies on native OS dialogs (e.g. `gpui::open_directory`). When the Console frontend runs against a remote backend (e.g., dev container, cloud VM, remote headless server over SSH/WebSocket), local OS file pickers cannot access the remote host's filesystem.

### 1.2 Quick File Navigation (`Cmd+P`)
Developers expect a VS Code / Zed style quick-open fuzzy file palette (`Cmd+P`) scoped to the currently active chat's project root (`cwd`). Selecting any file should immediately open a new File Tab in the main workspace.

---

## 2. Interaction Modes & Keybindings

Both workflows share a single, unified **Command Palette Modal Component** (`crates/console-ui/src/common/command_palette.rs`), operating in two modes:

```
+-----------------------------------------------------------------------+
|  🔍 [ > Search files or paths...                                  ]   |
+-----------------------------------------------------------------------+
|  📁 src/components/                                                   |
|     📄 app.tsx                                  src/components/       |
|     📄 header.tsx                               src/components/       |
|     📄 sidebar.tsx                              src/components/       |
|  📁 tests/                                                            |
|     📄 app.test.tsx                             tests/                |
+-----------------------------------------------------------------------+
|  ↑↓ Navigate    ↵ Open Tab    Esc Close                               |
+-----------------------------------------------------------------------+
```

### 2.1 Mode A: Remote Directory Browser / Project Picker (`Cmd+O`)
- **Trigger**: `Cmd+O` / `Ctrl+O` or clicking "Add Project" / "Open Directory".
- **Functionality**:
  - Starts at current user's remote home directory (`~`) or `/`.
  - Lists folders and files returned by backend `GET /api/fs/browse?path=...`.
  - Allows drilling down folders with `Enter` or clicking.
  - Top action bar has a prominent **"Select Current Folder as Project"** button.
  - Adds the selected directory as a tracked project on the backend via `POST /api/projects`.

### 2.2 Mode B: Quick File Fuzzy Search (`Cmd+P`)
- **Trigger**: `Cmd+P` / `Ctrl+P`.
- **Context**: Automatically uses the `cwd` / `projectId` of the currently focused workspace pane / chat.
- **Functionality**:
  - Queries backend search endpoint `GET /api/fs/find?cwd=...&query=...` with debounced input.
  - Backend uses fast directory walker / ripgrep filtering, ignoring `node_modules`, `.git`, `dist`, `target`, etc.
  - Renders fuzzy-highlighted matches with file icons, relative path, and matched substring highlights.
  - Pressing `Enter` or clicking an item creates/focuses a new **File Viewer Tab** (`WorkspaceTabConfig::File { path }`) in the active pane.

---

## 3. Backend API Specification (`apps/server`)

### 3.1 `GET /api/fs/browse`
Navigates directories for remote project selection.
- **Query Params**:
  - `path` (string, optional): Directory to list. Defaults to remote user home `os.homedir()`.
  - `showHidden` (boolean, optional): Include dotfiles. Defaults to `false`.
  - `onlyDirs` (boolean, optional): Filter to only directories when picking project root. Defaults to `true` for project picking.
- **Response**:
  ```json
  {
    "success": true,
    "data": {
      "currentPath": "/home/user/Developer/project",
      "parentPath": "/home/user/Developer",
      "entries": [
        { "name": "apps", "path": "/home/user/Developer/project/apps", "isDir": true },
        { "name": "packages", "path": "/home/user/Developer/project/packages", "isDir": true },
        { "name": "package.json", "path": "/home/user/Developer/project/package.json", "isDir": false }
      ]
    }
  }
  ```

### 3.2 `GET /api/fs/find`
Fast fuzzy search within a project root.
- **Query Params**:
  - `cwd` (string, required): Root project path to search within.
  - `query` (string, optional): Search filter text. If empty, returns recent / top-level files.
  - `limit` (number, optional, default: 50): Max matches.
- **Response**:
  ```json
  {
    "success": true,
    "data": {
      "files": [
        {
          "name": "workspace_content.rs",
          "relativePath": "apps/desktop/src/view/workspace_content.rs",
          "absolutePath": "/Users/user/project/apps/desktop/src/view/workspace_content.rs",
          "isDir": false
        }
      ]
    }
  }
  ```

---

## 4. Desktop Client Architecture (`apps/desktop`)

### 4.1 Global Keybinding Hooks
In `apps/desktop/src/view/mod.rs`:
- Bind `Action::OpenProjectPalette` (`Cmd+O` / `Ctrl+O`) $\rightarrow$ Opens Palette in `DirectoryBrowse` mode.
- Bind `Action::QuickOpenFile` (`Cmd+P` / `Ctrl+P`) $\rightarrow$ Opens Palette in `FileSearch` mode with current pane's `cwd`.

### 4.2 File Tab Integration
When a file is selected in `QuickOpenFile` mode:
```rust
this.open_or_focus_file_tab_for_pane(&active_pane_id, absolute_path, cx);
```
- If a file tab with `absolute_path` already exists in the pane, activate that tab.
- Otherwise, insert a new `WorkspaceTabConfig::File { file_path }` and focus it.

---

## 5. Summary of Tasks for Implementation

- [ ] **Backend (`apps/server`)**:
  - Implement `GET /api/fs/browse` in `fs.service.ts` / `routes/fs.ts`.
  - Implement `GET /api/fs/find` with smart ignore patterns.
- [ ] **UI Component (`crates/console-ui`)**:
  - Create reusable `CommandPaletteModal` component with search input, list, keyboard selection (`Up`/`Down`/`Enter`/`Esc`).
- [ ] **Desktop Integration (`apps/desktop`)**:
  - Hook `Cmd+O` and `Cmd+P` global keybindings.
  - Connect file selection to `WorkspaceTabConfig::File`.
