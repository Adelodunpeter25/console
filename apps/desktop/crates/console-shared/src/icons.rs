//! File-type icon resolution: a path, file name or code-fence language to the
//! name of its icon (`rust`, `typescript`, ...).
//!
//! Both clients draw the same SVG set (`assets/icons/file-types/<key>.svg`
//! on desktop, `assets/icons/file-types/<key>.svg` in the Android assets), so
//! the key is the whole contract: this module decides *which* icon, each
//! platform decides how to paint it. Resolution runs on file-name rules first
//! (lockfiles, configs, dotfiles), then the extension, then falls back to
//! `file`.
//!
//! The tables are the union of what each client had before they were shared;
//! on any disagreement the desktop's answer was kept (§3.0 of
//! docs/plan/console-shared.md).

/// The icon for a file name or full path (either separator style).
pub fn file_icon_key(path_or_name: &str) -> &'static str {
    let name = path_or_name
        .rsplit(['/', '\\'])
        .next()
        .unwrap_or(path_or_name)
        .to_ascii_lowercase();
    if let Some(key) = named_icon(&name) {
        return key;
    }
    let extension = match name.rfind('.') {
        Some(dot) if dot > 0 => &name[dot + 1..],
        _ => "",
    };
    extension_icon(extension).unwrap_or("file")
}

/// The icon for a fenced-code language tag (`ts`, `python`, ...), or `None`
/// for an unknown or empty tag so the caller can show a text-only header.
pub fn language_icon_key(tag: &str) -> Option<&'static str> {
    let tag = tag.trim().to_ascii_lowercase();
    if tag.is_empty() {
        return None;
    }
    Some(match tag.as_str() {
        "astro" => "astro",
        "bash" => "console",
        "c" => "c",
        "c#" => "csharp",
        "c++" => "cpp",
        "cc" => "cpp",
        "cfg" => "settings",
        "cjs" => "javascript",
        "class" => "java",
        "clj" => "clojure",
        "cljc" => "clojure",
        "cljs" => "clojure",
        "clojure" => "clojure",
        "cls" => "tex",
        "cmake" => "cmake",
        "cmakelists" => "cmake",
        "conf" => "settings",
        "config" => "settings",
        "console" => "console",
        "cpp" => "cpp",
        "cr" => "crystal",
        "crystal" => "crystal",
        "cs" => "csharp",
        "csharp" => "csharp",
        "css" => "css",
        "cts" => "typescript",
        "cxx" => "cpp",
        "dart" => "dart",
        "diff" => "diff",
        "docker" => "docker",
        "dockerfile" => "docker",
        "edn" => "clojure",
        "elixir" => "elixir",
        "elm" => "elm",
        "erl" => "erlang",
        "erlang" => "erlang",
        "ex" => "elixir",
        "exs" => "elixir",
        "fish" => "console",
        "gemfile" => "ruby",
        "go" => "go",
        "golang" => "go",
        "gql" => "graphql",
        "graphql" => "graphql",
        "h" => "c",
        "haskell" => "haskell",
        "helm" => "helm",
        "hh" => "cpp",
        "hpp" => "cpp",
        "hrl" => "erlang",
        "hs" => "haskell",
        "htm" => "html",
        "html" => "html",
        "hxx" => "cpp",
        "ini" => "settings",
        "java" => "java",
        "javascript" => "javascript",
        "jl" => "julia",
        "js" => "javascript",
        "json" => "json",
        "json5" => "json",
        "jsonc" => "json",
        "jsx" => "react",
        "julia" => "julia",
        "kotlin" => "kotlin",
        "kt" => "kotlin",
        "kts" => "kotlin",
        "less" => "css",
        "lhs" => "haskell",
        "lua" => "lua",
        "m" => "c",
        "make" => "makefile",
        "makefile" => "makefile",
        "markdown" => "markdown",
        "md" => "markdown",
        "mdx" => "markdown",
        "mjs" => "javascript",
        "ml" => "ocaml",
        "mli" => "ocaml",
        "mm" => "cpp",
        "mts" => "typescript",
        "mysql" => "database",
        "nix" => "nix",
        "node" => "javascript",
        "ocaml" => "ocaml",
        "patch" => "diff",
        "perl" => "perl",
        "php" => "php",
        "pl" => "perl",
        "plist" => "xml",
        "pm" => "perl",
        "postgres" => "database",
        "postgresql" => "database",
        "powershell" => "powershell",
        "prisma" => "prisma",
        "proto" => "proto",
        "ps1" => "powershell",
        "psm1" => "powershell",
        "py" => "python",
        "pyi" => "python",
        "python" => "python",
        "python3" => "python",
        "pyw" => "python",
        "rake" => "ruby",
        "rb" => "ruby",
        "rs" => "rust",
        "ruby" => "ruby",
        "rust" => "rust",
        "sass" => "sass",
        "sbt" => "scala",
        "sc" => "scala",
        "scala" => "scala",
        "scss" => "sass",
        "sh" => "console",
        "shell" => "console",
        "shellscript" => "console",
        "sol" => "solidity",
        "solidity" => "solidity",
        "sql" => "database",
        "sqlite" => "database",
        "sty" => "tex",
        "svelte" => "svelte",
        "svg" => "svg",
        "swift" => "swift",
        "terraform" => "terraform",
        "tex" => "tex",
        "tf" => "terraform",
        "tfvars" => "terraform",
        "toml" => "settings",
        "ts" => "typescript",
        "tsx" => "react",
        "typescript" => "typescript",
        "vue" => "vue",
        "wasm" => "webassembly",
        "wat" => "webassembly",
        "webassembly" => "webassembly",
        "xml" => "xml",
        "xsl" => "xml",
        "yaml" => "yaml",
        "yml" => "yaml",
        "zig" => "zig",
        "zsh" => "console",
        _ => return None,
    })
}

fn named_icon(name: &str) -> Option<&'static str> {
    if name.starts_with("readme") {
        Some("readme")
    } else if name.starts_with("license")
        || name.starts_with("licence")
        || name.starts_with("copying")
    {
        Some("certificate")
    } else if name.starts_with("dockerfile") || name.starts_with("compose.") {
        Some("docker")
    } else if name == "cmakelists.txt" || name.starts_with("cmake.") {
        Some("cmake")
    } else if name == "makefile" || name.starts_with("makefile.") || name == "justfile" {
        Some("makefile")
    } else if matches!(name, "cargo.toml" | "cargo.lock" | "rust-toolchain.toml") {
        Some("rust")
    } else if matches!(name, "go.mod" | "go.sum" | "go.work") {
        Some("go")
    } else if name == "pyproject.toml" || name == "pipfile" || name.starts_with("requirements") {
        Some("python")
    } else if matches!(name, "bun.lock" | "bun.lockb" | "bunfig.toml") {
        Some("bun")
    } else if name.starts_with("pnpm-") || name == ".pnpmfile.cjs" {
        Some("pnpm")
    } else if name == "yarn.lock" || name.starts_with(".yarnrc") {
        Some("yarn")
    } else if name == "package.json" {
        Some("nodejs")
    } else if name == "package-lock.json" {
        Some("npm")
    } else if name.starts_with("tsconfig.") || name == "tsconfig.json" {
        Some("typescript")
    } else if name.starts_with("jsconfig.") || name == "jsconfig.json" {
        Some("javascript")
    } else if name == ".gitignore"
        || name == ".gitattributes"
        || name == ".gitmodules"
        || name == ".gitconfig"
    {
        Some("git")
    } else if name == ".editorconfig" {
        Some("editorconfig")
    } else if name.starts_with(".env") {
        Some("settings")
    } else if name.starts_with(".prettier") || name.starts_with("prettier.config.") {
        Some("prettier")
    } else if name.starts_with(".eslint") || name.starts_with("eslint.config.") {
        Some("eslint")
    } else if name.starts_with("biome.json") {
        Some("biome")
    } else if name.starts_with(".babel") || name.starts_with("babel.config.") {
        Some("babel")
    } else if name.starts_with(".stylelint") || name.starts_with("stylelint.config.") {
        Some("stylelint")
    } else if name.starts_with("vite.config.") {
        Some("vite")
    } else if name.starts_with("vitest.config.") || name.starts_with("vitest.workspace.") {
        Some("vitest")
    } else if name.starts_with("webpack.") {
        Some("webpack")
    } else if name.starts_with("rollup.config.") {
        Some("rollup")
    } else if name.starts_with("next.config.") {
        Some("next")
    } else if name == "next-env.d.ts" {
        Some("next")
    } else if name.starts_with("nuxt.config.") || name == ".nuxtrc" {
        Some("nuxt")
    } else if name.starts_with("astro.config.") {
        Some("astro")
    } else if name == "angular.json" || name.ends_with(".component.ts") {
        Some("angular")
    } else if name == "nest-cli.json" {
        Some("nest")
    } else if name.starts_with("tailwind.config.") {
        Some("tailwindcss")
    } else if name.starts_with("svelte.config.") {
        Some("svelte")
    } else if name.starts_with("vue.config.") {
        Some("vue")
    } else if name == "firebase.json" || name == ".firebaserc" {
        Some("firebase")
    } else if name == "supabase.toml" {
        Some("supabase")
    } else if name.starts_with("prisma.config.") {
        Some("prisma")
    } else if name == "turbo.json" {
        Some("turborepo")
    } else if name.starts_with("deno.json") || name == "deno.lock" {
        Some("deno")
    } else if name == ".gitlab-ci.yml" || name == ".gitlab-ci.yaml" {
        Some("gitlab")
    } else if name == "kustomization.yaml" || name == "kustomization.yml" {
        Some("kubernetes")
    } else if name == "chart.yaml" || name == "values.yaml" {
        Some("helm")
    } else if name == "nginx.conf" {
        Some("nginx")
    } else if name == ".nvmrc" || name == ".node-version" {
        Some("nodejs")
    } else if name == "build.gradle"
        || name == "settings.gradle"
        || name == "gradlew"
        || name == "gradlew.bat"
    {
        Some("gradle")
    } else if name.contains(".stories.") || name.contains(".story.") {
        Some("storybook")
    } else if name == "gemfile" || name == "gemfile.lock" {
        Some("ruby")
    } else if name == "pom.xml" {
        Some("java")
    } else {
        None
    }
}

fn extension_icon(ext: &str) -> Option<&'static str> {
    Some(match ext {
        "rs" => "rust",
        "js" => "javascript",
        "mjs" => "javascript",
        "cjs" => "javascript",
        "ts" => "typescript",
        "mts" => "typescript",
        "cts" => "typescript",
        "jsx" => "react",
        "tsx" => "react",
        "py" => "python",
        "pyi" => "python",
        "pyw" => "python",
        "go" => "go",
        "c" => "c",
        "h" => "c",
        "m" => "c",
        "cc" => "cpp",
        "cpp" => "cpp",
        "cxx" => "cpp",
        "hh" => "cpp",
        "hpp" => "cpp",
        "hxx" => "cpp",
        "mm" => "cpp",
        "cs" => "csharp",
        "swift" => "swift",
        "kt" => "kotlin",
        "kts" => "kotlin",
        "java" => "java",
        "class" => "java",
        "rb" => "ruby",
        "php" => "php",
        "html" => "html",
        "htm" => "html",
        "css" => "css",
        "less" => "css",
        "scss" => "sass",
        "sass" => "sass",
        "json" => "json",
        "jsonc" => "json",
        "jsonl" => "json",
        "yaml" => "yaml",
        "yml" => "yaml",
        "toml" => "settings",
        "ini" => "settings",
        "cfg" => "settings",
        "conf" => "settings",
        "config" => "settings",
        "xml" => "xml",
        "xsl" => "xml",
        "plist" => "xml",
        "md" => "markdown",
        "mdx" => "markdown",
        "markdown" => "markdown",
        "sh" => "console",
        "bash" => "console",
        "zsh" => "console",
        "fish" => "console",
        "ps1" => "powershell",
        "psm1" => "powershell",
        "sql" => "database",
        "db" => "database",
        "sqlite" => "database",
        "sqlite3" => "database",
        "csv" => "database",
        "xls" => "database",
        "xlsx" => "database",
        "png" => "image",
        "jpg" => "image",
        "jpeg" => "image",
        "gif" => "image",
        "webp" => "image",
        "avif" => "image",
        "ico" => "image",
        "tiff" => "image",
        "svg" => "svg",
        "pdf" => "pdf",
        "mp3" => "audio",
        "wav" => "audio",
        "flac" => "audio",
        "ogg" => "audio",
        "m4a" => "audio",
        "mp4" => "video",
        "mov" => "video",
        "avi" => "video",
        "webm" => "video",
        "mkv" => "video",
        "zip" => "zip",
        "gz" => "zip",
        "tgz" => "zip",
        "bz2" => "zip",
        "xz" => "zip",
        "7z" => "zip",
        "rar" => "zip",
        "tar" => "zip",
        "jar" => "zip",
        "wasm" => "webassembly",
        "wat" => "webassembly",
        "svelte" => "svelte",
        "vue" => "vue",
        "tf" => "terraform",
        "tfvars" => "terraform",
        "graphql" => "graphql",
        "gql" => "graphql",
        "lua" => "lua",
        "dart" => "dart",
        "astro" => "astro",
        "coffee" => "coffee",
        "cson" => "coffee",
        "cr" => "crystal",
        "ex" => "elixir",
        "exs" => "elixir",
        "elm" => "elm",
        "erl" => "erlang",
        "hrl" => "erlang",
        "clj" => "clojure",
        "cljs" => "clojure",
        "cljc" => "clojure",
        "edn" => "clojure",
        "hs" => "haskell",
        "lhs" => "haskell",
        "hx" => "haxe",
        "hxml" => "haxe",
        "jinja" => "jinja",
        "jinja2" => "jinja",
        "j2" => "jinja",
        "jl" => "julia",
        "ml" => "ocaml",
        "mli" => "ocaml",
        "pl" => "perl",
        "pm" => "perl",
        "prisma" => "prisma",
        "pug" => "pug",
        "jade" => "pug",
        "scala" => "scala",
        "sbt" => "scala",
        "sc" => "scala",
        "sol" => "solidity",
        "tex" => "tex",
        "sty" => "tex",
        "cls" => "tex",
        "xaml" => "xaml",
        "zig" => "zig",
        "nix" => "nix",
        "proto" => "proto",
        "diff" => "diff",
        "patch" => "diff",
        "exe" => "exe",
        "dll" => "exe",
        "so" => "exe",
        "dylib" => "exe",
        "lock" => "lock",
        "env" => "settings",
        _ => return None,
    })
}
