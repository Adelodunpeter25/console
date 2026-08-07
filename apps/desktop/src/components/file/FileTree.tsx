import React from "react";
import { ChevronRight, ChevronDown, File, Folder } from "lucide-react";
import type { FsTreeEntry } from "@console/types";

interface FileTreeNodeProps {
  entry: FsTreeEntry;
  onFileSelect?: (path: string, name: string) => void;
  level?: number;
}

export function FileTreeNode({ entry, onFileSelect, level = 0 }: FileTreeNodeProps) {
  const [isOpen, setIsOpen] = React.useState(false);
  const isDir = entry.isDir;

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (isDir) {
      setIsOpen((prev) => !prev);
    } else if (onFileSelect) {
      onFileSelect(entry.path, entry.name);
    }
  };

  return (
    <div>
      <button
        onClick={handleClick}
        style={{ paddingLeft: `${level * 12 + 8}px` }}
        className="w-full flex items-center gap-1.5 py-1 px-2 text-xs text-foreground-muted hover:text-foreground hover:bg-surface-hover rounded transition-colors text-left truncate"
      >
        {isDir ? (
          <>
            {isOpen ? <ChevronDown size={14} className="shrink-0" /> : <ChevronRight size={14} className="shrink-0" />}
            <Folder size={14} className="shrink-0 text-primary-light" />
          </>
        ) : (
          <>
            <span className="w-3.5 shrink-0" />
            <File size={14} className="shrink-0 text-foreground-muted" />
          </>
        )}
        <span className="truncate">{entry.name}</span>
      </button>

      {isDir && isOpen && entry.children && entry.children.length > 0 && (
        <div>
          {entry.children.map((child) => (
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
  onFileSelect?: (path: string, name: string) => void;
}

export function FileTree({ tree, onFileSelect }: FileTreeProps) {
  if (!tree || tree.length === 0) {
    return (
      <div className="p-4 text-xs text-foreground-muted text-center">
        No files in workspace directory.
      </div>
    );
  }

  return (
    <div className="py-1">
      {tree.map((entry) => (
        <FileTreeNode key={entry.path} entry={entry} onFileSelect={onFileSelect} />
      ))}
    </div>
  );
}
