import React from "react";
import { MultiFileDiff, PatchDiff } from "@pierre/diffs/react";

interface DiffViewerProps {
  patch?: string;
  oldContent?: string;
  newContent?: string;
  fileName?: string;
  language?: string;
}

export function DiffViewer({ patch, oldContent, newContent, fileName = "diff", language }: DiffViewerProps) {
  if (patch) {
    return (
      <div className="h-full w-full overflow-auto bg-screen p-2 text-xs">
        <PatchDiff patch={patch} />
      </div>
    );
  }

  if (oldContent !== undefined && newContent !== undefined) {
    return (
      <div className="h-full w-full overflow-auto bg-screen p-2 text-xs">
        <MultiFileDiff
          oldFile={{ name: fileName, contents: oldContent, lang: language }}
          newFile={{ name: fileName, contents: newContent, lang: language }}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full w-full items-center justify-center text-xs text-foreground-muted">
      No diff data available.
    </div>
  );
}
