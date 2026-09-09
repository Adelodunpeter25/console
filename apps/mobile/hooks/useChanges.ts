import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useValue } from "@legendapp/state/react";
import { gitService } from "@console/api";
import { sessionsView$ } from "@/stores/useSessionStore";
import { app$ } from "@/stores/useAppStore";
import { project$ } from "@/stores/useProjectStore";
import { useGitStatus } from "@/hooks/useGit";
import {
  baseOf,
  buildRows,
  parseUnifiedDiff,
  stripRepoPrefix,
  sumTotals,
} from "@/utils/changes";

/**
 * View-model for the Changes tab: a live `git status` file list.
 * Keeps data-fetching, grouping, and diff caching out of the screen so the
 * screen only renders.
 *
 * Perf notes:
 * - totals/rows are memoized; rows rebuild only when the git file list,
 *   collapse set, or repoPath identity changes.
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

  const { summary, loading: isLoading, error, refetch } = useGitStatus(repoPath);

  // Ignored files (!) are not working-tree changes.
  const files = useMemo(
    () => (summary?.files ?? []).filter((f) => f.status !== "!"),
    [summary],
  );

  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [diffText, setDiffText] = useState<string | null>(null);
  const [diffLoading, setDiffLoading] = useState(false);
  const diffCache = useRef(new Map<string, string | null>());
  const diffReqId = useRef(0);

  const totals = useMemo(() => sumTotals(files), [files]);
  const rows = useMemo(
    () => buildRows(files, collapsed, repoPath),
    [files, collapsed, repoPath],
  );
  // Fingerprint drives LegendList extraData so count updates re-render
  // rows without changing row identities.
  const rowsFingerprint = useMemo(
    () => `${rows.length}:${totals.additions}:${totals.deletions}`,
    [rows.length, totals.additions, totals.deletions],
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
  }, [refetch]);

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
    () => (selectedPath ? files.find((c) => c.path === selectedPath) ?? null : null),
    [selectedPath, files],
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
    branch: summary?.branch ?? null,
    isLoading,
    error,
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
