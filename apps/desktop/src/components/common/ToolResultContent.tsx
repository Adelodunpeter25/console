import React from "react";
import {
  FolderTree,
  Search,
  Globe,
  FilePlus,
  SquarePen,
  HelpCircle,
  CheckCircle2,
  XCircle,
} from "lucide-react";
import type { ToolResult } from "@console/types";
import { MarkdownRenderer } from "./MarkdownRenderer";
import { formatUnknown } from "../../utils/format";

/* ------------------------------------------------------------------ */
/* Helpers                                                             */
/* ------------------------------------------------------------------ */

/** Extract text from a ToolResult's content array. */
function resultText(result: ToolResult): string {
  let content = result.content;

  // Older persisted results may still contain the tool's transport envelope.
  // Unwrap it defensively so history remains readable after the backend fix.
  while (
    content &&
    typeof content === "object" &&
    !Array.isArray(content) &&
    "content" in content
  ) {
    content = (content as { content: unknown }).content;
  }

  if (typeof content === "string") return content;
  if (content && typeof content === "object" && !Array.isArray(content) && "text" in content) {
    const text = (content as { text: unknown }).text;
    if (typeof text === "string") return text;
  }
  if (Array.isArray(content)) {
    return content
      .map((c) => {
        if (typeof c === "string") return c;
        if (c && typeof c === "object" && c.type === "text" && typeof c.text === "string")
          return c.text;
        return "";
      })
      .join("\n");
  }
  return formatUnknown(content);
}

/** Map file extension to a language string for syntax highlighting. */
const EXT_LANG_MAP: Record<string, string> = {
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  json: "json",
  html: "html",
  css: "css",
  scss: "scss",
  py: "python",
  rb: "ruby",
  go: "go",
  rs: "rust",
  java: "java",
  kt: "kotlin",
  swift: "swift",
  c: "c",
  h: "c",
  cpp: "cpp",
  hpp: "cpp",
  cs: "csharp",
  php: "php",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  yml: "yaml",
  yaml: "yaml",
  toml: "toml",
  xml: "xml",
  md: "markdown",
  markdown: "markdown",
  sql: "sql",
  lua: "lua",
  r: "r",
  dart: "dart",
  vue: "html",
  svelte: "html",
  graphql: "graphql",
  dockerfile: "dockerfile",
};

function langFromPath(filePath: string): string | undefined {
  const basename = filePath.split("/").pop() ?? "";
  if (basename === "Dockerfile") return "dockerfile";
  if (basename === "Makefile") return "makefile";
  const ext = basename.split(".").pop()?.toLowerCase();
  if (!ext) return undefined;
  return EXT_LANG_MAP[ext];
}

/* ------------------------------------------------------------------ */
/* Status badge — reusable one-liner for write/edit/ask results        */
/* ------------------------------------------------------------------ */

function StatusLine({
  icon: Icon,
  text,
  isError,
}: {
  icon: React.ElementType;
  text: string;
  isError?: boolean;
}) {
  return (
    <div className="flex items-center gap-2 py-1">
      <Icon size={13} className={isError ? "text-danger" : "text-success"} />
      <span
        className={`text-xs font-mono selectable-text ${
          isError ? "text-danger" : "text-foreground-secondary"
        }`}
      >
        {text}
      </span>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* readFile — syntax-highlighted code                                  */
/* ------------------------------------------------------------------ */

function ReadFileResult({ text, filePath }: { text: string; filePath?: string }) {
  const parsed = React.useMemo(() => {
    // Parse the normal header, but still highlight raw content when an older
    // backend/persisted result omitted or changed the metadata lines.
    const lines = text.split("\n");
    const headerEnd = lines.findIndex((l, i) => i > 0 && l === "");
    const hasHeader = headerEnd > 0 && headerEnd <= 6 && lines[0]?.startsWith("File:");
    const headerLines = hasHeader ? lines.slice(0, headerEnd) : [];
    const codeLines = hasHeader ? lines.slice(headerEnd + 1) : lines;

    // readFile includes display line numbers in its transport text. Streamdown
    // supplies the actual code line numbers, so remove the transport prefix to
    // avoid rendering two sets of numbers.
    const numberedLines = codeLines.filter((line) => /^\s*\d+:\s?/.test(line)).length;
    const normalizedCodeLines =
      numberedLines > codeLines.length / 2
        ? codeLines.map((line) => line.replace(/^\s*\d+:\s?/, ""))
        : codeLines;

    // Use the metadata only to determine the language; successful results show
    // the file contents without repeating the transport/header details.
    const fileMatch = headerLines.find((l) => l.startsWith("File:"));
    const path = fileMatch?.replace(/^File:\s*/, "") ?? filePath ?? "";
    const lang = langFromPath(path);

    return { lang, normalizedCodeLines, raw: text };
  }, [text, filePath]);

  if (!parsed.lang) return <RawResult text={parsed.raw} />;

  // Build markdown code block for highlighting
  const fence = "```";
  const markdown = `${fence}${parsed.lang}\n${parsed.normalizedCodeLines.join("\n")}\n${fence}`;

  return <MarkdownRenderer content={markdown} />;
}

/* ------------------------------------------------------------------ */
/* bash — exit code badge + stdout/stderr sections                     */
/* ------------------------------------------------------------------ */

function BashResult({ text, isError }: { text: string; isError?: boolean }) {
  const parsed = React.useMemo(() => {
    // Parse: "Exit code: N\nWorking directory: /path\n\nstdout:\n...\n\nstderr:\n..."
    const exitMatch = text.match(/^Exit code:\s*(\d+)/);
    const cwdMatch = text.match(/^Working directory:\s*(.+)/m);
    const exitCode = exitMatch ? parseInt(exitMatch[1]!, 10) : undefined;
    const cwd = cwdMatch?.[1];

    const stdoutIdx = text.indexOf("stdout:");
    const stderrIdx = text.indexOf("stderr:");

    if (stdoutIdx === -1) {
      return { hasStdout: false } as const;
    }

    const stdoutStart = stdoutIdx + "stdout:".length;
    const stdoutEnd = stderrIdx > stdoutStart ? stderrIdx : text.length;
    const stdoutText = text.slice(stdoutStart, stdoutEnd).trim();
    const stderrText = stderrIdx > -1 ? text.slice(stderrIdx + "stderr:".length).trim() : "";

    return { hasStdout: true, exitCode, cwd, stdoutText, stderrText } as const;
  }, [text]);

  if (!parsed.hasStdout) {
    return <RawResult text={text} isError={isError} />;
  }

  const { exitCode, cwd, stdoutText, stderrText } = parsed;

  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-1.5">
        {exitCode !== undefined && (
          <span
            className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono ${
              exitCode === 0 ? "bg-success/15 text-success" : "bg-danger/15 text-danger"
            }`}
          >
            {exitCode === 0 ? <CheckCircle2 size={10} /> : <XCircle size={10} />}
            Exit {exitCode}
          </span>
        )}
        {cwd && <span className="text-[10px] font-mono text-foreground-muted truncate">{cwd}</span>}
      </div>
      {stdoutText && (
        <div>
          <p className="text-[10px] uppercase tracking-wide text-foreground-muted mb-0.5">stdout</p>
          <pre className="text-xs font-mono text-foreground-secondary whitespace-pre-wrap break-all bg-black/30 rounded p-2 max-h-48 overflow-y-auto selectable-text">
            {stdoutText}
          </pre>
        </div>
      )}
      {stderrText && (
        <div>
          <p className="text-[10px] uppercase tracking-wide text-foreground-muted mb-0.5">stderr</p>
          <pre
            className={`text-xs font-mono whitespace-pre-wrap break-all bg-danger/5 rounded p-2 max-h-48 overflow-y-auto selectable-text ${
              isError ? "text-danger" : "text-foreground-secondary"
            }`}
          >
            {stderrText}
          </pre>
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* listDir — tree header + monospace tree                              */
/* ------------------------------------------------------------------ */

function ListDirResult({ text }: { text: string }) {
  const parsed = React.useMemo(() => {
    // Parse: "Directory: <path> <note>\n<tree>"
    const lines = text.split("\n");
    const dirMatch = lines[0]?.match(/^Directory:\s*(.+)/);
    const dirPath = dirMatch?.[1] ?? "";
    const treeLines = lines.slice(1);
    return { dirPath, treeText: treeLines.join("\n") };
  }, [text]);

  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <FolderTree size={11} className="text-foreground-muted" />
        <span className="text-[10px] font-mono text-foreground-muted truncate">{parsed.dirPath}</span>
      </div>
      <pre className="text-xs font-mono text-foreground-secondary whitespace-pre-wrap bg-black/30 rounded p-2 max-h-64 overflow-y-auto selectable-text">
        {parsed.treeText}
      </pre>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* grep — search header + results                                      */
/* ------------------------------------------------------------------ */

function GrepResult({ text }: { text: string }) {
  const parsed = React.useMemo(() => {
    const lines = text.split("\n");
    const headerMatch = lines[0]?.match(/^Found\s+(\d+)\s+match/);
    if (!headerMatch) return null;
    const header = lines[0];
    const bodyLines = lines.slice(1).filter((l) => l !== "");
    return { header, bodyText: bodyLines.join("\n") };
  }, [text]);

  if (!parsed) {
    return <RawResult text={text} />;
  }

  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Search size={11} className="text-foreground-muted" />
        <span className="text-[10px] font-mono text-foreground-muted">{parsed.header}</span>
      </div>
      <pre className="text-xs font-mono text-foreground-secondary whitespace-pre-wrap bg-black/30 rounded p-2 max-h-64 overflow-y-auto selectable-text">
        {parsed.bodyText}
      </pre>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* glob — file list                                                    */
/* ------------------------------------------------------------------ */

function GlobResult({ text }: { text: string }) {
  const parsed = React.useMemo(() => {
    const lines = text.split("\n");
    const headerMatch = lines[0]?.match(/^Found\s+(\d+)\s+file/);
    if (!headerMatch) return null;
    const header = lines[0];
    const files = lines.slice(1).filter((l) => l.trim());
    return { header, filesText: files.join("\n") };
  }, [text]);

  if (!parsed) {
    return <RawResult text={text} />;
  }

  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <FolderTree size={11} className="text-foreground-muted" />
        <span className="text-[10px] font-mono text-foreground-muted">{parsed.header}</span>
      </div>
      <pre className="text-xs font-mono text-foreground-secondary whitespace-pre-wrap bg-black/30 rounded p-2 max-h-48 overflow-y-auto selectable-text">
        {parsed.filesText}
      </pre>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* webSearch / fetch — markdown rendered                               */
/* ------------------------------------------------------------------ */

function WebSearchResult({ text }: { text: string }) {
  const markdown = React.useMemo(
    () =>
      // Convert to markdown: the tool already outputs structured text with
      // numbered results, URLs, and snippets. Wrap URLs in markdown links.
      text
        .split("\n")
        .map((line) => {
          const urlMatch = line.match(/^\s*URL:\s*(https?:\/\/\S+)/);
          if (urlMatch) return `    <${urlMatch[1]}>`;
          return line;
        })
        .join("\n"),
    [text],
  );

  return (
    <div className="max-h-80 overflow-y-auto rounded bg-black/20 p-2">
      <MarkdownRenderer content={markdown} />
    </div>
  );
}

function FetchResult({ text }: { text: string }) {
  const parsed = React.useMemo(() => {
    // Parse: "URL: ...\nStatus: ...\nContent-Type: ...\n\nBody:\n<content>"
    const bodyIdx = text.indexOf("Body:\n");
    const headerText = bodyIdx > -1 ? text.slice(0, bodyIdx) : "";
    const bodyText = bodyIdx > -1 ? text.slice(bodyIdx + "Body:\n".length) : text;

    const urlMatch = headerText.match(/^URL:\s*(.+)/m);
    const statusMatch = headerText.match(/^Status:\s*(\d+)/m);

    // Try to render body as markdown (works for HTML-converted text and JSON)
    const bodyIsJson = bodyText.trim().startsWith("{") || bodyText.trim().startsWith("[");
    const renderedBody = bodyIsJson ? "```json\n" + bodyText + "\n```" : bodyText;

    return { url: urlMatch?.[1], status: statusMatch?.[1], renderedBody };
  }, [text]);

  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <Globe size={11} className="text-foreground-muted" />
        {parsed.url && (
          <span className="text-[10px] font-mono text-foreground-muted truncate">
            {parsed.url}
          </span>
        )}
        {parsed.status && (
          <span
            className={`text-[10px] font-mono px-1 rounded ${
              parseInt(parsed.status, 10) < 400
                ? "text-success bg-success/10"
                : "text-danger bg-danger/10"
            }`}
          >
            {parsed.status}
          </span>
        )}
      </div>
      <div className="max-h-80 overflow-y-auto rounded bg-black/20 p-2">
        <MarkdownRenderer content={parsed.renderedBody} />
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* writeFile / editFile — compact status line                          */
/* ------------------------------------------------------------------ */

function WriteFileResult({ text, isError }: { text: string; isError?: boolean }) {
  // "Written: /path\n  Bytes: 123\n  Lines: 42"
  const firstLine = React.useMemo(() => text.split("\n")[0] ?? text, [text]);
  return <StatusLine icon={isError ? XCircle : FilePlus} text={firstLine} isError={isError} />;
}

function EditFileResult({ text, isError }: { text: string; isError?: boolean }) {
  // "Edited: /path\n  Replaced 3 line(s) with 5 line(s) (+2 lines)"
  const fullText = React.useMemo(() => {
    const firstLine = text.split("\n")[0] ?? text;
    const summary = text.split("\n")[1]?.trim();
    return summary ? `${firstLine} — ${summary}` : firstLine;
  }, [text]);
  return <StatusLine icon={isError ? XCircle : SquarePen} text={fullText} isError={isError} />;
}

/* ------------------------------------------------------------------ */
/* ask — compact answer line                                           */
/* ------------------------------------------------------------------ */

function AskResult({ text }: { text: string }) {
  // "[User Answer]: \"option text\""
  return <StatusLine icon={HelpCircle} text={text} />;
}

/* ------------------------------------------------------------------ */
/* Raw fallback — existing <pre> rendering                             */
/* ------------------------------------------------------------------ */

function RawResult({ text, isError }: { text: string; isError?: boolean }) {
  return (
    <pre
      className={`text-xs font-mono whitespace-pre-wrap break-all bg-black/30 rounded p-2 max-h-64 overflow-y-auto selectable-text ${
        isError ? "text-danger" : "text-foreground-secondary"
      }`}
    >
      {text}
    </pre>
  );
}

/* ------------------------------------------------------------------ */
/* Main component — switches on tool name                              */
/* ------------------------------------------------------------------ */

interface ToolResultContentProps {
  toolName?: string;
  result: ToolResult;
  /** File path extracted from the tool call arguments, used for language detection. */
  callFilePath?: string;
}

export function ToolResultContent({ toolName, result, callFilePath }: ToolResultContentProps) {
  const text = resultText(result);
  const isError = result.isError;

  // Errors always render raw with danger styling
  if (isError) {
    switch (toolName) {
      case "writeFile":
        return <WriteFileResult text={text} isError />;
      case "editFile":
        return <EditFileResult text={text} isError />;
      default:
        return <RawResult text={text} isError />;
    }
  }

  switch (toolName) {
    case "readFile":
      return <ReadFileResult text={text} filePath={callFilePath} />;
    case "bash":
      return <BashResult text={text} />;
    case "listDir":
      return <ListDirResult text={text} />;
    case "grep":
      return <GrepResult text={text} />;
    case "glob":
      return <GlobResult text={text} />;
    case "webSearch":
      return <WebSearchResult text={text} />;
    case "fetch":
      return <FetchResult text={text} />;
    case "writeFile":
    case "batchWrite":
      return <WriteFileResult text={text} />;
    case "editFile":
      return <EditFileResult text={text} />;
    case "ask":
      return <AskResult text={text} />;
    default:
      return <RawResult text={text} />;
  }
}
