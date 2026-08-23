import React, { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Pressable, RefreshControl, View, Text } from "react-native";
import { FlashList } from "@shopify/flash-list";
import { ChevronDown, ChevronRight, Folder, FolderOpen } from "lucide-react-native";
import { FileIcon } from "@/components/icons/file-icon";
import { theme } from "../../styles/theme";
import type { FsTreeEntry } from "@console/types";
import {
  buildFileTree,
  defaultExpandedTreePaths,
  flattenFileTree,
  type VisibleFileTreeNode,
} from "../../utils/fileTree";

const FILE_TREE_INITIAL_RENDER_COUNT = 20;
const OPTIMISTIC_SELECTION_TIMEOUT_MS = 1000;

function ancestorPaths(path: string): ReadonlyArray<string> {
  const parts = path.split("/").filter(Boolean);
  const ancestors: string[] = [];
  for (let index = 1; index < parts.length; index += 1) {
    ancestors.push(parts.slice(0, index).join("/"));
  }
  return ancestors;
}

const FileTreeRow = memo(function FileTreeRow(props: {
  readonly item: VisibleFileTreeNode;
  readonly selected: boolean;
  readonly expanded: boolean;
  readonly onPressDirectory: (path: string) => void;
  readonly onPressFile: (path: string) => void;
}) {
  const { node, depth } = props.item;
  const isDir = node.isDir;

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={node.path}
      onPress={() => {
        if (isDir) {
          props.onPressDirectory(node.path);
        } else {
          props.onPressFile(node.absolutePath);
        }
      }}
      className={`mx-2 min-h-[42px] flex-row items-center gap-2 rounded-[12px] px-2 active:opacity-80 ${props.selected ? "bg-card border border-border" : ""}`}
      style={{ paddingLeft: 8 + depth * 18 }}
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
        <FileIcon filename={node.name} size={17} />
      )}

      <Text
        className={`min-w-0 flex-1 text-sm leading-normal ${props.selected ? "font-semibold text-foreground" : "font-medium text-foreground-secondary"}`}
        numberOfLines={1}
      >
        {node.name}
      </Text>

      {isDir ? (
        <Text className="text-2xs font-medium text-foreground-tertiary">
          {node.children.length}
        </Text>
      ) : node.gitStatus ? (
        <Text
          className={`text-[10px] font-mono font-bold px-1 rounded ${node.gitStatus === "M" ? "text-amber-400 bg-amber-400/10" : node.gitStatus === "?" || node.gitStatus === "untracked" ? "text-emerald-400 bg-emerald-400/10" : node.gitStatus === "D" || node.gitStatus === "deleted" ? "text-rose-400 bg-rose-400/10" : "text-foreground-muted"}`}
        >
          {node.gitStatus === "?" ? "U" : node.gitStatus}
        </Text>
      ) : null}
    </Pressable>
  );
});

export interface FileTreeBrowserProps {
  readonly entries: ReadonlyArray<FsTreeEntry>;
  readonly projectRoot: string;
  readonly error: string | null;
  readonly isPending: boolean;
  readonly searchQuery: string;
  readonly selectedPath: string | null; // absolute path
  readonly onRefresh: () => void;
  readonly onSelectFile: (absolutePath: string) => void;
}

export function FileTreeBrowser(props: FileTreeBrowserProps) {
  const [expandedPaths, setExpandedPaths] = useState<ReadonlySet<string>>(() => new Set());
  const [pendingSelection, setPendingSelection] = useState<{
    readonly path: string;
    readonly selectedPathAtPress: string | null;
  } | null>(null);

  const { selectedPath: controlledSelectedPath, onSelectFile } = props;
  const controlledSelectedPathRef = useRef(controlledSelectedPath);
  const pendingSelectionTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  controlledSelectedPathRef.current = controlledSelectedPath;

  // For selectedPath matching we compare relative paths; convert absolute to relative
  const selectedRelativePath = useMemo(() => {
    if (!controlledSelectedPath || !props.projectRoot) return controlledSelectedPath;
    const root = props.projectRoot.replace(/\/$/, "");
    if (controlledSelectedPath === root) return "";
    if (controlledSelectedPath.startsWith(root + "/")) {
      return controlledSelectedPath.slice(root.length + 1);
    }
    return controlledSelectedPath;
  }, [controlledSelectedPath, props.projectRoot]);

  const optimisticSelectedRelativePath =
    pendingSelection?.selectedPathAtPress === controlledSelectedPath
      ? (() => {
          const p = pendingSelection.path;
          const root = props.projectRoot.replace(/\/$/, "");
          if (p.startsWith(root + "/")) return p.slice(root.length + 1);
          return p;
        })()
      : selectedRelativePath;

  const tree = useMemo(
    () => buildFileTree(props.entries, props.projectRoot),
    [props.entries, props.projectRoot],
  );

  const defaultExpanded = useMemo(() => defaultExpandedTreePaths(tree), [tree]);

  const visibleNodes = useMemo(
    () =>
      flattenFileTree({
        nodes: tree,
        expanded: expandedPaths,
        searchQuery: props.searchQuery,
      }),
    [expandedPaths, props.searchQuery, tree],
  );

  useEffect(() => {
    setExpandedPaths((current) => {
      if (current.size > 0 || defaultExpanded.size === 0) return current;
      return new Set(defaultExpanded);
    });
  }, [defaultExpanded]);

  useEffect(() => {
    if (!selectedRelativePath) return;
    setExpandedPaths((current) => {
      const ancestors = ancestorPaths(selectedRelativePath);
      if (ancestors.every((a) => current.has(a))) return current;
      const next = new Set(current);
      for (const a of ancestors) next.add(a);
      return next;
    });
  }, [selectedRelativePath]);

  useEffect(
    () => () => {
      if (pendingSelectionTimeoutRef.current !== null) {
        clearTimeout(pendingSelectionTimeoutRef.current);
      }
    },
    [],
  );

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
      onSelectFile(absolutePath);
    },
    [onSelectFile],
  );

  const renderItem = useCallback(
    ({ item }: { item: VisibleFileTreeNode }) => (
      <FileTreeRow
        item={item}
        selected={item.node.path === optimisticSelectedRelativePath}
        expanded={expandedPaths.has(item.node.path)}
        onPressDirectory={toggleDirectory}
        onPressFile={handleSelectFile}
      />
    ),
    [expandedPaths, handleSelectFile, optimisticSelectedRelativePath, toggleDirectory],
  );

  if (props.error && props.entries.length === 0) {
    return (
      <View className="flex-1 bg-screen px-4 py-5">
        <Text className="text-sm font-bold text-foreground">Files unavailable</Text>
        <Text className="mt-1 text-xs leading-normal text-foreground-muted">{props.error}</Text>
      </View>
    );
  }

  return (
    <View className="flex-1 bg-screen">
      <FlashList
        data={visibleNodes}
        keyExtractor={(item) => item.node.path}
        estimatedItemSize={42}
        contentContainerStyle={{ paddingTop: 8, paddingBottom: 16 }}
        refreshControl={
          <RefreshControl
            refreshing={props.isPending}
            onRefresh={props.onRefresh}
            tintColor={theme.colors.text.muted}
            colors={[theme.colors.text.muted]}
          />
        }
        renderItem={renderItem}
        ListEmptyComponent={
          <View className="px-4 py-5">
            {props.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.text.muted} />
            ) : (
              <>
                <Text className="text-sm font-bold text-foreground">No files found</Text>
                <Text className="mt-1 text-xs leading-normal text-foreground-muted">
                  {props.searchQuery.trim().length > 0
                    ? "Try a different search."
                    : "The workspace file index is empty."}
                </Text>
              </>
            )}
          </View>
        }
      />
    </View>
  );
}
