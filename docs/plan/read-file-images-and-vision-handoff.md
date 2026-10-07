# Read Tool Images + Vision Handoff — `read_file` returns images, the vision role describes them when the run model is blind

`read_file` today is text-only: it stats, reads up to 512 KB, splits on `\n`, and returns line-numbered prose. Point it at a PNG and the model gets a wall of binary mojibake. This plan makes it return real image parts, and — the interesting part — when the active run model cannot see images, it spends one call on the configured `vision` role model and returns *that model's description* as the tool result, so the run continues on its original model instead of going blind.

Target: `internal/agent/tools/file_tools.go`, new `internal/agent/tools/image_view.go`, new `internal/agent/vision/vision.go`, `internal/agent/compaction/tokens.go`, `internal/agent/loop/breakdown.go`, `internal/run/models.go`, `internal/run/turns.go`, `go.mod`.

---

## 0. Non-Goals

- **No PDF support.** OpenCode's legacy path sniffs `%PDF` and returns it as an attachment; its V2 core path rejects PDFs as binary. We match V2 core — PDFs stay a read error. Revisit separately.
- **No WebP decode/resize.** Go's stdlib cannot decode WebP. WebP passes through untouched when it fits the byte budget, and fails with a clear message when it does not. Adding a WebP decoder is a later, opt-in change.
- **No SVG images.** SVG is XML text; the existing line reader already handles it correctly. It is deliberately not in the sniff table.
- **No model switch mid-run.** See §2 — swapping models inside the loop is off the table for good reasons.
- **No new settings field in the first cut.** Vision describe turns on implicitly when a `vision` role is configured and the run model is image-blind. A kill switch is Phase 5.
- **No client work.** Tool-result images are already stripped before broadcast (`run/turns.go:473`), so desktop and Android need no changes.

---

## 1. Design Principles

1. **The transport already exists — use it, don't build a second one.** Tool results in this harness already carry `{"type":"image","data":<base64>,"mimeType":<mime>}` parts. The browser tool (`agent/tools/browser_tool.go:117`), the MCP adapter (`services/mcp/adapter.go:178`), and the CUA driver (`services/cua/manager.go:274`) all produce them. Claude renders them as native image blocks (`providers/claude/convert.go:47`); every other provider replaces them with `[image omitted: provider does not support images in tool results]` (`providers/shared/json.go:56`). `read_file` becomes just another producer of a shape we own.

2. **Detect by magic bytes, never by extension.** A `.png` that is actually a text file, or a binary blob named `.txt`, must not be routed by filename. Sniff the first bytes; extension is at most a tiebreaker we do not need.

3. **Images never pass through the line reader.** Sniff on a small prefix, branch immediately. This is the single most important structural point — it is why OpenCode's read tool is fast on images and ours would be pathological.

4. **The run model never changes.** The loop is `for { turn(); execute tools }` with the provider fixed for the whole `run()` (`agent/loop/loop.go:231`). Vision help arrives as a *description*, not as a model swap.

5. **Fail loud, degrade once.** No binary support and no vision role → say so in the tool result with the reason and the remedy. Never silently return `[image omitted]` with no explanation.

6. **One extra model call is a real cost.** Vision describe is off the critical path only in the sense that it is bounded: one request, no tools, non-recursive. It must be guarded so it cannot loop.

---

## 2. The Two Constraints That Shape Everything

### 2.1 The transport is done; only the producer is missing

```
read_file.Execute
  └─ returns []map[string]any{{"type":"image","data":..., "mimeType":...}}
       └─ loop.ToolResultMessage  (agent/loop/loop.go:270)
            ├─ claude     → toolResultContent → Anthropic image block   ✅
            ├─ codex      → shared.ToolResultText → RedactImages        ❌ placeholder
            ├─ opencode   → shared.ToolResultText → RedactImages        ❌ placeholder
            └─ antigravity→ shared.ToolResultText → RedactImages        ❌ placeholder
```

Three of four providers drop it today. That is the problem §5 Phase 3 solves.

### 2.2 The vision fallback cannot work mid-loop

The existing fallback (`run/turns.go:272-283`) is gated on `len(dto.Attachments) > 0` and runs **once, before the loop**:

```go
if len(dto.Attachments) > 0 && !model.SupportsImages {
    if vision, ok := s.resolveVision(model); ok {
        model, modelID, providerID = vision, vision.ID, vision.Provider
        // ...
    }
}
```

`dto.Attachments` are *user-supplied* attachments. A `read_file` image arrives several turns later, produced by the tool executor, long after `model` and `provider` were frozen for the run. So:

- **Swapping in-loop is not an option.** The assistant turn that emitted the `tool_use` came from model A; the follow-up turn would come from model B. Anthropic requires every `tool_use` to have a matching `tool_result` in the same conversation, and reasoning/thinking signatures are provider- and position-bound. Mid-loop switching produces malformed requests in a way that is very hard to debug.
- **So we do not switch.** We *call* the vision model as a side-channel and hand its prose back as the tool result. The run continues on model A, the tool-result shape stays text, and **it works on all four providers** with zero provider work.

This is the same shape Claude Code uses, and it composes with what is already here: `titles.Generate` (`agent/titles/titles.go:63`) is a working one-shot call — build a `loop.TurnRequest`, drain `EventText`. `loop.UserMessage` already carries `Attachments []ImageAttachment`, and all four providers already render it. The describer is that function with an attachment attached.

### 2.3 Behavior matrix

| Run model sees images? | `vision` role resolves to an image-capable model? | `read_file` on a PNG returns |
|---|---|---|
| yes | — | image part + short text line. Zero extra calls. |
| no | yes | text description from the vision model, labelled as such |
| no | no / unset | text placeholder naming the path, mime, dimensions, and the fix |

---

## 3. Reading OpenCode's Implementation

The reference checkout is not on this machine; `opencode` is a compiled Bun binary. Clone it shallow:

```sh
git clone --depth 1 https://github.com/sst/opencode.git /tmp/opencode-src
```

There are **two** read tools in that repo and only the second is current:

| Path | Status |
|---|---|
| `packages/opencode/src/tool/read.ts` | legacy V1 — still bundled, not the live path |
| `packages/core/src/tool/read.ts` + `read-filesystem.ts` | **live V2 path** |

### 3.1 What to steal

| Concern | File | Idea |
|---|---|---|
| Magic-byte sniff | `packages/core/src/tool/read-filesystem.ts:136` (`imageMime`) | PNG `89 50 4E 47 0D 0A 1A 0A`, JPEG `FF D8 FF`, GIF `47 49 46 38`, WebP `RIFF` at 0 + `WEBP` at 8. Content-addressed, no extension trust. |
| Sniff-then-branch | `read-filesystem.ts:183-211` | Read a prefix, sniff, and if it is an image stream the whole file in 64 KB chunks and base64 it. Never route an image into the line splitter. |
| Ingest cap | `read-filesystem.ts:13` | `MAX_MEDIA_INGEST_BYTES = 20 MB` — hard ceiling before any work is done. |
| Binary detection | `read-filesystem.ts:143` (`binary`) | extension denylist **plus** NUL byte **plus** `>30%` non-printable. Redundant heuristics, cheapest first. |
| Line paging | `read-filesystem.ts:233-320` | 64 KB chunks, per-line 2000-char cap, 50 KB total cap, explicit `next` cursor so the model knows how to continue. |
| Bypass the text renderer for images | `packages/core/src/tool/read.ts:45` (`toModelOutput`) | Returns `[]` for non-images — the normal text path handles those. Images get their own branch entirely. |
| Resize budget | `packages/core/src/image.ts` | Defaults 2000×2000, 5 MB base64. Walks ~32 successive scales ×0.75, trying PNG then JPEG at `[80,85,70,55,40]` per scale and taking the first that fits. |
| Provider capability matrix | `packages/opencode/src/session/message-v2.ts:147-169` | Which providers accept media *inside a tool result* vs needing it extracted into a separate user message. |
| Graceful resizer failure | `packages/core/src/tool/read.ts:89` | `ResizerUnavailableError` is caught and the oversized original passes through anyway. |

### 3.2 What NOT to copy

- **Their two-implementations split.** We have one `read_file`. Do not introduce a parallel one.
- **Their 20 MB ingest cap is generous.** We start at 10 MB; the resize step exists to keep us under provider limits anyway.
- **Their PDF attachment path.** V1-only; V2 core rejects PDFs as binary. Match V2.
- **Their photon/WASM resizer.** It is a Rust WASM blob. We use pure Go (§4.2) — no cgo, no wasm, no build-script patching.

---

## 4. Data Model

### 4.1 Tool-side contract

```go
// internal/agent/tools/image_view.go (new)

// SupportedImageMIMEs are the formats we return as image parts. Anything
// sniffed but absent here stays on the text/binary path.
var SupportedImageMIMEs = map[string]bool{
    "image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
}

// ImageDescriber renders an image to prose for a run model that cannot see
// one. ok=false means no vision path is available and the caller must fall
// back to a placeholder — never a silent omission.
type ImageDescriber interface {
    // Describe returns a concise visual description. hint carries the file
    // path so the description can refer to what was read.
    Describe(ctx context.Context, mime string, data []byte, hint string) (description string, ok bool)
}

// ImagePayload is a sniffed, size-checked image ready for passthrough or describe.
type ImagePayload struct {
    Path   string
    Mime   string
    Data   []byte // post-resize bytes, i.e. what gets base64'd
    Width  int
    Height int
    Resized bool
}
```

`ReadFile` changes from a package-level `var` to a constructor, matching `NewTodoTool`, `NewBashTool`, `NewBrowserTool`:

```go
func NewReadFileTool(opts ReadFileOptions) Tool
type ReadFileOptions struct {
    // CanSeeImages is true when the run model declares image support, in
    // which case the image part is returned directly and Describe is unused.
    CanSeeImages bool
    Describer    ImageDescriber // nil disables describe; falls back to placeholder
    MaxImageBytes int           // default readMaxImageBytes
}
```

`tools.DefaultTools()` keeps returning a `read_file` with zero-value options so static registration (and `subagent`'s simulated path) still works.

### 4.2 Pure-Go resize budget

`golang.org/x/image` is **not currently a dependency** (`go.mod` has only `golang.org/x/{oauth2,sync,text,time,net,sys,exp}`). Add `golang.org/x/image` for `draw.CatmullRom`. `image/png`, `image/jpeg`, `image/gif` are stdlib.

```go
// internal/agent/tools/image_view.go
const (
    readMaxImageBytes = 10 * 1024 * 1024 // ingest ceiling, pre-resize
    imageMaxEdge      = 2000             // max width or height after resize
    imageMaxBase64    = 5 * 1024 * 1024 // max base64 payload after re-encode
    imageJPEGQuality  = 85               // tried before dropping below PNG
)
```

`fitImage(raw []byte, mime string) (ImagePayload, error)`:
1. If `len(raw) <= imageMaxBase64/4*3` and dimensions unknown-or-small → return as-is.
2. Decode with the stdlib decoder matching `mime`. **WebP fails here** → return it unchanged if it fits the budget, else a `SizeError`-style error naming the path.
3. If width and height are both `<= imageMaxEdge` and base64 fits → return as-is.
4. Scale by `min(1, imageMaxEdge/w, imageMaxEdge/h)`; encode PNG first (lossless, right for screenshots and diagrams). If base64 still over budget, re-encode JPEG at `imageJPEGQuality`. If still over, halve and retry, up to 4 attempts.
5. Return with `Resized: true` and the new mime.

Deliberately simpler than OpenCode's 32-scale ladder — four halvings covers the realistic range, and each step is a decode-free re-encode of an already-downscaled buffer.

**Placement:** put `fitImage`, `sniffImageMime`, and the limits in a small shared package (e.g. `internal/utils/image`) rather than keeping them unexported inside the tools package. The browser and MCP paths both want them later (§7), and getting the location right now costs nothing while doing it as an extraction later costs a refactor.

### 4.3 Vision side

```go
// internal/agent/vision/vision.go (new package, mirrors internal/agent/titles)

// Describe performs one non-streaming, tool-free call to a vision model and
// returns its prose description. Returns "" on any failure so the caller can
// fall back to a placeholder. Mirrors titles.Generate.
func Describe(ctx context.Context, provider loop.Provider, model, mime string, data []byte, hint string) string
```

Implementation is `titles.Generate` with the attachment populated:

```go
req := loop.TurnRequest{
    Model:        model,
    SystemPrompt: visionSystemPrompt,
    Messages: []any{loop.UserMessage{
        Role:        loop.RoleUser,
        Content:     visionUserPrompt(hint),
        Attachments: []loop.ImageAttachment{{Data: base64.StdEncoding.EncodeToString(data), MimeType: mime}},
    }},
    // Tools deliberately nil: the describer must not be able to read another
    // image and recurse.
}
```

```go
const visionSystemPrompt = "You describe images for a text-only coding agent. " +
    "Report, in this order and concisely: what the image shows; any visible text " +
    "verbatim; UI state, dialogs, or error messages; layout and structure; anything " +
    "that looks like a failure. Be factual. Do not speculate, do not apologise, " +
    "do not ask questions, and do not use markdown headings."
```

`run` provides the wiring, since `roles`/`providers` are unreachable from `tools` (import cycle):

```go
// internal/run/models.go
func (s *Service) imageDescriber(model types.Model) tools.ImageDescriber {
    if model.SupportsImages { return nil }        // never pay for what the model can do
    if _, ok := s.resolveVision(model); !ok { return nil }
    return &visionDescriber{s: s, fallback: model}
}
```

`visionDescriber.Describe` resolves the role, `s.Lookup`s the provider, calls `vision.Describe`, and returns `ok=false` if the role model or the call fails. It wraps each call in `context.WithTimeout(ctx, 30*time.Second)` so a slow vision model cannot stall the run indefinitely.

**Decision — trust the user's declared vision role (fix A).** `roles.ResolveRoleModel` (`internal/agent/roles/roles.go:78-81`) synthesizes a model for an unknown id with a hardcoded `ContextWindow: 128_000` and never sets `SupportsImages`, so it is `false`. Combined with the `!vision.SupportsImages` gate in `run/models.go:67`, a `vision` role pointing at a model id the catalog does not know **can never win the fallback, silently** — the user configures a vision model, the UI shows it set, and images are still redacted to `[image omitted: ...]`.

This is live, not theoretical. `providers/opencode/models.go:113-115` hardcodes `supportsImages := false` and only sets it for the literal id `space-bunny-free`, so on the OpenCode provider every other model reports `false`. Claude, Codex, and Antigravity read the flag from live API metadata (`InputModalities`, `capabilities.image_input.supported`) and so rarely hit this path. `tests/agent/roles_test.go:58-79` asserts the synthesized case's `ContextWindow` but says nothing about `SupportsImages`, so nothing catches it.

**The fix:** in the synthesize branch only, set `SupportsImages: true` when `role == Vision`. Constraints:

- `ResolveRoleModel` already accepts a `role string` parameter and **never reads it** — this gives it its first purpose, so no signature change.
- Gate **only** the synthesize branch. Both `return fallback` paths (empty ref, unknown provider) must keep the fallback model's real capabilities; a catalogued model's flag is never overwritten.
- **No logging inside `roles`** — it is a pure function package with no logger and `roles_test.go` tests it as pure. Emit `slog.Warn` from the caller: `run` already uses package-level `slog.Warn` (`run/hub.go:163`), so `resolveVision` logs when it accepts an uncatalogued model on the user's word.
- Rejected the alternative — dropping the `SupportsImages` gate. It would let a genuinely blind model through and surface as an opaque provider 400 mid-turn, which is far harder to debug than trusting the user.
- This ships **independently** of the rest of this plan. It silently disables a feature the user believes they turned on, which is worth fixing whether or not the image work lands.

---

## 5. Phased Implementation

### Phase 0 — Fix token accounting for images (blocking, ships first)

This must land before any image is ever returned, or we will trigger runaway compaction.

Both estimators `json.Marshal` the tool result and count **every** character, base64 included:

- `internal/agent/compaction/tokens.go:153` — `resultChars` → `len(raw)`
- `internal/agent/loop/breakdown.go:151` — `contentChars` → counts marshalled length

A 1.5 MB PNG becomes ~2 MB of base64, so `2_000_000 / 4 ≈ 500_000` tokens attributed to `ToolResults["read_file"]` — against a real cost of ~1,000. Two images would look like a blown context, firing the emergency-compaction and overflow-retry paths in `agent/loop/loop.go:297` on every single turn. Meanwhile the correct flat rate (`imageTokensEach = 1000`, `tokens.go:17`) is applied *only* to `UserMessage.Attachments` and never to tool results.

Add an image-aware walk to both: strip `{"type":"image"}` parts before counting characters and add `imageTokensEach` per part, exactly as `UserMessage.Attachments` already does. Share one helper so the two estimators cannot drift.

- **Verification**: `cd apps/server-go && go test ./tests/agent/ -run TestBreakdown -v` and `cd apps/server-go && go test ./tests/compaction/ -run TestEstimateMessageTokens -v` — a `ToolResultMessage` carrying a 2 MB base64 image estimates at ~1,000 tokens, not ~500,000; text-only results are unchanged; multiple images add linearly.

### Phase 1 — Detect images and pass them through

New `internal/agent/tools/image_view.go`:
- `sniffImageMime(prefix []byte) string` — magic bytes only, per §3.1.
- `isBinaryFile(path string, prefix []byte) bool` — extension denylist + NUL byte + `>30%` non-printable, ported from OpenCode. Replaces the current silent mojibake behaviour.
- `imagePart(mime string, data []byte) map[string]any` → `{"type":"image","data":<base64>,"mimeType":mime}`.

Rework `ReadFile` in `file_tools.go:56` to `os.Open` + read a 4 KB prefix before branching:
- image → enforce `readMaxImageBytes`, read the rest, emit `[imagePart, textLine]`
- binary (non-image) → `isError` tool result naming the path and the reason
- otherwise → the existing line reader, unchanged

Binary detection must run **before** the line splitter, never after.

- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run TestReadFileImages -v` — real PNG/JPEG/GIF fixtures produce an `image` part with a base64 payload and correct mime; a text file named `.png` still returns line-numbered text (magic bytes win over extension); a truncated PNG is rejected as binary; a directory still returns the `list_dir` hint; the empty-file and offset-past-EOF messages are unchanged.

### Phase 2 — Resize and re-encode to fit the budget

`fitImage` per §4.2, plus `golang.org/x/image` in `go.mod`. Called from the image branch of `read_file`. When the WASM-free resize cannot help (oversized WebP), return a clear error naming the path and the limit instead of emitting a payload that will 400 at the provider.

Configurable later via the same shape OpenCode uses — not in this phase.

- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run TestFitImage -v` — a 6000×4000 PNG comes back `<= 2000` on the long edge with base64 `<= 5 MB`; a small PNG is byte-identical (no needless re-encode); a photographic JPEG over budget comes back as JPEG; an oversized WebP produces a named error; dimension and mime updates propagate to the emitted part.

### Phase 3 — Vision handoff

New `internal/agent/vision/vision.go` per §4.3. `Describe` returns `""` on any error — never panics, never blocks indefinitely.

New tool behaviour, the heart of the plan:

```
sniffed image
  ├─ opts.CanSeeImages            → [imagePart, "Image read: <path> (<mime>, WxH)."]
  ├─ opts.Describer != nil        → Describe() ok
  │      → text only:
  │        "Image: <path> (<mime>, WxH, <n> KB)"
  │        "This model cannot view images; described by vision model <provider/model>:"
  │        "<description>"
  └─ otherwise                    → text only:
         "Cannot view <path>: <mime>, WxH. The active model
          (<model>) has no image support and no vision model is
          configured. Set one in Settings → Models → Vision, or read
          the file as text."
```

The last branch matters. Today the model would receive `[image omitted: provider does not support images in tool results]` and have no idea what happened or what to do about it. Naming the remedy is the difference between a dead end and a recoverable turn.

- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run TestReadFileVisionHandoff -v` — with `CanSeeImages: true` the describer is never called and an image part is returned; with `CanSeeImages: false` and a stub describer the result is text-only, mentions the vision model, and embeds the stub's description; with a nil describer the result names the path, mime, dimensions, and the settings remedy; a describer returning `ok=false` falls through to the placeholder; the describer receives the correct mime and bytes.

### Phase 4 — Wire it into the run

`internal/run/turns.go` — add to the tool-swap switch at line 296:

```go
case "read_file":
    toolList = append(toolList, tools.NewReadFileTool(tools.ReadFileOptions{
        CanSeeImages: model.SupportsImages,
        Describer:    s.imageDescriber(model),
    }))
```

Note `model` here is the **post-vision-fallback** variable. If the run already swapped to the vision model at line 274 because the user attached an image, `model.SupportsImages` is then `true`, so a subsequent `read_file` image goes straight through as an image part. Correct, and it falls out for free.

`internal/run/models.go` — add `imageDescriber`. Reuse `resolveVision` so the `SupportsImages` gate stays in one place.

Also land the §4.3 fix here, in two small pieces:
- `internal/agent/roles/roles.go:78-81` — `SupportsImages: true` in the synthesize branch when `role == Vision`.
- `internal/run/models.go` `resolveVision` — `slog.Warn` when the role resolved to an uncatalogued id, naming the provider/model, so the trust decision is visible in logs.

Subagents inherit `Model` from the parent (`agent/loop/subagent.go:21`), and a subagent's `read_file` should get its own describer rather than the parent's. Wire `SubagentContext` the same way if subagents can read files.

- **Verification**: `cd apps/server-go && go test ./tests/agent/ -run TestReadFileImageWiring -v` — a vision-capable run model gets `CanSeeImages: true` and no describer; an image-blind run model with a `vision` role in a temp `settings.json` (via `CONSOLE_SETTINGS_PATH`, following `tests/agent/roles_test.go:329`) gets a describer bound to the role model; with no role configured it gets nil.
- **Verification**: `cd apps/server-go && go test ./tests/agent/ -run TestResolveRoleModel -v` — extend the existing table in `tests/agent/roles_test.go` with `TestResolveRoleModelVisionCapability`: a `vision` role resolving to an uncatalogued id synthesizes with `SupportsImages == true`; the same ref under the `smol` role still synthesizes with `SupportsImages == false`; a catalogued model's real flag is preserved and never overwritten; an empty ref and an unknown provider both return the fallback unchanged.

### Phase 5 — Prompt text, kill switch, docs

- Update the `read_file` description to say it reads images and PDFs correctly — **drop "and PDFs"**, we reject PDFs (§0).
- Kill switch: add `vision.describeImages` (default `true`) to the `Settings` struct in `internal/services/settings_service.go`, checked in `imageDescriber`. No proto change — `Settings` is plain JSON with `ModelRoles` already alongside it.
- Optional per-run memo: key a description cache on `path + mtime + size` so re-reading one image does not pay twice. Only if profiling shows it matters.

- **Verification**: `cd apps/server-go && go test ./tests/agent/ -run TestVisionDescribeDisabled -v` — with the switch off, `imageDescriber` returns nil and the placeholder branch is used.

---

## 6. Risks

| Risk | Mitigation |
|---|---|
| Token blowout from base64 in tool results | Phase 0 lands first and is independently tested. This is the one that would hurt in production. |
| Vision describe becomes a cost/loop sink | One call, no tools passed (cannot recurse), 30 s timeout, no retry. `ok=false` degrades to a placeholder. |
| Description quality is worse than the model seeing the image | Inherent to the handoff. Mitigated by a prompt that prioritises verbatim visible text and error state — the things coding agents actually need from screenshots. |
| `golang.org/x/image` is a new dependency | Pulls nothing else; `x/image` is leaf-only and stdlib-adjacent. Verify `go mod tidy` adds no transitive surprises. |
| Provider rejects an image payload | Bounded by `imageMaxBase64` and `imageMaxEdge`; Phase 2 verifies the bounds hold. |
| `read_file` behaviour change breaks existing tests | `tests/tools/read_file_test.go` exists and must stay green. Text paths are byte-identical by design — only new branches are added. |
| Synthetic role model has `SupportsImages: false` | Fixed per §4.3 (trust the declared vision role) + `slog.Warn` from the caller. Ships independently of this plan. |

---

## 7. Open Questions

- Should the vision description be cached per run? Costs one model call per image read otherwise.
- Should `write_file` refuse binary writes, or is that out of scope?
- Does `imageTokensEach = 1000` hold for the vision role models we route to, or should descriptions count as text against their context?
- Should the browser tool's screenshot path route through `fitImage` too? **Separate follow-up, not part of this plan.** To be explicit, since it was unclear before: nothing gets replaced. `read_file` reads bytes off local disk; browser screenshots are captured by the desktop client and arrive as `res.ImageBase64` (`browser_tool.go:117`). Only the pure `fitImage` helper would be shared. Both paths are currently unbounded — the browser tool has no size guard at all, and the MCP adapter caps image *count* at 4 (`mcp/adapter.go:118`) but not per-image bytes — so both would benefit. Worth its own change; bundled here it would just be scope creep.