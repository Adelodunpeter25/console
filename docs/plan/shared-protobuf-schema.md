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
| Go      | `apps/server-go/internal/types` + inline structs | Many responses are ad-hoc `fiber.Map` (~150) |
| Rust    | `apps/desktop/crates/console-core/src/types`     | ~2.5k lines, serde, 7 `tag = "type"` enums   |
| Kotlin  | `apps/android/.../data/model`                    | ~800 lines, kotlinx.serialization            |

Adding or renaming a field means three edits; missing one fails silently as
a JSON default/null.

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
├── buf.gen.yaml          # codegen for go / rust / kotlin
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

- **buf** (`buf generate`, `buf lint`, `buf breaking`) drives everything.
- Generated code is **committed** (no protoc needed to build any app):

| Target  | Generator                                   | Output                                             |
|---------|---------------------------------------------|----------------------------------------------------|
| Go      | `protoc-gen-go` + `protojson`                | `apps/server-go/internal/gen/consolev1/`           |
| Rust    | `prost` + `pbjson-build` (serde impls)       | new crate `apps/desktop/crates/console-proto/`     |
| Kotlin  | Square **Wire** (+ its Moshi JSON adapter)   | `apps/android/app/src/main/kotlin/.../data/proto/` |

Rust option: generate via `build.rs` in `console-proto` instead of committing,
if we'd rather not check in generated Rust. Decide in Phase 0.

Kotlin note: Wire's JSON adapter follows protojson. If it proves awkward next
to kotlinx.serialization, the fallback is `protoc-gen-kotlin` +
`protobuf-java-util` `JsonFormat` (heavier, but canonical).

- `make proto` regenerates all targets.
- CI (`console-server.yml`) adds a step: `buf lint`, `buf breaking --against main`,
  and `make proto && git diff --exit-code` so stale generated code fails the build.

## 4. Conventions

- Package `console.v1`. Breaking changes go to `v2`, never edited in place.
- Never reuse or renumber field tags; `reserved` removed ones.
- Use `optional` for fields where "unset" ≠ zero value.
- Timestamps: `int64` epoch ms (serialized as JSON string — see §5) unless a
  domain truly needs `google.protobuf.Timestamp`.
- Free-form maps/blobs (tool args, MCP config): `google.protobuf.Struct` /
  `Value`, not `string`-encoded JSON.
- Enums: `STATUS_UNSPECIFIED = 0` first; values prefixed with the enum name.
- Each endpoint gets explicit `XxxRequest` / `XxxResponse` messages. Shared
  error envelope in `common.proto` replaces `fiber.Map{"success":..,"error":..}`.

## 5. Wire-Shape Changes to Expect

| Today                                    | protojson                                     |
|------------------------------------------|-----------------------------------------------|
| `{"type":"turnStart","prompt":"…"}`      | `{"turnStart":{"prompt":"…"}}` (`oneof`)      |
| `"running"`                              | `"STATUS_RUNNING"`                            |
| `123` for int64                          | `"123"` (string)                              |
| zero/empty fields present                | omitted unless `EmitUnpopulated: true`        |

Server marshals with `protojson.MarshalOptions{EmitUnpopulated: true}` during
migration so clients don't trip on missing keys; revisit later.

SSE framing (`event:` / `data:` in `routes/sse.go`) is unchanged; only the
`data` payload encoding changes. `sseStream.SendJSON` gets a proto-aware
variant (`SendProto`) that uses `protojson` into the same buffered writer.

## 6. Compatibility

Remote servers and Android apps can be on different versions. Each migrated
endpoint therefore:

1. Bumps an API version advertised by the server (e.g. `GET /api/version` →
   `{"apiVersion": N}`), which clients already need to check on connect.
2. Clients that see an older server show an "update server" notice rather
   than mis-parsing. No dual-format serving — keeps the server simple.

Desktop and server ship from the same repo, so they move together; Android is
the one to watch.

## 7. Phases

Each phase = one domain migrated end-to-end (proto → Go → Rust → Kotlin),
hand-written types for that domain deleted in the same change.

### Phase 0 — Scaffolding
- Add `proto/`, `buf.yaml`, `buf.gen.yaml`, `make proto`, CI check.
- Create `console-proto` crate, Go `gen` package, Kotlin Wire setup in Gradle.
- `common.proto` with error envelope + API version endpoint.
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

### Phase 4 — Agent event stream
`events.proto` — `AgentSessionEvent`, model stream parts, permission / ask /
browser-action requests, subagent events.
- Hot path: benchmark `SendProto` vs current `SendJSON` per-token cost before
  switching; keep the batched-flush behavior.
- Update desktop chat reducers and Android `data/stream` to match on `oneof`.

### Phase 5 — Cleanup
- Delete remaining hand-written wire types and the old `SendJSON` path.
- Remove `EmitUnpopulated` if clients handle omitted fields.
- Note in this doc which internal (non-wire) Go types intentionally remain.

## 8. Verification per Phase

- Go: `cd apps/server-go && go test ./tests/<area>/ -run <Test> -v`, plus a
  golden-JSON test per migrated message.
- Rust: `cd apps/desktop && cargo test -p console-core <test>` decoding the
  same golden JSON fixtures.
- Android: `cd apps/android && ./gradlew :app:assembleDebug`; add a JVM unit
  test decoding the golden fixtures.
- Golden fixtures live in `proto/testdata/<domain>/*.json`, shared by all three.

## 9. Out of Scope

- gRPC / Connect transport (possible later; schemas will already exist).
- Binary protobuf on the wire.
- Changing the PTY byte stream or SSE framing.
