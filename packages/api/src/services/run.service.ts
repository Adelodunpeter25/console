import { getConsoleApiClient } from "../client.js";
import type { RunPromptDto } from "@console/types";

export const runService = {
  async abortRun(sessionId: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post(`/api/sessions/${sessionId}/abort`);
    return res.data;
  },

  /**
   * Helper to initiate agent run endpoint. Real-time consumption is usually handled via EventSource / SSE.
   */
  async runSessionPrompt(sessionId: string, payload: RunPromptDto): Promise<Response> {
    const baseUrl = getConsoleApiClient().defaults.baseURL || "";
    return fetch(`${baseUrl}/api/sessions/${sessionId}/run`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
  },
};
