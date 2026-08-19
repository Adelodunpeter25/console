import type { ToolCall, ToolResult } from "@console/types";

/* ------------------------------------------------------------------ */
/* File Extension to Syntax Highlighter Language Mapping              */
/* ------------------------------------------------------------------ */

export const EXT_LANG_MAP: Record<string, string> = {
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

export function langFromPath(filePath: string): string | undefined {
  const basename = filePath.split("/").pop() ?? "";
  if (basename === "Dockerfile") return "dockerfile";
  if (basename === "Makefile") return "makefile";
  const ext = basename.split(".").pop()?.toLowerCase();
  if (!ext) return undefined;
  return EXT_LANG_MAP[ext];
}

/* ------------------------------------------------------------------ */
/* Tool Metadata Mapping                                              */
/* ------------------------------------------------------------------ */

export const TOOL_META: Record<string, { label: string }> = {
  readFile: { label: "Read File" },
  writeFile: { label: "Write File" },
  batchWrite: { label: "Batch Write" },
  editFile: { label: "Edit File" },
  bash: { label: "Run Command" },
  grep: { label: "Search Code" },
  glob: { label: "Find Files" },
  listDir: { label: "List Directory" },
  fetch: { label: "Fetch URL" },
  webSearch: { label: "Web Search" },
  subagent: { label: "Subagent" },
  ask: { label: "Ask Question" },
  todo: { label: "Todo" },
};

export function getToolMeta(name: string): { label: string } {
  return TOOL_META[name] ?? { label: name };
}

/* ------------------------------------------------------------------ */
/* String and Formatting Helpers                                      */
/* ------------------------------------------------------------------ */

export function formatUnknown(val: unknown): string {
  if (val === undefined) return "undefined";
  if (val === null) return "null";
  if (typeof val === "string") return val;
  try {
    return JSON.stringify(val, null, 2);
  } catch {
    return String(val);
  }
}

/** Extract a short summary string from tool arguments. */
export function argSummary(call: ToolCall): string | null {
  const args = call.arguments;
  if (!args || typeof args !== "object") return null;
  const obj = args as Record<string, unknown>;
  if (typeof obj.path === "string") return obj.path;
  if (typeof obj.filePath === "string") return obj.filePath;
  if (typeof obj.command === "string") {
    const cmd = obj.command as string;
    return cmd.length > 45 ? cmd.slice(0, 42) + "…" : cmd;
  }
  if (typeof obj.pattern === "string") return obj.pattern;
  if (typeof obj.query === "string") {
    const q = obj.query as string;
    return q.length > 45 ? q.slice(0, 42) + "…" : q;
  }
  if (typeof obj.url === "string") return obj.url;
  if (typeof obj.directory === "string") return obj.directory;
  if (typeof obj.question === "string") {
    const q = obj.question as string;
    return q.length > 45 ? q.slice(0, 42) + "…" : q;
  }
  if (Array.isArray(obj.paths) && obj.paths.length > 0) {
    return `${(obj.paths as unknown[]).length} files`;
  }
  if (Array.isArray(obj.operations) && obj.operations.length > 0) {
    return `${(obj.operations as unknown[]).length} operations`;
  }
  return null;
}

/** Extract text from a ToolResult's content array or string. */
export function resultText(result: ToolResult): string {
  let content: unknown = result.content;

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
        if (c && typeof c === "object" && "type" in c && c.type === "text" && typeof (c as any).text === "string") {
          return (c as any).text;
        }
        return "";
      })
      .join("\n");
  }
  return formatUnknown(content);
}

export function formatDuration(milliseconds: number): string {
  const seconds = Math.max(0, Math.round(milliseconds / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;
}
