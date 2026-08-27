# Non-Project (Scratchpad) Sessions Specification

## 1. Overview & Problem Statement

When a user initiates a chat and selects **"No project"** / **"Don't work in a project"**, the session should function as a standalone, general-purpose assistant session without being tied to any specific codebase or repository.

### Previous Flaws
1. **Process CWD Leak**: The desktop/client fell back to `std::env::current_dir()` (the process launch folder, often the `console` repo or user home directory).
2. **Accidental Project Re-Association**: The backend server called `getProjectByDir(cwd)` on session creation, automatically linking a project ID if the launch path matched a known project.
3. **Unintended File/Tool Mutations**: Agent bash commands and file operations were executed directly in the process launch folder, potentially modifying arbitrary local files.

---

## 2. Standard Specification

To match modern AI agent standards (e.g. Cursor, Windsurf, Claude Code), projectless sessions follow this contract:

| Property | Value | Purpose |
| :--- | :--- | :--- |
| **`projectId`** | `null` | Explicitly unattached to any project. Disables git branch tracking and project-level file search. |
| **`cwd`** | `~/.console/scratch` | Sandboxed working directory ensuring any temporary file creation or shell execution remains isolated. |
| **UI Grouping** | **"General"** | Displayed in sidebar session lists under a dedicated top-level "General" section rather than matching a folder path. |

---

## 3. Implementation Contract

### Client (Desktop / Mobile)
- When "No project" is selected in the project picker:
  - Send `projectId: null` (or omit `projectId`).
  - Do not pass the app process's `current_dir()`. Either pass the dedicated scratch directory or let the server assign the default scratch path.
- In sidebar history, group sessions with `projectId == null` under **"General"**.

### Server Backend (`apps/server`)
- When creating a session where `projectId` is not provided:
  - If `projectId` is explicitly omitted/null, do **not** run `getProjectByDir(cwd)` to re-link an existing project.
  - Set `cwd` to the user's isolated scratchpad directory (`path.join(os.homedir(), ".console", "scratch")`).
  - Ensure `~/.console/scratch` directory exists automatically upon session creation.
