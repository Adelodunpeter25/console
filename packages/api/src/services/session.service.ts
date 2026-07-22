import { getConsoleApiClient } from "../client.js";
import type {
  SessionHeader,
  SessionDetailResponse,
  CreateSessionDto,
  UpdateSessionDto,
} from "@console/types";

export const sessionService = {
  async getSessions(): Promise<SessionHeader[]> {
    const res = await getConsoleApiClient().get("/api/sessions");
    return res.data;
  },

  async getSession(id: string): Promise<SessionDetailResponse> {
    const res = await getConsoleApiClient().get(`/api/sessions/${id}`);
    return res.data;
  },

  async createSession(payload: CreateSessionDto): Promise<SessionHeader> {
    const res = await getConsoleApiClient().post("/api/sessions", payload);
    return res.data;
  },

  async updateSession(id: string, payload: UpdateSessionDto): Promise<SessionHeader> {
    const res = await getConsoleApiClient().patch(`/api/sessions/${id}`, payload);
    return res.data;
  },

  async deleteSession(id: string): Promise<{ success: boolean }> {
    const res = await getConsoleApiClient().delete(`/api/sessions/${id}`);
    return res.data;
  },
};
