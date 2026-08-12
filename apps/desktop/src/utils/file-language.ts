import { resolveFileIconToken } from "./file-icons";
import type { BuiltInFileIconToken } from "./file-icons";

/** Map file icon tokens directly to Monaco Editor language identifiers. */
const TOKEN_TO_MONACO_LANG: Record<BuiltInFileIconToken, string> = {
  astro: "html",
  babel: "javascript",
  bash: "shell",
  biome: "json",
  bootstrap: "css",
  browserslist: "plaintext",
  bun: "typescript",
  c: "c",
  claude: "markdown",
  cpp: "cpp",
  css: "css",
  database: "sql",
  default: "plaintext",
  docker: "dockerfile",
  eslint: "json",
  font: "plaintext",
  git: "plaintext",
  go: "go",
  graphql: "graphql",
  html: "html",
  image: "plaintext",
  javascript: "javascript",
  json: "json",
  markdown: "markdown",
  mcp: "json",
  nextjs: "typescript",
  npm: "json",
  oxc: "json",
  postcss: "css",
  prettier: "json",
  python: "python",
  react: "typescript",
  ruby: "ruby",
  rust: "rust",
  sass: "scss",
  stylelint: "json",
  svelte: "html",
  svg: "xml",
  svgo: "javascript",
  swift: "swift",
  table: "plaintext",
  tailwind: "css",
  terraform: "hcl",
  text: "plaintext",
  typescript: "typescript",
  vite: "typescript",
  vscode: "json",
  vue: "html",
  wasm: "plaintext",
  webpack: "javascript",
  yml: "yaml",
  zig: "zig",
  zip: "plaintext",
};

/**
 * Infer Monaco Editor language for a file path, utilizing the centralized file icon resolver.
 */
export function inferLanguage(filePath: string): string {
  if (!filePath) return "plaintext";
  const ext = filePath.slice(filePath.lastIndexOf(".") + 1).toLowerCase();
  if (ext === "xml") return "xml";
  if (ext === "gradle") return "groovy";
  if (ext === "rs") return "rust";
  if (ext === "py") return "python";
  if (ext === "ts" || ext === "tsx") return "typescript";
  if (ext === "js" || ext === "jsx" || ext === "mjs" || ext === "cjs") return "javascript";
  if (ext === "json" || ext === "jsonc" || ext === "json5") return "json";
  if (ext === "sh" || ext === "bash" || ext === "zsh") return "shell";

  const token = resolveFileIconToken(filePath);
  return TOKEN_TO_MONACO_LANG[token] ?? "plaintext";
}