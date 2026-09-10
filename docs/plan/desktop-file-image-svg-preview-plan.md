# Desktop File Viewer: Image + SVG Preview Plan

## Goal

File tabs opened via Quick Open (`Cmd+P`), transcript file links, and the inspector open raster images (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.bmp`, `.ico`) as a centered image preview and SVGs (`.svg`) as a rendered preview with a `Preview | Source` toggle, instead of the current behavior (infinite `"Loading file content..."` for rasters, raw XML source for SVG).

## Current State (verified in code)

- File-tab render has exactly two branches (`apps/desktop/src/view/workspace_content.rs:73-132`): markdown → `MarkdownViewer`, everything else → `FileViewer` (pure `CodeViewer` text).
- Text content flows through `open_file_contents: HashMap<String, String>` (`apps/desktop/src/state/app.rs:210`), fed by `client.fs.read_file()` (`apps/desktop/crates/console-core/src/services/fs.rs:134`) → `GET /api/fs/file` (`apps/server/api/src/routes/fs.ts:112`) → `FsService.readFileContent()` (`apps/server/api/src/services/fs.service.ts:153`).
- The server text gate **intentionally blocks images**: `BINARY_FILE_EXTENSIONS` in `packages/types/src/fs.ts:38-52` includes `.png/.jpg/.jpeg/.gif/.webp/.bmp/.ico/...`, and `readFileContent` rejects them with `FilePreviewBlockedError("BINARY_FILE")` plus a NUL-byte content sniff and a 512 KB cap (`MAX_FILE_PREVIEW_BYTES`). Desktop logs `Failed to read file for tab ...` (`workspace_panes.rs:697`) and the UI sits on `"Loading..."` forever — there is no error panel.
- `.svg` is **not** in the blocklist (it is XML text), so it opens as source code today. Keep that as the `Source` fallback.
- Reusable precedent: chat already decodes + renders raster images correctly — `attachment_image()` (`crates/console-ui/src/common/attachment.rs:15`), `decode_data_url()` (`crates/console-ui/src/markdown/render.rs:1381`), `message_bubble.rs:364-393` (`ImageFormat::from_mime_type` + `Image::from_bytes` + `img(...).object_fit(ScaleDown)`), and `ImageViewerModal` (`crates/console-ui/src/common/image_viewer.rs`) does the centered max-size + `ScaleDown` layout.
- `gpui::ImageFormat` covers raster formats only (no SVG). SVG must be rasterized on the desktop before handing bytes to `gpui::Image`. `resvg/usvg` already exist transitively in `Cargo.lock` (via gpui); they need a direct, version-matched dependency.
- Fetch paths that duplicate the read must both be updated: `open_file_tab_in_pane` (`workspace_panes.rs:640-704`) and the lazy fetch in `workspace_content.rs:75-91`.

## Non-Goals (v1)

- No zoom/pan, actual-size toggle, slideshow, or EXIF panel. Fit-to-view (`ScaleDown`) only; click-to-zoom reuses the existing modal if trivial, otherwise deferred.
- No editing, no drag-to-save, no mobile parity (mobile keeps its gating).
- No video/audio/pdf/archive preview — those stay blocked with a proper blocked panel instead of infinite loading.
- No changes to Quick Open search, file-tree icons, or the text/diff viewers.

## Design Overview

```
File tab path
  └─ classify extension → Markdown | Svg | RasterImage | Text | Blocked
       ├─ Markdown/Text → existing viewers (unchanged)
       ├─ RasterImage  → GET /api/fs/file/raw → bytes → gpui::Image → ImagePreview
       ├─ Svg          → GET /api/fs/file/raw (or text path, capped) → resvg rasterize
       │                 → ImagePreview + Preview|Source toggle (source = existing FileViewer)
       └─ Blocked/other binary → blocked panel (title + message, no spinner)
```

Key invariants:

1. Image bytes never flow through `open_file_contents: HashMap<String, String>` (NUL-sniff + UTF-8 would reject/corrupt them). They get a parallel binary cache.
2. The text gate stays the enforcement layer for text; images get their own size cap (~10 MB, not the 512 KB text cap).
3. SVG is rasterized, never rendered as raw markup in a webview — `<script>`/event handlers stay inert.
4. One classification helper is shared by both fetch sites and the render branch so all three always agree.

## Backend (server) Changes

1. **`packages/types/src/fs.ts`**
   - Add `IMAGE_PREVIEW_EXTENSIONS = { ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico" }` and `SVG_EXTENSION = ".svg"`.
   - Add `IMAGE_MAX_BYTES = 10 * 1024 * 1024` (tune: 10 MB covers photos/screenshots; larger stays blocked).
   - Add `imageMimeForExtension(ext): string | null` mapping (`png→image/png`, `jpg/jpeg→image/jpeg`, `gif→image/gif`, `webp→image/webp`, `bmp→image/bmp`, `ico→image/x-icon`, `svg→image/svg+xml`).
   - Add helpers `isPreviewableImageName(fileName)` and `isSvgFileName(fileName)`. Do **not** remove image extensions from `BINARY_FILE_EXTENSIONS` — that set means "not text"; the new helpers mean "renderable as image".
2. **`apps/server/api/src/services/fs.service.ts`**
   - Add `readFileBytes(filePath): Promise<{ bytes: Buffer; mimeType: string; sizeBytes: number }>`:
     - `stat`; reject non-files; reject `sizeBytes > IMAGE_MAX_BYTES` with `FilePreviewBlockedError("FILE_TOO_LARGE", ..., { status: 413 })`.
     - Allow only image/svg extensions (via the new helpers); lockfiles still blocked.
     - No NUL-sniff (meaningless for binary); read with `fs.readFile`.
   - Leave `readFileContent()` untouched.
3. **`apps/server/api/src/routes/fs.ts`**
   - Add `GET /api/fs/file/raw?path=<abs>`:
     - Calls `readFileBytes`, sets `Content-Type` from mime map, `Content-Length`, `Cache-Control: private, max-age=30` (file may change on disk; fs-watch invalidation is future work).
     - Error mapping mirrors the existing `/file` handler: `FilePreviewBlockedError` → `{ success: false, error, code, ...detail }` with `err.status`; other errors → 400.
4. **Server tests** (`apps/server/tests/`): new `fs-file-raw.test.ts` — raster allowed with correct content-type; `.svg` allowed; `.zip`/lockfile still blocked; oversized file → 413 with `sizeBytes/maxBytes`; missing path → 400. Follow the style of `tests/api.test.ts:59-71`.

## Desktop `console-core` Changes

1. **`crates/console-core/src/services/fs.rs`**
   - Add `read_file_bytes(&self, path: &str) -> Result<(Vec<u8>, String)>`: GETs `/api/fs/file/raw`, returns `(bytes, content-type)`. Surfaces the JSON error body (`code`, `detail`) as `anyhow` errors so the UI can render blocked vs failed distinctly.
2. **New classification helper** (put in `console-core` so app + ui share it, e.g. `crates/console-core/src/utils/file_kind.rs`):
   ```rust
   pub enum FileKind { Markdown, Svg, RasterImage, Text, Blocked }
   pub fn file_kind_for_path(path: &str) -> FileKind
   ```
   - Markdown: `.md/.markdown/.mdx` (must match `workspace_content.rs:99-102` exactly).
   - Svg: `.svg`. RasterImage: `.png/.jpg/.jpeg/.gif/.webp/.bmp/.ico` (mirror the TS set; add a unit test that asserts parity).
   - Everything else → Text (server still has final say; client pre-check is only a fast path).

## Desktop `console-ui` Viewer Changes

1. **New `crates/console-ui/src/viewer/image_preview.rs`** — `ImagePreview` (`IntoElement`, same builder style as `FileViewer`):
   - Props: `path: String`, `image: Arc<gpui::Image>`, `meta: ImageMeta { w, h, size_bytes, mime }`, optional `view_mode` + `on_view_mode` (SVG toggle only).
   - Layout: header strip (basename, `WxH · 123 KB · PNG`) + centered stage with checkerboard/dark backdrop (`theme.surface`, border, rounded, `ScaleDown`, `max_w/max_h` like `ImageViewerModal:68-86`).
   - For SVG: `Preview | Source` segmented toggle in the header (VS Code-style). `Source` renders nothing itself — the parent swaps in the existing `FileViewer`.
   - Empty/error sub-states live in the parent (below), not in this component.
2. **`crates/console-ui/src/viewer/mod.rs`**: add `pub mod image_preview; pub use image_preview::{ImagePreview, ImageMeta};`
3. **SVG rasterization** (same file or `crates/console-ui/src/viewer/svg.rs`):
   - Add direct deps in `crates/console-ui/Cargo.toml`: `resvg` + `usvg` pinned to the exact versions already in the workspace `Cargo.lock` (check first; do not let cargo unify a second copy), plus whatever encoder the chosen path needs (`png` is already in the tree).
   - `rasterize_svg(bytes: &[u8], max_dim: u32 /* e.g. 2048 */) -> Option<(Vec<u8> /*png*/, u32, u32)>`:
     - Parse with `usvg::Tree::from_data` (no network, no scripts executed).
     - Scale to fit `max_dim` preserving aspect ratio, ~2x for retina crispness.
     - `resvg::render` into a `tiny-skia::Pixmap`, encode to PNG bytes for `Image::from_bytes`.
     - Cap output dimensions; return `None` on parse/render failure → parent falls back to `Source` view with a notice.
   - Animated SVG: render first frame only (document it).

## App Wiring (`apps/desktop/src`) Changes

1. **`state/app.rs`**
   - Add parallel cache (text cache untouched):
     ```rust
     pub enum ImageFileState { Loading, Loaded { image: Arc<gpui::Image>, w: u32, h: u32, size_bytes: u64, mime: String }, Failed { message: String }, Blocked { title: String, message: String } }
     pub open_image_contents: HashMap<String, ImageFileState>
     ```
   - Init in both constructors (`app.rs:794` and the `908` site), extend `trim_file_caches()` (`workspace_panes.rs:951-960`) and the tab-close cleanup (`workspace_panes.rs:1007`) to evict image entries with the same `MAX_CACHED_FILES` bound.
   - SVG source text reuses `open_file_contents` (it is text); the rasterized bitmap lives in `open_image_contents`. Add `svg_preview_mode: HashMap<String, SvgViewMode>` (`Preview` default) or store the toggle locally in the view — prefer app state so it survives re-renders.
2. **`state/workspace_panes.rs`**
   - `open_file_tab_in_pane`: classify first. Text/markdown → existing `read_file` fetch. Image/SVG → insert `ImageFileState::Loading`, spawn `read_file_bytes`, decode:
     - Raster: `ImageFormat::from_mime_type(&mime)` → `Arc::new(Image::from_bytes(format, bytes))`; record dimensions via the `image`/`png` decoder or `gpui::Image` size accessor (verify which exists; fallback to omitting `WxH` rather than blocking).
     - SVG: try `rasterize_svg`; success → `Loaded`, failure → `Blocked`-style notice + default to source view.
     - Server `code: BINARY_FILE/FILE_TOO_LARGE/LOCKFILE_BLOCKED` → `Blocked { title, message }` (reuse `FilePreviewBlock` copy); transport errors → `Failed`.
3. **`view/workspace_content.rs:73-132`**
   - Replace the two-branch `is_markdown` check with `file_kind_for_path`:
     - `RasterImage/Svg(Preview)` → ensure `ImageFileState`; on `Loading` show the existing loading row; on `Loaded` render `ImagePreview`; on `Failed/Blocked` render a blocked panel (icon + title + message, matching `error_banner`/`notice_banner` styling — never an infinite spinner).
     - `Svg(Source)` and `Text/Markdown` → existing viewers unchanged.
   - Mirror the lazy-fetch fix in the `!contains_key` branch (`workspace_content.rs:75-91`) so a tab restored without a prefetch (e.g. window rehydrate) still loads images.

## Blocked / Loading / Error States

- Images: Loading (spinner row, current style) → Loaded | Blocked (server `code` → friendly title/message from `FilePreviewBlock`) | Failed (transport/decode error + `Retry` button that re-dispatches the fetch).
- This also fixes the adjacent bug where blocked rasters spin forever; as a side effect, non-image binaries (`.zip`, `.pdf`) should get the same blocked panel instead of `"Loading..."` (small extra branch, same component style).

## Caching / Memory

- Same `MAX_CACHED_FILES` bound and open-tab retention predicate as text; evict image entries for paths not open anywhere.
- Decode once per path; `Arc<gpui::Image>` is shared by reference across re-renders.
- `IMAGE_MAX_BYTES` (10 MB) fetch cap + `max_dim` raster cap bound worst-case memory; a 50 MP photo must be rejected or downscaled, never decoded at full size into the window.

## Security

- Serve raw bytes with correct `Content-Type`; desktop never interprets them as HTML/JS.
- SVG: rasterize only. No webview, no external resource loading (`usvg` options must disable network), scripts inert by construction.
- Path handling reuses the existing `resolve_file_link` + server validation; no new path surface.

## Tests

- Rust: `file_kind_for_path` unit tests (all image/svg/markdown/text/blocked extensions + case-insensitivity + no-extension files); `rasterize_svg` test on a tiny fixture SVG (valid → dimensions; malformed → `None`); assert the Rust extension set matches the TS set (a test that hardcodes both lists so drift fails loudly).
- Server: `fs-file-raw.test.ts` per above (allowed/blocked/oversized/missing/content-type).
- No inline `#[cfg(test)]` modules in source files per repo rules — dedicated `tests/` files only.

## Implementation Order

1. Shared types: extension sets + mime map + caps (`packages/types/src/fs.ts`).
2. Server: `readFileBytes` + `GET /api/fs/file/raw` + tests.
3. Desktop core: `read_file_bytes` + `file_kind_for_path` + tests.
4. UI: `ImagePreview` component + blocked/loading panel (raster end-to-end first, SVG still source-only at this point — shippable slice).
5. SVG: `resvg` dep + rasterize + `Preview|Source` toggle.
6. App wiring: caches, both fetch sites, eviction, retry; replace infinite-loading for blocked binaries.
7. Polish: header meta line, checkerboard, retina scale, oversized-image message.

## Verification

- `Cmd+P` → open `.png/.jpg/.gif/.webp` → centered preview with correct meta; `.svg` → rendered preview, toggle shows source; `.zip`/lockfile/oversized → blocked panel (no spinner); missing file → failed + retry.
- Transcript file link → image opens in workspace tab; external URLs unaffected.
- `cargo check` / `cargo test` in `apps/desktop`; `bun tests/fs-file-raw.test.ts` (single-file run) in `apps/server`; no full-suite runs per repo rules.
- Memory: open 10+ large images, close tabs, confirm cache trims (log or debug counter).

## Open Questions

- Exact `IMAGE_MAX_BYTES`: 10 MB default — confirm against largest expected screenshots/photos.
- Zoom/pan in v2? (Deferred; modal reuse only if free.)
- Should `fs.watch` invalidate the image cache on file change? Proposed: no for v1 (30 s `Cache-Control` + reopen refreshes).
