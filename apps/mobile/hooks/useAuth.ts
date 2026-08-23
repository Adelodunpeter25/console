import { useCallback, useEffect, useState } from "react";
import * as Linking from "expo-linking";
import type { OAuthProviderId, ProviderId } from "@console/types";
import { useAuthStore } from "@/stores/useAuthStore";

/**
 * Auth status + OAuth login flow for mobile.
 *
 * The desktop app runs a local callback server to catch the OAuth redirect;
 * mobile can't do that, so it opens the auth URL in the system browser and
 * handles the redirect via a deep link (`expo-linking`). The deep-link URL
 * must be registered in the app config; this hook listens for it.
 */
export function useAuth() {
  const status = useAuthStore((state) => state.status);
  const loading = useAuthStore((state) => state.loading);
  const loggingIn = useAuthStore((state) => state.loggingIn);
  const loadStatus = useAuthStore((state) => state.loadStatus);
  const loginWithBrowser = useAuthStore((state) => state.loginWithBrowser);
  const loginCodebuff = useAuthStore((state) => state.loginCodebuff);
  const projectIds = useAuthStore((state) => state.projectIds);
  const saveProjectId = useAuthStore((state) => state.saveProjectId);
  const savingProjectId = useAuthStore((state) => state.savingProjectId);
  const error = useAuthStore((state) => state.error);
  const reset = useAuthStore((state) => state.reset);

  // Load auth status on mount.
  useEffect(() => {
    loadStatus().catch(() => {});
  }, [loadStatus]);

  const isLoggedIn = useCallback(
    (provider: OAuthProviderId) => Boolean(status?.[provider]?.loggedIn),
    [status],
  );

  const handleLogin = useCallback(
    async (provider: OAuthProviderId) => {
      await loginWithBrowser(provider);
    },
    [loginWithBrowser],
  );

  const refetch = useCallback(() => loadStatus(), [loadStatus]);

  return {
    status,
    isLoading: loading,
    loggingIn,
    refetch,
    isLoggedIn,
    getLoginUrlFor: handleLogin,
    login: handleLogin,
    loginCodebuff,
    projectIds,
    saveProjectId,
    savingProjectId,
    error,
    submitCallback: async (
      provider: OAuthProviderId,
      code: string,
      state?: string,
    ) => {
      // The backend consumes the code via /api/auth/login/callback. Mobile
      // opens the auth URL in the browser; if the redirect lands back in the
      // app via deep link, the code/state arrive here and we exchange them.
      const { authService } = await import("@console/api");
      await authService.handleCallback({ provider, code, state });
      await loadStatus();
    },
    isFetchingLoginUrl: loggingIn !== null,
    isSubmittingCallback: false,
    reset,
  };
}

/** Listen for the OAuth deep link (scheme://auth?code=...&state=...) and exchange it. */
export function useOAuthDeepLink() {
  const { submitCallback } = useAuth();

  useEffect(() => {
    const handleUrl = (event: { url: string }) => {
      const { hostname, queryParams } = Linking.parse(event.url);
      if (hostname !== "auth") return;
      const code = queryParams?.code;
      const state = queryParams?.state;
      const provider = (queryParams?.provider as OAuthProviderId) ?? "antigravity";
      if (typeof code === "string") {
        submitCallback(provider, code, typeof state === "string" ? state : undefined).catch(
          (err) => console.error("OAuth callback exchange failed:", err),
        );
      }
    };
    const sub = Linking.addEventListener("url", handleUrl);
    // Also handle the case where the app was opened via the link already.
    Linking.getInitialURL().then((url) => {
      if (url) handleUrl({ url });
    });
    return () => sub.remove();
  }, [submitCallback]);
}
