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
    match extension {
        "rs" => FileTypeIcon::Rust.path(),
        "js" | "mjs" | "cjs" => FileTypeIcon::Javascript.path(),
        "ts" | "mts" | "cts" => FileTypeIcon::Typescript.path(),
        "jsx" | "tsx" => FileTypeIcon::React.path(),
        "py" | "pyi" | "pyw" => FileTypeIcon::Python.path(),
        "go" => FileTypeIcon::Go.path(),
        "c" | "h" | "m" => FileTypeIcon::C.path(),
        "cc" | "cpp" | "cxx" | "hh" | "hpp" | "hxx" | "mm" => FileTypeIcon::Cpp.path(),
        "cs" => FileTypeIcon::Csharp.path(),
        "swift" => FileTypeIcon::Swift.path(),
        "kt" | "kts" => FileTypeIcon::Kotlin.path(),
        "java" | "class" => FileTypeIcon::Java.path(),
        "rb" => FileTypeIcon::Ruby.path(),
        "php" => FileTypeIcon::Php.path(),
        "html" | "htm" => FileTypeIcon::Html.path(),
        "css" | "less" => FileTypeIcon::Css.path(),
        "scss" | "sass" => FileTypeIcon::Sass.path(),
        "json" | "jsonc" | "jsonl" => FileTypeIcon::Json.path(),
        "yaml" | "yml" => FileTypeIcon::Yaml.path(),
        "toml" | "ini" | "cfg" | "conf" | "config" => FileTypeIcon::Settings.path(),
        "xml" | "xsl" | "plist" => FileTypeIcon::Xml.path(),
        "md" | "mdx" | "markdown" => FileTypeIcon::Markdown.path(),
        "sh" | "bash" | "zsh" | "fish" => FileTypeIcon::Console.path(),
        "ps1" | "psm1" => FileTypeIcon::Powershell.path(),
        "sql" | "db" | "sqlite" | "sqlite3" | "csv" | "xls" | "xlsx" => {
            FileTypeIcon::Database.path()
        }
        "png" | "jpg" | "jpeg" | "gif" | "webp" | "avif" | "ico" | "tiff" => {
            FileTypeIcon::Image.path()
        }
        "svg" => FileTypeIcon::Svg.path(),
        "pdf" => FileTypeIcon::Pdf.path(),
        "mp3" | "wav" | "flac" | "ogg" | "m4a" => FileTypeIcon::Audio.path(),
        "mp4" | "mov" | "avi" | "webm" | "mkv" => FileTypeIcon::Video.path(),
        "zip" | "gz" | "tgz" | "bz2" | "xz" | "7z" | "rar" | "tar" | "jar" => {
            FileTypeIcon::Zip.path()
        }
        "wasm" | "wat" => FileTypeIcon::Webassembly.path(),
        "svelte" => FileTypeIcon::Svelte.path(),
        "vue" => FileTypeIcon::Vue.path(),
        "tf" | "tfvars" => FileTypeIcon::Terraform.path(),
        "graphql" | "gql" => FileTypeIcon::Graphql.path(),
        "lua" => FileTypeIcon::Lua.path(),
        "dart" => FileTypeIcon::Dart.path(),
        "astro" => FileTypeIcon::Astro.path(),
        "coffee" | "cson" => FileTypeIcon::Coffee.path(),
        "cr" => FileTypeIcon::Crystal.path(),
        "ex" | "exs" => FileTypeIcon::Elixir.path(),
        "elm" => FileTypeIcon::Elm.path(),
        "erl" | "hrl" => FileTypeIcon::Erlang.path(),
        "clj" | "cljs" | "cljc" | "edn" => FileTypeIcon::Clojure.path(),
        "hs" | "lhs" => FileTypeIcon::Haskell.path(),
        "hx" | "hxml" => FileTypeIcon::Haxe.path(),
        "jinja" | "jinja2" | "j2" => FileTypeIcon::Jinja.path(),
        "jl" => FileTypeIcon::Julia.path(),
        "ml" | "mli" => FileTypeIcon::Ocaml.path(),
        "pl" | "pm" => FileTypeIcon::Perl.path(),
        "prisma" => FileTypeIcon::Prisma.path(),
        "pug" | "jade" => FileTypeIcon::Pug.path(),
        "scala" | "sbt" | "sc" => FileTypeIcon::Scala.path(),
        "sol" => FileTypeIcon::Solidity.path(),
        "tex" | "sty" | "cls" => FileTypeIcon::Tex.path(),
        "xaml" => FileTypeIcon::Xaml.path(),
        "zig" => FileTypeIcon::Zig.path(),
        "nix" => FileTypeIcon::Nix.path(),
        "proto" => FileTypeIcon::Proto.path(),
        "diff" | "patch" => FileTypeIcon::Diff.path(),
        "exe" | "dll" | "so" | "dylib" => FileTypeIcon::Exe.path(),
        "lock" => FileTypeIcon::Lock.path(),
        _ => FileTypeIcon::File.path(),
    }
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

/// Basename of a path, handling both separator styles.
pub fn base_name(path: &str) -> &str {
    path.rsplit(['/', '\\']).next().unwrap_or(path)
}

#[cfg(test)]
mod tests {
    use super::{FileTypeIcon, file_icon_for_path, file_icon_for_name};

    #[test]
    fn file_icons_follow_names_and_extensions() {
        assert_eq!(file_icon_for_name("main.rs"), FileTypeIcon::Rust.path());
        assert_eq!(file_icon_for_name("Panel.tsx"), FileTypeIcon::React.path());
        assert_eq!(file_icon_for_name("README.md"), FileTypeIcon::Readme.path());
        assert_eq!(
            file_icon_for_name("Dockerfile.dev"),
            FileTypeIcon::Docker.path()
        );
        assert_eq!(file_icon_for_name("bun.lock"), FileTypeIcon::Bun.path());
        assert_eq!(
            file_icon_for_name("pnpm-lock.yaml"),
            FileTypeIcon::Pnpm.path()
        );
        assert_eq!(
            file_icon_for_name("vite.config.ts"),
            FileTypeIcon::Vite.path()
        );
        assert_eq!(file_icon_for_name(".env.local"), FileTypeIcon::Settings.path());
        assert_eq!(file_icon_for_name("unknown.data"), FileTypeIcon::File.path());
    }

    #[test]
    fn paths_resolve_through_their_basename() {
        assert_eq!(
            file_icon_for_path("src/lib/main.rs"),
            FileTypeIcon::Rust.path()
        );
        assert_eq!(
            file_icon_for_path("C:\\repo\\src\\app.tsx"),
            FileTypeIcon::React.path()
        );
    }

    #[test]
    fn base_names_strip_both_separator_styles() {
        assert_eq!(super::base_name("src/lib/main.rs"), "main.rs");
        assert_eq!(super::base_name("C:\\repo\\app.tsx"), "app.tsx");
        assert_eq!(super::base_name("README.md"), "README.md");
    }
}
