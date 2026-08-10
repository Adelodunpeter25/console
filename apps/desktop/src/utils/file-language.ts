import { basename } from "./format";

const EXTENSION_LANGUAGE_MAP: Record<string, string> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  mjs: "javascript",
  cjs: "javascript",
  json: "json",
  md: "markdown",
  py: "python",
  rs: "rust",
  go: "go",
  rb: "ruby",
  java: "java",
  kt: "kotlin",
  swift: "swift",
  cs: "csharp",
  cpp: "cpp",
  cc: "cpp",
  cxx: "cpp",
  c: "c",
  h: "c",
  hpp: "cpp",
  html: "html",
  css: "css",
  scss: "scss",
  sass: "sass",
  less: "less",
  yaml: "yaml",
  yml: "yaml",
  toml: "toml",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  sql: "sql",
  php: "php",
  vue: "vue",
  svelte: "svelte",
  lua: "lua",
  r: "r",
  dockerfile: "dockerfile",
};

/** Best-effort Shiki language name for a file path, by extension. */
export function inferLanguage(filePath: string): string | undefined {
  const ext = filePath.slice(filePath.lastIndexOf(".") + 1).toLowerCase();
  if (EXTENSION_LANGUAGE_MAP[ext]) return EXTENSION_LANGUAGE_MAP[ext];
  const base = basename(filePath).toLowerCase();
  if (base === "dockerfile") return "dockerfile";
  return undefined;
}