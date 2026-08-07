import { create } from "zustand";
import type { AuthStatusResponse, ProviderId } from "@console/types";
import { tauriApi } from "../lib/tauri-api";

export type { OAuthProviderId, ProviderId } from "@console/types";

interface AuthState {
  status: AuthStatusResponse | null;
  loading: boolean;
  /** The provider currently going through the browser login flow (null when idle). */
  loggingIn: ProviderId | null;
  error: string | null;
  /** Per-provider configured project ID (from backend config file). */
  projectIds: Partial<Record<ProviderId, string | undefined>>;
  savingProjectId: boolean;

  loadStatus: () => Promise<void>;
  /** Full automatic OAuth flow: opens browser, catches redirect, exchanges code. */
  loginWithBrowser: (provider: ProviderId) => Promise<void>;
  /** Codebuff device-code login flow (browser open + status polling). */
  loginCodebuff: () => Promise<void>;
  /** Save a configured project ID to the backend for a provider. */
  saveProjectId: (provider: ProviderId, projectId: string | undefined) => Promise<void>;
  reset: () => void;
}

const INITIAL_STATUS: AuthStatusResponse = {
  gemini: { loggedIn: false },
  antigravity: { loggedIn: false },
  codebuff: { loggedIn: false },
};

export const useAuthStore = create<AuthState>((set, get) => ({
  status: null,
  loading: false,
  loggingIn: null,
  error: null,
  projectIds: {},
  savingProjectId: false,

  loadStatus: async () => {
    set({ loading: true, error: null });
    try {
      const status = await tauriApi.getAuthStatus();
      set({
        status,
        loading: false,
        projectIds: {
          gemini: status.gemini.configuredProjectId,
          antigravity: status.antigravity.configuredProjectId,
        },
      });
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

  /** Codebuff device-code login: opens browser, polls until approved. */
  loginCodebuff: async () => {
    set({ loggingIn: "codebuff", error: null });
    try {
      await tauriApi.loginCodebuff();
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

  saveProjectId: async (provider: ProviderId, projectId: string | undefined) => {
    set({ savingProjectId: true, error: null });
    try {
      await tauriApi.setProjectId(provider, projectId?.trim() || undefined);
      set((state) => ({
        projectIds: { ...state.projectIds, [provider]: projectId?.trim() || undefined },
        savingProjectId: false,
      }));
    } catch (e) {
      set({
        savingProjectId: false,
        error: e instanceof Error ? e.message : "Failed to save project ID",
      });
      throw e;
    }
  },

  reset: () =>
    set({
      loggingIn: null,
      error: null,
    }),
}));
