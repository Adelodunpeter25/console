import { useCallback } from "react";
import { useAuthStatus, useGetLoginUrl, useHandleOAuthCallback } from "@console/api";
import type { OAuthProviderId } from "@console/types";

/** Auth status + OAuth login flow for mobile (server owns the tokens). */
export function useAuth() {
  const { data: status, isLoading, refetch } = useAuthStatus();
  const getLoginUrl = useGetLoginUrl();
  const handleCallback = useHandleOAuthCallback();

  const isLoggedIn = useCallback(
    (provider: OAuthProviderId) => Boolean(status?.[provider]?.loggedIn),
    [status],
  );

  /** Fetch a login URL for a provider so the app can open it in a browser. */
  const getLoginUrlFor = useCallback(
    async (provider: OAuthProviderId) => {
      const result = await getLoginUrl.mutateAsync({ provider });
      return result.authUrl;
    },
    [getLoginUrl],
  );

  const submitCallback = useCallback(
    async (provider: OAuthProviderId, code: string, state?: string) => {
      await handleCallback.mutateAsync({ provider, code, state });
      refetch();
    },
    [handleCallback, refetch],
  );

  return {
    status,
    isLoading,
    refetch,
    isLoggedIn,
    getLoginUrlFor,
    submitCallback,
    isFetchingLoginUrl: getLoginUrl.isPending,
    isSubmittingCallback: handleCallback.isPending,
  };
}
