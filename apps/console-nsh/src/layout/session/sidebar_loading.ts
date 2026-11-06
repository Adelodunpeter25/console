// Loading-state presentation for sidebar chat rows, ported from
// console-ui's `layout/sidebar_loading.rs`. The sidebar receives chat
// headers from the backend while the desktop run and its prompts live in
// app state; the merge here keeps one source of truth per row for
// Working/Waiting display.

import type { SessionSummary } from "console-nsh/native";

export type LoadingState = {
  kind: "working";
  /** Unix seconds. */
  startedAt: number;
} | {
  kind: "waiting";
};

/// Resolve the visible loading state for one chat. The backend status covers
/// chats restored from an existing server run; the explicit inputs cover the
/// local optimistic window while a newly submitted run is connecting or
/// waiting for a permission/question response. Each row resolves
/// independently so multiple panes can each show their own running chat.
export function sessionLoadingState(
  session: SessionSummary,
  runningStartedAt: number | null,
  isWaiting: boolean,
): LoadingState | null {
  if (isWaiting || session.status === "needs_attention") return { kind: "waiting" };
  if (runningStartedAt !== null && runningStartedAt > 0) {
    return { kind: "working", startedAt: normalizeTimestamp(runningStartedAt) };
  }
  if (session.status === "working") {
    return { kind: "working", startedAt: normalizeTimestamp(session.updatedAt) };
  }
  return null;
}

/// Placeholder titles the server's autotitle treats as unset. Mirrors
/// `titles.IsGenericTitle` in the server; keep the two lists in step.
export function isGenericTitle(title: string): boolean {
  const trimmed = title.trim();
  return (
    trimmed === "" ||
    trimmed === "New Session" ||
    trimmed === "New mobile session" ||
    trimmed === "New Chat" ||
    trimmed === "New chat" ||
    trimmed === "Untitled"
  );
}

/// The title is still the placeholder but the first prompt is already sent,
/// so the server is generating a real one: show a loading label instead of
/// the placeholder text.
export function titleIsPending(session: SessionSummary): boolean {
  return isGenericTitle(session.title) && session.status === "working";
}

/// The live elapsed label for a working row ("Working for 0:42"). Waiting
/// rows deliberately retain the ordinary time label because they are paused
/// for input.
export function workingLabel(state: LoadingState, now: number): string | null {
  if (state.kind !== "working") return null;
  const elapsed = Math.max(0, Math.floor(now - normalizeTimestamp(state.startedAt)));
  const minutes = Math.floor(elapsed / 60);
  const seconds = elapsed % 60;
  return `Working for ${minutes}:${String(seconds).padStart(2, "0")}`;
}

function normalizeTimestamp(timestamp: number): number {
  // Server timestamps arrive in milliseconds; work in seconds.
  return timestamp > 10_000_000_000 ? Math.floor(timestamp / 1000) : timestamp;
}
