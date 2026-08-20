import { QueryClient } from "@tanstack/react-query";
import { setupAppFocusManager } from "./utils/app-focus-manager";

// Initialize AppState event listener for automatic focus & stale refetching on mobile
setupAppFocusManager();

/**
 * Shared QueryClient for mobile.
 * Configured with 15-second staleTime and refetchOnWindowFocus enabled,
 * allowing automatic syncing when foregrounding the app while preserving
 * battery with refetchIntervalInBackground: false.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: 2,
      refetchOnWindowFocus: true,
      refetchOnReconnect: true,
    },
  },
});
