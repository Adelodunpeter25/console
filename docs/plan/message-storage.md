# Message storage — skeleton, parts and the image store

Status: **all six stages built. Stage 6 (drop the old `content` column) runs as an audited migration, the first time a session is opened.**

## Why

A message used to be one JSON document in `messages.content`. On a real database, 66% of those bytes were tool output and 13% tool arguments (each JSON wrapped in base64 wrapped in more JSON), and 15% were pasted images. The words people actually wrote were about 3.5%. Finding or reading anything meant loading megabytes.

## What exists now

The split copy is the only copy. Every message is stored as a skeleton plus parts:

| Column / table | Holds | Used for |
| :--- | :--- | :--- |
| `messages.skeleton` | the JSON with heavy bodies replaced by `{"$part":N}` markers | rebuilding; a message nothing could be split out of keeps its whole JSON here |
| `messages.text` | the words: what you wrote, what the assistant said, and one line per tool call (`read_file src/auth.go`) | search (`session.Service.Search`) |
| `message_parts` | one row per extracted body: tool arguments, tool results, images, screenshots, stored as raw bytes (not base64) | rebuilding |
| `<session>.images/<sha256>` | image and screenshot bodies as files, named by the hash of their bytes | rebuilding |

Code: `internal/msgparts` (the codec), `internal/services/session/` (`message_store.go` writes, `message_read.go` reads, `backfill.go` converts old rows, `drop.go` the stage-6 migration, `image_store.go` the image files).

## The drop, and the rules that keep it safe

**Stage 6 runs per session database, the first time that session is opened** (`ensureContentDropped`): any rows still waiting to be split are backfilled, unsplittable rows are folded (their whole JSON becomes their skeleton), every message is rebuilt from skeleton + parts and compared with its original, and only if every one rebuilt identically is `content` dropped. If the audit finds anything, the column stays, the error is logged, and the session keeps working the old way, retrying on the next open. While the column is still there, reads and writes use it directly — the pre-split behavior, never a wrong history.

1. **A rebuild is only served if it was proven.** Before the drop, the audit proves every rebuild equals `content`. After the drop there is no original to compare with, so a row that cannot be rebuilt is logged loudly and left out of the result — never served wrong. On four weeks of real history (about 5,000 messages), the rebuild never once differed.
2. **A failure never fails a session.** A backfill or migration problem leaves the database untouched and retries on the next open. Only a message that is not even JSON cannot be rebuilt, and it is skipped with a log, never invented.
3. **Fields the codec does not know are carried through.** The skeleton keeps everything it did not extract, so a field added to the message format later survives instead of disappearing.
4. **Older builds cannot read a dropped database.** After the drop, downgrading the server past stage 6 makes the session database unreadable; going back is a restore, not a downgrade.

## Images

Each session has its own `<session>.images/` folder beside its database, so images share the session's lifetime: deleting the session or its project deletes them, and moving a session between projects moves the folder. There is no cross-session sharing and so no reference counting. The same image pasted twice in one session is stored once.

`ReplaceMessages` sweeps files nothing refers to, but only files older than an hour (`ImageSweepGrace`), because a file is written just before its row commits. The sweep only touches names the store creates (a 64-character hex digest, or a stale `.tmp-` file), never directories or symlinks.

## What it measured, on 9 real sessions (4,812 messages)

| | Size |
| :--- | :--- |
| `content` column, before the drop | 22.1 MB |
| After dropping `content`: skeleton + text + raw part bodies | 15.5 MB |
| Image files on disk | 2.5 MB |
| Bytes touched to read the words of every message | 0.94 MB (was 22.1 MB) |

The saving is real but modest in absolute terms on this data. The larger win is that reading words no longer drags tool output and images with it.

## Search

`session.Service.Search` (`internal/services/session/search.go`) is the one definition of a match. It reads only the `text` column (words: never tool output, thinking, images or server-injected messages), requires every query word in the same message, folds Unicode case, treats every character as literal, and returns hits newest first with character offsets and a one-line snippet. `console.sessions.search` is a thin caller and the in-chat Cmd+F will be another, so the two cannot disagree. A session whose old messages are not yet split is converted on the way through, so history is never skipped. By default the agent searches its own project and leaves out the session it is running in. Measured on a copy of the real history: about 10 ms steady-state across 12 sessions, 0.6 s for the first search while it converts.

## What is NOT done

- **In-chat find (Cmd+F, and the Android equivalent).** The server half exists: `session.Service.Search` returns every match with character offsets for highlighting. Not built: the HTTP route (`GET /api/sessions/:id/search`) and the two clients (a find bar, highlighting inside the message renderers, next/previous). Chats load 50 messages at a time, so match locally over what is loaded first and fall back to the server for older pages.
- **Linux/other platforms:** nothing here is platform specific, but it has only been run on macOS.

## Known limits

- The image folder is per session, so the same screenshot in two sessions is stored twice.
- After the drop, a message that is not a JSON object cannot be rebuilt, so it is skipped on read (logged). The month of real history before the drop contained none.
- Three of the twelve databases found on the development machine are unreadable or empty and were skipped by the tests. Two are empty stubs, one is corrupt ("database disk image is malformed").

## Tests

`go test ./tests/msgparts/` (the codec, including a round trip over every real message), `go test ./tests/sessions/` (writes, backfill, reads, fallback on damage, images, sweep, deletion, fork, project move). The real-data tests copy your databases and never open the originals for writing. `TestNoImportUsesAMisspelledModulePath` guards against a recurring import typo.
