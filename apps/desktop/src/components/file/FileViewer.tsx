import React from "react";
import Editor from "@monaco-editor/react";

interface FileViewerProps {
  content: string;
  fileName?: string;
  language?: string;
}

/**
 * FileViewer — Monaco-powered file viewer component for code tabs.
 * Provides IDE-grade syntax highlighting, line numbers, code folding, and smooth scrolling.
 */
export function FileViewer({ content, fileName = "file", language }: FileViewerProps) {
  // Normalize language for Monaco Editor
  const monacoLanguage = React.useMemo(() => {
    if (!language) return "plaintext";
    if (language === "tsx" || language === "jsx") return "typescript";
    return language;
  }, [language]);

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
