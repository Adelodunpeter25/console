# File Preview Gating — Mobile Now, Server Later

## Problem

Opening a file in the mobile Files screen hits `GET /api/fs/file`, whose server
implementation (`FsService.readFileContent` in
`apps/server/api/src/services/fs.service.ts`) reads the **entire** file with
`Bun.file(path).text()` — no size limit and no binary check. Tapping any of
these on a phone today pulls megabytes over the network and renders garbage or
hangs the preview:

- Generated lockfiles: `bun.lockb`, `Cargo.lock`, `yarn.lock`,
  `package-lock.json`, `Podfile.lock`, `pnpm-lock.yaml`, … (the repo's own
  `package-lock.json` is ~352 KB; real-world ones reach several MB)
- Binary blobs: images, archives, executables, fonts, `.wasm`, `.pdf`, `.db`, …

## What's enforced now (mobile-only stopgap)

`apps/mobile/utils/file-guards.ts` blocks the preview **before** any request is
fired, using data the tree already carries (`stat.size` per entry, threaded
through `FileTreeBrowser.onSelectFile`):

| Rule | Detail |
| --- | --- |
| Lockfiles | basename in `{package-lock.json, npm-shrinkwrap.json, pnpm-lock.yaml, composer.lock}` **or** suffix `.lock` / `.lockb` |
| Binary | extension denylist (~50 extensions across images, media, archives, executables/objects, fonts, opaque docs) |
| Too large | size > 512 KB (`MAX_FILE_PREVIEW_BYTES`) |

The Files screen disables `useReadFile` for blocked paths and shows an inline
"can't preview" panel with the reason instead.

## Why this must move to the server

Mobile-only gating is a stopgap. The canonical gate belongs on
`GET /api/fs/file` because:

1. **The server pays the cost.** It loads the whole file into memory and ships
   it over the wire; blocking there saves bandwidth and memory, not just render
   time.
2. **One enforcement point for every client.** Desktop (`apps/desktop`) and any
   future web/cli file views get the same protection without duplicating rules.
3. **Client checks are advisory.** Name/extension lists drift and can't detect
   binary content reliably; the server can sniff bytes (e.g. null-byte /
   UTF-8 validity check) regardless of extension.
4. **Size is authoritative at the source.** A client-side stat can be stale;
   the server should decide from the actual file before reading it.

## Proposed server contract (follow-up)

In `readFileContent` / `GET /api/fs/file`:

- Stat first; reject files above a server-side cap (suggest 512 KB–1 MB) with
  HTTP **413** and a structured body:
  `{ success: false, error: { code: "FILE_TOO_LARGE", sizeBytes, maxBytes } }`
- Reject known-binary payloads (extension denylist + content sniff) with HTTP
  **415**: `{ success: false, error: { code: "BINARY_FILE" } }`
- Optionally tag lockfiles/generated files as `FILE_TYPE_BLOCKED`.
- Consider adding a `previewable: boolean` flag to `FsTreeEntry` so clients can
  grey out rows instead of discovering blocks on tap.

Once that ships, keep the mobile util purely as a fast-path UX optimization
(no round trip), with the server as the enforcement layer of record.
