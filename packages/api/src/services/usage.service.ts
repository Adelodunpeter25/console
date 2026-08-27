import { getConsoleApiClient } from "../client";
import type { UsageReport } from "@console/types";

function unwrapData<T>(body: { success?: boolean; data?: T; error?: string }, action: string): T {
  if (body?.success === false) {
    throw new Error(body?.error || `Failed to ${action}`);
  }
  // data may be null when not logged in — that is valid
  return body.data as T;
}

export const usageService = {
  async getProviderUsage(providerId: string): Promise<UsageReport | null> {
    const res = await getConsoleApiClient().get(`/api/providers/${providerId}/usage`);
    return unwrapData<UsageReport | null>(res.data, `get usage for ${providerId}`);
  },

  async getAllUsage(): Promise<Record<string, UsageReport | null>> {
    const res = await getConsoleApiClient().get("/api/usage");
    return unwrapData<Record<string, UsageReport | null>>(res.data, "get all usage");
  },
};
