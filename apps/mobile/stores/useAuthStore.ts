import { create } from "zustand";
import type { AuthStatusResponse, OAuthProviderId, ProviderId } from "@console/types";
import { authService, getConsoleApiClient } from "@console/api";

export type { OAuthProviderId, ProviderId } from "@console/types";

const INITIAL_STATUS: AuthStatusResponse = {
  gemini: { loggedIn: false },
  antigravity: { loggedIn: false },
  codebuff: { loggedIn: false },
  codex: { loggedIn: false },
};

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
  /** Full OAuth browser flow: fetch login URL, open browser, then load status. */
  loginWithBrowser: (provider: OAuthProviderId) => Promise<void>;
  /** Codebuff device-code login flow (browser open + status polling). */
  loginCodebuff: () => Promise<void>;
  /** Save a configured project ID to the backend for a provider. */
  saveProjectId: (provider: ProviderId, projectId: string | undefined) => Promise<void>;
  reset: () => void;
}

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
      const status = await authService.getAuthStatus();
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

  loginWithBrowser: async (provider: OAuthProviderId) => {
    set({ loggingIn: provider, error: null });
    try {
      // Mobile has no local callback server. Fetch the login URL, open the
      // browser, then let the caller handle the redirect callback via
      // `submitOAuthCallback`.
      const { authUrl } = await authService.getLoginUrl({ provider });
      // Open the auth URL in the system browser.
      await openAuthUrl(authUrl);
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
      // Device-code flow: get a login URL + fingerprint params, open the
      // system browser, then poll until the user approves or it expires.
      const start = await authService.startCodebuffLogin();
      await openAuthUrl(start.loginUrl);

      const deadline = Number(start.expiresAt) * 1000;
      while (Date.now() < deadline) {
        await sleep(2000);
        let completed = false;
        try {
          // Transient poll errors are retried until the deadline.
          const poll = await authService.pollCodebuffStatus({
            fingerprintId: start.fingerprintId,
            fingerprintHash: start.fingerprintHash,
            expiresAt: start.expiresAt,
          });
          completed = poll.completed;
        } catch {}
        if (completed) {
          await get().loadStatus();
          return;
        }
      }
      throw new Error("Codebuff login timed out.");
    } catch (e) {
      const message = e instanceof Error ? e.message : "Codebuff login failed";
      set({ error: message });
      throw e;
    } finally {
      set({ loggingIn: null });
    }
  },

  saveProjectId: async (provider: ProviderId, projectId: string | undefined) => {
    set({ savingProjectId: true, error: null });
    try {
      // The server exposes project-id persistence under /api/auth/project-id.
      await getConsoleApiClient().post("/api/auth/project-id", {
        provider,
        projectId: projectId?.trim() || undefined,
      });
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

/** Open a URL in the system browser (uses expo-linking). */
async function openAuthUrl(url: string): Promise<void> {
  // Use expo-linking if available; otherwise fall back to window.open on web.
  const { openURL } = await import("expo-linking");
  await openURL(url);
}

/** Poll-interval sleep built on Promise.withResolvers (project convention). */
function sleep(ms: number): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  setTimeout(resolve, ms);
  return promise;
}
