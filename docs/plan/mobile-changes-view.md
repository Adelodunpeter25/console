# Mobile Changes View Plan

Reference: RepoGo Changes screenshot 2026-09-03 at 12.14.09 PM - branch header with 11 changes +755 -531, grouped folders with collapse chevrons, file rows with +N -M in green red and chevron to diff, bottom bar with Create Branch and Push and Commit.

## 1 Goals
- Parity with desktop Changes tab on mobile chat session.
- Grouped by directory tree, not flat list.
- Sticky summary in header, per-file counts, full-row tap to diff.
- Support ScreenHeader title + subtitle for file name + relative path.
- Fix file tree expanded-state loss on file open and back.

## 2 Current State
- Desktop inspector shows working changes + session changes, status letters M A D R, +N -M, diff tab on select.
- Server session-changes table stores path, status, additions, deletions, turnIndex, updatedAt. Sessions route exposes GET changes.
- Mobile has DiffView for tool results and progressive highlight file preview, but no session changes screen.
- FileTreeBrowser keeps expandedPaths in local useState, screen unmounts tree when previewing file, so back always resets to collapsed.

## 3 UX Proposal
- Entry: new Changes button in ChatScreen header next to Files and Terminal, plus optional row in session details. Opens full-screen ChangesScreen for selectedSessionId.
- Header: ScreenHeader title is session title, subtitle is N files +A -D plus scope label. Below header: segmented toggle This Turn and All Turns.
- List: LegendList grouped by folder. Folder row shows chevron, folder name, aggregated + - counts. File row shows status badge A M D, FileIcon, file name on line 1, relative path on line 2, +N -M on right, chevron.
- Empty: No working tree changes with hint text.
- Diff: tap file pushes diff view using existing DiffView, data from session changes diffText when available, fallback to git diff endpoint. Reuse progressive highlight path.
- Bottom: phase 1 read-only, no commit actions. Phase 2 optional stage and commit and push if server adds endpoints. Respect safe-area so last row is never covered.

## 4 File Tree Bug Fix
- Repro: Files tab, expand folder, tap file inside, press back or Android system back, list returns with folder collapsed.
- Cause: FilesScreen conditionally renders preview instead of tree, so FileTreeBrowser unmounts and its expandedPaths useState is discarded. No persistence across preview.
- Fix: lift expandedPaths to FilesScreen state and pass as controlled props to FileTreeBrowser, or persist to Legend observable keyed by projectRoot. Keep selection prefetch. Ensure Android back from file only clears selectedFile, not expanded set. Preserve search and scroll position.
- Acceptance: expand, open, back keeps same folders open and same scroll offset. Works for search mode and normal mode.

## 5 API and Data
- Add sessionService getChanges in packages api with optional turnIndex param.
- Add useSessionChanges hook in mobile hooks queries with short staleTime, refetch on focus, invalidate on run events and on foreground resume.
- Server follow-ups as separate tasks: return diffText, support turnIndex filter, include aggregated totals. Mobile should tolerate missing fields.

## 6 Implementation Steps
- 1: add getChanges client + hook + types for totals.
- 2: build ChangesScreen and ChangesTree components under screens changes and components changes, reuse FileIcon, DiffView, ScreenHeader subtitle.
- 3: wire ChatScreen header button and navigation via app store activeTab or modal route.
- 4: implement This Turn and All Turns filter client-side first, then server param when ready.
- 5: fix FileTreeBrowser expanded state lift plus prefetch plus keep mounted or lifted state.
- 6: typecheck mobile, manual test on device with 50+ line files and 10+ file session.

## 7 Verification
- Start run that edits multiple files, open Changes, confirm counts match server.
- Toggle This Turn and All Turns, confirm filtering.
- Tap file, confirm diff opens with name + relative path subtitle.
- File tree: expand 2 folders, open file, back, confirm still expanded. Repeat with Android system back.
- Background and foreground during run, confirm Changes refreshes without duplicate rows.
- Check safe-area: last row not covered by bottom bar, subtitle truncates with ellipsis.

## 8 Out of Scope
- Commit, push, branch creation on mobile.
- Git index lock handling and LSP.
- iOS specific nav, Android-first but keep iOS working via same header back.
