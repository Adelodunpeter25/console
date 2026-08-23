import { useCallback, useEffect, useState } from "react";
import { getConsoleApiClient } from "@console/api";
import type { FileSearchResult } from "@console/types";
import { useAppStore } from "@/stores/useAppStore";

/** FFF-backed fuzzy file search for a session's working directory (@ refs). */
export function useFileSearch(sessionId?: string) {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<FileSearchResult[]>([]);
  const [loading, setLoading] = useState(false);

  const search = useCallback(
    async (q: string) => {
      if (!sessionId || !backendUrl) return;
      setLoading(true);
      try {
        const res = await getConsoleApiClient().get(`/api/assist/${sessionId}/search`, {
          params: { q },
        });
        setResults((res.data?.data?.items as FileSearchResult[]) ?? []);
      } catch (err) {
        console.error("File search error:", err);
        setResults([]);
      } finally {
        setLoading(false);
      }
    },
    [backendUrl, sessionId],
  );

  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      return;
    }
    const timer = setTimeout(() => search(query.trim()), 150);
    return () => clearTimeout(timer);
  }, [query, search]);

  return { query, setQuery, results, loading };
}
