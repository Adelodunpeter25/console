import React from "react";
import { ChevronDown, ChevronRight, Folder, FolderOpen, Loader2 } from "lucide-react";
import type { FsTreeEntry } from "@console/types";
import { tauriApi } from "../../lib/tauri-api";
import { resolveFileIconToken } from "../../utils/file-icons";
import { getBuiltInSpriteSheet } from "@pierre/trees";
import { useVirtualList } from "../../hooks/useVirtualList";

const FULL_SPRITE_SHEET_SVG = getBuiltInSpriteSheet("complete");

export interface FlatTreeItem {
  id: string;
  name: string;
  path: string;
  isDir: boolean;
  level: number;
  isOpen: boolean;
  loading: boolean;
  hasChildrenLoaded: boolean;
}

interface FileTreeProps {
  tree: FsTreeEntry[];
  onFileSelect?: (path: string) => void;
}

export function FileTree({ tree, onFileSelect }: FileTreeProps) {
  // Flattened items list for virtualization
  const [expandedPaths, setExpandedPaths] = React.useState<Set<string>>(new Set());
  const [childrenMap, setChildrenMap] = React.useState<Map<string, FsTreeEntry[]>>(new Map());
  const [loadingPaths, setLoadingPaths] = React.useState<Set<string>>(new Set());

  // Flatten the recursive tree structure into a virtualizable array
  const flatItems = React.useMemo(() => {
    const list: FlatTreeItem[] = [];

    function walk(entries: FsTreeEntry[], level: number) {
      for (const entry of entries) {
        const isOpen = expandedPaths.has(entry.path);
        const isLoading = loadingPaths.has(entry.path);
        const fetchedChildren = childrenMap.get(entry.path);
        const children = fetchedChildren ?? entry.children ?? [];

        list.push({
          id: entry.path,
          name: entry.name,
          path: entry.path,
          isDir: entry.isDir,
          level,
          isOpen,
          loading: isLoading,
          hasChildrenLoaded: fetchedChildren !== undefined || (entry.children && entry.children.length > 0) || false,
        });

        if (entry.isDir && isOpen && children.length > 0) {
          walk(children, level + 1);
        }
      }
    }

    walk(tree, 0);
    return list;
  }, [tree, expandedPaths, childrenMap, loadingPaths]);

  const { parentRef, virtualizer, virtualItems, totalSize } = useVirtualList({
    items: flatItems,
    estimateSize: 26,
    overscan: 10,
  });

  const handleToggle = async (item: FlatTreeItem) => {
    if (!item.isDir) {
      if (onFileSelect) onFileSelect(item.path);
      return;
    }

    const nextExpanded = new Set(expandedPaths);
    if (nextExpanded.has(item.path)) {
      nextExpanded.delete(item.path);
      setExpandedPaths(nextExpanded);
      return;
    }

    nextExpanded.add(item.path);
    setExpandedPaths(nextExpanded);

    if (!item.hasChildrenLoaded) {
      setLoadingPaths((prev) => new Set(prev).add(item.path));
      try {
        const res = await tauriApi.browseDirectory(item.path);
        setChildrenMap((prev) => new Map(prev).set(item.path, res.entries));
      } catch (err) {
        console.error(`Failed to load directory ${item.path}:`, err);
      } finally {
        setLoadingPaths((prev) => {
          const next = new Set(prev);
          next.delete(item.path);
          return next;
        });
      }
    }
  };

  if (!tree || tree.length === 0) {
    return (
      <div className="p-4 text-xs text-foreground-muted text-center select-none">
        No files in workspace directory.
      </div>
    );
  }

  return (
    <div ref={parentRef} className="h-full w-full overflow-auto select-none py-1">
      <div dangerouslySetInnerHTML={{ __html: FULL_SPRITE_SHEET_SVG }} />
      <div
        style={{
          height: `${totalSize}px`,
          width: "100%",
          position: "relative",
        }}
      >
        {virtualItems.map((virtualRow) => {
          const item = flatItems[virtualRow.index];
          if (!item) return null;

          const token = resolveFileIconToken(item.name);

          return (
            <div
              key={item.id}
              onClick={() => handleToggle(item)}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                height: `${virtualRow.size}px`,
                transform: `translateY(${virtualRow.start}px)`,
                paddingLeft: `${item.level * 12 + 10}px`,
              }}
              className="relative flex items-center gap-1.5 px-2 text-xs text-foreground-secondary hover:text-foreground hover:bg-white/[0.06] rounded cursor-pointer transition-colors group truncate"
            >
              {/* Indentation guide lines */}
              {Array.from({ length: item.level }).map((_, idx) => (
                <div
                  key={idx}
                  style={{ left: `${idx * 12 + 14}px` }}
                  className="absolute top-0 bottom-0 w-[1px] bg-border/40 pointer-events-none group-hover:bg-border/70"
                />
              ))}

              {item.isDir ? (
                <>
                  <span className="shrink-0 text-foreground-muted group-hover:text-foreground">
                    {item.loading ? (
                      <Loader2 size={13} className="animate-spin" />
                    ) : item.isOpen ? (
                      <ChevronDown size={13} />
                    ) : (
                      <ChevronRight size={13} />
                    )}
                  </span>
                  {item.isOpen ? (
                    <FolderOpen size={14} className="shrink-0 text-primary-light" />
                  ) : (
                    <Folder size={14} className="shrink-0 text-primary-light" />
                  )}
                </>
              ) : (
                <>
                  <span className="w-3.5 shrink-0" />
                  <svg className="w-3.5 h-3.5 shrink-0 text-foreground-muted">
                    <use href={`#file-tree-builtin-${token}`} />
                  </svg>
                </>
              )}
              <span className="truncate">{item.name}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
