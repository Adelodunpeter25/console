# Message storage — skeleton, parts and the image store

Status: **stages 1 to 5 built and committed. Stage 6 (drop the old `content` column) is deliberately not done.**

## Why

A message used to be one JSON document in `messages.content`. On a real database, 66% of those bytes were tool output and 13% tool arguments (each JSON wrapped in base64 wrapped in more JSON), and 15% were pasted images. The words people actually wrote were about 3.5%. Finding or reading anything meant loading megabytes.

## What exists now

Every message is stored twice during the transition, and the copies are kept provably identical:

| Column / table | Holds | Used for |
| :--- | :--- | :--- |
| `messages.content` | the original JSON, untouched | **still the source of truth**; the fallback for everything |
| `messages.skeleton` | the JSON with heavy bodies replaced by `{"$part":N}` markers | rebuilding |
| `messages.text` | the words: what you wrote, what the assistant said, and one line per tool call (`read_file src/auth.go`) | search (`session.Service.Search`) |
| `messages.split_state` | `0` not yet split, `1` split, `2` could not be split (read `content`) | the migration |
| `message_parts` | one row per extracted body: tool arguments, tool results, images, screenshots, stored as raw bytes (not base64) | rebuilding |
| `<session>.images/<sha256>` | image and screenshot bodies as files, named by the hash of their bytes | rebuilding |

Code: `internal/msgparts` (the codec), `internal/services/session/` (`message_store.go` writes, `message_read.go` reads, `backfill.go` converts old rows, `image_store.go` the image files).

## The rules that keep it safe

1. **`content` is never modified.** Writes add to it; the backfill reads it; nothing rewrites it.
2. **A rebuild is only served if it equals `content`.** `Load` rebuilds each message from skeleton + parts (+ image file) and compares it with `content` by meaning (key order and whitespace ignored). Equal: serve the rebuild. Different, or anything unreadable: serve `content`, log it, and count it. `session.ParityStats()` reports checked and mismatched; **mismatched should be zero.**
3. **A failure never fails a message.** A message that cannot be split is stored whole and marked `2`. An image the store cannot write stays in the database. A missing or damaged file falls back to `content`.
4. **Fields the codec does not know are carried through.** The skeleton keeps everything it did not extract, so a field added to the message format later survives instead of disappearing.
5. **Each message is verified once per process.** After that it is served from `content` without repeating the work. Rewriting a session's history clears this.

## Images

Each session has its own `<session>.images/` folder beside its database, so images share the session's lifetime: deleting the session or its project deletes them, and moving a session between projects moves the folder. There is no cross-session sharing and so no reference counting. The same image pasted twice in one session is stored once.

`ReplaceMessages` sweeps files nothing refers to, but only files older than an hour (`ImageSweepGrace`), because a file is written just before its row commits. The sweep only touches names the store creates (a 64-character hex digest, or a stale `.tmp-` file), never directories or symlinks.

## What it measured, on 9 real sessions (4,812 messages)

| | Size |
| :--- | :--- |
| `content` column today | 22.1 MB |
| After dropping `content`: skeleton + text + raw part bodies | 15.5 MB |
| Image files on disk | 2.5 MB |
| Bytes touched to read the words of every message | 0.94 MB (was 22.1 MB) |

The saving is real but modest in absolute terms on this data. The larger win is that reading words no longer drags tool output and images with it.

## Search

`session.Service.Search` (`internal/services/session/search.go`) is the one definition of a match. It reads only the `text` column (words: never tool output, thinking, images or server-injected messages), requires every query word in the same message, folds Unicode case, treats every character as literal, and returns hits newest first with character offsets and a one-line snippet. `console.sessions.search` is a thin caller and the in-chat Cmd+F will be another, so the two cannot disagree. A session whose old messages are not yet split is converted on the way through, so history is never skipped. By default the agent searches its own project and leaves out the session it is running in. Measured on a copy of the real history: about 10 ms steady-state across 12 sessions, 0.6 s for the first search while it converts.

## What is NOT done

- **Stage 6: dropping `content`.** This is the only step that is hard to undo, so it is a separate, deliberate change. Do it only after a stretch of real use with `ParityStats()` showing zero mismatches. Until then storage is *larger* than before, not smaller.
- **In-chat find (Cmd+F, and the Android equivalent).** The server half exists: `session.Service.Search` returns every match with character offsets for highlighting. Not built: the HTTP route (`GET /api/sessions/:id/search`) and the two clients (a find bar, highlighting inside the message renderers, next/previous). Chats load 50 messages at a time, so match locally over what is loaded first and fall back to the server for older pages.
- **Runs still load whole transcripts** through `Load`, so they pay for the rebuild. Measured in Python on the worst real session, a full rebuild cost roughly 2x a plain read; a run that bypasses the JSON and hands parts straight to the model would be faster than today, but that second read path is not written.
- **Linux/other platforms:** nothing here is platform specific, but it has only been run on macOS.

## Known limits

- The image folder is per session, so the same screenshot in two sessions is stored twice.
- A session database written by the new code and then opened by an older server still works (`content` is complete), but any message the old server appends has no skeleton or parts until the new server reads that session again, which converts it.
- Three of the twelve databases found on the development machine are unreadable or empty and were skipped by the tests. Two are empty stubs, one is corrupt ("database disk image is malformed").

## Tests

`go test ./tests/msgparts/` (the codec, including a round trip over every real message), `go test ./tests/sessions/` (writes, backfill, reads, fallback on damage, images, sweep, deletion, fork, project move). The real-data tests copy your databases and never open the originals for writing. `TestNoImportUsesAMisspelledModulePath` guards against a recurring import typo.
