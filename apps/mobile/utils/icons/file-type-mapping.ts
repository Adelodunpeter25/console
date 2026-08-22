/**
 * Single source for resolving a file path / filename / code-fence language to
 * its file-type icon key (see ./file-type-registry) and canonical language id
 * (used for syntax highlighting).
 */
import { FILE_ICONS, type FileIconName } from "./file-type-registry";

const EXACT_NAMES: Record<string, FileIconName> = {
  "dockerfile": "docker",
  "package.json": "nodejs",
  "package-lock.json": "nodejs",
  "bun.lock": "bun",
  "bun.lockb": "bun",
  "pnpm-lock.yaml": "pnpm",
  "yarn.lock": "yarn",
  "cargo.toml": "rust",
  "cargo.lock": "rust",
  "go.mod": "go",
  "go.sum": "go",
  "makefile": "makefile",
  "gemfile": "ruby",
  "cmakelists.txt": "cmake",
};

/** Extension -> icon key. */
const EXT_ICONS: Record<string, FileIconName> = {
  ts: "typescript",
  tsx: "react",
  mts: "typescript",
  cts: "typescript",
  js: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  jsx: "react",
  rs: "rust",
  py: "python",
  pyi: "python",
  go: "go",
  java: "java",
  kt: "kotlin",
  kts: "kotlin",
  swift: "swift",
  rb: "ruby",
  php: "php",
  zig: "zig",
  lua: "lua",
  dart: "dart",
  ex: "elixir",
  exs: "elixir",
  erl: "erlang",
  hrl: "erlang",
  hs: "haskell",
  clj: "clojure",
  cljs: "clojure",
  scala: "scala",
  cs: "csharp",
  c: "c",
  h: "c",
  cpp: "cpp",
  cc: "cpp",
  cxx: "cpp",
  hpp: "cpp",
  sol: "solidity",
  json: "json",
  jsonc: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "settings",
  ini: "settings",
  cfg: "settings",
  env: "settings",
  md: "markdown",
  mdx: "markdown",
  txt: "file",
  sh: "console",
  bash: "console",
  zsh: "console",
  fish: "console",
  ps1: "powershell",
  sql: "database",
  css: "css",
  scss: "sass",
  sass: "sass",
  less: "css",
  html: "html",
  htm: "html",
  svg: "svg",
  xml: "xml",
  pdf: "pdf",
  zip: "zip",
  gz: "zip",
  tar: "zip",
  png: "image",
  jpg: "image",
  jpeg: "image",
  gif: "image",
  webp: "image",
  ico: "image",
  mp3: "audio",
  wav: "audio",
  ogg: "audio",
  mp4: "video",
  mov: "video",
  mkv: "video",
  proto: "proto",
  graphql: "graphql",
  gql: "graphql",
  prisma: "prisma",
  wat: "webassembly",
  wasm: "webassembly",
  tex: "tex",
  ml: "ocaml",
  mli: "ocaml",
  pl: "perl",
  pm: "perl",
  pug: "pug",
  astro: "astro",
  vue: "vue",
  svelte: "svelte",
  nix: "nix",
  diff: "diff",
  patch: "diff",
  lock: "lock",
  exe: "exe",
  dll: "exe",
};

/** Exact filename -> syntax-highlighting language id. */
const EXACT_LANGS: Record<string, string> = {
  dockerfile: "docker",
  makefile: "bash",
  gemfile: "ruby",
  "cmakelists.txt": "cmake",
};

/** Extension -> syntax-highlighting language id. */
const EXT_LANGS: Record<string, string> = {
  ts: "typescript",
  mts: "typescript",
  cts: "typescript",
  tsx: "tsx",
  js: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  jsx: "jsx",
  rs: "rust",
  py: "python",
  pyi: "python",
  go: "go",
  java: "java",
  kt: "kotlin",
  kts: "kotlin",
  swift: "swift",
  rb: "ruby",
  php: "php",
  zig: "zig",
  lua: "lua",
  dart: "dart",
  ex: "elixir",
  exs: "elixir",
  erl: "erlang",
  hrl: "erlang",
  hs: "haskell",
  clj: "clojure",
  cljs: "clojure",
  scala: "scala",
  cs: "csharp",
  c: "c",
  h: "c",
  cpp: "cpp",
  cc: "cpp",
  cxx: "cpp",
  hpp: "cpp",
  json: "json",
  jsonc: "json",
  yaml: "yaml",
  yml: "yaml",
  md: "markdown",
  mdx: "markdown",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  ps1: "powershell",
  sql: "sql",
  css: "css",
  scss: "sass",
  sass: "sass",
  less: "css",
  html: "markup",
  htm: "markup",
  svg: "markup",
  xml: "markup",
  graphql: "graphql",
  gql: "graphql",
};

/** Code-fence language id -> icon key. */
const LANG_ALIASES: Record<string, FileIconName> = {
  typescript: "typescript",
  javascript: "javascript",
  ts: "typescript",
  jsx: "react",
  tsx: "react",
  python: "python",
  rust: "rust",
  go: "go",
  golang: "go",
  ruby: "ruby",
  php: "php",
  kotlin: "kotlin",
  swift: "swift",
  java: "java",
  c: "c",
  cpp: "cpp",
  "c++": "cpp",
  csharp: "csharp",
  "c#": "csharp",
  scala: "scala",
  haskell: "haskell",
  elixir: "elixir",
  erlang: "erlang",
  clojure: "clojure",
  lua: "lua",
  zig: "zig",
  dart: "dart",
  solidity: "solidity",
  shell: "console",
  bash: "console",
  sh: "console",
  zsh: "console",
  console: "console",
  powershell: "powershell",
  sql: "database",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "settings",
  markdown: "markdown",
  md: "markdown",
  css: "css",
  scss: "sass",
  sass: "sass",
  less: "css",
  html: "html",
  xml: "xml",
  svg: "svg",
  graphql: "graphql",
  dockerfile: "docker",
  docker: "docker",
  makefile: "makefile",
  diff: "diff",
};

/** Lowercased basename of a path (handles both / and \ separators). */
function baseName(pathOrName: string): string {
  return pathOrName.split(/[/\\]/).pop()?.toLowerCase() ?? "";
}

/**
 * Resolves a filename or path to a file-type icon key.
 */
export function getFileIconKey(pathOrName: string): FileIconName {
  const filename = baseName(pathOrName);

  // 1. Exact filename matches
  if (filename in EXACT_NAMES) return EXACT_NAMES[filename]!;
  if (filename.startsWith("readme")) return "readme";
  if (filename.startsWith(".env")) return "settings";
  if (filename.startsWith(".eslintrc")) return "eslint";
  if (filename.startsWith(".prettierrc")) return "prettier";

  // 2. Extension matches
  const dot = filename.lastIndexOf(".");
  if (dot > 0) {
    const ext = filename.slice(dot + 1);
    const icon = EXT_ICONS[ext];
    if (icon) return icon;
  }

  return "file";
}

/**
 * Resolves a filename or path to a canonical language id for syntax
 * highlighting. Falls back to the raw extension; empty string when unknown.
 */
export function getFileTypeLanguage(pathOrName: string): string {
  const filename = baseName(pathOrName);

  // 1. Exact filename matches
  if (filename in EXACT_LANGS) return EXACT_LANGS[filename]!;

  // Dotfiles & rc-style configs highlight well as shell
  if (filename.startsWith(".env") || filename.startsWith(".git") || filename.endsWith("rc")) {
    return "bash";
  }

  // 2. Extension matches
  const dot = filename.lastIndexOf(".");
  if (dot > 0) {
    const ext = filename.slice(dot + 1);
    return EXT_LANGS[ext] ?? ext;
  }

  return "";
}

/**
 * Resolves a code-fence language id to a file-type icon key.
 */
export function getLanguageIconKey(language: string): FileIconName {
  const lang = language.trim().toLowerCase();
  if (!lang) return "file";
  if (lang in LANG_ALIASES) return LANG_ALIASES[lang]!;
  if (lang in FILE_ICONS) return lang as FileIconName;
  return "file";
}

export function getFileIconXml(pathOrName: string): string {
  return FILE_ICONS[getFileIconKey(pathOrName)];
}
