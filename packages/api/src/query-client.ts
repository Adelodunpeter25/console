import { QueryClient } from "@tanstack/react-query";

/**
 * Creates a QueryClient optimized for cross-platform Web & Mobile apps
 */
export function createConsoleQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 1000 * 60 * 5, // 5 minutes default stale time
        retry: 2,
        refetchOnWindowFocus: false, // Prevents excessive refetches on mobile background/foreground toggles
      },
    },
  });
}

export const consoleQueryClient = createConsoleQueryClient();
