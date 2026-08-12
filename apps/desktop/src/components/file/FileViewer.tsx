import React from "react";
import Editor, { BeforeMount, OnMount } from "@monaco-editor/react";
import { inferLanguage } from "../../utils/file-language";

interface FileViewerProps {
  content: string;
  fileName?: string;
  language?: string;
}

/**
 * FileViewer — Monaco-powered file viewer component for code tabs.
 * Configured with JetBrains Mono font, clean read-only document, dark theme matching app background,
 * hidden scrollbars, and full draggable text selection.
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
        "editor.lineHighlightBackground": "#1e1e1e40",
        "editorLineNumber.foreground": "#555555",
        "editorLineNumber.activeForeground": "#cccccc",
        "editor.selectionBackground": "#264f7880",
        "editor.inactiveSelectionBackground": "#3a3d4150",
      },
    });
  };

  const handleMount: OnMount = (editor) => {
    // Focus editor so keyboard shortcuts & text drag selection work smoothly
    editor.focus();
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
        onMount={handleMount}
        options={{
          readOnly: true,
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          fontSize: 12,
          fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
          fontLigatures: true,
          lineNumbers: "on",
          selectOnLineNumbers: true,
          selectionClipboard: true,
          contextmenu: true,
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
