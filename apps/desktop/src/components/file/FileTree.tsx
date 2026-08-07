import React from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { ChevronDown, ChevronRight, Folder, FolderOpen, Loader2 } from "lucide-react";
import type { FsTreeEntry } from "@console/types";
import { tauriApi } from "../../lib/tauri-api";
import { resolveFileIconToken } from "../../utils/file-icons";

// Minimal SVG sprite sheet embedding for Pierre built-in icons
const SPRITE_SHEET_SVG = `<svg data-icon-sprite aria-hidden="true" width="0" height="0" style="position: absolute; width: 0; height: 0; overflow: hidden;">
  <symbol id="file-tree-builtin-default" viewBox="0 0 16 16"><path fill="currentColor" d="M8 1v3a3 3 0 0 0 3 3h3v5.5a2.5 2.5 0 0 1-2.5 2.5h-7A2.5 2.5 0 0 1 2 12.5v-9A2.5 2.5 0 0 1 4.5 1z" opacity=".4"/><path fill="currentColor" d="M9.5 1a.5.5 0 0 1 .354.146l4 4A.5.5 0 0 1 14 5.5V6h-3a2 2 0 0 1-2-2V1z"/></symbol>
  <symbol id="file-tree-builtin-typescript" viewBox="0 0 16 16"><path fill="currentColor" d="M8 1C2.24 1 1 2.24 1 8s1.24 7 7 7 7-1.24 7-7-1.24-7-7-7" opacity=".2"/><path fill="currentColor" d="M8.1 9.64h.95c.04.62.28.76 1.28.76s1.2-.14 1.2-.85c0-.66-.2-.85-1.2-1.07-1.79-.38-2.18-.7-2.18-1.86C8.15 5.3 8.54 5 10.31 5c1.67 0 2.04.26 2.07 1.42h-.95c-.02-.43-.23-.53-1.1-.53-1 0-1.22.14-1.22.74 0 .52.22.7 1.24.92 1.76.38 2.15.73 2.15 2 0 1.44-.4 1.75-2.24 1.75-1.8 0-2.18-.3-2.15-1.66m-3 1.57V5.99H3.5v-.9h4.21v.9H6.1v5.22z"/></symbol>
  <symbol id="file-tree-builtin-javascript" viewBox="0 0 16 16"><path fill="currentColor" d="M8 1C2.24 1 1 2.24 1 8s1.24 7 7 7 7-1.24 7-7-1.24-7-7-7" opacity=".2"/><path fill="currentColor" d="M8.1 9.64h.95c.04.62.28.76 1.28.76s1.2-.14 1.2-.85c0-.66-.2-.85-1.2-1.07-1.79-.38-2.18-.7-2.18-1.86C8.15 5.3 8.54 5 10.31 5c1.67 0 2.04.26 2.07 1.42h-.95c-.02-.43-.23-.53-1.1-.53-1 0-1.22.14-1.22.74 0 .52.22.7 1.24.92 1.76.38 2.15.73 2.15 2 0 1.44-.4 1.75-2.24 1.75-1.8 0-2.18-.3-2.15-1.66M3.5 9.5h.98c0 .76.15.92.85.92.77 0 .94-.18.94-1.02V5.1h1v4.34c0 1.54-.35 1.87-1.92 1.87-1.55 0-1.89-.32-1.86-1.8"/></symbol>
  <symbol id="file-tree-builtin-react" viewBox="0 0 16 16"><path fill="currentColor" d="M8 6.65c.73 0 1.31.6 1.31 1.35S8.73 9.35 8 9.35 6.69 8.75 6.69 8 7.27 6.65 8 6.65"/><path fill="currentColor" fill-rule="evenodd" d="M8 2.55c1.3-.99 2.59-1.34 3.5-.8.92.55 1.27 1.87 1.08 3.53C14.06 5.94 15 6.9 15 8s-.94 2.06-2.42 2.72c.19 1.65-.16 2.98-1.08 3.52-.91.55-2.2.2-3.5-.8-1.3 1-2.58 1.35-3.5.8-.91-.54-1.27-1.87-1.08-3.52C1.94 10.06 1 9.1 1 8s.94-2.06 2.42-2.72c-.19-1.66.17-2.98 1.08-3.52s2.2-.2 3.5.8M4.26 11.2c-.08 1.34.28 2.03.68 2.26s1.15.22 2.25-.52l.11-.09a12 12 0 0 1-1.24-1.39 11 11 0 0 1-1.8-.41zm7.47-.15q-.83.27-1.79.41-.6.8-1.24 1.4l.11.08c1.1.74 1.86.76 2.25.52.4-.23.76-.92.68-2.26zm-3.04.54a14 14 0 0 1-1.38 0q.34.38.69.7.35-.32.7-.7M8 5.29q-.76 0-1.47.1A13 13 0 0 0 5.07 8a14 14 0 0 0 1.46 2.62 13 13 0 0 0 2.94 0A13 13 0 0 0 10.93 8a14 14 0 0 0-1.46-2.62A13 13 0 0 0 8 5.3M4.64 9.18q-.15.5-.25.96.44.16.94.27a15 15 0 0 1-.7-1.23m6.73 0a15 15 0 0 1-.7 1.23q.5-.11.95-.27a10 10 0 0 0-.25-.96M3.44 6.26C2.27 6.86 1.87 7.53 1.87 8s.4 1.14 1.57 1.74l.13.07q.18-.88.55-1.81a12 12 0 0 1-.55-1.8q-.07.02-.13.06m8.99-.07A12 12 0 0 1 11.88 8q.36.94.55 1.8l.13-.06c1.17-.6 1.56-1.27 1.56-1.74s-.39-1.14-1.56-1.74zm-7.1-.6q-.5.11-.94.27.1.46.25.96a15 15 0 0 1 .69-1.23m5.34 0a15 15 0 0 1 .7 1.23q.14-.5.24-.96-.44-.15-.94-.27M7.18 3.06c-1.09-.74-1.85-.76-2.24-.52s-.76.92-.69 2.26l.01.15a11 11 0 0 1 1.8-.41q.6-.8 1.24-1.4zm3.88-.52c-.4-.24-1.15-.22-2.25.52l-.12.08q.65.6 1.25 1.4.96.15 1.8.41v-.14c.08-1.35-.28-2.04-.68-2.27M8 3.7a10 10 0 0 0-.7.7 14 14 0 0 1 1.4 0 10 10 0 0 0-.7-.7" clip-rule="evenodd"/></symbol>
  <symbol id="file-tree-builtin-json" viewBox="0 0 16 16"><path fill="currentColor" d="M13.25 11.5V9.75a.5.5 0 0 1 .36-.48l.55-.15a1.16 1.16 0 0 0 0-2.24l-.55-.15a.5.5 0 0 1-.36-.48V4.5a2.5 2.5 0 0 0-2.5-2.5h-.25a.5.5 0 0 0 0 1h.25a1.5 1.5 0 0 1 1.5 1.5v1.75a1.5 1.5 0 0 0 1.09 1.44l.54.15a.16.16 0 0 1 0 .32l-.54.15a1.5 1.5 0 0 0-1.09 1.44v1.75a1.5 1.5 0 0 1-1.5 1.5h-.25a.5.5 0 0 0 0 1h.25a2.5 2.5 0 0 0 2.5-2.5m-10.5 0V9.75a.5.5 0 0 0-.36-.48l-.55-.15a1.16 1.16 0 0 1 0-2.24l.55-.15a.5.5 0 0 0 .36-.48V4.5A2.5 2.5 0 0 1 5.25 2h.25a.5.5 0 0 1 0 1h-.25a1.5 1.5 0 0 0-1.5 1.5v1.75a1.5 1.5 0 0 1-1.09 1.44l-.54.15a.16.16 0 0 0 0 .32l.54.15a1.5 1.5 0 0 1 1.09 1.45v1.74a1.5 1.5 0 0 0 1.5 1.5h.25a.5.5 0 0 1 0 1h-.25a2.5 2.5 0 0 1-2.5-2.5"/></symbol>
  <symbol id="file-tree-builtin-markdown" viewBox="0 0 16 16"><path fill="currentColor" d="M1 12V4h2l2 2.5L7 4h2v8H7V7.5l-2 2-2-2V12zm9-3 3 3.5L16 9h-2V4h-2v5z"/></symbol>
</svg>`;

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

  const parentRef = React.useRef<HTMLDivElement>(null);

  // Virtualizer instance for fast high-performance rendering of huge file trees
  const rowVirtualizer = useVirtualizer({
    count: flatItems.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 26,
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
      <div dangerouslySetInnerHTML={{ __html: SPRITE_SHEET_SVG }} />
      <div
        style={{
          height: `${rowVirtualizer.getTotalSize()}px`,
          width: "100%",
          position: "relative",
        }}
      >
        {rowVirtualizer.getVirtualItems().map((virtualRow) => {
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
              className="flex items-center gap-1.5 px-2 text-xs text-foreground-secondary hover:text-foreground hover:bg-white/[0.06] rounded cursor-pointer transition-colors group truncate"
            >
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
