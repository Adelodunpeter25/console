# Clickable file paths plan (desktop + mobile)

## Goal

Paths mentioned in chat (`apps/server/index.ts`, `src/foo.rs:12:3`) open the file on click/tap. Desktop opens a workspace tab; mobile opens the file preview.

## Answers from code review

- Only `[label](url)` + bare `http(s)://` are clickable today (`parser.rs linkify_bare_urls`, mobile `link` rule → `Linking.openURL`). File links would wrongly open in a browser.
- `LinkHandler` / `with_link_handler` (`markdown/render.rs`) exists but is never wired; everything falls through to `open_url` / `Linking`.
- Desktop has the resolution context: `sessions.rs` sets `transcript.set_session_cwd(header.cwd)`; `open_file_tab_in_pane` (`workspace_panes.rs:481`) targets the active pane with preview-tab reuse and fetches via `fs.read_file`.
- Mobile `FilesScreen` previews via local `selectedFile` state + `useReadFile` under the selected `projectRoot`; chat `MarkdownRenderer` instances get no cwd/project props.

## Desktop

1. Linkify file paths next to URL linkify in `crates/console-ui/src/markdown/parser.rs`: relative paths with file extensions, `file://` URLs, `path:line:col` suffixes. Keep code spans non-clickable for phase 1 (matches the existing `style.code` guard).
2. Wire one app-owned `LinkHandler` (same shape as `on_preview_image`): `TranscriptView::set_on_open_file` → assistant bubbles / thinking / tool-call markdown / subagent views. Route: `http(s)` → `cx.open_url` (unchanged); file-like → resolve + `open_file_tab`.
3. Resolve: absolute → as-is; relative → join session cwd, fallback to pane project path (same sources the inspector/quick-open path uses). Attempt open; read failure stays a warn, no new error UI.
4. Phase 1 opens the file only. `path:line:col` suffix is parsed and stripped, but jumping to the line waits (needs viewer scroll support; `WorkspaceTabConfig::File` carries no line).

## Mobile

1. Add optional `onOpenFile(path)` (+ cwd/project root) to `MarkdownRenderer`; route file-like hrefs there, keep `http(s)` on `Linking.openURL`.
2. Thread session cwd / selected project root from chat screens into the renderer call sites (`message-bubbles`, `run-activity`, `tool-result-content`).
3. Cross-tab open: lift `FilesScreen` selected-file state (or add a small store/event) so chat can set files tab + preview path, reusing the existing `useReadFile` preview and gating.

## Verification

- Desktop: bare path, markdown file link, `file://`, and `path:line` render as links; click opens the right file in the active pane; external URLs still open in browser; missing file degrades to current warn path.
- Mobile: tap on a file link switches to Files with the preview loaded; external links still use `Linking`.
- `cargo check` for desktop; existing mobile typecheck/lint.
