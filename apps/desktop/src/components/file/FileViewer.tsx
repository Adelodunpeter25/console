import React from "react";
import Editor from "@monaco-editor/react";
import { inferLanguage } from "../../utils/file-language";

interface FileViewerProps {
  content: string;
  fileName?: string;
  language?: string;
}

/**
 * FileViewer — Monaco-powered file viewer component for code tabs.
 * Uses central file icon language resolver for accurate language inferencing.
 */
export function FileViewer({ content, fileName = "file", language }: FileViewerProps) {
  // Infer Monaco Editor language using centralized resolver
  const monacoLanguage = React.useMemo(() => {
    if (language && language !== "plaintext") return language;
    return inferLanguage(fileName);
  }, [language, fileName]);

  return (
    <div className="h-full w-full bg-screen overflow-hidden">
      <Editor
        height="100%"
        width="100%"
        language={monacoLanguage}
        value={content}
        theme="vs-dark"
        options={{
          readOnly: true,
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          fontSize: 12,
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
          lineNumbers: "on",
          renderLineHighlight: "all",
          folding: true,
          automaticLayout: true,
          padding: { top: 12, bottom: 12 },
          domReadOnly: true,
        }}
      />
    </div>
  );
}
