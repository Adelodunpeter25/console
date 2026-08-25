import { runService, sessionKeys } from "@console/api";
import { queryClient } from "@/query-client";
import { startNativeChatStream } from "@/utils/native-stream";
import { useSessionStore } from "@/stores/useSessionStore";
import { setStatus } from "@/stores/useSessionStatusStore";
import { useProjectStore } from "@/stores/useProjectStore";
import type { ChatSessionState } from "@/types";
import { EMPTY_CHAT_SESSION } from "@/types/chat-state";

export const ABORT_MESSAGES = [
  "This operation was aborted.",
  "Run was aborted.",
  "The operation was aborted.",
  "aborted",
];

export function isAbortError(msg: string): boolean {
  const lower = msg.toLowerCase();
  return ABORT_MESSAGES.some((m) => lower.includes(m.toLowerCase()));
}

export function syncSessionStatus(
  sessionId: string,
  status: "idle" | "working" | "done" | "needs_attention",
): void {
  setStatus(sessionId, status);
}

export function updateSession(
  sessions: Record<string, ChatSessionState>,
  sessionId: string,
  update: (state: ChatSessionState) => ChatSessionState,
): Record<string, ChatSessionState> {
  return {
    ...sessions,
    [sessionId]: update(sessions[sessionId] ?? EMPTY_CHAT_SESSION),
  };
}

export function randomUUID(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `run-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export function finalizeSessionRun(
  setSessions: (updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>) => void,
  sessionId: string,
  hadError: boolean,
): void {
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (sessionState) => {
      const runs = sessionState.runs.length > 0 ? [...sessionState.runs] : [];
      if (runs.length > 0) {
        const latest = runs[runs.length - 1]!;
        if (latest.status === "working") {
          runs[runs.length - 1] = {
            ...latest,
            status: hadError ? "failed" : "completed",
            elapsedMs: latest.startedAt ? Date.now() - latest.startedAt : latest.elapsedMs,
          };
        }
      }
      return {
        ...sessionState,
        running: false,
        streamingText: "",
        streamingThinking: "",
        activeToolCalls: [],
        runs,
      };
    }),
  );
  syncSessionStatus(sessionId, hadError ? "needs_attention" : "done");
  useProjectStore.getState().refreshSessionHeader(sessionId).catch(() => {});
  queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
  queryClient.invalidateQueries({ queryKey: sessionKeys.detail(sessionId) }).catch(() => {});
}

export async function abortSessionStream(
  setSessions: (updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>) => void,
  sessionId: string,
): Promise<void> {
  try {
    await runService.abortRun(sessionId);
  } catch {
    // ignore
  }
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (sessionState) => {
      const runs = sessionState.runs.length > 0 ? [...sessionState.runs] : [];
      if (runs.length > 0) {
        const latest = runs[runs.length - 1]!;
        if (latest.status === "working") {
          runs[runs.length - 1] = {
            ...latest,
            status: "aborted",
            elapsedMs: latest.startedAt ? Date.now() - latest.startedAt : latest.elapsedMs,
          };
        }
      }
      return {
        ...sessionState,
        running: false,
        streamingText: "",
        streamingThinking: "",
        pendingQuestions: [],
        pendingPermissions: [],
        activeToolCalls: [],
        runs,
      };
    }),
  );
  syncSessionStatus(sessionId, "done");
}
