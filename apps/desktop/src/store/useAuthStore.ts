import { create } from "zustand";
import type { AuthStatusResponse } from "@console/types";
import { tauriApi } from "../lib/tauri-api";

export type ProviderId = "gemini" | "antigravity";

interface AuthState {
  status: AuthStatusResponse | null;
  loading: boolean;
  /** The provider currently going through the browser login flow (null when idle). */
  loggingIn: ProviderId | null;
  error: string | null;

  loadStatus: () => Promise<void>;
  /** Full automatic OAuth flow: opens browser, catches redirect, exchanges code. */
  loginWithBrowser: (provider: ProviderId) => Promise<void>;
  reset: () => void;
}

const INITIAL_STATUS: AuthStatusResponse = {
  gemini: { loggedIn: false },
  antigravity: { loggedIn: false },
};

export const useAuthStore = create<AuthState>((set, get) => ({
  status: null,
  loading: false,
  loggingIn: null,
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

  loginWithBrowser: async (provider: ProviderId) => {
    set({ loggingIn: provider, error: null });
    try {
      await tauriApi.loginWithBrowser(provider);
      // Refresh status so `loggedIn` reflects the new credential.
      await get().loadStatus();
    } catch (e) {
      const message = e instanceof Error ? e.message : "Login failed";
      set({ error: message });
      throw e;
    } finally {
      set({ loggingIn: null });
    }
  },

  reset: () =>
    set({
      loggingIn: null,
      error: null,
    }),
}));
