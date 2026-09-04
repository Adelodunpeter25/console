import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useValue } from "@legendapp/state/react";
import { gitService } from "@console/api";
import { sessionsView$ } from "@/stores/useSessionStore";
import { app$ } from "@/stores/useAppStore";
import { project$ } from "@/stores/useProjectStore";
import { useSessionChanges } from "@/hooks/queries";
import { useGitStatus } from "@/hooks/useGit";
import {
  baseOf,
  buildRows,
  filterByScope,
  parseUnifiedDiff,
  stripRepoPrefix,
  sumTotals,
  type ChangesScope,
} from "@/utils/changes";

/**
 * View-model for the Changes tab. Keeps data-fetching, grouping, and diff
 * caching out of the screen so the screen only renders.
 *
 * Perf notes:
 * - scoped/totals/rows are memoized; rows rebuild only when scope, collapse
 *   set, or repoPath identity changes.
 * - collapsed is a Set keyed by dir; toggle creates one new Set (no array churn).
 * - diffs cached in a ref Map so revisiting a file never refetches.
 * - stale diff fetches cancelled via incrementing request id.
 */
export function useChanges() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  const projects = useValue(project$.projects);
  const sessionCwd = useValue(() =>
    selectedSessionId ? sessionsView$[selectedSessionId].sessionCwd.get() ?? null : null,
  );

  const project = useMemo(
    () =>
      projects.find((p) =>
        sessionCwd ? p.path === sessionCwd || sessionCwd.startsWith(p.path + "/") : false,
      ) ?? projects[0] ?? null,
    [projects, sessionCwd],
  );
  const repoPath = sessionCwd ?? project?.path ?? null;

  const { data: sessionChanges = [], isLoading, error, refetch, isFetching } =
    useSessionChanges(selectedSessionId ?? "");
  const { summary: gitSummary, refetch: refetchGit } = useGitStatus(repoPath);

  const [scope, setScope] = useState<ChangesScope>("all");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [diffText, setDiffText] = useState<string | null>(null);
  const [diffLoading, setDiffLoading] = useState(false);
  const diffCache = useRef(new Map<string, string | null>());
  const diffReqId = useRef(0);

  const scoped = useMemo(
    () => filterByScope(sessionChanges, scope),
    [sessionChanges, scope],
  );
  const totals = useMemo(() => sumTotals(scoped), [scoped]);
  const rows = useMemo(
    () => buildRows(scoped, collapsed, repoPath),
    [scoped, collapsed, repoPath],
  );
  // Fingerprint drives LegendList extraData so tool-count updates re-render
  // rows without changing row identities.
  const rowsFingerprint = useMemo(
    () => `${scope}:${rows.length}:${totals.additions}:${totals.deletions}:${isFetching}`,
    [scope, rows.length, totals.additions, totals.deletions, isFetching],
  );

  const toggleFolder = useCallback((name: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  const selectFile = useCallback((path: string) => setSelectedPath(path), []);
  const clearSelection = useCallback(() => setSelectedPath(null), []);

  const refresh = useCallback(() => {
    refetch();
    refetchGit();
  }, [refetch, refetchGit]);

  useEffect(() => {
    if (!selectedPath || !repoPath) {
      setDiffText(null);
      setDiffLoading(false);
      return;
    }
    const cached = diffCache.current.get(selectedPath);
    if (cached !== undefined) {
      setDiffText(cached);
      setDiffLoading(false);
      return;
    }
    const reqId = ++diffReqId.current;
    setDiffLoading(true);
    setDiffText(null);
    gitService
      .getDiff(repoPath, selectedPath)
      .then((d) => {
        if (diffReqId.current !== reqId) return;
        diffCache.current.set(selectedPath, d);
        setDiffText(d);
        setDiffLoading(false);
      })
      .catch(() => {
        if (diffReqId.current !== reqId) return;
        setDiffLoading(false);
      });
  }, [selectedPath, repoPath]);

  const selectedChange = useMemo(
    () => (selectedPath ? scoped.find((c) => c.path === selectedPath) ?? null : null),
    [selectedPath, scoped],
  );
  const selectedRel = selectedPath ? stripRepoPrefix(selectedPath, repoPath) : "";
  const selectedName = selectedPath ? baseOf(selectedPath) : "";
  const diff = useMemo(() => {
    if (!diffText) return null;
    try {
      return parseUnifiedDiff(diffText);
    } catch {
      return null;
    }
  }, [diffText]);

  return {
    selectedSessionId,
    repoPath,
    branch: gitSummary?.branch ?? null,
    isLoading,
    error,
    isFetching,
    scope,
    setScope,
    collapsed,
    toggleFolder,
    rows,
    rowsFingerprint,
    totals,
    selectedPath,
    selectFile,
    clearSelection,
    selectedChange,
    selectedRel,
    selectedName,
    diff,
    diffLoading,
    refresh,
  };
}
