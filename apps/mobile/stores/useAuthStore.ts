import { batch, observable } from "@legendapp/state";
import type { AuthStatusResponse, OAuthProviderId, ProviderId } from "@console/types";
import { authService, getConsoleApiClient } from "@console/api";

export type { OAuthProviderId, ProviderId } from "@console/types";

const INITIAL_STATUS: AuthStatusResponse = {
  gemini: { loggedIn: false },
  antigravity: { loggedIn: false },
  codebuff: { loggedIn: false },
  codex: { loggedIn: false },
};

/**
 * Backend auth state as Legend State observables.
 * See docs/legend-state-and-list-migration.md.
 *
 * Reads in components subscribe via `useValue(auth$.field)`;
 * imperative reads outside render use `.peek()`.
 */
export const auth$ = observable({
  status: null as AuthStatusResponse | null,
  loading: false,
  /** The provider currently going through the browser login flow (null when idle). */
  loggingIn: null as ProviderId | null,
  error: null as string | null,
  /** Per-provider configured project ID (from backend config file). */
  projectIds: {} as Partial<Record<ProviderId, string | undefined>>,
  savingProjectId: false,
});

export async function loadAuthStatus(): Promise<void> {
  batch(() => {
    auth$.loading.set(true);
    auth$.error.set(null);
  });
  try {
    const status = await authService.getAuthStatus();
    batch(() => {
      auth$.status.set(status);
      auth$.loading.set(false);
      auth$.projectIds.set({
        gemini: status.gemini.configuredProjectId,
        antigravity: status.antigravity.configuredProjectId,
      });
    });
  } catch (e) {
    batch(() => {
      auth$.status.set(INITIAL_STATUS);
      auth$.loading.set(false);
      auth$.error.set(e instanceof Error ? e.message : "Failed to load auth status");
    });
  }
}

/** Full OAuth browser flow: fetch login URL, open browser, then load status. */
export async function loginWithBrowser(provider: OAuthProviderId): Promise<void> {
  batch(() => {
    auth$.loggingIn.set(provider);
    auth$.error.set(null);
  });
  try {
    // Mobile has no local callback server. Fetch the login URL, open the
    // browser, then let the caller handle the redirect callback via
    // `submitOAuthCallback`.
    const { authUrl } = await authService.getLoginUrl({ provider });
    // Open the auth URL in the system browser.
    await openAuthUrl(authUrl);
    // Refresh status so `loggedIn` reflects the new credential.
    await loadAuthStatus();
  } catch (e) {
    auth$.error.set(e instanceof Error ? e.message : "Login failed");
    throw e;
  } finally {
    auth$.loggingIn.set(null);
  }
}

/** Codebuff device-code login: opens browser, polls until approved. */
export async function loginCodebuff(): Promise<void> {
  batch(() => {
    auth$.loggingIn.set("codebuff");
    auth$.error.set(null);
  });
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
        await loadAuthStatus();
        return;
      }
    }
    throw new Error("Codebuff login timed out.");
  } catch (e) {
    auth$.error.set(e instanceof Error ? e.message : "Codebuff login failed");
    throw e;
  } finally {
    auth$.loggingIn.set(null);
  }
}

/** Save a configured project ID to the backend for a provider. */
export async function saveProjectId(provider: ProviderId, projectId: string | undefined): Promise<void> {
  batch(() => {
    auth$.savingProjectId.set(true);
    auth$.error.set(null);
  });
  try {
    // The server exposes project-id persistence under /api/auth/project-id.
    await getConsoleApiClient().post("/api/auth/project-id", {
      provider,
      projectId: projectId?.trim() || undefined,
    });
    batch(() => {
      auth$.projectIds[provider].set(projectId?.trim() || undefined);
      auth$.savingProjectId.set(false);
    });
  } catch (e) {
    batch(() => {
      auth$.savingProjectId.set(false);
      auth$.error.set(e instanceof Error ? e.message : "Failed to save project ID");
    });
    throw e;
  }
}

export function resetAuth(): void {
  batch(() => {
    auth$.loggingIn.set(null);
    auth$.error.set(null);
  });
}

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
