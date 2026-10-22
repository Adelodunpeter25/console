# Visualize — native visual artifacts from a versioned spec

Add one lazy `visualize` tool that renders a chart, metric, table, or text card
inside the chat, plus a `/visualize` activation command modelled on
`/computer-use`. The tool produces a versioned, declarative visual spec; Android
and desktop paint that spec natively. No WebView, no embedded chart library, no
arbitrary code.

`zeron` PR 900 is a reference for the shape of the problem — one kernel, a
versioned spec, native painters — not a stack to copy. We are not bringing in
Luau, wgpu, or an iOS-only renderer.

Target: `internal/services/visualize/` (spec + validation + tool), a small
`VisualPart` addition to `proto/console/v1/message.proto`, `internal/run/` for
activation, `apps/android/app/src/main/kotlin/com/console/mobile/feature/chat/visual/`,
and `apps/desktop/crates/console-ui/src/chat/visual.rs`.

---

## 0. Non-Goals

- **No WebView renderer.** A second rendering engine would add a JS bridge,
  async load states, accessibility gaps, theme drift, and scroll jank. The
  native painters already exist in this codebase (`TerminalCanvasView`,
  `ContextRing` on Android; `usage_meter.rs` uses `PathBuilder` on desktop).
- **No arbitrary HTML, SVG, CSS, JavaScript, or shader code in a spec.** A spec
  is data. It cannot execute, navigate, fetch a remote asset, or set an absolute
  position.
- **No per-chart tools.** One `visualize` tool accepts a spec with several
  blocks. Do not add `bar_chart`, `line_chart`, `table`, and so on; that is the
  schema explosion `/computer-use` deliberately avoided.
- **No raw colors or fonts in the spec.** Clients map series indices to their
  own theme palette, so light/dark and contrast stay client concerns.
- **No interactive charts in v1.** No hover tooltips, pan, zoom, brush, legend
  toggles, or click-through. Read-only cards first.
- **No animation on entry.** It adds jank and makes screenshots unstable. A
  static card is fine.
- **No remote images, iframes, or SVG.** Text is plain text; there are no
  auto-links.
- **No visual artifacts produced inside subagents.** Subagent transcripts are
  folded into a summary. A visual made there would be invisible. Keep visuals
  in the main agent.
- **No always-on tool in v1.** `/visualize` activates it lazily, like
  `/computer-use`. Revisit only if users routinely ask for charts without the
  command.
- **No PNG/SVG export, sharing, or a saved-visual library.** The transcript is
  the first library, not a durable artifact store: compaction may fold away a
  very old turn and its card.

---

## 1. Sequence: contract first, then Android, then desktop

**Recommendation: server contract first, then Android, then desktop.** Not
Android-first from zero, because an Android-only spec would be invented against
nothing and then changed under the server.

The order is:

1. **Freeze the contract (server + shared fixtures).** The proto envelope, the
   v1 JSON spec, limits, and two golden fixtures. This is the only cross-client
   dependency. Everything after this can proceed in parallel if needed.
2. **Build the server path end to end.** `visualize` tool, validation,
   `/visualize` activation, persistence, wire event, autocomplete. A fake client
   can already see the tool result.
3. **Build Android first among clients.** Android is the tightest constraint:
   a small viewport, font scaling, Compose Canvas, and the cheapest place to
   discover that a table or a 2,000-point line is too heavy. If it is smooth and
   legible on a phone, desktop is easier.
4. **Build desktop second.** Reuse the same fixtures and spec; swap the painter
   and follow the existing GPUI transcript patterns.

A client-only spike is allowed in parallel with step 1 **only against a
checked-in fixture**, and it must not merge a competing spec. The contract stays
server-owned.

Why not Android-first end to end? Because the tool result shape, the activation
memory, and the version rule are server contracts. Building them twice is the
expensive mistake.

---

## 2. Why this shape

### 2.1 The tool result is the artifact boundary

The assistant message is persisted in `turnOnce` **before** tools execute
(`agent/loop/loop.go`). Mutating that already-persisted assistant turn to attach
a visual would race persistence, break replay, and make a live event differ from
history. The tool result is the natural boundary: it is created after the tool
runs, it already streams (`toolExecutionResult`), it already replays, and it
already persists in `AgentToolResultMessage`.

So the visual rides on `ToolResult`, not on `AssistantContentPart`.

```
visualize tool
  └─ Envelope{Content: short summary, Visual: artifact}
       └─ tools.ToolResult.Visual
            ├─ loop persistence → proto ToolResult.visual
            ├─ live wire → toolExecutionResult.result.visual
            ├─ Android / desktop render from the run's tool results
            └─ provider adapters read only Content
```

The summary is the only thing the model sees on later turns. The spec never
returns to the provider.

### 2.2 Native rendering, not rasterization

The server does not render PNGs. A server-side raster would be blurry at a
different DPI, fixed to one theme, inaccessible, and would add an image pipeline
to every visual. The spec is data; each client paints it with its own text
stack, theme, and DPI. That is also why there is no image attachment path for
visuals.

### 2.3 Declarative spec, not a scripting runtime

A JavaScript/Luau runtime would let the model draw anything, but it would also
mean executing model-written code on the client, sandboxing it, versioning an
API, and debugging two runtimes. A small spec covers the useful read-only v1:
text, metric, table, and four chart kinds. If a future visual cannot be
expressed, add a block type behind a spec version bump; do not add an escape
hatch.

### 2.4 `/visualize` mirrors `/computer-use`

CUA's pattern is the precedent:

- `cua.SplitInvocation` matches only a command at the start of the message.
- `run/computer_use.go` remembers activated sessions in memory.
- `run/turns.go` appends `cua.SkillText()` to setup every active turn, never to
  the persisted message.
- `cua.NewLoader(...).Load(registry)` appends the tools lazily.
- `routes/assist.go` lists `computer-use` so the composer autocompletes it.

`/visualize` does the same: the model has no tool and no skill until the user
invokes it. The command text the user typed remains exactly what is persisted
and displayed. The skill instructions travel in setup.

The trade-off is discoverability: a plain “make me a chart” request has no
visual tool. Autocomplete is the discovery path, same as `/computer-use`. If
that proves too weak, the follow-up is a one-line setup hint, not an always-on
schema.

---

## 3. The v1 visual spec

### 3.1 Envelope

```json
{
  "version": 1,
  "title": "Weekly signups",
  "subtitle": "Last 8 weeks",
  "blocks": []
}
```

| Field | Type | Rules |
|---|---|---|
| `version` | integer | Must be `1`. |
| `title` | string, optional | ≤ 160 chars, trimmed. |
| `subtitle` | string, optional | ≤ 240 chars, trimmed. |
| `blocks` | array | 1–12 blocks. |

Every block has a `type`. Unknown fields are rejected by the server so model
mistakes surface immediately instead of being silently dropped.

### 3.2 Blocks

**Text** — a short plain-text note.

```json
{ "type": "text", "text": "Organic remains the largest channel." }
```

- `text`: 1–1,000 chars. Newlines and tabs allowed. No Markdown rendering in
  v1; it is a plain text block. Rich prose belongs in the assistant message.

**Metric** — a single number with optional context.

```json
{
  "type": "metric",
  "label": "MRR",
  "value": "$42.1k",
  "delta": "+8.2%",
  "trend": "up",
  "caption": "vs. last month"
}
```

- `label` ≤ 80, `value` ≤ 80, `delta` ≤ 40, `caption` ≤ 160.
- `trend`: `up`, `down`, or `flat`, optional. If present, the client colors the
  delta and adds an arrow shape; the value is not color-only.
- Values are preformatted display strings. The model owns currency, percent, and
  unit formatting so the client never guesses.

**Table** — a compact summary table.

```json
{
  "type": "table",
  "caption": "Top channels",
  "columns": ["Channel", "Users", "Share"],
  "rows": [
    ["Organic", "640", "49.8%"],
    ["Referral", "312", "24.3%"]
  ]
}
```

- 1–6 columns, 1–30 rows, each cell ≤ 80 chars.
- Every row must have exactly `len(columns)` cells.
- Cells are strings. No nested objects, links, or images.
- Numeric-looking columns right-align; everything else stays left. This is a
  documented display heuristic, not a spec field. A future version can add
  explicit alignment if needed.
- On a narrow phone the table scrolls horizontally inside its own card; the
  transcript itself never scrolls sideways.

**Chart** — one chart with one or more series.

```json
{
  "type": "chart",
  "kind": "line",
  "title": "Signups by week",
  "xLabel": "Week",
  "yLabel": "Users",
  "series": [
    {
      "name": "This quarter",
      "points": [
        { "x": 0, "y": 120, "label": "W1" },
        { "x": 1, "y": 184, "label": "W2" },
        { "x": 2, "y": 164, "label": "W3" }
      ]
    }
  ]
}
```

- `kind`: `line`, `bar`, `area`, or `scatter`.
- `title` ≤ 160, `xLabel`/`yLabel` ≤ 80, series `name` ≤ 80.
- 1–8 series per chart. Series names must be unique.
- 1–2,000 points per series; ≤ 5,000 points total across the spec.
- Every point needs finite numeric `x` and `y`. `label` is optional and ≤ 40
  chars.
- `x` must be unique within a series. The server sorts points by `x` for
  deterministic output.
- For `line`, `area`, and `bar`, `x` is an ordinal position; `label` is the tick
  text. For `scatter`, `x` and `y` are both values.
- `bar` and `area` include zero in their baseline. `line` and `scatter` use the
  data range with padding. Negative values are supported.
- `line`/`area` with one point render a dot, not a zero-length path. A flat
  series is valid; the client expands the range to avoid division by zero.
- No stacked mode in v1. Multiple bar series are drawn side by side; multiple
  line/area series are overlaid.

### 3.3 Limits

| Limit | Value | Why |
|---|---:|---|
| Canonical spec size | 256 KiB | Keeps a card from bloating history and wire frames. |
| Blocks | 12 | A dashboard, not a document. |
| Chart series | 8 | Legend and palette stay readable. |
| Points per series | 2,000 | Enough for a phone-width chart without per-frame downsampling. |
| Points total | 5,000 | Bounds worst-case paint work and spec size. |
| Table columns | 6 | Fits a phone with horizontal scroll. |
| Table rows | 30 | A summary; full data belongs in a file. |
| Text block | 1,000 chars | A note, not an essay. |
| Title / subtitle | 160 / 240 chars | Wrapping stays predictable. |

The server rejects a spec over a limit with a path-specific error. It does not
silently clamp, truncate, or downsample. The skill tells the model to aggregate
first. A future kernel could add LTTB downsampling, but silent data loss is worse
than a clear error in v1.

### 3.4 Validation and normalization

`internal/services/visualize` is the kernel. It is pure and deterministic:

1. Decode the JSON object into Go structs with `DisallowUnknownFields`.
2. Check `version == 1`.
3. Validate every field, limit, enum, and cross-field rule.
4. Normalize: trim strings, drop empty optional strings, sort points by `x`,
   and re-marshal with fixed struct field order.
5. Return a canonical `json.RawMessage`.

Validation errors are `tools.NewToolError` with a JSON path, for example:

```
blocks[2].series[0].points[14].y: expected a finite number
blocks[1].rows[3]: expected 3 cells, got 2
blocks[0].series: series names must be unique
```

The model sees that error and can fix the spec in the same turn. A bad visual
never aborts the run.

### 3.5 Versioning

`version: 1` is the only version in v1. The server rejects anything else with
“unsupported visual spec version”. Clients check the version before decoding
and show a compact fallback card for a newer version:

> This visual needs a newer version of Console.

Because the spec is versioned JSON, a new block type does not require a proto
change or a regenerated client. The envelope stays typed; the payload evolves
behind the version. Golden fixtures in `proto/testdata/visual/` are the shared
contract and are decoded by Go, Kotlin, and Rust tests.

---

## 4. Server

### 4.1 Package layout

```
apps/server-go/internal/services/visualize/
  spec.go        # v1 structs, block union
  decode.go      # strict decode + custom block discriminator
  validate.go    # field and cross-field validation, limits
  normalize.go   # trim, sort, canonical JSON
  summary.go     # short model-facing summary
  tool.go        # NewTool, Envelope with Visual
  tools.go       # Loader (the lazy activation path)
  invoke.go      # /visualize prefix detection
  skill.go       # embedded SKILL.md
  SKILL.md       # playbook + examples + limits
  visualize_test.go
  fixtures_test.go
```

No external chart library. No network. No filesystem. The kernel only touches
the spec.

### 4.2 Artifact plumbing

Add a neutral artifact type in `agent/tools` so the tools package does not import
the visualize package:

```go
// agent/tools/tool.go

// VisualArtifact is a client-rendered artifact attached to a tool result.
// It is not part of the provider-facing content.
type VisualArtifact struct {
    Version uint32
    Spec    json.RawMessage
}

type Envelope struct {
    Content any
    IsError bool
    Visual  *VisualArtifact
}

type ToolResult struct {
    ...
    Visual *VisualArtifact `json:"visual,omitempty"`
}
```

`Executor.Execute` already unwraps `Envelope`; copy `Visual` through:

```go
if envelope, ok := out.(tools.Envelope); ok {
    content, isError = envelope.Content, envelope.IsError
    visual = envelope.Visual
}
return tools.ToolResult{..., Visual: visual}
```

The `visualize` tool returns:

```go
return tools.Envelope{
    Content: textResult(summary),
    Visual: &tools.VisualArtifact{
        Version: 1,
        Spec:    canonical,
    },
}, nil
```

`TierRead`: rendering a card changes no files and needs no approval.

### 4.3 Tool contract

```go
type visualizeInput struct {
    Spec map[string]any `json:"spec" jsonschema:"required,description=Version 1 visual spec object. See the /visualize instructions for the grammar, limits, and examples."`
}
```

`Spec` is an object, not a string. The provider schema stays tiny; the grammar
lives in `SKILL.md`, exactly as the CUA driver API lives in its skill rather than
in a giant schema.

Description:

> Render a native chart, metric, table, or text card in the chat from a
> versioned visual spec. Use `/visualize` instructions for the spec grammar.

The tool validates, normalizes, and returns the short summary plus the artifact.
It does not persist anything itself; the loop already owns persistence.

### 4.4 Activation

New files and edits follow CUA:

- `visualize/invoke.go` — `SplitInvocation(text)` with the same narrow prefix
  rule: command at the start after whitespace, followed by whitespace or end.
  `/visualizes` and mid-sentence mentions do not trigger.
- `run/visualize.go` — `visualizeSessions` with `mark`/`isActive`, in memory,
  exactly like `computerUseSessions`. A server restart forgets activation; the
  persisted visuals still render.
- `run/service.go` — add `visualize visualizeSessions` to `Service`.
- `run/turns.go`:
  1. After the synthetic-turn block, check `visualize.SplitInvocation(dto.Text)`
     and mark the session.
  2. After `setup := prompt.Setup`, append `visualize.SkillText()` when active.
  3. After the MCP and CUA loaders, load the visualize tool:
     `visualize.NewLoader(sessionID).Load(registry)`.
  - The loader appends one tool and returns `1`. It is idempotent and has no
    manager. Keeping the `Loader` shape makes the activation path identical to
    CUA and leaves room for future session state.
  - Append the tool after MCP/CUA so the provider's cached tool prefix stays
    stable.
  - Keep `visualize` out of the subagent `ToolSource`. The subagent is
    constructed before the lazy loader, but its later `ToolSource` reads the
    live registry; filter the tool there so a subagent cannot create a visual
    that has no transcript card.

`run/turns.go` already rebuilds a registry every turn, so the loader is the
restore path. The session memory only decides whether it runs.

The user message is never rewritten. The skill says the task follows the command;
if nothing follows, ask what to visualize.

### 4.5 Persistence and wire

Add only the envelope to proto:

```proto
// proto/console/v1/message.proto

message VisualPart {
  uint32 version = 1;
  // Canonical UTF-8 JSON for one versioned visual spec.
  bytes spec = 2;
}

message ToolResult {
  string tool_call_id = 1;
  optional string tool_name = 2;
  bytes content = 3;
  optional bool is_error = 4;
  // Client-rendered artifact. Never sent to a model provider.
  optional VisualPart visual = 5;
}
```

Why `bytes` for the spec? It is the same “UTF-8 JSON carried as bytes” convention
as tool arguments and result content. It is base64 on the protojson wire, decoded
by the existing `bytesToJson` / `decode_json_bytes` helpers. A `string` would be
more readable in a fixture but would break the convention and invite clients to
treat the spec as human-facing text.

Plumbing:

- `agent/loop/proto.go` — `toolResultsToProto` writes `Visual`; `protoToToolResults`
  restores it. A malformed persisted spec is dropped (the summary content
  remains) instead of failing history load.
- `run/wire.go` — `toolResultToProto` writes `Visual` for live
  `toolExecutionResult` and `toolExecutionEnd` frames.
- `tools.ToolResult` carries `visual` in memory; the loop’s
  `ToProtoBytes`/`toolResultsToProto` path is the only persistence path.
- `stripResultImages` leaves visuals alone. It only replaces inline image parts.
- `loop/compaction`, token estimation, and provider adapters keep reading
  `Content` only. The spec is deliberately invisible to context accounting
  because it is never sent to a provider. The summary is small; the visual is
  not context.

### 4.6 Autocomplete

`routes/assist.go` adds:

```go
visualizeCommandName        = "visualize"
visualizeCommandDescription = "Render a chart, table, or metric card in the chat"
```

to the builtin command list. Update `proto/testdata/assist/commands.json` and
the Android `AssistFixtureTest` expected count. No client autocomplete code
changes are needed; both clients already fetch `/api/assist/:sessionId/commands`.

Do **not** add `visualize` to `tools.DefaultTools()` or
`systemprompt.DefaultToolNames`. The tool is loaded lazily and the inventory is
static; CUA does not list `computer` there either.

---

## 5. Android

### 5.1 Decode

`ToolResult` gains:

```kotlin
@Serializable
data class VisualArtifact(
    val version: Int,
    val spec: JsonElement,
)

data class ToolResult(
    val toolCallId: String,
    val toolName: String? = null,
    val content: JsonElement? = null,
    val isError: Boolean = false,
    val visual: VisualArtifact? = null,
)
```

`MessageConverters.kt` reads the Wire `VisualPart`, converts `spec` bytes to JSON
with the existing `bytesToJson`, and keeps it as `JsonElement` on the result.
Do not fail the whole tool result if the spec is malformed; keep the result and
let the renderer show a fallback. `ChatEvents.kt` then needs no new event type:
live `toolExecutionResult` payloads already decode into `ToolResult`, and the
visual rides along.

`feature/chat/visual/VisualSpec.kt` holds the typed render models:

```kotlin
@Serializable
data class VisualSpec(
    val version: Int,
    val title: String? = null,
    val subtitle: String? = null,
    val blocks: List<VisualBlock> = emptyList(),
)

@Serializable
@JsonClassDiscriminator("type")
sealed interface VisualBlock {
    @Serializable @SerialName("text") data class Text(val text: String) : VisualBlock
    @Serializable @SerialName("metric") data class Metric(...) : VisualBlock
    @Serializable @SerialName("table") data class Table(...) : VisualBlock
    @Serializable @SerialName("chart") data class Chart(...) : VisualBlock
}
```

Use `ConsoleJson` with `ignoreUnknownKeys = true` for forward-compatible v1
fields. Reject/fallback when `version != 1` or decoding fails.

### 5.2 Transcript placement

Do **not** append a synthetic message to `session.messages`. Live tool results
already live in `RunActivityState.events` as
`ActivityEvent.ToolCallEvent.result`; the same results are restored from
`ToolResultMessage` by `ReconstructRuns.kt`. Rendering from the run activity
works live and after reload with no dedupe logic.

Change `RunActivity`:

- Collect `activity.events.filterIsInstance<ToolCallEvent>().mapNotNull { it.result?.visual }`.
- Render a `VisualResultCard` for each visual **after the “Worked for …” header
  and divider**, outside the `if (expanded)` block.
- Keep tool rows inside the fold. The visual is the artifact; the tool invocation
  is detail.
- If a visual tool result has no matching run (a partial-history edge case),
  `MessageBubbleItem` may render it as a standalone card. Do not add a second
  render path while the run is present.

This preserves the existing transcript filter that hides
`ToolResultMessage`; no changes to `visibleMessages` are required for the normal
path.

### 5.3 Renderer

New files under `feature/chat/visual/`:

- `VisualResultCard.kt` — card chrome, title/subtitle, block dispatch, fallback.
- `VisualMetric.kt` — label/value/delta/caption; responsive 1–2 column grid.
- `VisualTable.kt` — header row, body rows, horizontal scroll when columns
  exceed the card width.
- `VisualChart.kt` — Compose `Canvas` for line/bar/area/scatter.
- `VisualGeometry.kt` — pure scale/tick/path math, unit-testable.

Card style:

- `NewTheme.Surface`/`Raised`, 1dp border, 12dp radius, 12–16dp padding.
- Title 14sp semibold, subtitle 12sp secondary, body 13sp.
- No nested card shadows.

Chart:

- Height 180dp on phones, 220dp on wide layouts.
- Plot rect leaves room for 4–5 y ticks and a wrapped legend.
- “Nice” ticks using a 1/2/5 × 10ⁿ step; 4–5 ticks. Format with the platform
  locale and compact suffixes (`1.2k`, `3.4M`). Units belong in `yLabel`.
- `bar`/`area` include zero; `line`/`scatter` pad the range. Flat series expand
  the range so the line sits in the middle.
- Series colors come from `NewTheme.VisualSeries` (add 8 theme colors; reuse
  accent/success/warning/gauge and add four accessible hues). No spec colors.
- Line width 1.5dp; area fill at 12% alpha; bar width `min(24dp, slot * 0.7)`;
  scatter dot 3dp.
- Legend appears for multiple series; wrap below the chart, not over the plot.
- Draw text with Compose’s text measurer; measure on size/theme/font-scale
  change, not per frame.

Geometry and paint:

- Parse the spec once in `remember(artifact.spec)`.
- Precompute points, ticks, `Path` objects, and labels in `remember(spec, size,
  theme, fontScale)`.
- Use `Canvas`/`drawPath`/`drawRect`/`drawCircle`; no per-frame allocations.
- Cap the visible line points at one per horizontal pixel by taking min/max
  buckets. The server cap already bounds the data; this is a rendering safety
  net, not a data rewrite.

Fallback:

- Unsupported version or parse failure: a compact card with “Visual unavailable”
  and, if present, the title. Never crash the transcript.
- Empty or error results render nothing extra; the tool row already shows the
  error.

### 5.4 Accessibility and performance

- Chart canvas gets a generated content description: kind, title, series count,
  point count, range, and last value. Example: “Line chart, Signups by week. Two
  series, 8 points. Range 120 to 480. Last 480.”
- Table cells expose row/column semantics and header text. The card title and
  caption are readable text.
- Trend uses an arrow glyph plus the word/label, not color alone.
- Respect font scale: labels use `sp`; the table grows vertically rather than
  clipping.
- Do not put the chart in a vertical scroll container. The card is part of the
  transcript; only wide tables scroll horizontally.
- Manual check on a mid-range device: scroll a transcript containing a
  2,000-point line and a 30-row table; no dropped frames, no jank when the run
  activity expands/collapses.

New/edited Android files:

```
data/model/ToolModels.kt                 # VisualArtifact + ToolResult.visual
data/model/MessageConverters.kt          # Wire VisualPart -> artifact
core/chat/ChatModels.kt                  # no new event; result carries visual
feature/chat/RunActivity.kt              # render visuals outside the fold
feature/chat/MessageBubbles.kt           # orphan fallback only
feature/chat/visual/*.kt                 # card, chart, table, geometry
ui/theme/ThemeNew.kt                     # VisualSeries palette
tests/VisualFixtureTest.kt               # shared spec + tool result fixtures
```

---

## 6. Desktop

### 6.1 Types and decode

`console-core/src/types/agent.rs`:

```rust
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct VisualArtifact {
    pub version: u32,
    pub spec: serde_json::Value,
}

pub struct ToolResult {
    ...
    #[serde(default)]
    pub visual: Option<VisualArtifact>,
}
```

`ToolResult::from_proto` decodes `msg.visual.spec` bytes with
`decode_json_bytes`. A malformed spec becomes `None`; the renderer shows the
fallback. No new event type.

### 6.2 Transcript placement

Desktop already joins tool results to calls in `tool_activity_for_user` and
`ToolCalls`. Add the same rule as Android:

- `ToolCalls` collects entries whose `result.visual` is present.
- Render `VisualCards` below the run header and divider, outside the collapsible
  body, so a collapsed run still shows the artifact.
- `AgentMessage::ToolResult` stays hidden when it only carries transport.
- Add a standalone fallback only for an orphan visual result with no run.

`row_count` and `is_hidden_tool_transport` do not need to change for the normal
path; visuals are inside the user row with the run activity. If the orphan
fallback is added, include it in the row count.

### 6.3 Renderer

New file `console-ui/src/chat/visual.rs`:

- `VisualCard` — GPUI `div` card with `Theme::raised`, 1px border, radius 12,
  padding 16.
- `VisualMetric` — label/value/delta row.
- `VisualTable` — flex rows, header border, horizontal scroll when needed.
- `VisualChart` — `gpui::canvas` with `PathBuilder`/`paint_path` for lines and
  areas, `paint_quad` for bars, small quads/paths for scatter dots. Axis labels
  use `window.text_system().shape_line`, following `terminal_view/paint.rs`.
- Precompute geometry per `(artifact, bounds, theme)`; the paint closure should
  not rebuild paths on every scroll frame.
- Add `Theme::chart_1..chart_8` to both `dark()` and `light()`; no colors in the
  spec.
- Text is real GPUI text where possible so selection and contrast match the
  rest of the app.

Accessibility on desktop is limited by GPUI; the card must therefore carry a
visible title, axis labels, legend, and a one-line summary under the chart. That
summary is also the fallback for users who cannot read the plot.

New/edited desktop files:

```
crates/console-core/src/types/agent.rs      # VisualArtifact + ToolResult.visual
crates/console-core/tests/...               # fixture decode test
crates/console-ui/src/chat/visual.rs        # card + chart + table
crates/console-ui/src/chat/mod.rs           # export
crates/console-ui/src/chat/toolcalls.rs     # render visuals outside the fold
crates/console-ui/src/chat/transcript_view.rs # fallback / row wiring
crates/console-ui/src/theme/mod.rs          # chart palette
```

Tests go in `crates/console-core/tests/` and `crates/console-proto/tests/`, not
inline `#[cfg(test)]` modules.

---

## 7. Phases

### Phase 0 — Freeze the contract

- [x] Add `VisualPart` and `ToolResult.visual` to `proto/console/v1/message.proto`.
- [x] Regenerate with `make proto`.
- [x] Add `proto/testdata/visual/line.json` and `dashboard.json` (metric + table +
      text).
- [x] Add `proto/testdata/message/toolresult_visual.json` and
      `proto/testdata/event/tool_execution_result_visual.json`.
- [x] Add `/visualize` to `proto/testdata/assist/commands.json`.
- [x] Write the v1 rules in this plan and mirror the grammar in `SKILL.md`.

**Verify:** `make proto`; the tool-result fixtures parse as protojson; the raw
spec fixtures round-trip through the Go decoder. Android/Rust fixture tests are
still pending until Phase 2/3.

### Phase 1 — Server kernel, tool, activation

- [x] Build `internal/services/visualize` (spec, decode, validate, normalize,
      summary, tool, invoke, skill).
- [x] Add `tools.VisualArtifact`, `Envelope.Visual`, `ToolResult.Visual`, and the
      executor copy-through.
- [x] Plumb persistence (`agent/loop/proto.go`) and live wire (`run/wire.go`).
- [x] Add `run/visualize.go` and wire `run/turns.go` + `run/service.go`.
- [x] Add the builtin autocomplete entry in `routes/assist.go`.
- [x] Add Go tests for valid specs, limits, errors, canonical output, narrow
      command parsing, and the artifact surviving proto round-trip.
- [x] Add the end-to-end run test (`tests/agent/visualize_run_test.go`): the
      command activates the tool, the live event and the persisted row carry
      the artifact, and the provider only sees the summary.
- [x] Keep `visualize` out of the subagent tool source.

**Verify:**

```sh
cd apps/server-go && go test ./internal/services/visualize/ -v
cd apps/server-go && go test ./internal/run/ -run 'Visualize' -v
cd apps/server-go && go test ./internal/agent/loop/ -run 'Visual' -v
cd apps/server-go && go test ./tests/agent/ -run 'TestVisualize' -v
```

### Phase 2 — Android renderer

- [x] Add `VisualArtifact` to `ToolResult` and decode `VisualPart` in
      `MessageConverters.kt`.
- [x] Add typed spec models and defensive decoding.
- [x] Add `VisualResultCard`, metric/table/chart renderers, and geometry helpers.
- [x] Render visuals from `RunActivity` outside the fold.
- [x] Add the `VisualSeries` palette to `ThemeNew`.
- [x] Add `VisualFixtureTest` decoding the shared fixtures and testing scale/tick
      edge cases (flat series, negative values, single point, 2,000 points).
- [x] Update `AssistFixtureTest` for the new command count.

**Verify:**

```sh
cd apps/android && ./gradlew :app:testDebugUnitTest --tests "com.console.mobile.VisualFixtureTest"
cd apps/android && ./gradlew :app:testDebugUnitTest --tests "com.console.mobile.AssistFixtureTest"
cd apps/android && ./gradlew :app:assembleDebug
```

Manual: line/bar/area/scatter, 1 series and 8 series, negative values, flat
series, 6-column table, font scale 1.3, dark theme, collapse/expand the run.

### Phase 3 — Desktop renderer

- [ ] Add `VisualArtifact` to `console-core` and decode it in `from_proto`.
- [ ] Add `chat/visual.rs` and render visuals from `ToolCalls` outside the fold.
- [ ] Add chart colors to both themes.
- [ ] Add core/proto fixture tests.

**Verify:**

```sh
cd apps/desktop && cargo test -p console-core visual
cd apps/desktop && cargo check
```

Manual: same matrix as Android, plus window resize and light theme.

### Phase 4 — Hardening and docs

- [ ] Fallback card for unsupported versions and malformed specs on both clients.
- [ ] Accessibility summaries and table semantics.
- [ ] Update `SKILL.md` with the final grammar, limits, and three worked
      examples (line, dashboard, table).
- [ ] Document the “summary only reaches the provider” rule in the code comments.
- [ ] Record a short manual QA checklist in this plan or the PR description.

---

## 8. Verification matrix

| Concern | Check |
|---|---|
| Spec grammar | Go unit tests for every block kind and every limit. |
| Canonical output | Golden fixture byte comparison after normalization. |
| Activation | `SplitInvocation` tests: `/visualize`, `/visualize foo`, leading whitespace, `/visualizes`, inline mention. |
| Session memory | Go test that a second turn loads the tool without re-invoking. |
| Persistence | Proto round-trip keeps `Visual` and leaves `Content` unchanged. |
| Live wire | Fixture decode of `toolExecutionResult` with a visual. |
| Provider isolation | Test that the provider-facing `Content` is the short summary; the spec is on `Visual` only. |
| End to end | `tests/agent/visualize_run_test.go`: the command activates the tool, the live event and the persisted row carry the artifact, and a session without the command never sees the tool. |
| Android decode | `VisualFixtureTest` against shared fixtures. |
| Android paint | Manual 2,000-point/30-row scroll; `assembleDebug`. |
| Desktop decode | `console-core` / `console-proto` fixture tests. |
| Desktop paint | Manual light/dark and resize; `cargo check`. |
| Accessibility | TalkBack reads the chart summary and table headers. |
| Failure | Unsupported version and malformed spec show fallback, no crash. |

Success means: `/visualize` appears in autocomplete; a user can type
`/visualize chart our weekly signups` and get a native card; reopening the
session shows the same card; the next model turn sees only a short summary; and
a 2,000-point chart scrolls smoothly on a phone.

---

## 9. Deferred

- Tooltips, hover details, zoom/pan, legend toggles, and brushing.
- Stacked bars and stacked areas, pie/donut, heatmap, candlestick, maps, 3D.
- PNG/SVG export, copy-as-image, share, print, and a saved-visual library.
- Streaming partial charts (the tool returns one complete artifact).
- User-editable visuals or “edit this chart” affordances.
- Remote images, SVG, HTML, or Markdown inside a visual.
- Arbitrary code/JS/Luau visuals and a client-side sandbox.
- Visuals from subagents.
- Always-on `visualize` if `/visualize` discoverability proves too weak.
- Server-side downsampling; start with a clear limit and aggregate in the model.
- A durable visual library independent of transcript compaction.
