import React from "react";
import { FileTree as PierreFileTree, useFileTree } from "@pierre/trees/react";
import type { FsTreeEntry } from "@console/types";

function extractPaths(entries: FsTreeEntry[]): string[] {
  const paths: string[] = [];
  function walk(items: FsTreeEntry[]) {
    for (const item of items) {
      paths.push(item.path);
      if (item.children && item.children.length > 0) {
        walk(item.children);
      }
    }
  }
  walk(entries);
  return paths;
}

interface FileTreeProps {
  tree: FsTreeEntry[];
  onFileSelect?: (path: string) => void;
}

export function FileTree({ tree, onFileSelect }: FileTreeProps) {
  const paths = React.useMemo(() => extractPaths(tree), [tree]);

  const handleSelectionChange = React.useCallback(
    (selectedPaths: readonly string[]) => {
      if (selectedPaths.length > 0 && onFileSelect) {
        const selected = selectedPaths[0];
        if (selected) {
          onFileSelect(selected);
        }
      }
    },
    [onFileSelect],
  );

  const { model } = useFileTree({
    paths,
    onSelectionChange: handleSelectionChange,
  });

  if (!tree || tree.length === 0) {
    return (
      <div className="p-4 text-xs text-foreground-muted text-center">
        No files in workspace directory.
      </div>
    );
  }

  return (
    <div className="py-1 h-full w-full">
      <PierreFileTree model={model} className="h-full w-full" />
    </div>
  );
}
