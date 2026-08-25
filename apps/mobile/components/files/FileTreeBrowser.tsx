import React, { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Pressable, RefreshControl, View, Text } from "react-native";
import { LegendList } from "@legendapp/list/react-native";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { theme } from "@/styles/theme";
import { fsService } from "@console/api";
import type { FsTreeEntry } from "@console/types";
import { TreeRow, FileTreeRow, LoadingRow, EmptyFolderRow } from "./FileTreeRows";

const OPTIMISTIC_SELECTION_TIMEOUT_MS = 1000;

function ancestorPaths(path: string): ReadonlyArray<string> {
  const parts = path.split("/").filter(Boolean);
  const ancestors: string[] = [];
  for (let index = 1; index < parts.length; index += 1) {
    ancestors.push(parts.slice(0, index).join("/"));
  }
  return ancestors;
}

export interface FileTreeBrowserProps {
  /** Immediate children of the project root (root-level query lives in the screen). */
  readonly entries: ReadonlyArray<FsTreeEntry>;
  readonly projectRoot: string;
  readonly error: string | null;
  readonly isPending: boolean;
  readonly searchQuery: string;
  /** Server-side FFF results; non-null means search mode is active. */
  readonly searchResults?: ReadonlyArray<FsTreeEntry> | null;
  readonly isSearching?: boolean;
  readonly selectedPath: string | null; // absolute path
  readonly onRefresh: () => void;
  /** Second arg is the file's stat size (bytes) when known, for pre-fetch gating. */
  readonly onSelectFile: (absolutePath: string, fileSize?: number) => void;
}

/**
 * Lazy file tree: each directory loads its immediate children on expansion.
 * Children are cached by react-query (`["fs", "children", path]`), so
 * re-expanding a directory never refetches while data is fresh.
 *
 * Search is server-side (FFF-backed `/api/fs/search`) — there is no local
 * fallback; if the daemon is unreachable, search simply returns no results.
 */
export function FileTreeBrowser(props: FileTreeBrowserProps) {
  const queryClient = useQueryClient();
  const [expandedPaths, setExpandedPaths] = useState<ReadonlySet<string>>(() => new Set());
  const [pendingSelection, setPendingSelection] = useState<{
    readonly path: string;
    readonly selectedPathAtPress: string | null;
  } | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const controlledSelectedPathRef = useRef(props.selectedPath);
  controlledSelectedPathRef.current = props.selectedPath;
  const pendingSelectionTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // One query per expanded directory.
  const expandedList = useMemo(() => Array.from(expandedPaths), [expandedPaths]);
  const childrenQueries = useQueries({
    queries: expandedList.map((dirPath) => ({
      queryKey: ["fs", "children", dirPath] as const,
      queryFn: () => fsService.getFsEntries(dirPath, 1),
      staleTime: 15_000,
      enabled: true,
    })),
  });
  const childrenByPath = useMemo(() => {
    const map = new Map<string, { entries?: FsTreeEntry[]; pending: boolean }>();
    expandedList.forEach((dirPath, i) => {
      const q = childrenQueries[i];
      map.set(dirPath, { entries: q.data as FsTreeEntry[] | undefined, pending: q.isPending });
    });
    return map;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [childrenQueries.map((q) => `${q.status}:${q.data?.length ?? 0}`).join("|"), expandedList]);

  const toggleDirectory = useCallback((path: string) => {
    setExpandedPaths((current) => {
      const next = new Set(current);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  const handleSelectFile = useCallback(
    (entry: FsTreeEntry) => {
      if (pendingSelectionTimeoutRef.current !== null) {
        clearTimeout(pendingSelectionTimeoutRef.current);
      }
      setPendingSelection({
        path: entry.path,
        selectedPathAtPress: controlledSelectedPathRef.current,
      });
      pendingSelectionTimeoutRef.current = setTimeout(() => {
        pendingSelectionTimeoutRef.current = null;
        setPendingSelection((current) => (current?.path === entry.path ? null : current));
      }, OPTIMISTIC_SELECTION_TIMEOUT_MS);
      props.onSelectFile(entry.path, entry.size);
    },
    [props.onSelectFile],
  );

  const rootRelative = useCallback(
    (p: string) =>
      p.replace(new RegExp(`^${props.projectRoot.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/?`), ""),
    [props.projectRoot],
  );

  const buildRows = useCallback(
    (entries: ReadonlyArray<FsTreeEntry>, depth: number, out: TreeRow[]): void => {
      for (const entry of entries) {
        out.push({
          key: `entry:${entry.path}`,
          kind: "entry",
          entry,
          depth,
          relativePath: rootRelative(entry.path),
        });
        if (entry.isDir && expandedPaths.has(entry.path)) {
          const child = childrenByPath.get(entry.path);
          if (child?.pending) {
            out.push({ key: `loading:${entry.path}`, kind: "loading", depth: depth + 1 });
          } else {
            buildRows(child?.entries ?? [], depth + 1, out);
            if ((child?.entries?.length ?? 0) === 0) {
              out.push({ key: `empty:${entry.path}`, kind: "empty", depth: depth + 1 });
            }
          }
        }
      }
    },
    [expandedPaths, childrenByPath, rootRelative],
  );

  // Server search results replace the tree entirely while a query is active.
  const rows = useMemo<TreeRow[]>(() => {
    if (props.searchResults != null) {
      return props.searchResults.map((entry) => ({
        key: `entry:${entry.path}`,
        kind: "entry" as const,
        entry,
        depth: 0,
        relativePath: rootRelative(entry.path),
      }));
    }
    const out: TreeRow[] = [];
    buildRows(props.entries, 0, out);
    return out;
  }, [props.searchResults, props.entries, buildRows, rootRelative]);

  // Expand ancestors of the selected file so its row becomes reachable.
  const selectedRelativePath = props.selectedPath ? rootRelative(props.selectedPath) : null;
  useEffect(() => {
    if (!selectedRelativePath || props.searchResults != null) return;
    setExpandedPaths((current) => {
      const needed = ancestorPaths(selectedRelativePath).map((a) => `${props.projectRoot}/${a}`);
      if (needed.every((a) => current.has(a))) return current;
      const next = new Set(current);
      for (const a of needed) next.add(a);
      return next;
    });
  }, [selectedRelativePath, props.projectRoot, props.searchResults]);

  useEffect(
    () => () => {
      if (pendingSelectionTimeoutRef.current !== null) {
        clearTimeout(pendingSelectionTimeoutRef.current);
      }
    },
    [],
  );

  const renderItem = useCallback(
    ({ item }: { item: TreeRow }) => {
      if (item.kind === "loading") return <LoadingRow depth={item.depth} />;
      if (item.kind === "empty") return <EmptyFolderRow depth={item.depth} />;
      const entry = item.entry!;
      return (
        <FileTreeRow
          entry={entry}
          depth={item.depth}
          selected={
            props.selectedPath === entry.path || pendingSelection?.path === entry.path
          }
          expanded={entry.isDir && expandedPaths.has(entry.path)}
          onPressDirectory={toggleDirectory}
          onPressFile={handleSelectFile}
        />
      );
    },
    [expandedPaths, pendingSelection, props.selectedPath, toggleDirectory, handleSelectFile],
  );

  const handleRefresh = useCallback(async () => {
    setIsRefreshing(true);
    try {
      await queryClient.invalidateQueries({ queryKey: ["fs", "children"] });
      await props.onRefresh();
    } finally {
      setIsRefreshing(false);
    }
  }, [queryClient, props.onRefresh]);

  if (props.error && props.entries.length === 0) {
    return (
      <View className="flex-1 bg-screen px-4 py-5">
        <Text className="text-sm font-bold text-foreground">Files unavailable</Text>
        <Text className="mt-1 text-xs leading-normal text-foreground-muted">{props.error}</Text>
      </View>
    );
  }

  return (
    <LegendList
      data={rows}
      keyExtractor={(item) => item.key}
      renderItem={renderItem}
      contentContainerStyle={{ paddingBottom: 24 }}
      refreshControl={
        <RefreshControl
          refreshing={isRefreshing}
          onRefresh={handleRefresh}
          tintColor={theme.colors.text.muted}
        />
      }
      ListEmptyComponent={
        props.isPending ? (
          <View className="flex-1 items-center justify-center py-16">
            <ActivityIndicator color={theme.colors.text.muted} />
          </View>
        ) : props.isSearching ? (
          <View className="flex-1 items-center justify-center py-16">
            <ActivityIndicator size="small" color={theme.colors.text.muted} />
            <Text className="mt-2 text-xs text-foreground-muted">Searching project…</Text>
          </View>
        ) : props.searchResults != null ? (
          <View className="items-center py-16">
            <Text className="text-xs text-foreground-muted">No matches in this project</Text>
          </View>
        ) : (
          <View className="items-center py-16">
            <Text className="text-xs text-foreground-muted">This folder is empty</Text>
          </View>
        )
      }
    />
  );
}
