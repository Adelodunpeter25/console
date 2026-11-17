# Expanded file previews + inline images across desktop and mobile

## Context

The current file preview experience dead-ends on anything that isn't text or a raster image.
Opening a video like `ampcode-animation.mov` shows "Binary file — isn't a text file, so there's
nothing to preview here." on both desktop and Android. Separately, the existing todo asks for
inline images in the chat transcript and on tool call results on both platforms.

### What exists today

- **Desktop file tabs**: raster images and SVG render via `GET /api/fs/file/raw` → `ImagePreview`;
  everything in `BLOCKED_FILE_EXTENSIONS` (video, audio, PDF, archives, fonts, binaries) shows a
  `BlockedFilePanel`. Classification lives in `apps/desktop/crates/console-shared/src/file_kind.rs`.
- **Mobile file tabs**: mirrored classification in
  `apps/android/.../data/model/FsModels.kt` (`FilePreviewKind.IMAGE / BLOCKED / TEXT`), rendered in
  `feature/files/FilesScreen.kt`. Blocked files show the same dead-end message.
- **Desktop markdown**: `console-ui/src/markdown/render.rs` → `render_image` already handles data
  URLs and remote URLs, but has no workspace-path resolution.
- **Mobile markdown**: `CustomMarkdown.kt` renders `MdBlock.Image` via Coil `AsyncImage(model = url)`,
  again without auth or workspace-path resolution.
- **Tool result images**: `read_file` sniffs magic bytes and emits inline
  `{"type":"image","data":<base64>,"mimeType":...}` parts only when `CanSeeImages` is true.
  `toolResultsToProto` calls `StripImagePayloads` before persisting, so clients never see image
  bytes on tool results. The `VisualArtifact` channel (`VisualPart` field 5 in
  `proto/console/v1/message.proto`) is currently used only by the `visualize` tool.

## Decisions

1. **Rich file preview scope**: video, audio, and PDF render inline; all other opaque formats
   (archives, fonts, executables, databases, etc.) get a useful metadata/details card instead of
   the dead-end panel.
2. **Image delivery to chat/tool results**: server-authored reference in the existing
   `VisualArtifact` channel — a new v1 `image` block carrying a workspace path. No base64 in
   persisted transcripts; clients resolve the path through the authenticated `/api/fs/file/raw`
   endpoint.
3. **Docs**: no roadmap or docs/plan updates beyond this plan file; code + tests only.

## Workstream 1 — Unified rich-file classification (server + clients)

### Server (Go)

- `apps/server-go/internal/services/fs/preview.go`
  - Split the flat `binaryExtensions` map into typed sets: `videoExtensions`, `audioExtensions`,
    `pdfExtensions`, and an `opaqueExtensions` set for the details-card case.
  - Add a `PreviewKind` enum/detector: `image | video | audio | pdf | text | details`.
  - Stop treating video/audio/PDF as `BINARY_FILE` in `checkPreviewGate`; keep lockfiles gated.
  - Add `GetMediaMeta(path)` returning `{ kind, mimeType, sizeBytes, mtimeMs, width?, height?, duration? }`.
    Reuse `imageutil` for image dimensions; use `ffprobe` for video duration/size when available,
    omit otherwise (no hard dependency).
- `apps/server-go/internal/routes/fs.go`
  - Generalize `GET /api/fs/file/raw` (or add `/api/fs/file/media`) to serve video/audio/PDF bytes
    with correct `Content-Type`, `Content-Length`, `Accept-Ranges`, ETag/304, and range-request
    support (required for video scrubbing). Images/SVG keep current behavior.

### Shared classification (keep in sync across three places)

- `apps/desktop/crates/console-shared/src/file_kind.rs`: extend `FileKind` with `Video`, `Audio`,
  `Pdf`, `Details`; update `file_kind_for_path` and the extension tables.
- `apps/android/.../data/model/FsModels.kt`: extend `FilePreviewKind` with `VIDEO`, `AUDIO`,
  `PDF`, `DETAILS`; replace `getFilePreviewBlock` with a `getFileDetails` builder that produces a
  metadata block (title, MIME label, size, dimensions/duration when known).
- `apps/server-go/internal/services/fs/preview.go`: mirror the same lists on the server.

## Workstream 2 — Desktop file tab rendering

- `apps/desktop/src/state/workspace_tabs/file_and_diff.rs`
  - Extend `fetch_file_tab_content` to fetch bytes/meta for the new kinds (reuse/generalize
    `read_file_bytes` for ranged media).
- `apps/desktop/src/view/workspace_content.rs` + new components in `console-ui`
  - **Video**: native player surface. Reuse the WKWebView-based host pattern from
    `console-ui/src/browser/host.rs` and `console-ui/src/devices/view.rs` with an HTML
    `<video controls src="/api/fs/file/media?path=...">` document; inject the session token the same
    way the browser host injects cookies. Fallback: OS "open externally" action if embedding fails.
  - **Audio**: WKWebView `<audio controls>` player or a compact GPUI controls card streaming from the
    same endpoint.
  - **PDF**: WKWebView displaying the PDF response directly (native PDF rendering).
  - **Details card**: replace `BlockedFilePanel` with a card showing icon, filename, MIME label,
    formatted size, and "Open externally" (`open -R` / reveal in Finder).
- Tests: extend `apps/desktop/crates/console-shared/tests/file_kind_test.rs` for every new extension.

## Workstream 3 — Mobile file preview

- `apps/android/.../feature/files/FilesScreen.kt`
  - Add preview branches for `VIDEO`, `AUDIO`, `PDF`, `DETAILS`.
  - **Video**: `AndroidView` with Media3/ExoPlayer if already a dependency; otherwise
    `android.widget.VideoView` against the authenticated media URL. Check deps first.
  - **Audio**: Media3/ExoPlayer or `MediaPlayer` controller card.
  - **PDF**: AndroidView `PdfRenderer` streaming from bytes, with "Open externally" fallback via
    `Intent.ACTION_VIEW`.
  - **Details card**: Compose card with the same metadata fields as desktop.
- `apps/android/.../data/api/OkHttpConsoleApi.kt`: authenticated media fetch / URL builder with
  headers for the player.
- Tests: JVM fixture tests under `apps/android/tests/` for classification and details formatting
  (follow the `VisualFixtureTest` pattern). Run `:app:assembleDebug` to verify compilation.

## Workstream 4 — Images in chat transcript + tool results (both platforms)

### Server

- Extend the v1 visual spec (`apps/server-go/internal/services/visualize/`) with
  `ImageBlock { type: "image", path: string, alt?: string }` carrying a workspace path, not bytes.
- In `apps/server-go/internal/agent/tools/file_tools.go`: when `read_file` detects an image
  (magic-byte sniff), attach a `VisualArtifact` with an image block pointing at the absolute path,
  in addition to the provider-facing inline image part. This survives `StripImagePayloads`.
- `toolResultsToProto` already persists `Visual`; no proto change needed (reuse `VisualPart` field 5).
- Resolve workspace paths (including `file://` URLs and relative paths against the session cwd) to
  absolute paths when building the image block.

### Desktop

- `console-core/src/types/visual.rs`: add `VisualBlock::Image { path, alt }`; update
  `console-core/tests/types/visual_test.rs`.
- `console-ui/src/chat/visual/mod.rs` + `VisualCard`: render image blocks by fetching bytes via the
  fs service (or an authenticated URL for GPUI `img`) with bounded height, rounded corners, caption,
  and click-to-zoom matching the file-tab image viewer.
- `console-ui/src/markdown/render.rs` `render_image`: detect local/workspace paths in `![alt](url)`
  and route through the image resolver instead of a raw GPUI URL load.

### Mobile

- `feature/chat/visual/VisualSpec.kt`: parse the image block; add fixtures under
  `proto/testdata/visual/` and extend `apps/android/tests/VisualFixtureTest.kt`.
- `feature/chat/visual/VisualResultCard.kt`: render image blocks with Coil `AsyncImage` using an
  authenticated `ImageRequest` to `/api/fs/file/raw`.
- `feature/chat/markdown/CustomMarkdown.kt`: same path resolution for markdown images in assistant
  messages (reuse the authenticated image loader).
- `ToolCallRow.kt` / `RunActivity.kt`: verify tool results with a `VisualArtifact` go through
  `VisualResultCard`; patch if they only render text today.

## Verification (targeted, per AGENTS.md)

- `cd apps/server-go && go test ./internal/services/fs/... ./internal/agent/loop/... -run '<new tests>' -v`
- `cd apps/desktop && cargo test -p console-shared file_kind_test && cargo test -p console-core --test visual_test`
- `cd apps/android && ./gradlew :app:testDebugUnitTest --tests "com.console.mobile.VisualFixtureTest"`
- `cd apps/android && ./gradlew :app:assembleDebug`
- Manual: open a `.mov` file on desktop and mobile; check video playback, PDF, audio, and a zip details
  card; ask the agent to read an image file and confirm the image appears in the tool result on both
  platforms; check a markdown `![]()` with a workspace path.

## Risks / notes

- Desktop video embedding is the riskiest piece: WKWebView needs the session token injected for the
  media URL (reuse the browser host's cookie injection). Fallback is "open externally", which still
  beats the current dead end.
- Keep all image/media bytes behind authenticated endpoints; never persist raw base64 in transcripts.
- Confirm Media3/PDF renderer dependency availability on Android before choosing the player.
- Keep the three extension lists (Go, Rust, Kotlin) in sync; a shared generated list is out of scope.
