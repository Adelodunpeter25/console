import { useCallback, useEffect, useRef, useState } from "react";
import * as Linking from "expo-linking";
import type { OAuthProviderId } from "@console/types";
import { authService } from "@console/api";
import {
  addAuthCallbackListener,
  addAuthCompleteListener,
  isLocalAuthServerAvailable,
  startAuthServer,
  stopAuthServer,
  type AuthCallbackResult,
} from "@/modules/local-auth-server";
import { useAuthStore } from "@/stores/useAuthStore";

/**
 * Orchestrates the full mobile OAuth login flow on Android using a native
 * localhost server to catch the loopback redirect.
 *
 * Flow:
 *   1. POST /api/auth/login/url → { authUrl, redirectUri }
 *   2. Parse port + callbackPath from redirectUri
 *   3. Start native localhost server on that port
 *   4. Open authUrl in the system browser
 *   5. User authenticates → Google redirects to http://localhost:PORT/callback
 *   6. Native server catches it, emits onAuthCallback { code, state }
 *   7. POST /api/auth/login/callback { provider, code, state }
 *   8. Backend exchanges code for tokens, saves credentials
 *   9. Refresh auth status → UI shows connected
 *
 * The hook owns the server lifecycle: it stops the server on success, error,
 * timeout, or unmount. A 2-minute timeout covers the case where the user
 * lingers in the browser and Android reclaims the process.
 *
 * Android-only. On iOS the manual paste flow is used instead (the app gets
 * suspended when the browser opens and the server dies).
 */

const OAUTH_TIMEOUT_MS = 2 * 60 * 1000;

export interface UseLocalOAuthLoginResult {
  /** True while a login flow is in progress (server up, waiting for redirect). */
  isLoggingIn: boolean;
  /** Last error from the flow, cleared when a new flow starts. */
  error: string | null;
  /** True when the native localhost server is available on this platform. */
  isAvailable: boolean;
  /**
   * Start the OAuth login flow for a provider. Resolves when the token
   * exchange completes (success) or throws on error/timeout.
   */
  loginWithLocalServer: (provider: OAuthProviderId) => Promise<void>;
  /** Cancel any in-flight login and stop the server. */
  cancel: () => void;
}

export function useLocalOAuthLogin(): UseLocalOAuthLoginResult {
  const [isLoggingIn, setIsLoggingIn] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const isAvailable = isLocalAuthServerAvailable();

  const loadStatus = useAuthStore((state) => state.loadStatus);

  // Refs so the cleanup function always sees the latest values.
  const cleanupRef = useRef<(() => void) | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const stopServerAndCleanup = useCallback(async () => {
    cleanupRef.current?.();
    cleanupRef.current = null;
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    await stopAuthServer().catch(() => {});
  }, []);

  // Stop the server if the hook unmounts mid-flow.
  useEffect(() => {
    return () => {
      stopServerAndCleanup();
    };
  }, [stopServerAndCleanup]);

  const loginWithLocalServer = useCallback(
    async (provider: OAuthProviderId): Promise<void> => {
      if (!isLocalAuthServerAvailable()) {
        throw new Error(
          "Local OAuth server is not available on this platform. Use the manual paste flow.",
        );
      }

      // Reset state from any previous attempt.
      await stopServerAndCleanup();
      setError(null);
      setIsLoggingIn(true);

      try {
        // 1. Fetch the auth URL + redirect URI from the backend.
        const { authUrl, redirectUri } = await authService.getLoginUrl({ provider });

        // 2. Parse the port and callback path from the redirect URI.
        //    Backend returns e.g. "http://localhost:8085/oauth2callback".
        const parsed = new URL(redirectUri);
        const port = Number(parsed.port);
        const callbackPath = parsed.pathname;
        if (!port || !callbackPath) {
          throw new Error(`Unexpected redirect URI from server: ${redirectUri}`);
        }

        // 3. Start the native localhost server. This resolves true once the
        //    socket is bound; the redirect itself arrives later as an event.
        const started = await startAuthServer(port, callbackPath);
        if (!started) {
          throw new Error(
            `Could not start the auth server on port ${port}. Another app may be using it.`,
          );
        }

        // 4. Set up the event listeners before opening the browser so we
        //    don't miss a fast redirect.
        let resolveFlow: () => void;
        let rejectFlow: (err: Error) => void;
        const flowPromise = new Promise<void>((resolve, reject) => {
          resolveFlow = resolve;
          rejectFlow = reject;
        });

        const unsubCallback = addAuthCallbackListener(
          async (result: AuthCallbackResult) => {
            // 7-8. Exchange the code on the backend.
            try {
              await authService.handleCallback({
                provider,
                code: result.code,
                state: result.state || undefined,
              });
              // 9. Refresh auth status so the UI reflects the new credential.
              await loadStatus().catch(() => {});
              resolveFlow();
            } catch (err: unknown) {
              rejectFlow(
                err instanceof Error
                  ? err
                  : new Error("Token exchange failed on the server."),
              );
            }
          },
          (err) => {
            rejectFlow(new Error(err.error));
          },
        );

        const unsubComplete = addAuthCompleteListener(() => {
          // The server handled the redirect; we can tear it down now.
          stopServerAndCleanup();
        });

        cleanupRef.current = () => {
          unsubCallback();
          unsubComplete();
        };

        // 5. Timeout — if the user lingers in the browser and the process
        //    gets reclaimed, or they never complete auth, we give up.
        timeoutRef.current = setTimeout(() => {
          rejectFlow(new Error("Login timed out. Please try again."));
        }, OAUTH_TIMEOUT_MS);

        // 6. Open the auth URL in the system browser.
        await Linking.openURL(authUrl);

        // Wait for the callback exchange to complete (or fail/timeout).
        await flowPromise;
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : "Login failed.";
        setError(message);
        throw err;
      } finally {
        await stopServerAndCleanup();
        setIsLoggingIn(false);
      }
    },
    [loadStatus, stopServerAndCleanup],
  );

  const cancel = useCallback(() => {
    stopServerAndCleanup();
    setIsLoggingIn(false);
  }, [stopServerAndCleanup]);

  return {
    isLoggingIn,
    error,
    isAvailable,
    loginWithLocalServer,
    cancel,
  };
}
