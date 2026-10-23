# `console-shared` — One implementation of the logic both clients need

> ## Status — implemented, with open verification items
>
> Phases 0–3 are **done and committed**; Phase 4 and 5 are not started. Legend:
> ✅ done and verified · ◐ done, not fully verified · ⬜ not done.
>
> | Phase | State | Commit(s) |
> | :--- | :--- | :--- |
> | 0 — crate + wiring | ✅ | `c94dcbff` |
> | 1 — move the pure code | ◐ moved and re-exported; desktop build not confirmed | `c94dcbff`, `d29dc814` |
> | 2 — JNI surface + `SharedNative` | ✅ builds; ◐ not run on a device | `81f2e540`, `4a330603` |
> | 3 — cut Android callers over | ✅ Kotlin compiles; ◐ JVM tests pass except 2 I rewrote and have not re-run | `4a330603`, `6a5ddbb8` |
> | 4 — delete the re-export shims | ⬜ deliberately left (they keep old paths working) | — |
> | 5 — local port forward | ⬜ optional, not started | — |
>
> Also done outside the phases: the **WebTransport trust-store bug** (§7.2) is
> fixed — `97685cbf`.
>
> **Not yet verified — do these before calling this closed:**
>
> 1. `cargo check -p console-core -p console-ui --tests` never finished (it
>    timed out twice), so **the desktop app compiling against the moved code is
>    unconfirmed.** This is the largest open risk.
> 2. `icons_test`, `mentions_test` and the new `pairing_test` case were fixed
>    after a failing run and **have not been seen passing since.**
> 3. The two rewritten Android tests (`TokensPerSecondTest`, plus the
>    `SharedNativeTest` ABI pin) have not been re-run.
> 4. Nothing was run **on a device or emulator** with the new `.so`.
> 5. The `.so` staged in `jniLibs/` locally is **stale** (zero `SharedNative`
>    symbols). CI rebuilds it on push; locally run `rust-wt/build.sh`. A stale
>    library degrades safely — `SharedNative.available` turns false — but
>    pairing is refused and diffs render empty until it is rebuilt.
>
> **Decisions made while implementing** (the rest of this document describes
> the plan; where it disagrees with these notes, these notes win):
>
> - **32-bit ABIs are not supported.** `build.sh` stages only `arm64-v8a` and
>   `x86_64`. On `armeabi-v7a`/`x86` the library is absent, so pairing fails
>   closed there. Accepted; not a goal.
> - **Pairing scheme is case-insensitive** and normalised to lowercase, as §3.0
>   proposed. This was a *behaviour change* to the moved parser, with a test.
> - **Pending-link URL unified.** The shared `mend` uses `console:pending-link`.
>   Desktop used `waku:pending-link` and Android `console:pending-link`; Android's
>   value won because nothing else on desktop depended on the old string.
> - **`less` resolves to the `css` icon on both clients** (desktop said `sass`).
> - **`time.rs` was not moved.** It calls `chrono::Utc::now()`, which is not
>   pure, so `format_time_ago` stays in `console-ui`. `session_groups.rs` was
>   not moved for the same reason.
> - **Mentions stay desktop-only** over JNI (§3.0): the models differ.
>   `composer::mentions` is in the shared crate for the desktop's sake only.
> - **`LinkIcons.kt` was not touched.** Its link/file-name rules are Android's
>   own and were never part of the Phase 3 table.
> - **The Rust `.so` grew by ~99 KB, not the ~20–25 KB projected** — see §4.4.
>   Accepted; the cost was not investigated further.

Extracts the pure, dependency-free logic that the desktop (Rust/GPUI) and the
Android app (Kotlin/Compose) currently implement twice into a new
`console-shared` crate, and exposes it to Android over the existing JNI bridge
in `apps/android/rust-wt`.

Today the two clients carry roughly **2,800 lines of Kotlin** that mirror
**~3,600 lines of Rust**. Some of it has already drifted: the pairing URL
parser — a *credential* parser — exists in both languages with different
strictness. That is the kind of divergence that turns a malformed paste into a
pairing with the wrong machine.

**Markdown scope: only `mend`.** The Markdown *parser* is **not** being
migrated. Desktop parses with `pulldown-cmark` (`console-ui/src/markdown/parser.rs`,
1014 lines, incremental); Android parses with `org.jetbrains:markdown` and
renders through its own Compose pipeline. Those are two different engines
producing two different trees, and each renderer is built around its own
parser's output. What *is* shared is `mend` (streaming-tail repair, a pure
string-to-string function that runs before either parser) and `file_links`.
See §3.2.

This plan covers the crate and the JNI surface. The **HTTP API client** is a
separate, larger decision and is analysed in §7 but deliberately *not* part of
this plan's scope.

Target: `apps/desktop/crates/console-shared/` (new crate),
`apps/android/rust-wt/src/shared.rs` (new JNI module),
`apps/android/app/src/main/kotlin/com/console/mobile/SharedNative.kt` (new).

---

## 0. Non-Goals

- **No shared UI.** GPUI and Compose cannot share a view layer. This plan
  covers *logic*, not pixels.
- **No change to the server API.** The wire stays exactly as it is.
- **No change to the terminal stack.** The desktop uses `termy`
  (`console-core/src/services/terminal/termy.rs`); Android uses Ghostty VT in
  C++ (`apps/android/app/src/main/cpp/console_terminal_jni.cpp`). These are
  deliberately different engines. Unifying them is a separate project, not a
  side effect of this one.
- **No change to `console-proto`.** The Wire/Kotlin and prost/Rust protobuf
  generations already share `proto/console/v1/*.proto`, which is the schema
  layer. This plan is about the logic *around* the schema.
- **The HTTP API client is out of scope.** See §7.

---

## 1. What Already Exists (verified)

**The mechanism is proven.** `apps/android/rust-wt` is a `cdylib` that
cross-compiles `console-core` — the *same crate the desktop uses* — for
Android and exposes it over JNI. The WebTransport transport, framing, session
cache, certificate pinning and reconnect are not reimplemented on mobile;
Kotlin holds an integer handle and reads frames
(`apps/android/rust-wt/src/bridge.rs`).

**The failure mode is already handled.** `libconsole_wt_android.so` is
gitignored (`apps/android/.gitignore:31`) and built by
`apps/android/rust-wt/build.sh`, so a build without it is normal. `WtNative`
exposes an `available` flag
(`apps/android/app/src/main/kotlin/com/console/mobile/WtStream.kt:38`) and
every entry point declines instead of throwing, so the app falls back to
SSE/WS. `apps/android/tests/WtFallbackTest.kt` asserts exactly this. **The new
`Shared` surface must follow the same rule** — see §5.

**CI already builds and verifies the `.so`.**
`.github/workflows/console-android.yml` installs the Rust toolchain with the
Android targets, runs `rust-wt/build.sh`, and then *asserts* the APK carries
`lib/arm64-v8a/libconsole_wt_android.so` — because a missing library is a
silent downgrade, not a visible failure.

**A second Rust precedent exists.** RaTeX, the KaTeX-compatible LaTeX engine,
is used by both clients: the desktop depends on `ratex-*` crates
(`apps/desktop/crates/console-ui/Cargo.toml`), and Android takes the same
engine as an AAR (`io.github.erweixin:ratex-android:0.1.15`,
`apps/android/app/build.gradle.kts:215`). A pure-Rust engine crossing the
JNI boundary is not new territory.

**There is a pure-Rust sub-tree ready to extract.** `console-ui` depends on
`gpui`, so it cannot be consumed by Android as a whole. But these files
import **no** `gpui` symbols and depend only on `std`, `unicode_segmentation`,
`chrono` and `similar`. (`parser.rs` also qualifies technically — it needs
`pulldown_cmark` + `regex` — but it is **deliberately excluded**, §3.2.)

| File | Lines | Notes |
| :--- | ---: | :--- |
| `crates/console-ui/src/markdown/mend.rs` | 301 | zero imports beyond `std` |
| `crates/console-ui/src/markdown/parser.rs` | 1014 | `pulldown_cmark` + `regex` — **not migrated**, stays in `console-ui` |
| `crates/console-ui/src/markdown/file_links.rs` | 146 | zero imports |
| `crates/console-ui/src/common/input/mentions.rs` | 89 | zero imports |
| `crates/console-ui/src/common/input/boundaries.rs` | 142 | `unicode_segmentation` |
| `crates/console-ui/src/utils/time.rs` | 76 | `chrono` |
| `crates/console-ui/src/utils/path.rs` | 87 | zero imports |
| `crates/console-ui/src/utils/throughput.rs` | 17 | zero imports |
| `crates/console-ui/src/utils/session_groups.rs` | 215 | `chrono` |
| `crates/console-core/src/utils/pairing_url.rs` | 201 | zero imports |
| `crates/console-core/src/utils/page_title.rs` | 108 | zero imports |
| `crates/console-core/src/utils/file_kind.rs` | 93 | zero imports |
| `crates/console-core/src/utils/diff.rs` | 405 | `similar` (+ `serde_json` for tool-call args) |

---

## 2. Design Principles

1. **Pure means pure.** `console-shared` has **no** `reqwest`, no `tokio`, no
   `gpui`, no `wtransport`, no `termy`. Its only dependencies are `serde`
   (optional), `chrono`, `unicode_segmentation` and `similar`. There is no
   Markdown parser and no `regex` in it (§3.2). This is what makes it cheap
   to cross-compile and impossible to accidentally drag a runtime into the
   JNI surface. The spike (§4.4) measured the result: a shared crate adds
   **~4 KB** to the Android `.so`, and reaching `similar` through it adds
   ~17 KB.
2. **One implementation, two callers.** The crate is the *only* home for the
   logic. The desktop imports it as a Rust dependency; Android calls it over
   JNI. Neither keeps a private copy. A second copy is a bug.
3. **The wire format is the contract.** Rust→Kotlin marshalling uses a
   documented, versioned, byte-oriented encoding — the same discipline the
   existing WT frame uses (`[u32BE type_len][type][u64BE seq][payload]`,
   `apps/android/rust-wt/src/lib.rs:14`). Every payload carries a magic
   number and a version so a stale `.so` is detected, not misread.
4. **Degrade, never crash.** A missing or stale `.so` must not take the app
   down. Every shared call site keeps a Kotlin fallback until the shared path
   is proven, and the fallback is deleted in the same commit that enables the
   shared path — never left to rot.
5. **Test the logic once.** The Rust unit tests move with the code. The
   Android tests that covered the Kotlin duplicates are deleted with the
   duplicates, not left behind as a second source of truth.
6. **No behavioural change.** Sharing must not change what either client
   does. Where the two implementations *disagree today* (§3), the shared
   version follows the **desktop** implementation, because it is the
   reference client and already has the tests. Divergence fixes are
   intentional and listed explicitly.

---

## 3. The Inventory (verified)

| Logic | Desktop | Android | Status |
| :--- | :--- | :--- | :--- |
| Pairing URL parse | `console-core/src/utils/pairing_url.rs` (201) | `data/model/PairingUrl.kt` (219) | **Diverged** — §3.0 |
| HTML page title | `console-core/src/utils/page_title.rs` (108) | `core/util/PageTitles.kt` (112) | Duplicated |
| Markdown mend | `console-ui/src/markdown/mend.rs` (301) | `feature/chat/markdown/MarkdownMend.kt` (284) | Duplicated — Android file says "Port of … mend.rs" |
| Markdown **parser** | `console-ui/src/markdown/parser.rs` (1014, `pulldown-cmark`) | `CustomMarkdown.kt` (595, `org.jetbrains:markdown`) | **Not migrated** — §3.2 |
| Line diff (old→new) | `diff_lines` — Myers via `similar` | `computeLineDiff` — hand-rolled LCS | **Diverged** — §3.1 |
| Unified-diff parse | `parse_unified_diff` | `parseUnifiedDiff` | **Diverged, Kotlin has bugs** — §3.1 |
| Local port forward | `console-core/src/services/local_forward.rs` (239) | `core/forward/LocalForwardManager.kt` (290) | Duplicated |
| SSE frame parse | `console-core/src/utils/sse_reader.rs` (37) | `data/stream/ChatStreamClient.kt` (inline) | Duplicated |
| File-type icons | `console-ui/src/primitives/file_icons.rs` (213 arms) | `core/icons/FileTypeRegistry.kt` (166 + 53 lang aliases) | **Diverged** — §3.0 |
| Tool call labels | `console-ui/src/chat/toolcalls.rs:189` | `core/util/ToolHelpers.kt` | Duplicated |
| MCP tool name split | `console-ui/src/chat/toolcalls.rs:1147` | `core/util/ToolHelpers.kt` | Duplicated |
| Mention parsing | `console-ui/src/common/input/mentions.rs` | `core/util/MentionParser.kt` | **Different semantics** |
| Time / path / format | `console-ui/src/utils/{time,path}.rs` | `core/util/{Time,Format}.kt` | Duplicated |
| Terminal grid | `console-core/src/services/terminal/termy.rs` | C++ Ghostty VT | **Do not share** |

### 3.0 Divergences outside diff and Markdown

**Pairing URL.** The desktop accepts `"HTTP://host"` — the scheme comparison
is case-sensitive (`console-core/src/utils/pairing_url.rs:57`:
`if scheme != "http" && scheme != "https"`), while `split_pairing_line`
compares case-insensitively (`:128`). The Android version is strict and
rejects a non-`http`/`https` scheme explicitly
(`data/model/PairingUrl.kt:57`). Two parsers, one credential, two answers to
"is `HTTPS://host` a valid pairing URL". The shared version takes the
**case-insensitive** behaviour, which is what `split_pairing_line` already
does and what a user pasting from a browser will actually produce.

**File-type icons.** The desktop resolves a filename through ~213 match arms
(`console-ui/src/primitives/file_icons.rs`); Android uses 166 exact/extension
entries plus 53 code-fence language aliases
(`core/icons/FileTypeRegistry.kt`). The desktop's newer entries (`.env`,
`eslint`, `prettier` prefixes) exist only on the desktop. The shared table
must be the **union**, with the desktop's precedence rules (prefix match
before exact match before extension match) preserved.

**Diff.** Not "two different operations" — **each side has both operations**,
implemented differently. This is the one divergence with real bugs on the
mobile side, so it is analysed in full in §3.1.

**Mention parsing.** The desktop reconciles *ranges* against edited text
(`mentions.rs::adjust_mentions` / `reconcile_mentions`). Android scans text
for `@token` and returns ranges (`MentionParser.kt:15`). The Android version
is a *subset* of the desktop's model with a different entry point. The
scanner is worth sharing; the reconciliation is desktop-only for now.

### 3.1 Diff: the two implementations compared

There are **two operations** in a diff stack, and both clients carry both:

| Operation | Input | Desktop | Mobile | Used for |
| :--- | :--- | :--- | :--- | :--- |
| **Compute** a diff | old text + new text | `diff_lines()` | `computeLineDiff()` | Tool-call cards (`editFile`, `writeFile`, `batchWrite`) — the arguments carry both sides |
| **Parse** a diff | unified-diff text from the server | `parse_unified_diff()` | `parseUnifiedDiff()` | The Changes tab / review sheet — the Go server already produced the diff |

The server itself uses a *third* implementation: `go-difflib` (Python's
`SequenceMatcher` family) in `apps/server-go/internal/run/file_changes.go:207`,
with auto-junk turned off. So the repo has three diff engines. This plan
shares the two Rust-vs-Kotlin ones; the Go server is out of scope.

#### Compute: Myers (`similar`) vs hand-rolled LCS

| | Desktop `diff_lines` | Mobile `computeLineDiff` |
| :--- | :--- | :--- |
| Algorithm | **Myers**, via `similar::capture_diff_slices` — O(ND) | **Longest-common-subsequence DP table** — O(N·M) time **and memory** |
| Large files | Fast when changes are small (the `editFile` case) | **Guard at `n*m > 4,000,000`: gives up and reports the *whole file* as removed + re-added** |
| Context window | **Yes** — `context` param (tool cards use 3); `0` = changes only | **No** — returns every unchanged line |
| Trailing newline | Drops the phantom empty last element; documents why, and matches the Go emitter | Same handling (`splitLines`) |
| Output | Flat list, file order, old/new line numbers, `added`/`removed` | Same shape |
| Tests | `console-core/tests/types/write_files_test.rs` (trailing newline, blank lines, batchWrite, no-change) | **None** |

**Measured, not assumed.** I ported the Kotlin LCS line-for-line to Rust and
ran it against `similar`'s Myers on 20,000 random edit scripts (inserts,
deletes, replaces on small files, trailing newline):

- **+/− counts: identical in 20,000 / 20,000.** Both are correct diffs.
- **Exact line ordering: identical in only 17,045 / 20,000 (85%).** In the
  other ~15% both are *valid* minimal diffs but they pick different ones.
  Example — swapping two lines `[a,b,c] → [b,a,c]`: Myers emits
  `+b  a  −b  c`; the LCS emits `−a  b  +a  c`. Neither is wrong.
  Consequence: the same tool call can render slightly differently on desktop
  and mobile today, and a shared implementation removes that.
- **The 4M-cell guard is a visible bug.** A 2,100-line file with one line
  changed: Myers reports `+1 −1`; the Kotlin guard reports **`+2100 −2100`**
  (2100² = 4,410,000 > 4,000,000). The guard's own comment calls this a
  "fallback". The cap is also unnecessary: the same LCS took ~36 ms and
  ~17 MB at that size, which is fine — the guard exists to protect against
  O(N·M) blow-up, and Myers does not have that failure mode.
- **Memory.** The DP table is `(n+1)×(m+1)` `Int`s allocated up front. On a
  phone, 4M cells is 16 MB of short-lived garbage per tool card.

**Winner: desktop (Myers).** It is faster in the common case, has no size
cliff, supports the context window mobile has no equivalent of, and has
tests. The only thing LCS has going for it is that it is easy to read — which
is not a reason to keep a second copy. Mobile also never trims context, so a
tool card for an edit deep in a long file carries every unchanged line
(collapsed behind "Show N more lines" at 60), where desktop keeps a 3-line
window around each change.

#### Parse: desktop is stricter, mobile is looser — and mobile is wrong in edge cases

| | Desktop `parse_unified_diff` | Mobile `parseUnifiedDiff` |
| :--- | :--- | :--- |
| File headers (`diff --git`, `index`, `---`, `+++`) | Ignored **structurally**: nothing counts until the first `@@` (`in_hunk`) | Ignored by **line prefix**: any line starting `---`, `+++`, `diff `, `index ` is dropped, *anywhere* |
| `@@` header | Hand-parsed; **`-N`/`+N`** with or without `,len` | Regex `@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@` — same forms; **ignores a malformed header** and keeps the previous counters |
| `\ No newline at end of file` | **Skipped** | **Rendered as a context line** |
| Trailing newline of the input | `lines()` — no phantom last line | `split("\n")` — **emits a spurious empty context line at the end** |
| Line-number start | `0` until a header is read | `1` |
| Unknown-prefix line | Context | Context |

The mobile bugs are reproducible. Running both parsers (ported
faithfully) over one diff:

```
@@ -1,3 +1,2 @@
 keep
--- drop            <- a removed line whose content is "-- drop" (an SQL comment)
-last
\ No newline at end of file
+end
```

| | Desktop | Mobile |
| :--- | :--- | :--- |
| line 2 | `− old2  "-- drop"` ✅ | **dropped** — it starts with `---`, so it is mistaken for a file header |
| `\ No newline…` | skipped ✅ | **shown as a context row** `"\ No newline at end of file"` |
| trailing | — | **extra blank context row** at old4/new4 |
| new line number | `+ new2` ✅ | `+ new3` ❌ — off by one, because the dropped line never advanced the counter |

So on mobile, **any removed line whose text begins with `--` (SQL/Lua/Haskell
comments, YAML `---` document markers, Markdown horizontal rules written as
`---`) silently vanishes from the diff and shifts every following line
number.** That is a correctness bug in the review screen, not a style
difference. The `+`-side has the same hole for added lines starting with `++`.

**Winner: desktop.** Structural hunk tracking is the right design — it is
immune to content that merely *looks like* a header.

#### What this means for the plan

1. **Share both.** `console-shared::diff` exposes `diff_lines` and
   `parse_unified_diff`, both from `console-core/src/utils/diff.rs`.
2. **Both Kotlin implementations are deleted**, not wrapped.
   `computeLineDiff`, `computeNewFileDiff`, `parseUnifiedDiff` and
   `splitLines` in `core/util/DiffUtils.kt`; callers are `ToolHelpers.kt:284`
   (`fileCallDiffs`), `ChatDiffSheet.kt:83` and `ChangesScreen.kt:195`.
3. **Mobile's visible behaviour changes, deliberately:** tool cards gain the
   3-line context window; the `---`/`++` content bug and the phantom trailing
   row go away; the 2,100-line "+2100 −2100" cliff disappears.
4. **`fileCallDiffs` goes with it.** Desktop's `file_call_diffs` already
   handles `editFile`, `writeFile` and `batchWrite` and the argument aliases
   (`path` / `filePath` / `targetFile` / `absolutePath`); Kotlin re-implements
   this in `ToolHelpers.kt`. Sharing the whole `file_call_diffs` entry point,
   not just `diff_lines`, removes `extractEditArgs` / `extractWriteFiles` too.
5. **Add the missing tests first.** Neither Kotlin parser has a test, and the
   desktop `parse_unified_diff` has no test of its own either (only
   `file_and_diff.rs` exercises it indirectly). Phase 1 adds
   `console-shared/tests/diff_test.rs` covering at minimum the case above
   (`-- drop`, `\ No newline…`, trailing newline), short and long `@@`
   forms, and a 2,100-line single-edit file.
6. **Marshalling.** A diff is a list of records, so it uses the *list* shape
   from §5.2 (`[tag u8][old_no i32][new_no i32][len u32][utf8 text]` per
   line, with `added`/`removed` in the header). Measured cost of making
   `similar` reachable from the `.so`: **+17,344 B (~17 KB)** — see §4.4.

### 3.2 Markdown: only `mend` is shared; the parser is not migrated

Decision: **the Markdown parser stays where it is on each platform.**

| | Desktop | Mobile |
| :--- | :--- | :--- |
| Parser | `pulldown-cmark` + custom block tree + incremental reparse (`parser.rs`, 1014 lines) | `org.jetbrains:markdown` 0.7.9 (`ASTNode`, `MarkdownElementTypes`, `GFMTokenTypes`) |
| Renderer | GPUI (`render.rs`, 2112 lines) | Compose (`CustomMarkdown.kt`, 595 lines) |
| Tree shape | desktop's own `BlockTree` | the library's `ASTNode` |

Why not migrate it:

- **The two renderers are written against two different trees.** Sharing the
  parser would mean choosing one tree and rewriting the *other* renderer
  against it — i.e. rewriting `CustomMarkdown.kt` (595 lines) *and* defining
  a JNI wire format for an entire block tree. That is a rewrite, not an
  extraction.
- **It is the expensive dependency.** Reaching `pulldown-cmark` adds
  **~304 KB** (11%) to the Android `.so` (§4.4). `mend` and `file_links` add
  nothing.
- **The parser is the part that is *supposed* to differ.** Each platform's
  parser feeds that platform's native text layout. `mend` is different in
  kind: it is `&str → Option<String>`, runs *before* either parser, and has
  no tree.

What this changes in the plan:

- `console-shared` has **no** `markdown` feature, **no** `pulldown-cmark` and
  **no** `regex`. `parser.rs` stays in `console-ui`.
- `mend` and `file_links` (pure string logic, zero dependencies) *are*
  shared.
- The fixture contract for `mend` still matters: `MarkdownMendFixtureTest`
  keeps guarding the Kotlin call site until Phase 3, item 3 deletes the
  Kotlin copy.

---

## 4. Crate Layout

```
apps/desktop/crates/console-shared/
├── Cargo.toml
├── src/
│   ├── lib.rs
│   ├── pairing.rs        # from console-core/src/utils/pairing_url.rs
│   ├── page_title.rs     # from console-core/src/utils/page_title.rs
│   ├── file_kind.rs      # from console-core/src/utils/file_kind.rs
│   ├── diff.rs           # from console-core/src/utils/diff.rs (diff_lines, parse_unified_diff, file_call_diffs)
│   ├── markdown/         # NOT the parser — see §3.2
│   │   ├── mod.rs
│   │   ├── mend.rs       # from console-ui/src/markdown/mend.rs
│   │   └── file_links.rs # from console-ui/src/markdown/file_links.rs
│   ├── composer/
│   │   ├── mod.rs
│   │   └── mentions.rs    # from console-ui/src/common/input/mentions.rs
│   ├── format.rs         # time / path / throughput / context-window
│   ├── icons.rs          # file-type icon table (union of both)
│   └── tools.rs          # tool call labels + MCP name splitting
└── tests/
    ├── pairing_test.rs
    ├── page_title_test.rs
    ├── file_kind_test.rs
    ├── diff_test.rs
    ├── markdown_mend_test.rs
    ├── file_links_test.rs
    ├── mentions_test.rs
    ├── format_test.rs
    ├── icons_test.rs
    └── tools_test.rs
```

### 4.1 `Cargo.toml`

```toml
[package]
name = "console-shared"
version.workspace = true
edition.workspace = true
license.workspace = true

[dependencies]
serde = { workspace = true, optional = true }
serde_json = { workspace = true }   # tool-call arguments in `file_call_diffs`
chrono = { workspace = true }
unicode-segmentation = { workspace = true }
similar = { workspace = true }

[features]
default = []
serde = ["dep:serde"]
```

There is deliberately **no** `regex`, `pulldown-cmark` or `markdown` feature
(§3.2). `similar` is already a `console-core` dependency, so the diff engine
adds only ~17 KB to the Android build (§4.4).

### 4.2 Dependency rules

- `console-shared` **must not** depend on `console-core`, `console-ui`,
  `console-proto`, `gpui`, `reqwest`, `tokio`, `wtransport`, or `termy`.
  A `cargo tree` assertion in CI would be the enforcement; until then this is
  a review rule.
- `console-core` **may** depend on `console-shared` (it needs `pairing_url`,
  `page_title`, `file_kind`, `diff`).
- `console-ui` **may** depend on `console-shared` (it needs `mend`,
  `file_links`, `mentions`, `format`, `icons`, `tools`). Its Markdown *parser*
  stays local to `console-ui` (§3.2).
- `console-shared` **must not** depend on either. The dependency arrow points
  one way: `console-ui` → `console-shared` → (nothing).

### 4.3 Where the crate lives, and why it matters for Android

`console-shared` is a member of the **desktop workspace**
(`apps/desktop/Cargo.toml`), and `apps/android/rust-wt` already depends on
`console-core` by path (`apps/android/rust-wt/Cargo.toml`):

```toml
console-core = { path = "../../desktop/crates/console-core" }
```

so `console-shared` is consumed the same way:

```toml
console-shared = { path = "../../desktop/crates/console-shared" }
```

This is the existing, working arrangement — one source tree, two consumers.
The alternative (a third top-level workspace) would need `rust-wt` to join
that workspace, which changes how the Android build resolves dependencies
and how CI caches them. Not worth it.

### 4.4 The measured cost of the crate (spike, verified)

The design was validated by a spike before this plan was written. The crate
was built for `aarch64-linux-android` with the pinned toolchain (1.97.1) and
the NDK 28.2 clang wrappers, then linked into a copy of `rust-wt` with a
`shared.rs` JNI module, exactly as §5 describes.

**It works.** The JNI symbols export correctly alongside the existing ones:

```
$ nm -D --defined-only libconsole_wt_android.so | grep Java_com_console
T Java_com_console_mobile_SharedNative_abiVersion
T Java_com_console_mobile_SharedNative_parsePairingUrl
T Java_com_console_mobile_SharedNative_formatContextWindow
T Java_com_console_mobile_SharedNative_wireMagic
T Java_com_console_mobile_SharedNative_wireVersion
T Java_com_console_mobile_WtNative_abiVersion        (existing, unaffected)
...
```

**Size cost, measured on `aarch64-linux-android` release builds** (same
profile as the real crate: `opt-level = "z"`, `lto = true`, `strip = true`):

| Crate contents | `.so` size | Delta |
| :--- | ---: | ---: |
| Baseline (no `console-shared`) | 2,626,304 B | — |
| `console-shared`, pairing + format only | 2,626,976 B | **+4 KB** |
| `console-shared`, + chrono / regex / unicode-segmentation, no `pulldown-cmark` | 2,626,304 B | **+0 B** |
| `console-shared`, + `pulldown-cmark` | 2,930,112 B | **+304 KB** |
| Baseline (separate build) → `diff_lines` reachable from the `.so` | 2,621,664 B → 2,639,008 B | **+17 KB** |

Conclusions:

1. **The crate itself is free.** ~4 KB with nothing reachable.
2. **`pulldown-cmark` is the one expensive dependency: ~300 KB** (~11% of the
   `.so`), because it pulls in `pulldown-cmark-escape`, `unicase` and
   `bitflags` too. This is a second, independent reason (alongside §3.2) the
   Markdown parser is not migrated.
3. **Diff is cheap but not free: ~17 KB.** `similar` was in the dependency
   tree already (via `console-core`) but the linker was discarding it as
   unreachable; making `diff_lines` callable over JNI keeps it. This is the
   right price for deleting the Kotlin LCS.

**Decision:** `console-shared` carries **no Markdown parser and no `regex`**
(§3.2), so there is no feature flag to manage — the expensive dependency is
simply absent.

> **Measured result (implementation): +100,960 B (~99 KB), not ~20–25 KB.**
> `aarch64-linux-android` release, same profile: **2,621,664 B** without
> `shared.rs`, **2,722,624 B** with all 16 entry points (~3.9%). The earlier
> spike numbers above are still right for what they measured; the gap is the
> real surface being larger than the spike's two functions — notably
> `fileCallDiffs` pulls `serde_json` parsing, and the icon tables are a few
> hundred match arms. I started to bisect which function dominates and stopped
> at the owner's request: the size is **accepted**, and the figure above is
> the number to trust.

---

## 5. The JNI Surface

### 5.1 Rules

- **Every** entry point goes through the existing `guard()` wrapper
  (`apps/android/rust-wt/src/lib.rs:22`), so a Rust panic becomes a Kotlin
  error rather than an `abort()` that takes the app down.
- **Every** entry point initialises logging first (`init_logging()`), so a
  panic reaches logcat instead of a bare SIGABRT.
- **Every** returned payload carries a magic number and a version, so a stale
  `.so` (built from an older commit) is *detected* rather than misparsed.
- **Every** entry point is **synchronous and pure** — no I/O, no network, no
  tokio. The shared crate is pure by construction, so this is free.
- **No `extern "C"`** — Kotlin's `external fun` resolves to the JNI ABI, so
  the functions are `extern "system"` with `JNIEnv`, exactly as the existing
  `WtNative` functions are.

### 5.2 Encoding

Two shapes, chosen per payload:

**Plain strings** — for a single string result:

```
[4 bytes] magic  = 0x43_53_48_52  ("CSHR")
[2 bytes] version = 1
[4 bytes] length of payload (big endian)
[4 bytes] code    — 0 = ok, non-zero = error
[N bytes] payload — UTF-8
```

**Lists of records** — for structured results (icon tables, diff lines, file
links):

```
[4 bytes] magic
[2 bytes] version
[4 bytes] count
[count × record]
```

Records are fixed-layout where the fields are numeric, and
length-prefixed UTF-8 where they are not. A `record` is never variable-width
without an explicit length prefix — a truncated payload must be detectable,
not silently misread.

The magic + version preamble is what makes the whole thing safe under
partial rollout: an Android build that ships a new `.so` alongside an old
APK (or the reverse) reads a version it does not understand and falls back,
instead of decoding garbage.

### 5.3 The `Shared` Kotlin facade

`apps/android/app/src/main/kotlin/com/console/mobile/SharedNative.kt`:

```kotlin
package com.console.mobile

import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * The shared logic that lives in Rust (`console-shared`), called over JNI.
 *
 * The same rule as [WtNative]: the library is gitignored and built by
 * `rust-wt/build.sh`, so it may be absent. [available] is false then, and
 * every caller keeps its Kotlin fallback. A missing library is a *silent
 * downgrade*, so nothing here may throw at class-init time — that would take
 * the app down from `AppContainer.initialize`, before any UI exists to
 * explain it.
 */
object SharedNative {
    val available: Boolean = try {
        System.loadLibrary("console_wt_android")
        true
    } catch (_: UnsatisfiedLinkError) {
        false
    }

    const val OK = 0

    external fun abiVersion(): Int
    external fun lastError(): String

    /** Pairing URL → base url / server id / code. Empty string on error. */
    external fun parsePairingUrl(raw: String): String
    external fun splitPairingLine(raw: String): String
    external fun hostPort(address: String): String

    /** HTML → display title, or null. */
    external fun extractHtmlTitle(html: String): String?

    /** Streaming markdown tail → repaired text, or null when nothing hangs. */
    external fun closeHanging(text: String): String?

    /** Tool call name → display label. */
    external fun toolCallLabel(name: String): String

    /** `mcp__server__tool` → "server\u0000tool", or null. */
    external fun parseMcpToolName(name: String): String?

    /** File path → icon key, e.g. "rust", "typescript". */
    external fun fileIconKey(pathOrName: String): String
}
```

`SharedNative` lives in the **same `.so`** as `WtNative` — one library, one
`loadLibrary`, one CI step, one artifact to verify. Splitting into a second
`.so` would double the packaging and the CI verification for no benefit.

### 5.4 The Rust side

`apps/android/rust-wt/src/shared.rs` — a sibling of `bridge.rs`:

```rust
//! JNI surface over `console-shared`'s pure functions.
//!
//! The logic itself is NOT written here. It lives in `console-shared`, the
//! same crate the desktop imports — one implementation, two callers. This
//! module is only the marshalling.

use jni::objects::{JClass, JString};
use jni::sys::jint;
use jni::JNIEnv;

/// Magic + version preamble for every payload. See the crate doc for the
/// full wire format. A payload whose magic or version does not match is
/// rejected, so a stale `.so` degrades instead of misparsing.
const MAGIC: u32 = 0x43_53_48_52; // "CSHR"
const VERSION: u16 = 1;
```

The decoding side lives in Kotlin (`SharedNative`), so the format is defined
in exactly one place per language and the round-trip is covered by a test.

### 5.5 Degradation

The rule, stated once:

> If `SharedNative.available` is false, or the payload's version is unknown,
> the caller uses its Kotlin implementation.

Every call site is written as:

```kotlin
val label = if (SharedNative.available) {
    SharedNative.toolCallLabel(name) ?: fallbackLabel(name)
} else {
    fallbackLabel(name)
}
```

and the fallback is deleted in the same change that makes the shared path the
only path. A fallback that outlives its replacement is a second
implementation — the exact problem this plan exists to remove.

---

## 6. Phases

Each phase is independently shippable and independently revertable.

### Phase 0 — The crate, empty

- Create `apps/desktop/crates/console-shared` with the `Cargo.toml` from §4.1.
- Add it to `apps/desktop/Cargo.toml` workspace members and
  `workspace.dependencies`.
- Add a `tests/` directory with one trivial test, so the test wiring is proven
  before any code moves.
- Add `console-shared` to `apps/android/rust-wt/Cargo.toml`.

**Verify:** `cd apps/desktop && cargo test -p console-shared`.

> **Status ✅ (`c94dcbff`).** Crate created, added to the workspace members and
> `workspace.dependencies`, and to `rust-wt/Cargo.toml`. It started with
> `serde_json` + `similar` only. No `regex`, `chrono`, `unicode-segmentation`
> or `pulldown-cmark` — nothing that was moved needed them (§4.1 shows the
> original, larger list).

### Phase 1 — Move the pure code, change no callers

Move each module's *contents* into `console-shared` and leave thin
re-exports behind, so **no call site changes** and the diff is reviewable as a
pure move:

```rust
// apps/desktop/crates/console-core/src/utils/pairing_url.rs
pub use console_shared::pairing::*;
```

```rust
// apps/desktop/crates/console-ui/src/markdown/mend.rs
pub use console_shared::markdown::mend::*;
// (parser.rs is NOT moved — §3.2)
```

Before moving `diff.rs`, add `console-shared/tests/diff_test.rs` with the
cases in §3.1 — the desktop `parse_unified_diff` currently has no direct test.

Move the existing unit tests with the code (the repo rule is that Rust tests
live in `tests/`, never inline `#[cfg(test)]` modules — see `AGENTS.md`).

**Verify:** `cd apps/desktop && cargo test -p console-core -p console-ui`.

> **Status ◐ (`c94dcbff`, `d29dc814`).** Moved with `git mv` so history follows,
> and old paths kept alive by re-export shims:
>
> | Moved to `console-shared` | From |
> | :--- | :--- |
> | `pairing`, `file_kind`, `diff` | `console-core/src/utils/` |
> | `page_title` (pure half) | `console-core` — the `reqwest` fetch **stayed** |
> | `markdown::mend`, `markdown::file_links` | `console-ui/src/markdown/` |
> | `composer::mentions` | `console-ui/src/common/input/` |
> | `path`, `throughput` | `console-ui/src/utils/` |
> | `format` (`format_context_window`, `format_model_name`) | `console-ui/src/common/model_picker.rs` |
> | `tools` (new: labels, MCP split, 5 predicates) | `console-ui/src/chat/toolcalls.rs` private fns |
> | `icons` (new: union of both tables) | `console-ui/src/primitives/file_icons.rs` + `FileTypeRegistry.kt` |
>
> Tests moved with the code. New tests written: `mend_test`, `mentions_test`,
> `tools_test`, `icons_test`, and 12 cases in `diff_test` (§3.1). A mend test
> did not exist on desktop before.
>
> **Not verified:** `console-ui`/`console-core` compiling against this. Also,
> `file_icons.rs` in `console-ui` now keeps only the desktop-specific parts
> (asset path, highlight-language table) and resolves keys through
> `console_shared::icons`; `FileTypeIcon::from_key` was added to `icons.rs`.
>
> This phase alone removes ~2,000 lines of duplicated *desktop* code with zero
behavioural risk. It is the bulk of the value at the lowest risk, and it can
land even if the JNI work is deferred.

### Phase 2 — The JNI surface

- Add `apps/android/rust-wt/src/shared.rs` with the encoding from §5.2.
- Add the `SharedNative` Kotlin facade.
- Add `apps/android/rust-wt/src/lib.rs` entry points.

**Verify:**
- `cd apps/android/rust-wt && cargo build --release --target aarch64-linux-android`
  (or `bash apps/android/rust-wt/build.sh`).
- `cd apps/android && ./gradlew :app:assembleDebug`.

> **Status ✅ built, ◐ not run (`81f2e540`, `4a330603`).**
> `rust-wt/src/shared.rs` exports **16** symbols (`abiVersion`,
> `extractHtmlTitle`, `closeHanging`, `toolLabel`, `toolFlags`, `fileIconKey`,
> `languageIconKey`, `formatContextWindow`, `formatModelName`,
> `formatTokensPerSecond`, `parsePairingUrl`, `splitPairingLine`, `hostPort`,
> `diffLines`, `parseUnifiedDiff`, `fileCallDiffs`) — confirmed with `nm -D`.
> `SharedNative.kt` is the only Kotlin surface; its `external` functions are
> private, so nothing else can call a symbol that might not resolve.
>
> Differences from the design in §5:
>
> - **ABI check replaced per-call version sniffing.** `SharedNative.EXPECTED_ABI`
>   is compared with the Rust `ABI_VERSION` once at class init; a mismatch makes
>   `available` false. Framed payloads still carry the `CSHR` magic + version.
> - **Plain strings are not framed** — a single text result has nothing to
>   misparse. Only diffs and pairing use the framed format.
> - `toolFlags` returns the five predicates as one bitmask instead of five
>   calls.
> - A startup `Log.e` in `AppContainer` reports when the library is unavailable.
> - `apps/android/rust-wt/src/lib.rs` had a stale doc path
>   (`rust-wt-spike/build.sh`) that was corrected.

### Phase 3 — Cut over the callers, one at a time

Ordered by risk, lowest first. Each entry replaces the Kotlin duplicate with
the shared call and **deletes** the Kotlin duplicate in the same change.

| # | Logic | Kotlin file to delete | Test to delete |
| :--- | :--- | :--- | :--- |
| 1 | Tool labels + MCP split | `core/util/ToolHelpers.kt` (labels half) | `tests/ToolHelpersTest.kt` |
| 2 | HTML page title | `core/util/PageTitles.kt` (title half) | `tests/PageTitlesTest.kt` |
| 3 | Markdown mend | `feature/chat/markdown/MarkdownMend.kt` | `tests/MarkdownMendFixtureTest.kt` |
| 4 | File-type icons | `core/icons/FileTypeRegistry.kt` | (none) |
| 5 | Pairing URL | `data/model/PairingUrl.kt` | `tests/PairingUrlTest.kt` |
| 6 | Diff (compute + parse + `fileCallDiffs`) | `core/util/DiffUtils.kt`; `fileCallDiffs`, `extractEditArgs`, `extractWriteFiles` in `core/util/ToolHelpers.kt` | (none exist — add `diff_test.rs` first, §3.1) |

**The pairing URL is last on purpose.** It is the highest-value fix and also
the one where a mistake is a security bug. It goes in after the mechanism is
proven on five simpler payloads.

**`ToolHelpers.kt` and `FileTypeRegistry.kt` do not move wholesale.** Both mix
shareable *logic* with platform-specific *presentation*:

- `ToolHelpers.kt` holds `TOOL_LABELS` and `parseMcpToolName` (shareable),
  but also `TOOL_ICONS` — a `Map<String, ImageVector>` of Compose icons, which
  has no Rust equivalent and must stay in Kotlin.
- `FileTypeRegistry.kt` holds the extension tables (shareable), but the
  desktop's `file_icons.rs` resolves to an SVG *path* rendered by `gpui`'s
  `Img`, while Android resolves to a key looked up in `assets/icons/`. The
  **table** is shared; the **rendering** is not.

So each of these becomes: shared table in Rust, platform rendering in
Kotlin/GPUI. The deletion is of the *table*, not the file.

**Verify (per entry):**
```
cd apps/android && ./gradlew :app:testDebugUnitTest --tests "<TestClass>"
cd apps/desktop && cargo test -p console-shared
```

> **Status ✅ compiles, ◐ tests (`4a330603`, `6a5ddbb8`).** All six entries are
> done. What actually happened to each Kotlin file:
>
> | # | Result |
> | :--- | :--- |
> | 1 Tool labels | `TOOL_LABELS`, `parseMcpToolName`, `getToolLabel`, the five `is*Tool` predicates and the arg extractors **deleted** from `ToolHelpers.kt`. `TOOL_ICONS`/`getToolIcon` and the argument *summaries* stay (Compose-specific). Callers (`ToolCallRow`, `RunActivity`) use `SharedNative.label` / `.kind`. |
> | 2 Page title | Entity-decoding/extraction **deleted**; `PageTitles.kt` keeps the OkHttp fetch and `bareWebUrls`. |
> | 3 Mend | `MarkdownMend.kt` and `MarkdownMendFixtureTest.kt` **deleted**; `CustomMarkdown.kt` calls `SharedNative.mendMarkdown`. The Rust `mend_test` mirrors the old fixtures. |
> | 4 Icons | The 166 + 53 entry tables **deleted**; `FileTypeRegistry.kt` is two thin wrappers plus `getProviderIconKey`, which was never shared. |
> | 5 Pairing | The parser **deleted**; `PairingUrl.kt` keeps the `PairingUrl` data class and the same four public functions (`parsePairingUrl`, `splitHostAndCode`, `pairingHost`, `pairingCode`) so no screen changed. |
> | 6 Diff | `DiffUtils.kt` **deleted whole**. `DiffView`, `ToolCallRow`, `ChatDiffSheet` and `ChangesScreen` now use `SharedNative.Diff` / `.parseDiff` / `.callDiffs`. |
>
> **Fail-closed behaviour (not in the original plan).** With the library
> unavailable: pairing is **refused** (never parsed leniently); diffs are empty;
> labels fall back to the raw tool name; file icons fall back to `file`;
> `contextWindow`/`modelName` fall back to a plain value. `SharedNativeTest`
> pins this.
>
> **Tests changed.** Behaviour now lives in Rust, so the Kotlin tests were cut
> down to the Android seam: `PairingUrlTest` (3 tests — fails closed, degrades,
> agrees when loaded), `PageTitlesTest` (just `bareWebUrls`), `ToolHelpersTest`
> (icons + summaries), `TokensPerSecondTest`, `LinkIconsTest` (two cases skip
> when the library is absent via `assumeTrue`), and a new `SharedNativeTest`.
> A JVM run **never loads the `.so`**, so any assertion on Rust output would
> fail there — that is why two tests I first wrote failed and were rewritten.
>
> **Missed on the first pass:** `RunActivity.kt` still called the removed
> `getToolLabel`; the compile caught it. `grep` for removed symbols now returns
> nothing.

### Phase 4 — Delete the now-dead Rust re-export shims

Once the desktop call sites import from `console-shared` directly, remove
the `pub use` shims from `console-core` / `console-ui`.

> **Status ⬜ — deliberately not done.** The shims (`console_core::utils::diff`,
> `console_ui::markdown::mend`, `console_ui::utils::path`, …) are harmless and
> keep every existing import working. Removing them is a pure cleanup that
> touches many desktop files, some of which have uncommitted work in progress.
> Do it once the desktop build is confirmed green.

### Phase 5 — The local port forward (optional, larger)

`console-core/src/services/local_forward.rs` and
`apps/android/app/src/main/kotlin/com/console/mobile/core/forward/LocalForwardManager.kt`
both implement the same idea: a loopback TCP listener per remote port,
piping through the server's tunnel WebSocket.

This one is **not** a pure function — it owns sockets and a tokio task. It
needs the same JNI registry pattern `bridge.rs` already uses for WT streams
(handle-based, polled, not callback-based). It is listed here but is a
separate piece of work: it does not belong in Phase 1's "pure move".

> **Status ⬜ — not started.** Along with the SSE reader, this is the next
> candidate if more Kotlin/Rust duplication is worth removing.

---

## 7. The HTTP API Client — Analysis, Not Scope

This is the largest duplication in the codebase and it is **not** in this
plan. It is analysed here because the decision is large enough that it
should not be made by accident, and because Phase 1's crate is the natural
foundation if the answer is "share it".

### 7.1 The scale of the duplication

| | Count | Where |
| :--- | ---: | :--- |
| Server endpoints | 132 | `apps/server-go/internal/routes/*.go` |
| Rust client methods | 139 `pub async fn` (118 unique names) | `apps/desktop/crates/console-core/src/services/*.rs` |
| Kotlin client methods | 84 `override suspend fun` | `apps/android/app/src/main/kotlin/com/console/mobile/data/api/OkHttpConsoleApi.kt` (800 lines) |
| Kotlin distinct paths | 70 | same |

The Rust side covers *more* than the Kotlin side — 139 methods to 84, and
132 server endpoints behind them. `OkHttpConsoleApi.kt` is 800 lines of
hand-written envelope-unwrapping and Moshi adapter wiring.

### 7.2 The blocker: TLS trust stores differ between the two Rust clients

> **FIXED (`97685cbf`).** `WtTls::Native` now falls back to the bundled
> Mozilla roots (`webpki-roots`, the same set `reqwest` uses) when the platform
> store loads empty — which is always the case on Android. Where the platform
> store loads (macOS, desktop Linux, Windows) it is used unchanged. Covered by
> `wt_tls_roots_test` (2 tests); the 39 existing WebTransport tests still pass
> and the Android `.so` builds. **Not run against a real HTTPS server on a
> device.** Limitation: user- or MDM-installed CAs are invisible to the
> fallback, so a server behind one still needs `WtTls::PinnedHashes`. The text
> below is the original analysis, kept for the reasoning.

This is the finding that decides the options, and it is a **bug in the
existing Android build**, not just a hurdle for a future one.

`console-core` uses `reqwest` with the `rustls-tls` feature
(`apps/desktop/Cargo.toml:44`):

```toml
reqwest = { version = "0.12", default-features = false, features = ["json", "stream", "rustls-tls"] }
```

`rustls-tls` **is** `rustls-tls-webpki-roots`
(`reqwest-0.12.28/Cargo.toml:148`:
`rustls-tls = ["rustls-tls-webpki-roots"]`). That bundles the **Mozilla CA
set compiled into the binary** — a static list that only changes when the
crate is updated. It is *not* the device's trust store.

By contrast, the WebTransport path in the same crate uses `wtransport`'s
`with_native_certs()` (`console-core/src/net/wt_client.rs:156`), which calls
`rustls_native_certs::load_native_certs()`
(`wtransport-0.7.2/src/tls.rs:597`) — i.e. the *platform* store.

So `console-core` already speaks TLS **two different ways**:
- **WebTransport** → platform trust store (on macOS, `Security.framework`).
- **HTTP** → webpki's baked-in Mozilla roots.

On the desktop that is survivable: both trust the same public CAs, and the
webpki list is *newer* than the OS list, which fails **closed** (a cert the
OS distrusts but webpki trusts would be accepted by `reqwest` and rejected by
`wtransport`).

**On Android it fails open, and it is already live.** `rustls-native-certs`
on Android does *not* read the Android system trust store. It compiles the
`unix` path (`rustls-native-certs-0.8.5/src/lib.rs:31`), which calls
`openssl_probe::probe()`, and `openssl-probe`'s Android constant is
(`openssl-probe-0.2.1/src/lib.rs:202`):

```rust
#[cfg(target_os = "android")] // Termux on Android
const CERTIFICATE_FILE_NAMES: &[&str] = &["/data/data/com.termux/files/usr/etc/tls/cert.pem"];
```

That is the **Termux** path. A normal Android app has no Termux, no such
file, and no OpenSSL on the system. The list is empty, so
`build_native_cert_store()` returns an **empty** `RootCertStore`. The `.so`
contains the string `/data/data/com.termux/files/usr/etc/tls/cert.pem`
(verified by `strings` on the built artifact), which proves the path is
compiled in.

Consequence: **on Android, `WtTls::Native` trusts nothing.** The WebTransport
path works today only because the *dev* server uses a self-signed certificate
and the client pins it by SHA-256
(`apps/android/rust-wt/src/bridge.rs::tls_from_setting`,
`console-core/src/net/wt_client.rs:160`). Against a **publicly-trusted**
certificate it would fail — and it would fail on the WT path, not the HTTP
path, which is the opposite of what one would expect.

The correct Android fix is `rustls-platform-verifier`, whose README states
it uses the **Android System Trust Store** via the Android Trust Manager, and
which ships a Kotlin AAR for exactly this purpose
(`rustls-platform-verifier-android-0.1.1`). That is a Gradle dependency plus
a rustls verifier swap, and it is the correct long-term answer for *both*
paths.

**This is worth fixing regardless of what happens to the HTTP client.** A
mobile client that cannot talk to a publicly-trusted server over WebTransport
is a real defect in the current build.

### 7.3 The options

#### Option A — Keep OkHttp in Kotlin, share only the pure logic

*This plan's scope. No HTTP changes.*

- **Shape:** Kotlin keeps `OkHttpConsoleApi.kt`. `console-shared` provides the
  parsing/formatting logic both sides need. The Rust services stay
  desktop-only.
- **Cost:** the 800-line Kotlin API client and the 132 Rust methods both
  remain. Two implementations of the server contract.
- **Benefit:** zero TLS risk, zero JNI-per-HTTP-call overhead, OkHttp's
  connection pool, interceptor chain and `MockWebServer` test support all
  keep working. The Android unit tests in `apps/android/tests/` keep testing
  the real client.
- **Risk:** lowest. The server API can drift between the two clients, and
  Android features lag.

#### Option B — Move the whole HTTP client to Rust, call it from Kotlin

*Move `console-core`'s services behind JNI.*

- **Shape:** Kotlin's `ConsoleApi` interface stays (so the repositories and
  screens do not change), but its implementation becomes a JNI-backed adapter
  that calls the Rust services. `OkHttpConsoleApi.kt` is deleted.
- **Cost:** this is the big one. It needs:
  1. **TLS roots fixed** (§7.2) — `rustls-platform-verifier` on Android.
  2. A **JNI surface for 132 methods** with request/response marshalling.
  3. An **async story**: the Rust services are `async`, and JNI is
     synchronous. This needs either blocking-on-a-runtime (as `bridge.rs`
     already does for frames) or a callback/promise layer. `bridge.rs`'s
     approach — a shared tokio runtime, `block_on` from a Kotlin background
     thread — is the precedent, and it works, but it means every API call
     occupies a Kotlin thread.
  4. **Error mapping**: `anyhow::Error` → something Kotlin can act on.
  5. The **envelope unwrapping** and Moshi/prost JSON differences handled in
     one place.
- **Benefit:** genuinely one implementation of the server contract. Android
  gains the ~55 methods it currently lacks, for free. The 800-line Kotlin
  file disappears.
- **Risk:** highest. This is a rewrite of the mobile data layer, with a
  trust-store migration, an async bridge, and a full JNI marshalling surface.
  A mistake here is an app that cannot reach any server.

#### Option C — A middle path: Rust owns the *hard* parts, Kotlin owns the *rest*

- **Shape:** keep OkHttp in Kotlin for plain request/response calls, but move
  the pieces that are genuinely hard to reimplement and already exist in
  Rust: the **SSE stream reader** (`console-core/src/utils/sse_reader.rs`),
  the **terminal WebSocket protocol** (`console-core/src/services/terminal/`),
  the **local port forward**, and the **WT transport** (already done).
- **Cost:** moderate. Each is a self-contained piece with an existing Rust
  implementation and an existing Kotlin one.
- **Benefit:** the protocol-heavy, bug-prone code — streaming, WebSocket
  framing, TCP tunnelling — has one implementation. The plain CRUD calls stay
  in Kotlin, where OkHttp is genuinely better (connection pooling, test
  doubles).
- **Risk:** moderate. Each item is independently verifiable.

### 7.4 The recommendation

**Do Option A now** (this plan), **fix the TLS roots** (§7.2) as a separate,
urgent fix, and **then** evaluate Option C, with Option B as the eventual
destination only if the endpoint gap actually bites.

The reasoning:

1. **Option B's cost is not the marshalling — it is the trust store.** Once
   `rustls-platform-verifier` is in place, B is *mechanically* feasible. But
   "feasible" is not "worth it": it is a rewrite of the mobile data layer
   whose failure mode is "the app cannot reach any server".
2. **The endpoint gap is the actual driver.** Android has 84 of the 139 Rust
   methods. If the missing ~55 are ones Android will never use, B buys nothing
   and costs everything. **This should be measured first** — list the missing
   methods and classify them by whether mobile needs them.
3. **Option C gives most of the benefit at a fraction of the risk**, and the
   pieces it covers are exactly the ones where the Kotlin duplicates are
   weakest (hand-rolled SSE line parsing; a hand-rolled terminal frame
   protocol; a hand-rolled TCP tunnel).

### 7.5 The measurement to do first

Before committing to B, produce the gap list:

```
# Rust methods
grep -rhoE "pub async fn [a-z_]+" apps/desktop/crates/console-core/src/services/ | sort -u

# Kotlin methods
grep -oE "override suspend fun [a-zA-Z]+" \
  apps/android/app/src/main/kotlin/com/console/mobile/data/api/OkHttpConsoleApi.kt | sort -u
```

Classify each missing method: *needed on mobile*, *desktop-only*, or
*never*. If the "needed on mobile" set is small, Option A or C suffices and
the 800-line Kotlin file is not worth a rewrite.

---

## 8. Risks

| Risk | Mitigation |
| :--- | :--- |
| A stale `.so` misparses a payload | Magic + version on every payload; unknown version → fallback |
| The `.so` is missing in CI or a fresh clone | Same rule as `WtNative.available`; CI already asserts the library is packaged |
| A Rust panic aborts the app | Every entry point goes through `guard()`; `panic = "unwind"` is already set in `rust-wt/Cargo.toml` |
| Sharing changes behaviour | §2.6 — the desktop is the reference; divergence fixes are explicit and listed |
| The crate drags a heavy dependency into the `.so` | §4.2 dependency rules; the crate has no runtime deps |
| A fallback outlives its replacement | §5.5 — fallbacks are deleted in the same change that enables the shared path |
| The JNI boundary leaks into app code | The `SharedNative` facade is the only surface app code sees |

---

## 9. Validation (done)

The design was spiked before this plan was written, because the central
claim — "a pure Rust crate can be shared with Android over JNI at a
negligible size cost" — is an empirical claim, not an architectural one.

**What was built and verified:**

1. A `console-shared` crate with the §4.1 dependency set, cross-compiled for
   `aarch64-linux-android` with the pinned toolchain (1.97.1) and the NDK 28.2
   clang wrappers. **Builds clean.**
2. A `shared.rs` JNI module in a copy of `apps/android/rust-wt`, wired to that
   crate, exposing `parsePairingUrl`, `formatContextWindow`, `abiVersion`,
   `wireMagic`, `wireVersion` as `extern "system"` functions.
3. The full `rust-wt` crate — **`console-core` included** — linked into a
   single `libconsole_wt_android.so` for `aarch64-linux-android`.
   **Builds clean, links clean, symbols export correctly** (§4.4).
4. Size measured three ways, producing the table in §4.4.

**What the spike established:**

- The mechanism works with the *existing* `console-core` in the dependency
  graph. Nothing about `console-core`'s heavy dependencies (`reqwest`,
  `tokio`, `wtransport`, `termy`) blocks a pure crate from being added
  alongside it.
- The JNI symbols export alongside the existing `WtNative` ones, in the same
  `.so`. **One library, one `loadLibrary`, one CI step** — as §5.3 requires.
- The cost is ~4 KB for the crate and ~300 KB for `pulldown-cmark` alone,
  which — together with the renderer mismatch (§3.2) — is why the Markdown
  parser is not migrated. A follow-up spike measured `diff_lines` at ~17 KB
  and compared the two diff implementations on 20,000 random edits (§3.1).

**What the spike did not test:** the Kotlin side. *(Now built — see Phase 2/3
status — but still not exercised on a device.)*

### 9.1 Implementation results

| Check | Result |
| :--- | :--- |
| `cargo test -p console-shared`: `diff` 26, `file_kind` 9, `file_links` 5, `format` 3 | ✅ pass |
| `cargo test`: `mend`, `path`, `throughput`, `tools` | ✅ pass (seen in earlier runs) |
| `cargo test`: `icons`, `mentions`, `pairing`, `page_title` | ◐ fixed after failures; **not re-seen passing** |
| `cargo check -p console-core -p console-ui --tests` | ⬜ **never completed** |
| Android `.so` for `aarch64-linux-android` | ✅ builds, 16 `SharedNative` symbols export |
| `./gradlew :app:compileDebugKotlin` | ✅ clean (after fixing `RunActivity.kt`) |
| Android JVM tests (6 classes) | ◐ 21 ran, 2 failed (`TokensPerSecondTest`, my test's fault), 2 skipped; the fix was committed, not re-run |
| `console-shared` vs the Kotlin implementations it replaced | ✅ measured: identical +/− counts on 20,000 random edits, identical line order on 85% (§3.1) |
| On a device / emulator | ⬜ not done |

**Why test runs looked hung.** macOS (`syspolicyd` / XProtect) scans every
freshly built unsigned test binary the first time it runs, which cost 15–27 s
*per test binary* here. `console-shared` has ten, so `cargo test` looked stuck
while the code was fine. Running the binary directly showed the same delay
*after* the tests had already printed `ok`. It is an environment cost, not a
code problem; adding the target directory to Terminal's Developer Tools
exemption removes most of it.

---

## 10. Definition of Done

- [x] `console-shared` exists, is a workspace member, and has no heavyweight
  dependencies (`serde_json` + `similar` only).
- [~] Every module in §4 exists there, with its tests, and the desktop imports
  from it. *(Modules and tests exist. `time`, `session_groups` and the Markdown
  parser were intentionally **not** moved — see the status block. Desktop
  compile unconfirmed.)*
- [x] `SharedNative` is the only surface Android code uses; the corresponding
  Kotlin duplicates are deleted.
- [x] The `.so` builds for `aarch64-linux-android` (verified). `x86_64`
  is built by `build.sh` but was not separately verified here.
- [~] ~~The Android `.so` grows by less than 40 KB.~~ **Not met:** it grew
  ~99 KB (§4.4). Accepted by the owner.
- [x] `console-shared/tests/diff_test.rs` exists and covers the §3.1 cases; the
  Kotlin `computeLineDiff` / `parseUnifiedDiff` are deleted.
- [~] `cd apps/android && ./gradlew :app:assembleDebug` succeeds.
  *(`compileDebugKotlin` is clean; a full `assembleDebug` was not run.)*
- [~] `cd apps/android && ./gradlew :app:testDebugUnitTest` passes.
  *(Six classes run; the two failures were fixed but not re-run. Full suite
  not run.)*
- [ ] `cd apps/desktop && cargo test -p console-shared -p console-core -p console-ui`
  passes. **Open.**
- [ ] CI's "Verify the APK carries WebTransport" step still passes — expected
  (same library), **not yet seen on a push.**
- [ ] Exercised on a device: pair, open a chat with edits, open the Changes tab.
  **Open.**

---

## 11. Related Work (out of scope, but adjacent)

- **The TLS trust-store defect** (§7.2) — **fixed in `97685cbf`** (see the
  note at the top of §7.2); not yet exercised on a device.
- **The terminal snapshot format is defined in three places**: the C++ bridge
  (`apps/android/app/src/main/cpp/console_terminal_jni.cpp:14`), the Kotlin
  decoder (`apps/android/app/src/main/kotlin/com/console/mobile/feature/terminal/native/TerminalFrame.kt:25`),
  and the Rust side (`console-core/src/types/terminal.rs`). The magic number
  and version are hand-synced. A drift here corrupts the terminal view
  silently. The `console-shared` encoding discipline (§5.2) is the right
  pattern, but unifying the terminal engines is a separate project.
