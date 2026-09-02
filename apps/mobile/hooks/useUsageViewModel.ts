import { useCallback } from "react";
import { useAuth } from "@/hooks/useAuth";
import { useAllUsage } from "@/hooks/useUsage";

/**
 * View-model for the Usage settings page.
 * Combines auth status + quota reports into display-ready cards.
 * This is the sole place where business logic lives — the screen stays presentational.
 */
export function useUsageViewModel() {
  const auth = useAuth();
  const { data: allUsage, isLoading, isRefetching, refetch, error } = useAllUsage();

  const cards = [
    {
      key: "antigravity" as const,
      displayName: "Google Antigravity",
      report: allUsage?.antigravity ?? null,
      loggedIn: Boolean(auth.status?.antigravity?.loggedIn),
      email: auth.status?.antigravity?.email ?? undefined,
    },
    {
      key: "codex" as const,
      displayName: "OpenAI Codex",
      report: allUsage?.codex ?? null,
      loggedIn: Boolean(auth.status?.codex?.loggedIn),
      email: auth.status?.codex?.email ?? undefined,
    },
  ];

  const onRefresh = useCallback(() => {
    void refetch();
  }, [refetch]);

  return {
    cards,
    isLoading: isLoading && !allUsage,
    isRefetching,
    onRefresh,
    error,
  };
}
