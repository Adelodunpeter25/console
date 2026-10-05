# Shared Protobuf Schema

Single source of truth for every type that crosses the wire between the Go
server, the GPUI desktop app, and the Android app. Transport stays HTTP + SSE;
payloads become **canonical protobuf JSON** (`protojson`). gRPC is explicitly
out of scope, but this keeps it a small step later.

---

## 1. Problem

The same shapes are hand-maintained three times:

| Side    | Location                                         | Notes                                       |
|---------|--------------------------------------------------|---------------------------------------------|
| Go      | `internal/types` (468 lines) + `fiber.Map`       | The real surface is ~189 `fiber.Map` sites, in `routes/` and `agent/` |
| Rust    | `apps/desktop/crates/console-core/src/types`     | 2.5k lines, serde, 8 `tag = "type"` enums          |
| Kotlin  | `apps/android/.../data/model`                    | ~910 lines, kotlinx.serialization                  |

Adding or renaming a field means three edits; missing one fails silently as
a JSON default/null. The drift is already measurable: Android's
`EventModels.kt` declares `compactedMessageCount`, `tokensBefore`,
`tokensAfter` and `reason`, which the server has never sent, while the Rust
event enum is missing fields Android does read.

`internal/types` itself is small and mostly dissolves into generated code.
Size the phases off the `fiber.Map` count, not off that directory.

## 2. Decision

- Schemas live in `.proto` files; code for all three apps is generated.
- Wire format is **canonical protojson**, not today's hand-shaped JSON. We
  accept the shape changes (see §5) and migrate clients domain by domain.
- Field names: proto `snake_case` → JSON `lowerCamelCase` (protojson default,
  matches what clients already use).

## 3. Layout & Tooling

```
proto/
├── buf.yaml              # module + lint/breaking rules
├── buf.gen.yaml          # codegen for go only
└── console/v1/
    ├── common.proto      # shared primitives (ids, timestamps, errors)
    ├── settings.proto
    ├── project.proto
    ├── fs.proto
    ├── git.proto
    ├── session.proto
    ├── message.proto     # messages, content parts, tool calls/results
    └── events.proto      # AgentSessionEvent and other SSE frames
```

- **buf** (`buf generate`, `buf lint`, `buf breaking`) drives the `.proto`
  files themselves and the Go generator. Rust and Kotlin generate from
  `proto/` by their own build steps, so `buf` is not involved for them.
- Only Go output is **committed**; Rust and Kotlin are generated at build
  time. The table and the three notes below give the reasons:

| Target  | Generator                                                              | Output                                                | Committed |
|---------|------------------------------------------------------------------------|-------------------------------------------------------|-----------|
| Go      | `protoc-gen-go` as a **buf remote plugin**                          | `apps/server-go/internal/gen/consolev1/`              | yes       |
| Rust    | `build.rs` in `console-proto` running `prost-build` + `pbjson-build`   | `$OUT_DIR`                                            | no        |
| Kotlin  | `wire-gradle-plugin` (`sourcePath { srcDir "<repo>/proto" }`)          | `app/build/generated/source/wire` (auto-registered)   | no        |

**Go.** Use a buf *remote* plugin in `buf.gen.yaml` so `make proto` needs
nothing but `buf` — no `go install protoc-gen-go`, no local toolchain drift.
`google.golang.org/protobuf` v1.36.12 is already in `go.mod` as an indirect
dependency, so this is a promote to direct plus the generator.

**Rust — decided: `build.rs`, not committed.** `pbjson-build` is a library
API rather than a protoc plugin, so its serde impls can only be produced from
a `build.rs`. Committing the `prost` structs while generating only the serde
impls would require a two-step descriptor dance for no benefit. Configure the
`build.rs` with `protoc-bin-vendored` so `cargo build` needs no system
`protoc`, and emit `cargo:rerun-if-changed` for every `.proto` file. Also
`.ignore_unknown_fields()` on the builder — see §6.

**Kotlin — decided: Wire only, drop the buf Kotlin target.** Wire reads
`.proto` directly, so running buf's Kotlin plugin alongside it would mean two
generators disagreeing over one language. Rejected alternative:
`protoc-gen-kotlin` + `protobuf-java-util` `JsonFormat` is canonical and
keeps a single JSON library, but it emits `oneof` as flat nullable fields
rather than a union, so every reducer needs hand-written wrappers. The
decision to use Wire still rests on two spikes (see §7), because both
`org.gradle.configuration-cache=true` in `apps/android/gradle.properties`
and Wire's plugin mutating the task graph through its
`protoPath`/`protoSource` configurations are known friction, and Wire's
generated sources may need `wire { prune }`/`root` — per Wire's own docs, "R8
and ProGuard have difficulty shrinking Wire-generated sources" — while your
release build has `isMinifyEnabled` and `isShrinkResources` both on.

Set `wire { kotlin { oneofMode = "sealed_class" } }`. It turns each proto
`oneof` into a Kotlin `sealed class` with a data class per field, which is
the shape `core/chat/ChatModels.kt`'s `ActivityEvent` already is — that is
what makes the Phase 3 and Phase 4 `when` rewrites mechanical.

Wire's JSON layer is Moshi, so from Phase 1 the app carries a second JSON
library alongside kotlinx.serialization. They do not overlap — kotlinx for
persisted and non-wire types, Moshi for wire types — but it is a real cost,
not a footnote.

- `make proto` regenerates Go only.
- CI: a **separate `proto` job** on `ubuntu-latest` that `build` depends on —
  not a step inside the build matrix, which would run it 5× (including on a
  macOS runner). The job runs `buf lint`, `buf breaking`, and
  `make proto && git diff --exit-code`.
  It needs `fetch-depth: 0`: `actions/checkout@v4` defaults to a 1-commit
  fetch, so `.git#branch=main` would not exist to break against.
- No workflow runs tests today (no `go test`, `cargo test`, or `./gradlew
  test` anywhere in `.github/workflows`). This job is also the place to run
  the golden-fixture tests from §8, otherwise they are unenforced.
- Android CI is `workflow_dispatch` only, so Kotlin codegen drift is never
  caught automatically; this Go/Rust job is the only automatic gate.

## 4. Conventions

- Package `console.v1`. Breaking changes go to `v2`, never edited in place.
- Never reuse or renumber field tags; `reserved` removed ones.
- Use `optional` for fields where "unset" ≠ zero value.
- Timestamps: `int64` epoch ms (serialized as JSON string — see §5) unless a
  domain truly needs `google.protobuf.Timestamp`.
- Free-form blobs split by how open they actually are:
  - **Tool arguments, tool result content, subagent `args`** — `bytes`
    carrying UTF-8 JSON. This is what they are today, it costs nothing, and
    it avoids a conversion helper at every access site. `console-core/src/types`
    touches `serde_json::Value` in 9 places and Android uses `JsonElement`;
    routing those through `google.protobuf.Struct` would need `pbjson-types`
    plus a `struct_to_json`/`json_to_struct` pair on Rust, `structpb.Struct` on
    Go, and `com.google.protobuf.Struct` on Kotlin — three conversions and a
    `args["path"].as_str()` rewrite in GPUI code, for shapes that were never
    schema'd in the first place.
  - **Genuinely open configuration, i.e. MCP server config** —
    `google.protobuf.Struct` / `Value`, where a schema would help.
- **Every JSON reader must tolerate unknown fields.** Add this as a standing
  rule now, because the three runtimes disagree by default and that
  disagreement is the single most likely way this migration breaks in
  production (see §6):
  - Go: `protojson.UnmarshalOptions{DiscardUnknown: true}` on every
    client-facing read.
  - Rust: `pbjson_build::Builder::ignore_unknown_fields()`.
  - Kotlin: Wire ignores unknown fields by default — do not "tighten" it.
- **Generated code owns the wire, never durable storage.** Two clients persist
  these exact shapes to disk: `data/local/ChatPersistence.kt` writes
  `AgentMessage` / `RunActivityState` / `ImageAttachment` into
  SharedPreferences under `PERSIST_VERSION = 1`, and
  `src/persistence/workspace_state.rs` serialises the saved workspace layout
  (which is built from 2 of the 8 tagged enums in `src/types/workspace.rs`).
  Replacing those types with generated ones silently orphans users' chat
  history and layout. Keep persisted types hand-written, add explicit
  converters, and never let codegen own the on-disk format.
- Enums: `STATUS_UNSPECIFIED = 0` first; values prefixed with the enum name.
- Each endpoint gets explicit `XxxRequest` / `XxxResponse` messages. Shared
  error envelope in `common.proto` replaces `fiber.Map{"success":..,"error":..}`.

## 5. Wire-Shape Changes to Expect

| Today                                    | protojson                                     |
|------------------------------------------|-----------------------------------------------|
| `{"type":"turnStart","prompt":"…"}`      | `{"turnStart":{"prompt":"…"}}` (`oneof`)      |
| `"running"`                              | `"STATUS_RUNNING"`                            |
| `123` for int64                          | `"123"` (string)                              |
| zero/empty fields present                | omitted                                       |

**Marshal with default options. Do not use `EmitUnpopulated`.** The
alternative — `EmitUnpopulated: true`, "so clients don't trip on missing
keys" — was considered and rejected: both clients handle missing keys
natively (prost maps them to `None`/zero, Wire to `null`/defaults), so it
buys nothing and it costs real money. Measured on generated Go types for one
model-stream frame (the per-token path), 3 runs:

| | ns/op | B/op | allocs/op |
|---|---|---|---|
| `protojson` canonical | 2800–4900 | 760 | 19 |
| `protojson` `EmitUnpopulated` | 5100–5800 | 1112 | 23 |
| `encoding/json` struct (today's fast path) | 1400–1800 | 152 | 4 |
| `fiber.Map` + `encoding/json` (today) | 2100–2500 | 176 | 8 |

So `EmitUnpopulated` is ~1.7× the cost of canonical output on the hottest
frame in the system, and it emits `"part": null` for unset message fields —
the least canonical thing protojson can produce. (Absolute numbers are from
a 2019 i7 and are noisy; the ratios hold.)

Canonical omission is also a **cleanup**. `routes/run.go` carries
`nonNilTodos`, `nonNilCalls` and `nonNilResults` purely so array fields come
out as `[]` and never `null`. Omitted is fine for both clients, so all three
go away in the phase that migrates them.

SSE framing (`event:` / `data:` in `routes/sse.go`) is unchanged; only the
`data` payload encoding changes. `sseStream.SendJSON` gets a proto-aware
variant (`SendProto`) that uses `protojson` into the same buffered writer.

## 6. Compatibility

Remote servers and Android apps can be on different versions.

**The failure mode is strictness, not shape.** All three runtimes' JSON
readers reject unknown fields by default — `protojson.Unmarshal` errors
unless `DiscardUnknown: true`, `pbjson-build` errors unless
`.ignore_unknown_fields()`, and Wire ignores them. So the first time a
migrated server sends a field an older client doesn't know about, the client
errors *during parse*, before any version notice can render. The §4 rules are
what make the notices below possible at all: with them, an unexpected field
is ignored rather than fatal.

Each migrated endpoint therefore:

1. Bumps an API version advertised by the server via `GET /api/version` →
   `{"apiVersion": N}`. **This endpoint does not exist today** — it is Phase
   0 work, not existing infrastructure, and Android's `ConnectionSettings`
   must call it on connect *before* any other request or POST.
2. Clients that see a server whose `apiVersion` is newer than they know about
   show an "update server" notice rather than mis-parsing. No dual-format
   serving — keeps the server simple.
3. **The gate runs before the first decode, not after the first failure.**

**One global integer, exact match.** Bumping `apiVersion` per phase means an
older Android app refuses to connect for *every* endpoint once any phase
ships, including the ~80% that never changed shape. That is the deliberate
trade: one integer and one check instead of a version map, at the cost of
each phase being a coordinated Android release. It is defensible because
desktop and server ship from the same repo and can never be mismatched, and
Android CI only runs on manual dispatch anyway. If that cost ever starts
hurting, the escape hatch is a per-domain version map
(`{"apiVersion": 3, "sessions": 3, "fs": 2}`) so a client can disable only
the domain it cannot parse.

Desktop and server ship from the same repo, so they move together; Android is
the one to watch.

## 7. Phases

Each phase = one domain migrated end-to-end (proto → Go → Rust → Kotlin),
hand-written types for that domain deleted in the same change.

### Phase 0 — Scaffolding
- Add `proto/`, `buf.yaml`, `buf.gen.yaml`, `make proto`, CI check (per §3:
  separate `proto` job, `fetch-depth: 0`, golden-fixture tests).
- Create `console-proto` crate with its `build.rs` (`prost-build` +
  `pbjson-build` + `protoc-bin-vendored`, `.ignore_unknown_fields()`), Go `gen`
  package, Wire setup in Gradle.
- **Two spikes to run before anything else lands**, both on Kotlin, since
  either could invalidate the Wire choice:
  1. Wire + R8: generate one message with a `oneof`, an `int64`, a `Struct`
     and an enum, then `assembleRelease` (not just debug) with
     `isMinifyEnabled`/`isShrinkResources` on, and configure
     `wire { prune }`/`root` if anything survives it.
  2. Wire + configuration cache: `org.gradle.configuration-cache=true` is
     already set in `apps/android/gradle.properties`, and Wire's plugin
     mutates the task graph through `protoPath`/`protoSource`.
- Create `apps/android/tests/` — `app/build.gradle.kts` already declares
  `java.srcDirs("../tests", ...)` but the directory does not exist, and §8's
  Android fixture test needs it.
- `common.proto` with error envelope + the API version endpoint (§6).
- Exit: all three apps build with generated code linked but unused.

### Phase 1 — Low-risk, request/response only
`settings`, `project`, `favorites`, `ports`, `usage`.
- Replace `fiber.Map` responses in those route files with proto messages.
- Rust services in `console-core/src/services/*` switch to `console_proto` types.
- Android repos in `data/repo` switch to Wire types.
- Exit: the pattern (handler helper, client decode helper) is proven and documented here.

### Phase 2 — Filesystem, git, scripts, terminal metadata
`fs`, `git`, `worktrees`, `project_scripts`, `terminal` (control messages only;
raw PTY bytes stay as they are).
- Includes the low-rate SSE streams (fs watch, git, ports, scripts).

### Phase 3 — Sessions & messages
`session`, `message` (content parts, tool calls/results, todos).
- Biggest Rust/Kotlin surface; content-part unions become `oneof`.
- **Durable storage is the hard part here, not the wire.** `AgentMessage` is
  written to SharedPreferences via `ChatPersistence.kt` under
  `PERSIST_VERSION = 1`. Decide explicitly: either keep a hand-written
  kotlinx `@Serializable` `AgentMessage` for persistence and convert to/from
  the Wire type at the boundary, or bump `PERSIST_VERSION` and discard the
  on-device cache (losing up to 25 cached sessions / 50 messages per
  session). Do not silently inherit either outcome.
- `subagentWire` in `routes/run.go` round-trips a payload through
  `map[string]any` just to inject a `"type"` key; that whole helper
  disappears once subagent frames are a `oneof`.

### Phase 4 — Agent event stream
`events.proto` — `AgentSessionEvent`, model stream parts, permission / ask /
browser-action requests, subagent events.
- Hot path: benchmark `SendProto` vs current `SendJSON` per-token cost before
  switching; keep the batched-flush behavior. §5's measurements say expect
  roughly 1.5–2× the time and 4× the bytes of today's struct.
- **Keep the per-token frame on hand-rolled JSON.** `routes/run.go` already
  builds `modelStreamPartFrame` as a struct rather than a `fiber.Map`, with a
  comment explaining that `encoding/json` sorts map keys on every call. That
  deliberateness survives: `modelStreamPart` is the one frame emitted per
  token and it stays hand-built, while every other frame goes through
  `SendProto`. Add a golden test asserting the hand-built bytes are identical
  to `protojson` output for the same message, so the fast path cannot drift
  from the schema.
- Update desktop chat reducers and Android `data/stream` to match on `oneof`.
- Phase 0's decision to build with `.ignore_unknown_fields()` is what lets an
  old client survive a new event variant — re-verify it here, since this is
  where an extra field would first appear.

### Phase 5 — Cleanup
- Delete remaining hand-written wire types and the old `SendJSON` path (the
  per-token frame and `SendProto` excepted).
- Note in this doc which internal (non-wire) Go types intentionally remain,
  and which persisted client types were kept hand-written on purpose (§4).

## 8. Verification per Phase

- Go: `cd apps/server-go && go test ./tests/<area>/ -run <Test> -v`, plus a
  golden-JSON test per migrated message.
- Rust: `cd apps/desktop && cargo test -p console-core <test>` decoding the
  same golden JSON fixtures. Put the tests in
  `crates/console-core/tests/<area>/` and add a `[[test]]` entry — AGENTS.md
  forbids inline `#[cfg(test)]` modules, and the existing suite is already
  wired that way.
- Android: `cd apps/android && ./gradlew :app:assembleDebug`, **plus
  `:app:assembleRelease`** whenever generated types change. Debug-only misses
  exactly the failure Wire would cause, since R8 only runs on release
  (release signing needs `key.properties`, so this is a manual or
  workflow-dispatch step rather than every run). Add a JVM unit test decoding
  the golden fixtures into `apps/android/tests/` (folder created in Phase 0).
- Golden fixtures live in `proto/testdata/<domain>/*.json`, shared by all three.
- **None of this runs in CI today.** No workflow in `.github/workflows`
  invokes `go test`, `cargo test`, or `./gradlew test` — the Go and desktop
  workflows only build and publish, and Android only runs on
  `workflow_dispatch`. Until the §3 `proto` job runs these, they are
  developer discipline only.

## 9. Out of Scope

- gRPC / Connect transport (possible later; schemas will already exist).
- Binary protobuf on the wire.
- Changing the PTY byte stream or SSE framing.
