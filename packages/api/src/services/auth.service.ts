import { getConsoleApiClient } from "../client.js";
import type { AuthStatusResponse, OAuthLoginUrlDto, OAuthCallbackDto } from "@console/types";

export const authService = {
  async getAuthStatus(): Promise<AuthStatusResponse> {
    const res = await getConsoleApiClient().get("/api/auth/status");
    return res.data;
  },

  async getLoginUrl(payload: OAuthLoginUrlDto): Promise<{ url: string; state: string }> {
    const res = await getConsoleApiClient().post("/api/auth/login/url", payload);
    return res.data;
  },

  async handleCallback(payload: OAuthCallbackDto): Promise<{ success: boolean; provider: string }> {
    const res = await getConsoleApiClient().post("/api/auth/login/callback", payload);
    return res.data;
  },
};
