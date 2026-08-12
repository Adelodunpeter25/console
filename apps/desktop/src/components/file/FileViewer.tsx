import React from "react";
import Editor, { BeforeMount } from "@monaco-editor/react";
import { inferLanguage } from "../../utils/file-language";

interface FileViewerProps {
  content: string;
  fileName?: string;
  language?: string;
}

/**
 * FileViewer — Monaco-powered file viewer component for code tabs.
 * Clean, read-only document, dark theme matching app background, hidden scrollbars, draggable text selection.
 */
export function FileViewer({ content, fileName = "file", language }: FileViewerProps) {
  // Infer Monaco Editor language using centralized resolver
  const monacoLanguage = React.useMemo(() => {
    if (language && language !== "plaintext") return language;
    return inferLanguage(fileName);
  }, [language, fileName]);

  const handleBeforeMount: BeforeMount = (monaco) => {
    monaco.editor.defineTheme("console-dark", {
      base: "vs-dark",
      inherit: true,
      rules: [],
      colors: {
        "editor.background": "#121212",
        "editorGutter.background": "#121212",
        "editor.lineHighlightBackground": "#1e1e1e30",
        "editorLineNumber.foreground": "#555555",
        "editorLineNumber.activeForeground": "#cccccc",
        "editor.selectionBackground": "#264f78",
        "editor.inactiveSelectionBackground": "#3a3d41",
      },
    });
  };

  return (
    <div className="h-full w-full bg-[#121212] overflow-hidden select-text">
      <Editor
        height="100%"
        width="100%"
        language={monacoLanguage}
        value={content}
        theme="console-dark"
        beforeMount={handleBeforeMount}
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
          scrollbar: {
            vertical: "hidden",
            horizontal: "hidden",
            handleMouseWheel: true,
            verticalScrollbarSize: 0,
            horizontalScrollbarSize: 0,
          },
          overviewRulerBorder: false,
          hideCursorInOverviewRuler: true,
        }}
      />
    </div>
  );
}
