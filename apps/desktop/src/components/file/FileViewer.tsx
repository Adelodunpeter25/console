import React from "react";
import { CodeView } from "@pierre/diffs/react";

interface FileViewerProps {
  content: string;
  fileName?: string;
  language?: string;
}

export function FileViewer({ content, fileName = "file", language }: FileViewerProps) {
  const items = React.useMemo(
    () => [
      {
        id: fileName,
        type: "file" as const,
        file: {
          name: fileName,
          contents: content,
          lang: language as any,
        },
      },
    ],
    [fileName, content, language],
  );

  return (
    <div className="h-full w-full overflow-auto bg-screen p-4 text-xs font-mono">
      <CodeView items={items} />
    </div>
  );
}
