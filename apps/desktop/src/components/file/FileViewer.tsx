import React from "react";
import { CodeView, File } from "@pierre/diffs/react";

interface FileViewerProps {
  content: string;
  fileName?: string;
  language?: string;
}

export function FileViewer({ content, fileName = "file", language }: FileViewerProps) {
  return (
    <div className="h-full w-full overflow-auto bg-screen p-4 text-xs font-mono">
      <File file={{ name: fileName, contents: content, lang: language }} />
    </div>
  );
}
