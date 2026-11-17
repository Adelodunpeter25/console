# Sidebar sections — custom chat groupings

User-defined, server-synced sections that sit between Pinned and project grouping in the sidebar. Born from wanting a "stashed/Later" bucket without a new lifecycle state: sections are pure organization, no semantics.

---

## 0. Decisions (settled in discussion)

- **Sections are global and top-level** — one list across all chats/projects, not nested inside project groups.
- **One section per chat.** `section_id` on the session header; null = unsectioned.
- **Sections do not replace project grouping.** Sidebar order: Pinned → custom sections (manual order) → project groups containing only unsectioned chats → Settled (unchanged, collapsed at bottom).
- **Date grouping is removed entirely.** The Date/Project `SidebarSortMode` toggle disappears; project grouping is the only mode, chats inside each group sorted by recency. Custom sections are the escape hatch for status/cross-project organization.
- **No "stashed" state.** A section named "Later" *is* the stash. No badges, no reminders, no lifecycle coupling.
- **Drafts sidebar section is removed** on desktop and mobile. Draft state already lives per-chat; closed tabs with unsent text get a draft indicator on the session row instead — an edit/pencil icon, the way Conductor and Amp do it. Mobile already computes `isDraftSession`/`draftPreview`; it keeps the pill/indicator and drops the section.
- **v1 assignment is drag & drop**: drag a session row onto a section header. The context-menu "Move to section" submenu is deferred (current menu has no submenu support).
- **Pinned and section membership are mutually exclusive**, matching the existing pinned↔settled exclusivity: pinning a sectioned chat removes it from the section; moving a pinned chat into a section unpins it. Settled chats leave every section, same as they leave pinned/drafts today.
- **Deleting a section unsections its chats** (they fall back into project grouping). Empty sections still render — they are drop targets and keep their manual position.

---

## 1. What Already Exists (verified)

- **`SessionHeader`** (`proto/console/v1/session.proto`) carries `pinned_at`, `settled_at`, `unread`, etc. Sessions PATCH (`apps/server-go/internal/routes/sessions.go`) already accepts `pinned` and distinguishes omitted-vs-null via `bodyFieldsPresent` — the exact pattern `sectionId` needs.
- **Projects are the template for a new synced domain**: `proto/console/v1/project.proto` (`ProjectInfo` DTO, no service/rpc — this API is REST-shaped with protojson bodies), `internal/routes/projects.go`, `internal/types`, `internal/db/schema.go` additive column lists.
- **Sidebar grouping** lives in `apps/desktop/crates/console-ui/src/utils/session_groups.rs`: `group_indices_by_project` (kept), `group_indices_by_date` + `SidebarSortMode` + `SessionDateGroup` (removed). `partition_settled` unchanged.
- **Ordering/membership logic** is duplicated in `apps/desktop/src/state/sessions.rs::visible_session_ids` (drives rendering *and* cmd+1-9) and `console-ui/src/layout/sidebar/mod.rs` — both must learn sections.
- **Drag & drop already works**: session rows emit `WorkspaceDrag` (`session_item.rs`, payload = `WorkspaceTabConfig::Chat`), panes consume it via `.drag_over`/`.on_drop` (`workspace/pane.rs`). Section headers can be drop targets with the same machinery; dropping on a section assigns instead of opening a tab. `source_pane_id: None` already marks sidebar-originated drags.
- **Drafts**: `draft_summaries()` (`state/drafts.rs`) filters to confirmed drafts (`sidebar_draft_ids`) plus `"new_chat"`; the sidebar Drafts section renders them and *excludes* those sessions from other groups. Removing the section means removing that exclusion and adding the row indicator.
- **Mobile**: `feature/home/HomeScreen.kt` builds grouped sections with pseudo-keys `"pinned"`, `"settled"`, `"Drafts"`; `core/chat/DraftUtils.kt` has `isDraftSession`/`draftPreview`.

---

## 2. Server (server-go)

### 2.1 Proto — new `proto/console/v1/section.proto`

```proto
message SessionSection {
  string id = 1;
  string name = 2;
  double position = 3;        // fractional ordering key; see §2.3
  int64 created_at = 4;
  int64 updated_at = 5;
}
message CreateSessionSectionRequest { string name = 1; }
message UpdateSessionSectionRequest {
  optional string name = 1;
  optional double position = 2;
}
message ReorderSectionsRequest { repeated string ids = 1; }  // full order, top to bottom
message DeleteSessionSectionResponse { string id = 1; bool deleted = 2; }
```

Add to `SessionHeader`: `optional string section_id = 20;`

### 2.2 REST domain — mirror projects

- `GET /api/sections` → bare JSON array of `SessionSection` (protojson, same envelope conventions as projects).
- `POST /api/sections` { name } → created section, appended at end.
- `PATCH /api/sections/:id` { name?, position? } → refreshed section.
- `DELETE /api/sections/:id` → `{id, deleted}`; member sessions' `section_id` set to NULL in the same transaction.
- `POST /api/sections/reorder` { ids: [...] } → applies the full ordering atomically.
- `PATCH /api/sessions/:id` gains `sectionId` (string or explicit null; use `bodyFieldsPresent` like `projectId`). Setting it clears `pinned_at` (mutual exclusivity); the pin path clears `section_id`.

Files: `internal/routes/sections.go` (new), registration in `internal/serve/serve.go` next to projects, `internal/services/sections.go` (new), `internal/types/section.go` (new).

### 2.3 DB — `internal/db/schema.go`

- New table `session_sections(id TEXT PK, name TEXT NOT NULL, position REAL NOT NULL, created_at INTEGER, updated_at INTEGER)`.
- `sessions` gains `section_id TEXT` (nullable; match how `project_id` is handled — verify FK conventions in schema.go).
- Position: fractional `REAL` keys (new section = max+1; reorder rewrites keys evenly) avoid a reorder-on-every-insert and keep `ReorderSectionsRequest` simple. If the codebase prefers integers, use int positions + the reorder endpoint rewriting all rows — pick one and stay consistent.

### 2.4 Fixtures

`proto/testdata/section/*.json` for list/create/patch/delete/reorder, plus a session-header fixture carrying `sectionId`, following the existing fixture pattern.

---

## 3. Desktop (apps/desktop)

### 3.1 console-core

- `types`: `SessionSection` struct + `section_id: Option<String>` on the session header type.
- Client methods: `sections.list/create/update/delete/reorder`, session update accepting `section_id`.

### 3.2 State (`src/state/`)

- `sections: Vec<SessionSection>` loaded alongside sessions on connect/refresh; `collapsed_sections: HashSet<String>` persisted like `collapsed_projects`.
- `move_session_to_section(session_id, Option<section_id>)` → optimistic update + PATCH; clears `pinned_at` locally when assigning (server enforces too).
- `create_section(name)`, `rename_section`, `delete_section`, `reorder_sections(ordered_ids)` — all optimistic with rollback, same pattern as `set_settled`.
- `visible_session_ids`: pinned → for each section in manual order (skip collapsed) its member sessions by recency → project groups over *unsectioned* chats → settled tail in search mode. Search path includes sectioned chats.
- **Remove**: `SidebarSortMode`, `sidebar_sort_mode`, `toggle_sidebar_sort_mode`, date-group branches in `visible_session_ids`, `group_indices_by_date`/`SessionDateGroup` usage, the sort toggle UI, and the persisted `sidebar_sort_mode` layout field (keep parsing tolerant of the old value).

### 3.3 console-ui sidebar

- New `layout/sidebar/section_header.rs` modeled on `project_header.rs`: name, count, collapse chevron, `.drag_over::<WorkspaceDrag>` highlight + `.on_drop` → assign session to section.
- Section rows render between the Pinned group and project groups; member sessions reuse `session_item.rs` unchanged (they already emit `WorkspaceDrag`).
- **Section reorder**: drag a section header with a small `SectionDrag { id }` payload onto another section header → `reorder_sections`. (GPUI drag machinery already in use; no new dependency.)
- **Draft indicator**: session rows with a confirmed draft show a pencil/edit icon (Conductor/Amp style) — replaces the Drafts section. `"new_chat"` draft has no session to badge: render it as a single row at the top of the main list (above project groups), since it *is* the next-chat composer.
- Remove the Drafts section header/rows and the draft-session exclusion from group rendering; `draft_summaries` stays as the data source for badges.
- Context menu on a sectioned row gains **"Remove from section"**; "Move to section" submenu deferred.
- Empty sections render their header (drop target + stable position).

### 3.4 Edge cases

- Settled chat in a section: hidden with settled, returns to its section on unsettle (section_id untouched).
- Deleting the active session's section: chat stays open, just unsectioned in the sidebar.
- cmd+1-9 follows `visible_session_ids` — sectioned chats get slots in section order.
- Draft on a sectioned chat: badge renders on the row inside the section.

---

## 4. Android (apps/android)

- `ConsoleApi`/`SessionRepository`: sections endpoints + `sectionId` on session models.
- `HomeScreen.kt`: replace the `"Drafts"` pseudo-section — drop it, show the draft edit-icon on rows (`isDraftSession`/`draftPreview` already exist); add real section groups between pinned and projects, collapsible, ordered by `position`.
- Minimal section management: long-press section header → rename/delete; create via overflow menu. Drag & drop is desktop-first; mobile assignment can be a follow-up (context menu "Move to section" once submenu/simple-picker exists).
- Keep the existing draft pill behavior on the chat screen itself — only the *home/sidebar section* goes away.

---

## 5. Rollout order

1. **Proto + server**: section.proto, routes, service, schema migration, fixtures. Server ships first — old clients ignore `section_id`/`sections` entirely.
2. **Desktop**: core types → state → sidebar rendering → drag & drop → drafts-section removal → date-grouping removal. Largest chunk; can land behind the new server alone.
3. **Android**: parity pass.

Date-grouping removal is independent of sections and could ship separately if the diff gets big — but doing them together avoids touching `visible_session_ids` twice.

---

## 6. Open questions deferred

- Context-menu "Move to section" submenu (needs menu nesting support).
- Section icons/colors, section descriptions.
- Mobile drag & drop / section management depth.
- Whether sections should be per-device or account-global (spec assumes global like projects).
