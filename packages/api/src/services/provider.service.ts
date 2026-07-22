import { getConsoleApiClient } from "../client.js";

export const providerService = {
  async getProviders(): Promise<Array<{ id: string; name: string }>> {
    const res = await getConsoleApiClient().get("/api/providers");
    return res.data;
  },

  async getProviderModels(providerId: string): Promise<Array<{ id: string; name: string }>> {
    const res = await getConsoleApiClient().get(`/api/providers/${providerId}/models`);
    return res.data;
  },
};
