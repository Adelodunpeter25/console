# Windows desktop runtime

Notes on the parts of the Windows client that are easy to misdiagnose from its
log (`%LOCALAPPDATA%\Console\logs\console.log`).

## Build identity

Every startup logs `Console <version> (<commit>) starting from <exe path>`.
`<exe path>` is the ground truth for which installed Console produced a log;
two installs can both be named `console.exe`.

## Single instance

Only one client is useful at a time: a second copy fights over the OAuth
loopback port and multiplies window-teardown noise. The first instance holds a
per-session named mutex (`Local\Console.Desktop.SingleInstance`) for its
lifetime; a later launch finds the existing window, restores it, and exits
before Tokio/GPUI start.

Builds that predate the guard do not create the mutex, so once the mutex is
won, the app also looks for an existing `Console` window (GPUI class
`Zed::Window`, title `Console`) owned by a `console.exe`. Finding one means an
older install is running: it is focused and a message names its path. Close or
uninstall that copy (Settings > Apps) before launching the new build. A Zed
editor window with a project named "Console" can match the title, but the
process check confirms it is not `console.exe` before acting on it.

## OAuth loopback

`POST /api/auth/login/url` returns a fixed `redirect_uri` per provider
(committed allowlist ports: Claude 53692, Antigravity 51121, Codex 1455). The
client binds only loopback interfaces — `127.0.0.1`, falling back to `::1` —
never `0.0.0.0` (a wildcard bind exposes the callback to the LAN and, on
Windows, shares the port with specific-address binds, which hides real
conflicts). Transient bind failures are retried with backoff.

The callback listener runs with a hard 120 s deadline and a nonblocking accept
loop, so an abandoned sign-in releases the port instead of pinning it for the
process lifetime. The success page is sent only when the returned `state`
matches the pending login.

Failures surface on the provider card in settings (instead of only in the
log), and the card offers a **Copy sign-in link** action while a sign-in is in
flight or failed. If the system browser opener truncates the authorize URL,
pasting the copied link works around it. `RUST_LOG=info` also logs the full
URL before it is opened.

## Log noise

Three GPUI window-teardown messages are expected and supply no signal:

- `Invalid window handle (0x80040102)`
- `Invalid window handle. (0x80070578)`
- `window not found`

They are races between a destroyed HWND and queued GPUI dispatcher work inside
one process — another instance cannot destroy this process's window handles.
The default log level suppresses exactly those messages from GPUI's own
targets; `RUST_LOG=debug` (or `trace`) restores them when debugging GPUI.
