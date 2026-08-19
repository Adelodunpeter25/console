import { QueryClient } from "@tanstack/react-query";

/**
 * Shared QueryClient for mobile.
 * 5-minute staleTime (no refetch-on-mount within that window) and
 * refetchOnWindowFocus disabled (prevents a full refetch storm when app foregrounds).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 5,
      retry: 2,
      refetchOnWindowFocus: false,
    },
  },
});
