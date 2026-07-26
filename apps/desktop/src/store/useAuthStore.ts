import { create } from "zustand";
import type { AuthStatusResponse } from "@console/types";
import { tauriApi } from "../lib/tauri-api";
import type { LoginUrlResult, OAuthCallbackResult } from "../types";

export type ProviderId = "gemini" | "antigravity";

interface AuthState {
  status: AuthStatusResponse | null;
  loading: boolean;
  /** The provider the user started a login flow for (null when idle). */
  pendingProvider: ProviderId | null;
  /** Last obtained OAuth login URL for `pendingProvider`. */
  loginUrl: LoginUrlResult | null;
  /** Result of the most recent OAuth callback exchange. */
  callbackResult: OAuthCallbackResult | null;
  error: string | null;

  loadStatus: () => Promise<void>;
  startLogin: (provider: ProviderId) => Promise<LoginUrlResult>;
  completeLogin: (
    provider: ProviderId,
    code: string,
    state?: string,
  ) => Promise<OAuthCallbackResult>;
  reset: () => void;
}

const INITIAL_STATUS: AuthStatusResponse = {
  gemini: { loggedIn: false },
  antigravity: { loggedIn: false },
};

export const useAuthStore = create<AuthState>((set, get) => ({
  status: null,
  loading: false,
  pendingProvider: null,
  loginUrl: null,
  callbackResult: null,
  error: null,

  loadStatus: async () => {
    set({ loading: true, error: null });
    try {
      const status = await tauriApi.getAuthStatus();
      set({ status, loading: false });
    } catch (e) {
      set({
        status: INITIAL_STATUS,
        loading: false,
        error: e instanceof Error ? e.message : "Failed to load auth status",
      });
    }
  },

  startLogin: async (provider: ProviderId) => {
    set({ pendingProvider: provider, error: null });
    const result = await tauriApi.getLoginUrl(provider);
    set({ loginUrl: result });
    return result;
  },

  completeLogin: async (provider: ProviderId, code: string, state?: string) => {
    set({ error: null });
    try {
      const result = await tauriApi.handleOAuthCallback(provider, code, state);
      set({ callbackResult: result, pendingProvider: null });
      // Refresh status so `loggedIn` reflects the new credential.
      await get().loadStatus();
      return result;
    } catch (e) {
      const message = e instanceof Error ? e.message : "OAuth callback failed";
      set({ error: message, pendingProvider: null });
      throw e;
    }
  },

  reset: () =>
    set({
      pendingProvider: null,
      loginUrl: null,
      callbackResult: null,
      error: null,
    }),
}));
