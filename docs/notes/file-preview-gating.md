# File Preview Gating — Server Enforcement + Mobile Fast Path

## Problem

Opening a file in the file preview hits `GET /api/fs/file`, whose implementation
(`FsService.readFileContent` in `apps/server/api/src/services/fs.service.ts`)
used to read the **entire** file with `Bun.file(path).text()` — no size limit
and no binary check. Tapping any of these pulled megabytes over the network and
rendered garbage or hung the client:

- Generated lockfiles: `bun.lockb`, `Cargo.lock`, `yarn.lock`,
  `package-lock.json`, `Podfile.lock`, `pnpm-lock.yaml`, … (the repo's own
  `package-lock.json` is ~352 KB; real-world ones reach several MB)
- Binary blobs: images, archives, executables, fonts, `.wasm`, `.pdf`, `.db`, …

## Shared rules — `packages/types/src/fs.ts`

One rule set is used by both sides:

| Rule | Detail |
| --- | --- |
| Lockfiles | basename in `{package-lock.json, npm-shrinkwrap.json, pnpm-lock.yaml, composer.lock}` **or** suffix `.lock` / `.lockb` |
| Binary | extension denylist (~50 extensions across images, media, archives, executables/objects, fonts, opaque docs) |
| Too large | size > `MAX_FILE_PREVIEW_BYTES` (512 KB) |

## Server enforcement (`GET /api/fs/file`) — layer of record

`FsService.readFileContent` now rejects before reading the body:

1. Name checks (lockfile → binary extension) via the shared predicates.
2. `stat`: reject non-regular files and anything over 512 KB.
3. Content sniff: first 8 KB scanned for NUL bytes — catches binaries whose
   extension isn't on the denylist.

Rejections throw `FilePreviewBlockedError`; the route translates them into:

| Code | HTTP | Extra fields |
| --- | --- | --- |
| `LOCKFILE_BLOCKED` | 415 | — |
| `BINARY_FILE` | 415 | — |
| `FILE_TOO_LARGE` | 413 | `sizeBytes`, `maxBytes` |

Body shape: `{ success: false, error: "<human message>", code, ...detail }`.
The shared client (`packages/api/src/services/fs.service.ts` `readFile`)
unwraps this so UIs show the real reason instead of axios's
"Request failed with status code 413".

## Mobile fast path

`apps/mobile/screens/files/files-screen.tsx` keeps a zero-round-trip pre-check:
tree entries carry `stat.size`, so blocked taps show an inline "can't preview"
panel and never fire the request. The server remains authoritative — the client
check is purely a UX optimization.

Tests: `cd apps/server && bun tests/fs-file-gating.test.ts`
