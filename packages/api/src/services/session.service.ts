import { getConsoleApiClient } from "../client";
import type {
  SessionHeader,
  SessionDetailResponse,
  CreateSessionDto,
  UpdateSessionDto,
} from "@console/types";

/**
 * Unwrap the standard `{ success, data }` API envelope.
 * Throws when the server reports failure or omits data.
 */
function unwrapData<T>(body: { success?: boolean; data?: T; error?: string }, action: string): T {
  if (body?.success === false || body?.data === undefined) {
    throw new Error(body?.error || `Failed to ${action}`);
  }
  return body.data;
}

export const sessionService = {
  async getSessions(params?: { cwd?: string; projectId?: string; onlyDeleted?: boolean }): Promise<SessionHeader[]> {
    const res = await getConsoleApiClient().get("/api/sessions", { params });
    return unwrapData(res.data, "list sessions");
  },

  async getSession(id: string): Promise<SessionDetailResponse> {
    const res = await getConsoleApiClient().get(`/api/sessions/${id}`);
    return unwrapData(res.data, "load session");
  },

  async createSession(payload: CreateSessionDto): Promise<SessionHeader> {
    const res = await getConsoleApiClient().post("/api/sessions", payload);
    return unwrapData(res.data, "create session");
  },

  async updateSession(id: string, payload: UpdateSessionDto): Promise<SessionHeader> {
    const res = await getConsoleApiClient().patch(`/api/sessions/${id}`, payload);
    return unwrapData(res.data, "update session");
  },

  async deleteSession(id: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().delete(`/api/sessions/${id}`);
    // delete returns `{ success, data: { id, deleted } }` — treat HTTP success as ok
    if (res.data?.success === false) {
      throw new Error(res.data?.error || "Failed to delete session");
    }
    return { success: true };
  },

  async restoreSession(id: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().post(`/api/sessions/${id}/restore`);
    if (res.data?.success === false) {
      throw new Error(res.data?.error || "Failed to restore session");
    }
    return { success: true };
  },

  async permanentlyDeleteSession(id: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().delete(`/api/sessions/${id}/permanent`);
    if (res.data?.success === false) {
      throw new Error(res.data?.error || "Failed to permanently delete session");
    }
    return { success: true };
  },
};
