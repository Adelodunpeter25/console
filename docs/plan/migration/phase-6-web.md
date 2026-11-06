# Phase 6 — web (later, not a priority): Vercel build connected to the server

**Priority**: low. The desktop migration (phases 0–5) comes first; this phase starts only after
the cutover, once the NSH app *is* the Console app.

**Goal**: the NSH app as a static site on Vercel, talking to the Go server from a browser.

## Build

- `nsh package --target web` → static site (content-hashed files, precompressed brotli/gzip,
  webmanifest, split bundles). Serve `.wasm` as `application/wasm`; enable brotli on the host.
- Deploy from CI on merges to main, same shape as the existing release workflow.

## Server

- The web app and the server are different origins unless served together: enable CORS on
  `apps/server-go` for the Vercel origin (or front both with one domain) and list hosts in
  `web.connect` / `capabilities.network`.
- Auth: cookie/header story for the browser target — decide during phase 1 when the first
  round-trip lands, since the transport is shared.

## What the browser build loses (accepted)

- Native-only features: global shortcuts, single-instance, WKWebView cookie injection, tray.
  The browser gets the browser's sandbox; the desktop apps keep the native paths.

## Done when

- A browser session can chat, browse the built-in browser (iframes/capabilities permitting),
  and stream devices? — no: device streaming stays desktop/mobile-only; the web target gets
  chat, settings, editor and `/visualise`.
- Cold load of the site is acceptable (runtime ≈ 1.1 MB gzipped before app code).
