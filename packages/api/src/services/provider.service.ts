import { getConsoleApiClient } from "../client";

function unwrapData<T>(body: { success?: boolean; data?: T; error?: string }, action: string): T {
  if (body?.success === false || body?.data === undefined) {
    throw new Error(body?.error || `Failed to ${action}`);
  }
  return body.data;
}

export const providerService = {
  async getProviders(): Promise<Array<{ id: string; name: string }>> {
    const res = await getConsoleApiClient().get("/api/providers");
    return unwrapData(res.data, "list providers");
  },

  async getProviderModels(
    providerId: string,
  ): Promise<{ provider: string; models: Array<{ id: string; name: string }> }> {
    const res = await getConsoleApiClient().get(`/api/providers/${providerId}/models`);
    return unwrapData(res.data, "list provider models");
  },
};
