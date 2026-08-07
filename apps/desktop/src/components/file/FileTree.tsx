import React from "react";
import { FileTree as PierreFileTree, useFileTree } from "@pierre/trees/react";
import type { FsTreeEntry } from "@console/types";
import { extractRelativePaths } from "../../utils/tree-paths";

interface FileTreeProps {
  tree: FsTreeEntry[];
  projectRoot?: string;
  onFileSelect?: (path: string) => void;
}

export function FileTree({ tree, projectRoot, onFileSelect }: FileTreeProps) {
  const paths = React.useMemo(() => extractRelativePaths(tree, projectRoot), [tree, projectRoot]);

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
