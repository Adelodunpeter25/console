//! Filename and path → file-type icon resolution.
//!
//! Selects from the compact, embedded subset of Material Icon Theme in
//! [`FileTypeIcon`] rather than shipping its entire icon catalog. Resolution
//! runs on exact-name rules first (lockfiles, configs, dotfiles), then on the
//! extension, falling back to a generic file icon.
//!
//! The resolved path feeds [`file_type_icon`], which renders the SVG as an
//! image so its authored colors survive — these are multicolor brand marks,
//! unlike the monochrome `icon()` set that is tinted via text color.

use std::path::Path;

use gpui::Img;

use crate::primitives::icons::FileTypeIcon;
use crate::primitives::file_icon;

/// Resolve a filename to its file-type icon asset path.
pub fn file_icon_for_name(name: &str) -> &'static str {
    let name = name.to_ascii_lowercase();
    let named_icon = if name.starts_with("readme") {
        Some(FileTypeIcon::Readme.path())
    } else if name.starts_with("license")
        || name.starts_with("licence")
        || name.starts_with("copying")
    {
        Some(FileTypeIcon::Certificate.path())
    } else if name.starts_with("dockerfile") || name.starts_with("compose.") {
        Some(FileTypeIcon::Docker.path())
    } else if name == "cmakelists.txt" || name.starts_with("cmake.") {
        Some(FileTypeIcon::Cmake.path())
    } else if name == "makefile" || name.starts_with("makefile.") || name == "justfile" {
        Some(FileTypeIcon::Makefile.path())
    } else if matches!(
        name.as_str(),
        "cargo.toml" | "cargo.lock" | "rust-toolchain.toml"
    ) {
        Some(FileTypeIcon::Rust.path())
    } else if matches!(name.as_str(), "go.mod" | "go.sum" | "go.work") {
        Some(FileTypeIcon::Go.path())
    } else if name == "pyproject.toml" || name == "pipfile" || name.starts_with("requirements") {
        Some(FileTypeIcon::Python.path())
    } else if matches!(name.as_str(), "bun.lock" | "bun.lockb" | "bunfig.toml") {
        Some(FileTypeIcon::Bun.path())
    } else if name.starts_with("pnpm-") || name == ".pnpmfile.cjs" {
        Some(FileTypeIcon::Pnpm.path())
    } else if name == "yarn.lock" || name.starts_with(".yarnrc") {
        Some(FileTypeIcon::Yarn.path())
    } else if name == "package.json" {
        Some(FileTypeIcon::Nodejs.path())
    } else if name == "package-lock.json" {
        Some(FileTypeIcon::Npm.path())
    } else if name.starts_with("tsconfig.") || name == "tsconfig.json" {
        Some(FileTypeIcon::Typescript.path())
    } else if name.starts_with("jsconfig.") || name == "jsconfig.json" {
        Some(FileTypeIcon::Javascript.path())
    } else if name == ".gitignore"
        || name == ".gitattributes"
        || name == ".gitmodules"
        || name == ".gitconfig"
    {
        Some(FileTypeIcon::Git.path())
    } else if name == ".editorconfig" {
        Some(FileTypeIcon::Editorconfig.path())
    } else if name.starts_with(".env") {
        Some(FileTypeIcon::Settings.path())
    } else if name.starts_with(".prettier") || name.starts_with("prettier.config.") {
        Some(FileTypeIcon::Prettier.path())
    } else if name.starts_with(".eslint") || name.starts_with("eslint.config.") {
        Some(FileTypeIcon::Eslint.path())
    } else if name.starts_with("biome.json") {
        Some(FileTypeIcon::Biome.path())
    } else if name.starts_with(".babel") || name.starts_with("babel.config.") {
        Some(FileTypeIcon::Babel.path())
    } else if name.starts_with(".stylelint") || name.starts_with("stylelint.config.") {
        Some(FileTypeIcon::Stylelint.path())
    } else if name.starts_with("vite.config.") {
        Some(FileTypeIcon::Vite.path())
    } else if name.starts_with("vitest.config.") || name.starts_with("vitest.workspace.") {
        Some(FileTypeIcon::Vitest.path())
    } else if name.starts_with("webpack.") {
        Some(FileTypeIcon::Webpack.path())
    } else if name.starts_with("rollup.config.") {
        Some(FileTypeIcon::Rollup.path())
    } else if name.starts_with("next.config.") {
        Some(FileTypeIcon::Next.path())
    } else if name == "next-env.d.ts" {
        Some(FileTypeIcon::Next.path())
    } else if name.starts_with("nuxt.config.") || name == ".nuxtrc" {
        Some(FileTypeIcon::Nuxt.path())
    } else if name.starts_with("astro.config.") {
        Some(FileTypeIcon::Astro.path())
    } else if name == "angular.json" || name.ends_with(".component.ts") {
        Some(FileTypeIcon::Angular.path())
    } else if name == "nest-cli.json" {
        Some(FileTypeIcon::Nest.path())
    } else if name.starts_with("tailwind.config.") {
        Some(FileTypeIcon::Tailwindcss.path())
    } else if name.starts_with("svelte.config.") {
        Some(FileTypeIcon::Svelte.path())
    } else if name.starts_with("vue.config.") {
        Some(FileTypeIcon::Vue.path())
    } else if name == "firebase.json" || name == ".firebaserc" {
        Some(FileTypeIcon::Firebase.path())
    } else if name == "supabase.toml" {
        Some(FileTypeIcon::Supabase.path())
    } else if name.starts_with("prisma.config.") {
        Some(FileTypeIcon::Prisma.path())
    } else if name == "turbo.json" {
        Some(FileTypeIcon::Turborepo.path())
    } else if name.starts_with("deno.json") || name == "deno.lock" {
        Some(FileTypeIcon::Deno.path())
    } else if name == ".gitlab-ci.yml" || name == ".gitlab-ci.yaml" {
        Some(FileTypeIcon::Gitlab.path())
    } else if name == "kustomization.yaml" || name == "kustomization.yml" {
        Some(FileTypeIcon::Kubernetes.path())
    } else if name == "chart.yaml" || name == "values.yaml" {
        Some(FileTypeIcon::Helm.path())
    } else if name == "nginx.conf" {
        Some(FileTypeIcon::Nginx.path())
    } else if name == ".nvmrc" || name == ".node-version" {
        Some(FileTypeIcon::Nodejs.path())
    } else if name == "build.gradle"
        || name == "settings.gradle"
        || name == "gradlew"
        || name == "gradlew.bat"
    {
        Some(FileTypeIcon::Gradle.path())
    } else if name.contains(".stories.") || name.contains(".story.") {
        Some(FileTypeIcon::Storybook.path())
    } else if name == "gemfile" || name == "gemfile.lock" {
        Some(FileTypeIcon::Ruby.path())
    } else if name == "pom.xml" {
        Some(FileTypeIcon::Java.path())
    } else {
        None
    };
    if let Some(icon) = named_icon {
        return icon;
    }

    let extension = Path::new(&name)
        .extension()
        .and_then(|extension| extension.to_str())
        .unwrap_or("");
    if let Some((icon, _)) = extension_meta(extension) {
        icon.path()
    } else {
        FileTypeIcon::File.path()
    }
}

/// Central extension → (icon, highlight language) table.
/// Single source for both icon resolution and `lang_tag_for_path`.
fn extension_meta(ext: &str) -> Option<(FileTypeIcon, Option<&'static str>)> {
    Some(match ext {
        "rs" => (FileTypeIcon::Rust, Some("rust")),
        "js" => (FileTypeIcon::Javascript, Some("javascript")),
        "mjs" => (FileTypeIcon::Javascript, Some("javascript")),
        "cjs" => (FileTypeIcon::Javascript, Some("javascript")),
        "ts" => (FileTypeIcon::Typescript, Some("typescript")),
        "mts" => (FileTypeIcon::Typescript, Some("typescript")),
        "cts" => (FileTypeIcon::Typescript, Some("typescript")),
        "jsx" => (FileTypeIcon::React, Some("javascript")),
        "tsx" => (FileTypeIcon::React, Some("typescript")),
        "py" => (FileTypeIcon::Python, Some("python")),
        "pyi" => (FileTypeIcon::Python, Some("python")),
        "pyw" => (FileTypeIcon::Python, Some("python")),
        "go" => (FileTypeIcon::Go, Some("go")),
        "c" => (FileTypeIcon::C, Some("c")),
        "h" => (FileTypeIcon::C, Some("c")),
        "m" => (FileTypeIcon::C, Some("c")),
        "cc" => (FileTypeIcon::Cpp, Some("cpp")),
        "cpp" => (FileTypeIcon::Cpp, Some("cpp")),
        "cxx" => (FileTypeIcon::Cpp, Some("cpp")),
        "hh" => (FileTypeIcon::Cpp, Some("cpp")),
        "hpp" => (FileTypeIcon::Cpp, Some("cpp")),
        "hxx" => (FileTypeIcon::Cpp, Some("cpp")),
        "mm" => (FileTypeIcon::Cpp, Some("cpp")),
        "cs" => (FileTypeIcon::Csharp, Some("csharp")),
        "swift" => (FileTypeIcon::Swift, Some("swift")),
        "kt" => (FileTypeIcon::Kotlin, Some("kotlin")),
        "kts" => (FileTypeIcon::Kotlin, Some("kotlin")),
        "java" => (FileTypeIcon::Java, Some("java")),
        "class" => (FileTypeIcon::Java, Some("java")),
        "rb" => (FileTypeIcon::Ruby, Some("ruby")),
        "php" => (FileTypeIcon::Php, Some("php")),
        "html" => (FileTypeIcon::Html, Some("html")),
        "htm" => (FileTypeIcon::Html, Some("html")),
        "css" => (FileTypeIcon::Css, Some("css")),
        "less" => (FileTypeIcon::Css, Some("css")),
        "scss" => (FileTypeIcon::Sass, Some("scss")),
        "sass" => (FileTypeIcon::Sass, Some("scss")),
        "json" => (FileTypeIcon::Json, Some("json")),
        "jsonc" => (FileTypeIcon::Json, Some("json")),
        "jsonl" => (FileTypeIcon::Json, Some("json")),
        "yaml" => (FileTypeIcon::Yaml, Some("yaml")),
        "yml" => (FileTypeIcon::Yaml, Some("yaml")),
        "toml" => (FileTypeIcon::Settings, Some("toml")),
        "ini" => (FileTypeIcon::Settings, Some("toml")),
        "cfg" => (FileTypeIcon::Settings, Some("toml")),
        "conf" => (FileTypeIcon::Settings, Some("toml")),
        "config" => (FileTypeIcon::Settings, Some("toml")),
        "xml" => (FileTypeIcon::Xml, Some("xml")),
        "xsl" => (FileTypeIcon::Xml, Some("xml")),
        "plist" => (FileTypeIcon::Xml, Some("xml")),
        "md" => (FileTypeIcon::Markdown, Some("markdown")),
        "mdx" => (FileTypeIcon::Markdown, Some("markdown")),
        "markdown" => (FileTypeIcon::Markdown, Some("markdown")),
        "sh" => (FileTypeIcon::Console, Some("bash")),
        "bash" => (FileTypeIcon::Console, Some("bash")),
        "zsh" => (FileTypeIcon::Console, Some("bash")),
        "fish" => (FileTypeIcon::Console, Some("bash")),
        "ps1" => (FileTypeIcon::Powershell, None),
        "psm1" => (FileTypeIcon::Powershell, None),
        "sql" => (FileTypeIcon::Database, Some("sql")),
        "db" => (FileTypeIcon::Database, None),
        "sqlite" => (FileTypeIcon::Database, None),
        "sqlite3" => (FileTypeIcon::Database, None),
        "csv" => (FileTypeIcon::Database, None),
        "xls" => (FileTypeIcon::Database, None),
        "xlsx" => (FileTypeIcon::Database, None),
        "png" => (FileTypeIcon::Image, None),
        "jpg" => (FileTypeIcon::Image, None),
        "jpeg" => (FileTypeIcon::Image, None),
        "gif" => (FileTypeIcon::Image, None),
        "webp" => (FileTypeIcon::Image, None),
        "avif" => (FileTypeIcon::Image, None),
        "ico" => (FileTypeIcon::Image, None),
        "tiff" => (FileTypeIcon::Image, None),
        "svg" => (FileTypeIcon::Svg, None),
        "pdf" => (FileTypeIcon::Pdf, None),
        "mp3" => (FileTypeIcon::Audio, None),
        "wav" => (FileTypeIcon::Audio, None),
        "flac" => (FileTypeIcon::Audio, None),
        "ogg" => (FileTypeIcon::Audio, None),
        "m4a" => (FileTypeIcon::Audio, None),
        "mp4" => (FileTypeIcon::Video, None),
        "mov" => (FileTypeIcon::Video, None),
        "avi" => (FileTypeIcon::Video, None),
        "webm" => (FileTypeIcon::Video, None),
        "mkv" => (FileTypeIcon::Video, None),
        "zip" => (FileTypeIcon::Zip, None),
        "gz" => (FileTypeIcon::Zip, None),
        "tgz" => (FileTypeIcon::Zip, None),
        "bz2" => (FileTypeIcon::Zip, None),
        "xz" => (FileTypeIcon::Zip, None),
        "7z" => (FileTypeIcon::Zip, None),
        "rar" => (FileTypeIcon::Zip, None),
        "tar" => (FileTypeIcon::Zip, None),
        "jar" => (FileTypeIcon::Zip, None),
        "wasm" => (FileTypeIcon::Webassembly, None),
        "wat" => (FileTypeIcon::Webassembly, None),
        "svelte" => (FileTypeIcon::Svelte, Some("html")),
        "vue" => (FileTypeIcon::Vue, Some("html")),
        "tf" => (FileTypeIcon::Terraform, None),
        "tfvars" => (FileTypeIcon::Terraform, None),
        "graphql" => (FileTypeIcon::Graphql, Some("graphql")),
        "gql" => (FileTypeIcon::Graphql, Some("graphql")),
        "lua" => (FileTypeIcon::Lua, None),
        "dart" => (FileTypeIcon::Dart, None),
        "astro" => (FileTypeIcon::Astro, None),
        "coffee" => (FileTypeIcon::Coffee, None),
        "cson" => (FileTypeIcon::Coffee, None),
        "cr" => (FileTypeIcon::Crystal, None),
        "ex" => (FileTypeIcon::Elixir, None),
        "exs" => (FileTypeIcon::Elixir, None),
        "elm" => (FileTypeIcon::Elm, None),
        "erl" => (FileTypeIcon::Erlang, None),
        "hrl" => (FileTypeIcon::Erlang, None),
        "clj" => (FileTypeIcon::Clojure, None),
        "cljs" => (FileTypeIcon::Clojure, None),
        "cljc" => (FileTypeIcon::Clojure, None),
        "edn" => (FileTypeIcon::Clojure, None),
        "hs" => (FileTypeIcon::Haskell, None),
        "lhs" => (FileTypeIcon::Haskell, None),
        "hx" => (FileTypeIcon::Haxe, None),
        "hxml" => (FileTypeIcon::Haxe, None),
        "jinja" => (FileTypeIcon::Jinja, None),
        "jinja2" => (FileTypeIcon::Jinja, None),
        "j2" => (FileTypeIcon::Jinja, None),
        "jl" => (FileTypeIcon::Julia, None),
        "ml" => (FileTypeIcon::Ocaml, None),
        "mli" => (FileTypeIcon::Ocaml, None),
        "pl" => (FileTypeIcon::Perl, None),
        "pm" => (FileTypeIcon::Perl, None),
        "prisma" => (FileTypeIcon::Prisma, None),
        "pug" => (FileTypeIcon::Pug, None),
        "jade" => (FileTypeIcon::Pug, None),
        "scala" => (FileTypeIcon::Scala, None),
        "sbt" => (FileTypeIcon::Scala, None),
        "sc" => (FileTypeIcon::Scala, None),
        "sol" => (FileTypeIcon::Solidity, None),
        "tex" => (FileTypeIcon::Tex, None),
        "sty" => (FileTypeIcon::Tex, None),
        "cls" => (FileTypeIcon::Tex, None),
        "xaml" => (FileTypeIcon::Xaml, None),
        "zig" => (FileTypeIcon::Zig, None),
        "nix" => (FileTypeIcon::Nix, None),
        "proto" => (FileTypeIcon::Proto, None),
        "diff" => (FileTypeIcon::Diff, Some("diff")),
        "patch" => (FileTypeIcon::Diff, Some("diff")),
        "exe" => (FileTypeIcon::Exe, None),
        "dll" => (FileTypeIcon::Exe, None),
        "so" => (FileTypeIcon::Exe, None),
        "dylib" => (FileTypeIcon::Exe, None),
        "lock" => (FileTypeIcon::Lock, None),
        _ => return None,
    })
}

/// Language tag for a file path, for syntax highlighting.
/// Centralized here so icon and language stay in sync.
pub fn lang_tag_for_path(path: &str) -> Option<&'static str> {
    let basename = path.rsplit(['/', '\\']).next().unwrap_or(path);
    match basename.to_ascii_lowercase().as_str() {
        "dockerfile" => return Some("dockerfile"),
        "makefile" => return Some("makefile"),
        _ => {}
    }
    let ext = Path::new(basename)
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_ascii_lowercase();
    extension_meta(&ext).and_then(|(_, lang)| lang)
}

/// Resolve a full path to its file-type icon asset path, using the basename.
pub fn file_icon_for_path(path: &str) -> &'static str {
    let name = Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(path);
    file_icon_for_name(name)
}

/// Render the multicolor file-type icon for a filename or path.
pub fn file_type_icon(path_or_name: &str, size: f32) -> Img {
    file_icon(file_icon_for_path(path_or_name), size)
}

/// Resolve a fenced-code language tag (e.g. "ts", "python") to its file-type icon.
/// Mirrors mobile's `LANG_ALIASES` so code-block headers show the same icon as
/// file rows. Returns `None` for unknown/empty tags so the caller can fall
/// back to a text-only header.
pub fn file_icon_for_language(tag: &str) -> Option<&'static str> {
    let tag = tag.trim().to_ascii_lowercase();
    if tag.is_empty() {
        return None;
    }
    Some(match tag.as_str() {
        "ts" | "typescript" | "mts" | "cts" => FileTypeIcon::Typescript.path(),
        "tsx" => FileTypeIcon::React.path(),
        "js" | "javascript" | "mjs" | "cjs" | "node" => FileTypeIcon::Javascript.path(),
        "jsx" => FileTypeIcon::React.path(),
        "py" | "python" | "python3" | "pyi" | "pyw" => FileTypeIcon::Python.path(),
        "rs" | "rust" => FileTypeIcon::Rust.path(),
        "go" | "golang" => FileTypeIcon::Go.path(),
        "rb" | "ruby" | "gemfile" | "rake" => FileTypeIcon::Ruby.path(),
        "php" => FileTypeIcon::Php.path(),
        "java" | "class" => FileTypeIcon::Java.path(),
        "kt" | "kotlin" | "kts" => FileTypeIcon::Kotlin.path(),
        "swift" => FileTypeIcon::Swift.path(),
        "c" | "h" | "m" => FileTypeIcon::C.path(),
        "cpp" | "cc" | "cxx" | "hh" | "hpp" | "hxx" | "mm" | "c++" => FileTypeIcon::Cpp.path(),
        "cs" | "csharp" | "c#" => FileTypeIcon::Csharp.path(),
        "hs" | "haskell" | "lhs" => FileTypeIcon::Haskell.path(),
        "ex" | "elixir" | "exs" => FileTypeIcon::Elixir.path(),
        "erl" | "erlang" | "hrl" => FileTypeIcon::Erlang.path(),
        "clj" | "clojure" | "cljs" | "cljc" | "edn" => FileTypeIcon::Clojure.path(),
        "lua" => FileTypeIcon::Lua.path(),
        "zig" => FileTypeIcon::Zig.path(),
        "dart" => FileTypeIcon::Dart.path(),
        "elm" => FileTypeIcon::Elm.path(),
        "cr" | "crystal" => FileTypeIcon::Crystal.path(),
        "jl" | "julia" => FileTypeIcon::Julia.path(),
        "ml" | "ocaml" | "mli" => FileTypeIcon::Ocaml.path(),
        "pl" | "perl" | "pm" => FileTypeIcon::Perl.path(),
        "scala" | "sbt" | "sc" => FileTypeIcon::Scala.path(),
        "sol" | "solidity" => FileTypeIcon::Solidity.path(),
        "tex" | "sty" | "cls" => FileTypeIcon::Tex.path(),
        "prisma" => FileTypeIcon::Prisma.path(),
        "proto" => FileTypeIcon::Proto.path(),
        "graphql" | "gql" => FileTypeIcon::Graphql.path(),
        "svelte" => FileTypeIcon::Svelte.path(),
        "vue" => FileTypeIcon::Vue.path(),
        "astro" => FileTypeIcon::Astro.path(),
        "nix" => FileTypeIcon::Nix.path(),
        "tf" | "terraform" | "tfvars" => FileTypeIcon::Terraform.path(),
        "docker" | "dockerfile" => FileTypeIcon::Docker.path(),
        "make" | "makefile" => FileTypeIcon::Makefile.path(),
        "cmake" | "cmakelists" => FileTypeIcon::Cmake.path(),
        "helm" => FileTypeIcon::Helm.path(),
        "json" | "jsonc" | "json5" => FileTypeIcon::Json.path(),
        "yaml" | "yml" => FileTypeIcon::Yaml.path(),
        "toml" | "ini" | "cfg" | "conf" | "config" => FileTypeIcon::Settings.path(),
        "xml" | "xsl" | "plist" => FileTypeIcon::Xml.path(),
        "md" | "markdown" | "mdx" => FileTypeIcon::Markdown.path(),
        "sh" | "bash" | "zsh" | "fish" | "shell" | "shellscript" | "console" => {
            FileTypeIcon::Console.path()
        }
        "ps1" | "powershell" | "psm1" => FileTypeIcon::Powershell.path(),
        "css" => FileTypeIcon::Css.path(),
        "scss" | "sass" | "less" => FileTypeIcon::Sass.path(),
        "html" | "htm" => FileTypeIcon::Html.path(),
        "svg" => FileTypeIcon::Svg.path(),
        "sql" | "postgres" | "postgresql" | "mysql" | "sqlite" => FileTypeIcon::Database.path(),
        "diff" | "patch" => FileTypeIcon::Diff.path(),
        "wasm" | "wat" | "webassembly" => FileTypeIcon::Webassembly.path(),
        _ => return None,
    })
}

/// Basename of a path, handling both separator styles.
pub fn base_name(path: &str) -> &str {
    path.rsplit(['/', '\\']).next().unwrap_or(path)
}
