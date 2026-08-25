import React, { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Pressable, RefreshControl, View, Text } from "react-native";
import { FlashList } from "@shopify/flash-list";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, Folder, FolderOpen } from "lucide-react-native";
import { FileIcon } from "@/components/icons/file-icon";
import { theme } from "../../styles/theme";
import type { FsTreeEntry } from "@console/types";
import { fsService } from "@console/api";

const OPTIMISTIC_SELECTION_TIMEOUT_MS = 1000;

function ancestorPaths(path: string): ReadonlyArray<string> {
  const parts = path.split("/").filter(Boolean);
  const ancestors: string[] = [];
  for (let index = 1; index < parts.length; index += 1) {
    ancestors.push(parts.slice(0, index).join("/"));
  }
  return ancestors;
}

interface TreeRow {
  readonly key: string;
  readonly kind: "entry" | "loading" | "empty";
  readonly entry?: FsTreeEntry;
  readonly depth: number;
  /** Relative-to-root path for search display. */
  readonly relativePath?: string;
}

const FileTreeRow = memo(function FileTreeRow(props: {
  readonly entry: FsTreeEntry;
  readonly depth: number;
  readonly selected: boolean;
  readonly expanded: boolean;
  readonly onPressDirectory: (path: string) => void;
  readonly onPressFile: (path: string) => void;
}) {
  const isDir = props.entry.isDir;

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={props.entry.path}
      onPress={() => {
        if (isDir) props.onPressDirectory(props.entry.path);
        else props.onPressFile(props.entry.path);
      }}
      className={`mx-2 min-h-[42px] flex-row items-center gap-2 rounded-[12px] px-2 active:opacity-80 ${props.selected ? "bg-card border border-border" : ""}`}
      style={{ paddingLeft: 8 + props.depth * 18 }}
    >
      {isDir ? (
        props.expanded ? (
          <ChevronDown size={12} color={theme.colors.text.muted} />
        ) : (
          <ChevronRight size={12} color={theme.colors.text.muted} />
        )
      ) : (
        <View className="w-3" />
      )}

      {isDir ? (
        props.expanded ? (
          <FolderOpen size={17} color={theme.colors.text.secondary} />
        ) : (
          <Folder size={17} color={theme.colors.text.secondary} />
        )
      ) : (
        <FileIcon filename={props.entry.name} size={17} />
      )}

      <Text
        className={`min-w-0 flex-1 text-sm leading-normal ${props.selected ? "font-semibold text-foreground" : "font-medium text-foreground-secondary"}`}
        numberOfLines={1}
      >
        {props.entry.name}
      </Text>
    </Pressable>
  );
});

export interface FileTreeBrowserProps {
  /** Immediate children of the project root (root-level query lives in the screen). */
  readonly entries: ReadonlyArray<FsTreeEntry>;
  readonly projectRoot: string;
  readonly error: string | null;
  readonly isPending: boolean;
  readonly searchQuery: string;
  readonly selectedPath: string | null; // absolute path
  readonly onRefresh: () => void;
  readonly onSelectFile: (absolutePath: string) => void;
}

/**
 * Lazy file tree: each directory loads its immediate children on expansion.
 * Children are cached by react-query (`["fs", "children", path]`), so
 * re-expanding is instant and nothing beyond the visible levels is ever
 * fetched or parsed on the device.
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

  // One query per expanded directory; react-query caches results by path so
  // re-expanding a directory never refetches while data is fresh.
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
    (absolutePath: string) => {
      if (pendingSelectionTimeoutRef.current !== null) {
        clearTimeout(pendingSelectionTimeoutRef.current);
      }
      setPendingSelection({
        path: absolutePath,
        selectedPathAtPress: controlledSelectedPathRef.current,
      });
      pendingSelectionTimeoutRef.current = setTimeout(() => {
        pendingSelectionTimeoutRef.current = null;
        setPendingSelection((current) => (current?.path === absolutePath ? null : current));
      }, OPTIMISTIC_SELECTION_TIMEOUT_MS);
      props.onSelectFile(absolutePath);
    },
    [props.onSelectFile],
  );

  const rootRelative = useCallback(
    (p: string) => p.replace(new RegExp(`^${props.projectRoot.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}/?`), ""),
    [props.projectRoot],
  );

  const buildRows = useCallback(
    (
      entries: ReadonlyArray<FsTreeEntry>,
      depth: number,
      out: TreeRow[],
      search: string | null,
    ): void => {
      for (const entry of entries) {
        if (!search || entry.name.toLowerCase().includes(search)) {
          out.push({ key: `entry:${entry.path}`, kind: "entry", entry, depth, relativePath: rootRelative(entry.path) });
        }
        if (entry.isDir && expandedPaths.has(entry.path)) {
          const child = childrenByPath.get(entry.path);
          if (child?.pending) {
            out.push({ key: `loading:${entry.path}`, kind: "loading", depth: depth + 1 });
          } else {
            buildRows(child?.entries ?? [], depth + 1, out, search);
            if ((child?.entries?.length ?? 0) === 0 && !search) {
              out.push({ key: `empty:${entry.path}`, kind: "empty", depth: depth + 1 });
            }
          }
        }
      }
    },
    [expandedPaths, childrenByPath, rootRelative],
  );

  const rows = useMemo(() => {
    const search = props.searchQuery.trim().toLowerCase();
    if (!search) {
      const out: TreeRow[] = [];
      buildRows(props.entries, 0, out, null);
      return out;
    }
    // Search mode: flat list over everything loaded so far.
    const out: TreeRow[] = [];
    const collect = (entries: ReadonlyArray<FsTreeEntry>, depth: number): void => {
      for (const entry of entries) {
        if (entry.name.toLowerCase().includes(search)) {
          out.push({ key: `entry:${entry.path}`, kind: "entry", entry, depth, relativePath: rootRelative(entry.path) });
        }
        if (entry.isDir && expandedPaths.has(entry.path)) {
          collect(childrenByPath.get(entry.path)?.entries ?? [], depth + 1);
        }
      }
    };
    collect(props.entries, 0);
    return out;
  }, [props.entries, props.searchQuery, expandedPaths, childrenByPath, buildRows, rootRelative]);

  // Expand ancestors of the selected file so its row becomes reachable.
  const selectedRelativePath = props.selectedPath ? rootRelative(props.selectedPath) : null;
  useEffect(() => {
    if (!selectedRelativePath) return;
    setExpandedPaths((current) => {
      const needed = ancestorPaths(selectedRelativePath).map(
        (a) => `${props.projectRoot}/${a}`,
      );
      if (needed.every((a) => current.has(a))) return current;
      const next = new Set(current);
      for (const a of needed) next.add(`${props.projectRoot}/${a}`);
      return next;
    });
  }, [selectedRelativePath, props.projectRoot]);

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
      if (item.kind === "loading") {
        return (
          <View className="min-h-[36px] flex-row items-center gap-2 pl-4" style={{ paddingLeft: 8 + item.depth * 18 }}>
            <ActivityIndicator size="small" color={theme.colors.text.muted} />
            <Text className="text-xs text-foreground-muted">Loading…</Text>
          </View>
        );
      }
      if (item.kind === "empty") {
        return (
          <View style={{ paddingLeft: 8 + item.depth * 18 }} className="py-1">
            <Text className="text-xs italic text-foreground-tertiary">(empty)</Text>
          </View>
        );
      }
      const entry = item.entry!;
      const isSelected =
        props.selectedPath === entry.path ||
        pendingSelection?.path === entry.path;
      return (
        <FileTreeRow
          entry={entry}
          depth={item.depth}
          selected={isSelected}
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
    <FlashList
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
        ) : props.searchQuery.trim() ? (
          <View className="items-center py-16">
            <Text className="text-xs text-foreground-muted">
              No matches among loaded folders — expand more folders to widen the search.
            </Text>
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
