package com.console.mobile.core.icons

/**
 * Port of apps/mobile/utils/icons/file-type-mapping.ts.
 * Resolves a filename/path (or code-fence language) to a file-type icon key.
 * SVGs live in assets/icons/file-types/<key>.svg (copied from desktop assets).
 */
private val EXACT_NAMES: Map<String, String> = mapOf(
        "bun.lock" to "bun",
        "bun.lockb" to "bun",
        "cargo.lock" to "rust",
        "cargo.toml" to "rust",
        "cmakelists.txt" to "cmake",
        "dockerfile" to "docker",
        "gemfile" to "ruby",
        "go.mod" to "go",
        "go.sum" to "go",
        "makefile" to "makefile",
        "package-lock.json" to "nodejs",
        "package.json" to "nodejs",
        "pnpm-lock.yaml" to "pnpm",
        "yarn.lock" to "yarn",
)

private val EXT_ICONS: Map<String, String> = mapOf(
        "astro" to "astro",
        "bash" to "console",
        "c" to "c",
        "cc" to "cpp",
        "cfg" to "settings",
        "cjs" to "javascript",
        "clj" to "clojure",
        "cljs" to "clojure",
        "cpp" to "cpp",
        "cs" to "csharp",
        "css" to "css",
        "cts" to "typescript",
        "cxx" to "cpp",
        "dart" to "dart",
        "diff" to "diff",
        "dll" to "exe",
        "env" to "settings",
        "erl" to "erlang",
        "ex" to "elixir",
        "exe" to "exe",
        "exs" to "elixir",
        "fish" to "console",
        "gif" to "image",
        "go" to "go",
        "gql" to "graphql",
        "graphql" to "graphql",
        "gz" to "zip",
        "h" to "c",
        "hpp" to "cpp",
        "hrl" to "erlang",
        "hs" to "haskell",
        "htm" to "html",
        "html" to "html",
        "ico" to "image",
        "ini" to "settings",
        "java" to "java",
        "jpeg" to "image",
        "jpg" to "image",
        "js" to "javascript",
        "json" to "json",
        "jsonc" to "json",
        "jsx" to "react",
        "kt" to "kotlin",
        "kts" to "kotlin",
        "less" to "css",
        "lock" to "lock",
        "lua" to "lua",
        "md" to "markdown",
        "mdx" to "markdown",
        "mjs" to "javascript",
        "mkv" to "video",
        "ml" to "ocaml",
        "mli" to "ocaml",
        "mov" to "video",
        "mp3" to "audio",
        "mp4" to "video",
        "mts" to "typescript",
        "nix" to "nix",
        "ogg" to "audio",
        "patch" to "diff",
        "pdf" to "pdf",
        "php" to "php",
        "pl" to "perl",
        "pm" to "perl",
        "png" to "image",
        "prisma" to "prisma",
        "proto" to "proto",
        "ps1" to "powershell",
        "pug" to "pug",
        "py" to "python",
        "pyi" to "python",
        "rb" to "ruby",
        "rs" to "rust",
        "sass" to "sass",
        "scala" to "scala",
        "scss" to "sass",
        "sh" to "console",
        "sol" to "solidity",
        "sql" to "database",
        "svelte" to "svelte",
        "svg" to "svg",
        "swift" to "swift",
        "tar" to "zip",
        "tex" to "tex",
        "toml" to "settings",
        "ts" to "typescript",
        "tsx" to "react",
        "txt" to "file",
        "vue" to "vue",
        "wasm" to "webassembly",
        "wat" to "webassembly",
        "wav" to "audio",
        "webp" to "image",
        "xml" to "xml",
        "yaml" to "yaml",
        "yml" to "yaml",
        "zig" to "zig",
        "zip" to "zip",
        "zsh" to "console",
)

private val LANG_ALIASES: Map<String, String> = mapOf(
        "bash" to "console",
        "c" to "c",
        "c#" to "csharp",
        "c++" to "cpp",
        "clojure" to "clojure",
        "console" to "console",
        "cpp" to "cpp",
        "csharp" to "csharp",
        "css" to "css",
        "dart" to "dart",
        "diff" to "diff",
        "docker" to "docker",
        "dockerfile" to "docker",
        "elixir" to "elixir",
        "erlang" to "erlang",
        "go" to "go",
        "golang" to "go",
        "graphql" to "graphql",
        "haskell" to "haskell",
        "html" to "html",
        "java" to "java",
        "javascript" to "javascript",
        "json" to "json",
        "jsx" to "react",
        "kotlin" to "kotlin",
        "less" to "css",
        "lua" to "lua",
        "makefile" to "makefile",
        "markdown" to "markdown",
        "md" to "markdown",
        "php" to "php",
        "powershell" to "powershell",
        "python" to "python",
        "ruby" to "ruby",
        "rust" to "rust",
        "sass" to "sass",
        "scala" to "scala",
        "scss" to "sass",
        "sh" to "console",
        "shell" to "console",
        "solidity" to "solidity",
        "sql" to "database",
        "svg" to "svg",
        "swift" to "swift",
        "toml" to "settings",
        "ts" to "typescript",
        "tsx" to "react",
        "typescript" to "typescript",
        "xml" to "xml",
        "yaml" to "yaml",
        "yml" to "yaml",
        "zig" to "zig",
        "zsh" to "console",
)

private fun baseNameOf(pathOrName: String): String {
    val i = maxOf(pathOrName.lastIndexOf('/'), pathOrName.lastIndexOf('\\'))
    return (if (i >= 0) pathOrName.substring(i + 1) else pathOrName).lowercase()
}

/** Filename or path -> file-type icon key (fallback "file"). */
fun getFileIconKey(pathOrName: String): String {
    val filename = baseNameOf(pathOrName)
    EXACT_NAMES[filename]?.let { return it }
    if (filename.startsWith("readme")) return "readme"
    if (filename.startsWith(".env")) return "settings"
    if (filename.startsWith(".eslintrc")) return "eslint"
    if (filename.startsWith(".prettierrc")) return "prettier"
    val dot = filename.lastIndexOf('.')
    if (dot > 0) {
        EXT_ICONS[filename.substring(dot + 1)]?.let { return it }
    }
    return "file"
}

/** Code-fence language id -> file-type icon key (fallback "file"). */
fun getLanguageIconKey(language: String): String {
    val lang = language.trim().lowercase()
    if (lang.isEmpty()) return "file"
    LANG_ALIASES[lang]?.let { return it }
    return "file"
}

/** Provider name -> provider icon asset key, or null when unknown. */
fun getProviderIconKey(provider: String?): String? {
    if (provider.isNullOrBlank()) return null
    return when (provider.lowercase()) {
        "antigravity" -> "antigravity"
        "openai", "codex" -> "openai"
        "opencode" -> "opencode"
        else -> null
    }
}
