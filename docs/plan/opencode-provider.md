# Direct OpenCode Zen Provider for the Go Server

Status: **in progress**.

## Goal

Add an `opencode` provider to `apps/server-go` that talks directly to the
OpenCode Zen HTTP API, without requiring the user to install or run the
OpenCode CLI. The provider must use the existing Go agent loop, including its
tool registry and executor, so OpenCode models can call the same tools as the
other providers.

The initial target is `space-bunny-free`, with the implementation designed for
all currently available Zen free models. The model catalog is discovered from
`GET /zen/v1/models`; the selected model is checked against that list before an
inference request is sent.

## Constraints and decisions

- Keep the implementation inside `apps/server-go`; do not resurrect the old
  `opencode serve` sidecar.
- Use `https://opencode.ai/zen/v1` as the default base URL, with a testable
  `BaseURL` override on the provider.
- Use the `loop.Provider` interface. The agent loop and run service should not
  need provider-specific branches.
- Do not add an API-key or OAuth flow. Zen free-tier requests use the public
  client identity described below.
- Preserve the provider boundary if Zen later changes its free-tier gate. The
  compatibility headers and tool shim must stay isolated in the OpenCode
  package.
- A model-list request is a preflight requirement, not merely a UI catalog
  refresh. `RunTurn` must check the selected model through the model-list
  endpoint before posting an inference request.

## What the TypeScript implementation established

The removed TypeScript provider was initially added in
`46d5619c20cd200dc266a50b84668b5d23e8dd5e` and removed in
`5301967a0e0f4ad28e1a427c7169cf1b9789e275`. The important history is:

| Commit | Behavior established |
| --- | --- |
| `46d5619c` | Direct Zen base URL, free-model discovery, Chat Completions conversion, SSE parsing, provider registration, and tests. |
| `4827da7f` / `332d1cab` | Refined the AI SDK message and tool-result conversion. |
| `cb2813b1` | Avoided duplicate tool calls when the SDK also emitted a complete `tool-call` event. |
| `77e9c198` | Split Models between Chat Completions and Responses. |
| `fe0d2601` | Added the OpenCode client User-Agent needed by the free tier. |
| `49c68675` | Added a stable `x-opencode-session` header. |
| `a49e1743` | Flattened tool-result content for the wire. |
| `570b5ae0` | Added Responses prompt-cache controls and honest cache usage reporting. |
| `86bb7426` | Added a `(continue)` fallback for empty/thinking-only history. |
| `5301967a` | Removed the direct provider and local `opencode serve` sidecar. |

The TypeScript split selected `/v1/responses` for Muse, GPT-5, and Grok model
families and `/v1/chat/completions` for the other models. The Go implementation
must preserve that model-based routing, but must not copy the TypeScript AI SDK
stream behavior literally: the Go loop treats every `EventToolCall` as a
separate executable call.

## Current OpenCode free-tier request contract

The current Zen request behavior is undocumented and may change, so these
values should be centralized in one constants file and covered by tests:

```text
Base URL:              https://opencode.ai/zen/v1
Authorization:         Bearer public
User-Agent:            opencode/latest/2.0.15/cli
x-opencode-client:     cli
x-opencode-session:    ses_...
```

The session header must be stable for the Go process. Read
`OPENCODE_SESSION_ID` when configured; otherwise generate one `ses_`-prefixed
identifier once at process startup. Do not use the current run's
`ConversationID` directly as the OpenCode session header: the run identity is
currently composed as `sessionID:providerID:modelID` and is better suited to
Responses prompt-cache identity.

The model-list and inference requests should use the same header builder so
that a model cannot be discoverable through one identity and rejected when
inference uses another. The old optional headers (`x-opencode-project`,
`x-session-affinity`, `x-session-id`, `b3`, and `traceparent`) were observed in
the CLI request but were not required by the direct-request test and should
not be added unless a future server response proves they are required.

## Target package layout

Add the following package and keep provider-specific wire logic out of the
agent loop:

```text
apps/server-go/internal/providers/opencode/
├── constants.go       # URL, headers, session ID, defaults, model policy
├── models.go          # GET /models, free-model filtering, static fallback
├── messages.go        # loop history -> Chat/Responses request messages
├── tools.go           # tool conversion and free-tier compatibility shim
├── stream.go          # HTTP, model routing, SSE parsing, loop.Event emission
└── *_test.go or focused tests under tests/providers/
```

Reuse `internal/providers/shared` for generic SSE parsing and Responses
function-call reassembly. The existing `shared.Accumulator` is designed for
Responses events keyed by `item_id` and `call_id` and should be used rather
than duplicating out-of-order handling.

## Provider registration and catalog

### Registry

Update `apps/server-go/internal/providers/registry.go` so `Lookup("opencode")`
returns `*opencode.Provider{}`. The returned provider must be usable without
credentials and must default to the live Zen base URL.

### Catalog

Update `apps/server-go/internal/providers/catalog.go` with:

- provider id `opencode`;
- display name `OpenCode Zen`;
- `AuthMethod: "none"`;
- a default context window of `200_000` unless the model-list response provides
  a better value;
- no image support and no selectable thinking levels initially;
- `DefaultOpenCodeModels()` for offline startup and discovery failure, with
  only `space-bunny-free` as the fallback;
- `OpenCodeModels(ctx)` for live discovery with a bounded timeout and a
  fallback to only the stealth model.

Model discovery should:

1. issue `GET {baseURL}/models` with the shared OpenCode headers;
2. accept model ids ending in `-free` and the special `big-pickle` id;
3. preserve server order, or sort deterministically if the existing catalog
   requires stable ordering;
4. return an empty result plus an error on HTTP, decode, or empty-list failure;
5. let the catalog route use the static fallback on that error.

The complete catalog must come from `GET {baseURL}/models`; do not maintain a
second hardcoded list in the server. The only offline fallback is the stealth
model `space-bunny-free`, so a temporary discovery failure still leaves one
usable selection.

### Provider route

Add an `opencode` case to
`apps/server-go/internal/routes/providers.go` for
`GET /api/providers/opencode/models`, using the same favorite sorting as the
other providers. No OAuth, credential, or usage route is required.

Also update any provider count assertions in
`apps/server-go/tests/providers/catalog_test.go`; the current test assumes
there are exactly three catalog providers.

## Request construction

### Shared tool conversion

`req.Tools` contains definitions produced by
`tools.Registry.Definitions()`. Convert each real definition to the correct
OpenCode wire shape:

- Chat Completions: `{type: "function", function: {name, description,
  parameters}}`;
- Responses: `{type: "function", name, description, parameters}`.

Use the existing JSON schema maps directly, with a default object schema when a
definition has no schema. Keep the real harness tool names and schemas so the
model can call `read_file`, `editFile`, `bash`, `ask`, and the other registered
tools.

### Free-tier compatibility tool catalog

Zen currently rejects a direct request unless the request contains these exact
case-sensitive tool names:

```text
edit
glob
grep
question
read
shell
```

This gate is based on the tool catalog, not on the User-Agent alone. The
OpenCode provider must prepend the six minimal compatibility definitions to
the wire tool list. Their descriptions and schemas may be intentionally
minimal; they are only there to satisfy the Zen free-tier compatibility check.

The compatibility definitions are provider-facing only. Do not register them
in the global Go `tools.Registry` and do not give them executable harness
implementations. Merge the real definitions by name so there are no duplicate
tools in the request:

- keep the real `glob` and `grep` definitions once, since those names are also
  present in the Go harness;
- use minimal shims for `edit`, `question`, `read`, and `shell`;
- append every other real `req.Tools` definition, including custom tools.

A compatibility name returned by the model must not accidentally execute an
unrelated registered tool. Return a controlled model-visible error for a
shim-only call, while forwarding calls whose names match the real harness
definitions unchanged.

The six shims must be isolated to OpenCode requests. Codex, Claude, and
Antigravity must continue sending their original tool lists unchanged.

### Message conversion

Implement a converter that accepts both typed `loop` messages and decoded map
messages from session history. It must handle:

- user text;
- user image attachments as data URLs;
- assistant text;
- assistant tool calls;
- tool results and their error state;
- plain `ThinkingPart` content, which must be omitted from the outgoing wire
  message because third-party reasoning text is not a valid Responses input
  item;
- empty or thinking-only history.

Empty history must become a single user message with text `(continue)` so the
provider never sends an empty message array.

For Chat Completions:

- user content uses OpenAI-compatible text/image content parts;
- assistant text becomes `content`;
- assistant tool calls become `tool_calls` entries with
  `{id, type: "function", function: {name, arguments}}`;
- each tool result becomes a `role: "tool"` message with `tool_call_id` and a
  stringified output.

For Responses, use Codex-compatible input items:

- user text and images use `input_text` and `input_image`;
- assistant text uses `output_text`;
- assistant tool calls use `function_call`;
- tool results use `function_call_output`.

Normalize string, array, map, and error tool results before placing them on
the wire. Empty tool arguments become `{}`.

### System prompt and cache identity

The system prompt becomes a `system` message for Chat Completions and
`instructions` for Responses. The existing `TurnRequest.ConversationID` is
used as the Responses `prompt_cache_key` when cache retention is not `none`;
`CacheLong` may add `prompt_cache_retention: "24h"`.

Chat Completions has no native prompt-cache controls for this integration, so
it must not claim cache participation. Responses usage may report the token
fields returned by Zen.

### Model-list preflight

At the beginning of `RunTurn`:

1. build the shared request headers;
2. call `GET {baseURL}/models` with a short context timeout;
3. find `req.Model` in the free-model list;
4. fail the event stream with a useful error if it is absent;
5. only then build and send the inference request.

The preflight must be covered by an `httptest.Server` that handles both
`/models` and the selected inference path. Do not silently turn a missing
model into a successful inference request, because the server's free-tier
error is less actionable than a model-catalog error.

## Endpoint routing

Centralize routing in a small function so it is independently testable:

```go
func IsResponsesModel(modelID string) bool
```

The initial policy matches the TypeScript implementation:

- true for `muse-*`, `gpt-5*`, and `grok-*`;
- false for all other model ids.

Then select:

- `ResponsesModel` -> `{baseURL}/responses`;
- otherwise -> `{baseURL}/chat/completions`.

Both paths must use the same model-list preflight and free-tier headers. Keep
the policy based on the model id rather than on whether a model happens to
return a particular error.

## Streaming and the Go event contract

`loop.Provider.RunTurn` must call `events.Fail(err)` for request, HTTP, decode,
or stream errors and `events.Complete()` on success. It must not return while
the event stream remains open.

### Chat Completions parser

Use `shared.ParseSSE` and inspect each `choices[0].delta`:

- `content` -> `EventText`;
- `reasoning_content` or the provider's equivalent reasoning field ->
  `EventThinking`;
- `tool_calls[]` -> an internal accumulator keyed by `index`.

Chat tool calls are often fragmented across SSE chunks. Accumulate id, name,
and function arguments until the call is complete, then emit exactly one
`EventToolCall` with a valid `json.RawMessage` argument value. Empty arguments
must become `{}`. Multiple tool-call indexes must remain distinct. Never emit
one `EventToolCall` per argument delta: the Go loop executes every received
tool-call event.

Capture the final usage chunk when available, push one `EventUsage` before
completion, and report `CacheUnsupported` for this wire type when no native
cache data is available.

### Responses parser

Mirror the existing Codex Responses parser in
`internal/providers/codex/stream.go`:

- `response.output_text.delta` and `response.refusal.delta` -> text;
- `response.reasoning_summary_text.delta` and
  `response.reasoning_text.delta` -> thinking;
- `response.output_item.added` for `function_call` -> register the call;
- `response.function_call_arguments.delta` -> append arguments;
- `response.function_call_arguments.done` -> finalize the call;
- `response.completed` and `response.incomplete` -> capture usage and inspect
  incomplete errors;
- `response.failed` -> fail the stream;
- error events -> fail the stream with the provider message.

Use `shared.Accumulator` and emit each finalized call only once. Flush
unfinalized calls after the stream ends, emit usage before `Complete` or
`Fail`, and preserve cancellation through the request context.

The OpenCode provider emits `loop.Event` values only. It must not write Fiber
SSE frames itself; `internal/run/turns.go` already translates provider events
into the canonical desktop/mobile event vocabulary.

## Execution through the existing harness

No new tool execution path is needed:

1. the run service builds the normal `tools.DefaultTools()` list;
2. the provider receives those definitions in `TurnRequest.Tools`;
3. the OpenCode adapter appends real definitions to its wire request after
   the compatibility shim;
4. returned real calls become `loop.EventToolCall`;
5. `loop.Agent` persists the assistant message and invokes the existing
   executor;
6. tool results are persisted and sent back on the next provider turn.

The OpenCode adapter must not own permissions, file changes, question routing,
background jobs, memory, or subagents. Those remain in the current run service
and tool executor.

## Error handling and observability

- Wrap non-2xx responses with provider name, status, and a bounded response
  body, using the existing `shared.HTTPError` style.
- Include the model-list preflight failure separately from an inference
  failure so users can tell a missing model from a provider outage.
- Treat `403 FreeTierError` as a compatibility-gate regression and include
  the OpenCode endpoint/model context without logging credentials (there are
  no credentials beyond the public identity).
- Preserve context cancellation and stop retrying after the request is
  canceled.
- Do not retry the two known currently failing free models indefinitely; the
  provider should surface the upstream error and allow the user to select
  another model.

## Focused tests

Add `apps/server-go/tests/providers/opencode_provider_test.go` and extend the
catalog tests. Use `httptest.Server` and inspect captured request headers and
JSON bodies.

### Constants and discovery

- model-list path is `/models`;
- all required headers are present, including the `ses_` session prefix;
- `Authorization` is exactly `Bearer public`;
- free suffix and `big-pickle` filtering;
- malformed/empty responses fall back to static models;
- `OpenCodeModels` has a timeout and no credential requirement.

### Request conversion

- user text and image attachment shapes;
- assistant text and tool calls;
- tool-result stringification and error results;
- thinking-only history produces no invalid assistant message;
- empty history produces `(continue)`;
- real tool definitions are present after the compatibility definitions;
- no duplicate `glob` or `grep` names;
- Chat and Responses tool wire shapes are correct.

### Chat provider

- route to `/chat/completions`;
- no CLI process or sidecar is started;
- text and reasoning deltas become the correct loop events;
- fragmented tool arguments become one valid call;
- multiple tool-call indexes remain distinct;
- usage is emitted before `Complete`;
- non-2xx and malformed streams call `Fail`.

### Responses provider

- route `muse-`, `gpt-5*`, and `grok-` models to `/responses`;
- route other models to `/chat/completions`;
- text, reasoning, and function-call events are normalized;
- out-of-order function-call deltas are reassembled;
- incomplete and failed responses call `Fail`;
- Responses cache controls and usage normalization match the existing Codex
  behavior.

### Registry and route

- `providers.Lookup("opencode")` succeeds;
- `ListProviders()` includes the provider with `AuthMethod: "none"`;
- `IsCatalogProvider("opencode")` succeeds;
- `GET /api/providers/opencode/models` returns the live list or static fallback.

## Acceptance criteria

The implementation is complete when:

- a user can select `opencode/space-bunny-free` in the existing client without
  installing the OpenCode CLI;
- the server calls `GET /zen/v1/models` before inference and rejects an unknown
  model clearly;
- the request carries the observed public Zen identity headers and a stable
  `ses_...` session header;
- the six compatibility tool names are present exactly as required by the
  current free-tier gate;
- the model's real harness tool calls are executed by the existing Go executor;
- Chat Completions models and Responses models use the correct endpoint;
- streamed text, thinking, tool calls, results, usage, cancellation, and
  errors work through the existing run SSE pipeline;
- discovery and inference have focused tests with no live-network dependency;
- no `opencode serve` sidecar or local CLI process is introduced.

## Follow-up risks

The compatibility gate and exact public identity headers are not a stable
public protocol. If Zen changes them, update only the OpenCode constants/tool
shim and the affected tests; the Go agent loop and existing providers should
remain unchanged. Live model availability also changes independently of this
adapter, which is why the catalog must remain discovery-driven and fall back
without breaking offline startup.
