import { getConsoleApiClient } from "../client";
import type { ProviderCatalogEntry, Model } from "@console/types";

function unwrapData<T>(body: { success?: boolean; data?: T; error?: string }, action: string): T {
  if (body?.success === false || body?.data === undefined) {
    throw new Error(body?.error || `Failed to ${action}`);
  }
  return body.data;
}

export const providerService = {
  async getProviders(): Promise<ProviderCatalogEntry[]> {
    const res = await getConsoleApiClient().get("/api/providers");
    return unwrapData(res.data, "list providers");
  },

  async getProviderModels(providerId: string): Promise<{ provider: string; models: Model[] }> {
    const res = await getConsoleApiClient().get(`/api/providers/${providerId}/models`);
    return unwrapData(res.data, "list provider models");
  },
};
