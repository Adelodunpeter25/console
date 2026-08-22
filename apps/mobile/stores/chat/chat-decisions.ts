import { runService } from "@console/api";
import type { ChatSessionState } from "../../types";
import { updateSession, syncSessionStatus } from "./chat-stream-runner";

export async function answerSessionQuestion(
  setSessions: (updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>) => void,
  getSessions: () => Record<string, ChatSessionState>,
  sessionId: string,
  requestId: string,
  answer: string | string[],
): Promise<void> {
  try {
    await runService.answerQuestion(sessionId, { requestId, answer });
  } catch (err) {
    console.error("answerQuestion error:", err);
    throw err;
  } finally {
    setSessions((sessions) =>
      updateSession(sessions, sessionId, (sessionState) => ({
        ...sessionState,
        pendingQuestions: sessionState.pendingQuestions.filter(
          (q) => q.request.requestId !== requestId,
        ),
      })),
    );
    const pendingCount = getSessions()[sessionId]?.pendingQuestions.length ?? 0;
    syncSessionStatus(sessionId, pendingCount > 0 ? "needs_attention" : "working");
  }
}

export async function approveSessionPermission(
  setSessions: (updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>) => void,
  getSessions: () => Record<string, ChatSessionState>,
  sessionId: string,
  requestId: string,
  allow: boolean,
): Promise<void> {
  try {
    await runService.approvePermission(sessionId, { requestId, allow });
  } catch (err) {
    console.error("approvePermission error:", err);
    throw err;
  } finally {
    setSessions((sessions) =>
      updateSession(sessions, sessionId, (sessionState) => ({
        ...sessionState,
        pendingPermissions: sessionState.pendingPermissions.filter(
          (p) => p.request.requestId !== requestId,
        ),
      })),
    );
    const pendingCount = getSessions()[sessionId]?.pendingPermissions.length ?? 0;
    syncSessionStatus(sessionId, pendingCount > 0 ? "needs_attention" : "working");
  }
}
