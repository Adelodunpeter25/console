import { getConsoleApiClient } from "../client";
import type { RunPromptDto, AnswerQuestionDto, ApproveToolPermissionDto } from "@console/types";

/**
 * Path for the run re-attach SSE endpoint (GET).
 * Pass `since` (last seen event seq) to replay buffered events newer than it;
 * omit to go live immediately.
 */
export function getRunStreamPath(sessionId: string, since?: number): string {
  const base = `/api/sessions/${sessionId}/run/stream`;
  return since !== undefined ? `${base}?since=${since}` : base;
}

export const runService = {
  async abortRun(sessionId: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post(`/api/sessions/${sessionId}/abort`);
    if (res.data?.success === false) {
      throw new Error(res.data?.error || "Failed to abort run");
    }
    return { success: true };
  },

  async answerQuestion(
    sessionId: string,
    payload: AnswerQuestionDto,
  ): Promise<{ answered: boolean }> {
    const res = await getConsoleApiClient().post(`/api/sessions/${sessionId}/answer`, payload);
    if (res.data?.success === false) {
      throw new Error(res.data?.error || "Failed to answer question");
    }
    return { answered: true };
  },

  async approvePermission(
    sessionId: string,
    payload: ApproveToolPermissionDto,
  ): Promise<{ approved: boolean }> {
    const res = await getConsoleApiClient().post(`/api/sessions/${sessionId}/approve`, payload);
    if (res.data?.success === false) {
      throw new Error(res.data?.error || "Failed to approve permission");
    }
    return { approved: payload.allow };
  },

  /**
   * Initiate an agent run and return the raw SSE Response for streaming.
   */
  async runSessionPrompt(
    sessionId: string,
    payload: RunPromptDto,
    signal?: AbortSignal,
  ): Promise<Response> {
    const baseUrl = getConsoleApiClient().defaults.baseURL || "";
    return fetch(`${baseUrl}/api/sessions/${sessionId}/run`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal,
    });
  },
};
