import React from "react";
import { ChevronDown, ChevronRight, FileCode, Folder, FolderOpen, Loader2 } from "lucide-react";
import type { FsTreeEntry } from "@console/types";
import { tauriApi } from "../../lib/tauri-api";

interface FileTreeNodeProps {
  entry: FsTreeEntry;
  onFileSelect?: (path: string) => void;
  level?: number;
}

function FileTreeNode({ entry, onFileSelect, level = 0 }: FileTreeNodeProps) {
  const [isOpen, setIsOpen] = React.useState(false);
  const [children, setChildren] = React.useState<FsTreeEntry[] | null>(entry.children ?? null);
  const [loading, setLoading] = React.useState(false);

  const isDir = entry.isDir;

  const handleToggle = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!isDir) {
      if (onFileSelect) onFileSelect(entry.path);
      return;
    }

    if (!isOpen && children === null) {
      setLoading(true);
      try {
        const res = await tauriApi.browseDirectory(entry.path);
        setChildren(res.entries);
      } catch (err) {
        console.error(`Failed to load directory contents for ${entry.path}:`, err);
      } finally {
        setLoading(false);
      }
    }

    setIsOpen((prev) => !prev);
  };

  return (
    <div>
      <button
        onClick={handleToggle}
        style={{ paddingLeft: `${level * 12 + 10}px` }}
        className="w-full flex items-center gap-1.5 py-1 px-2 text-xs text-foreground-secondary hover:text-foreground hover:bg-surface-hover rounded transition-colors text-left truncate group"
      >
        {isDir ? (
          <>
            <span className="shrink-0 text-foreground-muted group-hover:text-foreground">
              {loading ? (
                <Loader2 size={13} className="animate-spin" />
              ) : isOpen ? (
                <ChevronDown size={13} />
              ) : (
                <ChevronRight size={13} />
              )}
            </span>
            {isOpen ? (
              <FolderOpen size={14} className="shrink-0 text-primary-light" />
            ) : (
              <Folder size={14} className="shrink-0 text-primary-light" />
            )}
          </>
        ) : (
          <>
            <span className="w-3.5 shrink-0" />
            <FileCode size={14} className="shrink-0 text-foreground-muted" />
          </>
        )}
        <span className="truncate">{entry.name}</span>
      </button>

      {isDir && isOpen && children && children.length > 0 && (
        <div className="flex flex-col">
          {children.map((child) => (
            <FileTreeNode
              key={child.path}
              entry={child}
              onFileSelect={onFileSelect}
              level={level + 1}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface FileTreeProps {
  tree: FsTreeEntry[];
  onFileSelect?: (path: string) => void;
}

export function FileTree({ tree, onFileSelect }: FileTreeProps) {
  if (!tree || tree.length === 0) {
    return (
      <div className="p-4 text-xs text-foreground-muted text-center select-none">
        No files in workspace directory.
      </div>
    );
  }

  return (
    <div className="py-1 flex flex-col w-full select-none">
      {tree.map((entry) => (
        <FileTreeNode key={entry.path} entry={entry} onFileSelect={onFileSelect} />
      ))}
    </div>
  );
}
