import { getConsoleApiClient } from "../client";
import type { AuthStatusResponse, OAuthLoginUrlDto, OAuthCallbackDto } from "@console/types";

function unwrapData<T>(body: { success?: boolean; data?: T; error?: string }, action: string): T {
  if (body?.success === false || body?.data === undefined) {
    throw new Error(body?.error || `Failed to ${action}`);
  }
  return body.data;
}

export const authService = {
  async getAuthStatus(): Promise<AuthStatusResponse> {
    const res = await getConsoleApiClient().get("/api/auth/status");
    return unwrapData(res.data, "get auth status");
  },

  async getLoginUrl(
    payload: OAuthLoginUrlDto,
  ): Promise<{ authUrl: string; state: string; redirectUri: string }> {
    const res = await getConsoleApiClient().post("/api/auth/login/url", payload);
    return unwrapData(res.data, "get login url");
  },

  async handleCallback(payload: OAuthCallbackDto): Promise<{ success: boolean; provider: string }> {
    const res = await getConsoleApiClient().post("/api/auth/login/callback", payload);
    return unwrapData(res.data, "handle auth callback");
  },
};
