import { getConsoleApiClient } from "../client.js";
import type { RunPromptDto } from "@console/types";

export const runService = {
  async abortRun(sessionId: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post(`/api/sessions/${sessionId}/abort`);
    if (res.data?.success === false) {
      throw new Error(res.data?.error || "Failed to abort run");
    }
    return { success: true };
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
