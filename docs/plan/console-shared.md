# `console-shared` — One implementation of the logic both clients need

Extracts the pure, dependency-free logic that the desktop (Rust/GPUI) and the
Android app (Kotlin/Compose) currently implement twice into a new
`console-shared` crate, and exposes it to Android over the existing JNI bridge
in `apps/android/rust-wt`.

Today the two clients carry roughly **2,800 lines of Kotlin** that mirror
**~3,600 lines of Rust**. Some of it has already drifted: the pairing URL
parser — a *credential* parser — exists in both languages with different
strictness. That is the kind of divergence that turns a malformed paste into a
pairing with the wrong machine.

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
import **no** `gpui` symbols and depend only on `std`, `serde`, `regex`,
`pulldown_cmark`, `unicode_segmentation` and `chrono`:

| File | Lines | Notes |
| :--- | ---: | :--- |
| `crates/console-ui/src/markdown/mend.rs` | 301 | zero imports beyond `std` |
| `crates/console-ui/src/markdown/parser.rs` | 1014 | `pulldown_cmark` + `regex` |
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

---

## 2. Design Principles

1. **Pure means pure.** `console-shared` has **no** `reqwest`, no `tokio`, no
   `gpui`, no `wtransport`, no `termy`. Its only dependencies are `serde`
   (optional), `regex`, `chrono`, `unicode_segmentation`, `similar`, and
   `pulldown-cmark` (optional, behind the `markdown` feature). This is what
   makes it cheap to cross-compile and impossible to accidentally drag a
   runtime into the JNI surface. The spike (§4.4) measured the result: a
   shared crate with no reachable dependencies adds **~4 KB** to the Android
   `.so`.
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
| Pairing URL parse | `console-core/src/utils/pairing_url.rs` (201) | `data/model/PairingUrl.kt` (219) | **Diverged** — see below |
| HTML page title | `console-core/src/utils/page_title.rs` (108) | `core/util/PageTitles.kt` (112) | Duplicated |
| Markdown mend | `console-ui/src/markdown/mend.rs` (301) | `feature/chat/markdown/MarkdownMend.kt` (284) | Duplicated — Android file says "Port of … mend.rs" |
| Unified diff | `console-core/src/utils/diff.rs` (405) | `core/util/DiffUtils.kt` (98) | **Different algorithms** |
| Local port forward | `console-core/src/services/local_forward.rs` (239) | `core/forward/LocalForwardManager.kt` (290) | Duplicated |
| SSE frame parse | `console-core/src/utils/sse_reader.rs` (37) | `data/stream/ChatStreamClient.kt` (inline) | Duplicated |
| File-type icons | `console-ui/src/primitives/file_icons.rs` (213 arms) | `core/icons/FileTypeRegistry.kt` (166 + 53 lang aliases) | **Diverged** — see below |
| Tool call labels | `console-ui/src/chat/toolcalls.rs:189` | `core/util/ToolHelpers.kt` | Duplicated |
| MCP tool name split | `console-ui/src/chat/toolcalls.rs:1147` | `core/util/ToolHelpers.kt` | Duplicated |
| Mention parsing | `console-ui/src/common/input/mentions.rs` | `core/util/MentionParser.kt` | **Different semantics** |
| Time / path / format | `console-ui/src/utils/{time,path}.rs` | `core/util/{Time,Format}.kt` | Duplicated |
| Terminal grid | `console-core/src/services/terminal/termy.rs` | C++ Ghostty VT | **Do not share** |

### 3.1 The two divergences that matter

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

**Unified diff.** The desktop computes a *real* diff with `similar`'s Myers
algorithm (`console-core/src/utils/diff.rs:9`) and can diff two arbitrary
texts. Android *parses* a unified diff that the server already produced
(`core/util/DiffUtils.kt:8`). These are different operations, not the same
operation written twice. **Both** are worth sharing — the desktop's
`diff_lines` and Android's `parse_unified_diff` — but they are two functions,
not one.

**Mention parsing.** The desktop reconciles *ranges* against edited text
(`mentions.rs::adjust_mentions` / `reconcile_mentions`). Android scans text
for `@token` and returns ranges (`MentionParser.kt:15`). The Android version
is a *subset* of the desktop's model with a different entry point. The
scanner is worth sharing; the reconciliation is desktop-only for now.

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
│   ├── diff.rs           # from console-core/src/utils/diff.rs
│   ├── markdown/
│   │   ├── mod.rs
│   │   ├── mend.rs       # from console-ui/src/markdown/mend.rs
│   │   ├── parser.rs     # from console-ui/src/markdown/parser.rs  [feature = "markdown"]
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
regex = { workspace = true }
chrono = { workspace = true }
unicode-segmentation = { workspace = true }
similar = { workspace = true }
# ~300 KB of the Android `.so`. Feature-gated so the mobile build can leave
# it out — see §4.4 for the measurement.
pulldown-cmark = { workspace = true, optional = true }

[features]
default = []
serde = ["dep:serde"]
# The Markdown *parser* only. `mend` and `file_links` are pure string logic
# and are available without it.
markdown = ["dep:pulldown-cmark"]
```

`similar` is already a `console-core` dependency, so the diff engine costs
nothing extra in either build (§4.4).

### 4.2 Dependency rules

- `console-shared` **must not** depend on `console-core`, `console-ui`,
  `console-proto`, `gpui`, `reqwest`, `tokio`, `wtransport`, or `termy`.
  A `cargo tree` assertion in CI would be the enforcement; until then this is
  a review rule.
- `console-core` **may** depend on `console-shared` (it needs `pairing_url`,
  `page_title`, `file_kind`, `diff`).
- `console-ui` **may** depend on `console-shared` (it needs `mend`, `parser`,
  `file_links`, `mentions`, `format`, `icons`, `tools`).
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
| `console-shared`, minimal (no deps used) | 2,626,976 B | **+4 KB** |
| `console-shared`, full minus `similar` | 2,626,304 B | **+0 B** |
| `console-shared`, full (incl. `pulldown-cmark`) | 2,930,112 B | **+304 KB** |

Two conclusions, both load-bearing:

1. **The crate itself is free.** A `console-shared` with no reachable
   dependencies adds ~4 KB. The code is not the cost.
2. **`pulldown-cmark` is the cost: ~300 KB.** The Markdown parser pulls in
   `pulldown-cmark` + `pulldown-cmark-escape` + `unicase` + `bitflags`, and
   that is ~11% of the `.so`. `similar` adds **0 bytes** because it is
   already in the tree via `console-core` (`console-core/Cargo.toml`
   depends on it) and the linker was already carrying it.

**Decision:** `pulldown-cmark` stays behind a **feature flag** (§4.1), off by
default for the Android build. The Markdown *parser* is the one module that
needs it; `mend.rs` and `file_links.rs` are pure string logic and need
nothing. The desktop enables the feature; the Android `.so` does not, unless
a later phase decides the Android Markdown renderer should also be shared.

This keeps the Android `.so` at its current size while still sharing every
other module.

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
```

Move the existing unit tests with the code (the repo rule is that Rust tests
live in `tests/`, never inline `#[cfg(test)]` modules — see `AGENTS.md`).

**Verify:** `cd apps/desktop && cargo test -p console-core -p console-ui`.

This phase alone removes ~2,000 lines of duplicated *desktop* code with zero
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
| 6 | Diff | `core/util/DiffUtils.kt` | (none) |

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

### Phase 4 — Delete the now-dead Rust re-export shims

Once the desktop call sites import from `console-shared` directly, remove
the `pub use` shims from `console-core` / `console-ui`.

### Phase 5 — The local port forward (optional, larger)

`console-core/src/services/local_forward.rs` and
`apps/android/app/src/main/kotlin/com/console/mobile/core/forward/LocalForwardManager.kt`
both implement the same idea: a loopback TCP listener per remote port,
piping through the server's tunnel WebSocket.

This one is **not** a pure function — it owns sockets and a tokio task. It
needs the same JNI registry pattern `bridge.rs` already uses for WT streams
(handle-based, polled, not callback-based). It is listed here but is a
separate piece of work: it does not belong in Phase 1's "pure move".

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
  which is why §4.1 feature-gates it.

**What the spike did not test:** the Kotlin side. The JNI *encoding* is
specified in §5.2 but the Kotlin decoder and the `available` fallback are
Phase 2 work. The spike proved the Rust half and the packaging; the Kotlin
half is unproven.

---

## 10. Definition of Done

- `console-shared` exists, is a workspace member, and has no heavyweight
  dependencies.
- Every module in §4 exists there, with its tests, and the desktop imports
  from it (no duplicate implementations remain in `console-core` /
  `console-ui`).
- `SharedNative` is the only surface Android code uses; the corresponding
  Kotlin duplicates are deleted.
- The `.so` still builds for `aarch64-linux-android` and `x86_64-linux-android`
  via `apps/android/rust-wt/build.sh`.
- The Android `.so` size grows by **less than 20 KB** over the current
  2.6 MB baseline (§4.4) — i.e. `pulldown-cmark` stays behind the
  `markdown` feature and is not enabled for the mobile build.
- `cd apps/android && ./gradlew :app:assembleDebug` succeeds.
- `cd apps/android && ./gradlew :app:testDebugUnitTest` passes.
- `cd apps/desktop && cargo test -p console-shared -p console-core -p console-ui` passes.
- CI's "Verify the APK carries WebTransport" step still passes — the new
  surface ships in the same library.

---

## 11. Related Work (out of scope, but adjacent)

- **The TLS trust-store defect** (§7.2) is a live bug in the current Android
  build, independent of this plan. It deserves its own issue.
- **The terminal snapshot format is defined in three places**: the C++ bridge
  (`apps/android/app/src/main/cpp/console_terminal_jni.cpp:14`), the Kotlin
  decoder (`apps/android/app/src/main/kotlin/com/console/mobile/feature/terminal/native/TerminalFrame.kt:25`),
  and the Rust side (`console-core/src/types/terminal.rs`). The magic number
  and version are hand-synced. A drift here corrupts the terminal view
  silently. The `console-shared` encoding discipline (§5.2) is the right
  pattern, but unifying the terminal engines is a separate project.
