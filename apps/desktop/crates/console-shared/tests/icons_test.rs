//! File-type icon resolution, shared by both clients.

use console_shared::icons::{file_icon_key, language_icon_key};

#[test]
fn resolves_by_extension() {
    assert_eq!(file_icon_key("src/main.rs"), "rust");
    assert_eq!(file_icon_key("App.tsx"), "react");
    assert_eq!(file_icon_key("a/b/c.py"), "python");
    assert_eq!(file_icon_key("notes.md"), "markdown");
}

#[test]
fn file_name_rules_beat_the_extension() {
    assert_eq!(file_icon_key("package.json"), "nodejs", "not the json icon");
    assert_eq!(file_icon_key("Cargo.toml"), "rust", "not the settings icon");
    assert_eq!(file_icon_key("go.mod"), "go");
    assert_eq!(file_icon_key("Dockerfile"), "docker");
    assert_eq!(file_icon_key("makefile"), "makefile");
    assert_eq!(file_icon_key("README"), "readme");
}

#[test]
fn matching_ignores_case_and_accepts_either_separator() {
    assert_eq!(file_icon_key("SRC\\Main.RS"), "rust");
    assert_eq!(file_icon_key("/x/y/PACKAGE.JSON"), "nodejs");
}

#[test]
fn prefix_rules_cover_config_families() {
    assert_eq!(file_icon_key(".env.local"), "settings");
    assert_eq!(file_icon_key(".eslintrc.cjs"), "eslint");
    assert_eq!(file_icon_key("prettier.config.js"), "prettier");
    assert_eq!(file_icon_key("vite.config.ts"), "vite");
}

#[test]
fn desktop_only_and_android_only_entries_are_both_kept() {
    // Desktop had these, Android did not.
    assert_eq!(file_icon_key("lib.hs"), "haskell");
    assert_eq!(file_icon_key("main.zig"), "zig");
    // Android had `.env` as an extension; desktop only knew the prefix rule.
    assert_eq!(file_icon_key("prod.env"), "settings");
}

#[test]
fn unknown_and_extensionless_files_fall_back_to_file() {
    assert_eq!(file_icon_key("data.unknownext"), "file");
    assert_eq!(file_icon_key("LICENSEE_NOTES"), "file");
    assert_eq!(file_icon_key("trailingdot."), "file");
    assert_eq!(file_icon_key(""), "file");
}

#[test]
fn a_leading_dot_is_not_an_extension() {
    // ".rs" is a dotfile named "rs", not a Rust file.
    assert_eq!(file_icon_key(".rs"), "file");
}

#[test]
fn language_tags_resolve_with_aliases() {
    assert_eq!(language_icon_key("TypeScript"), Some("typescript"));
    assert_eq!(language_icon_key("ts"), Some("typescript"));
    assert_eq!(language_icon_key("golang"), Some("go"));
    assert_eq!(language_icon_key("sh"), Some("console"));
    assert_eq!(language_icon_key("  python3 "), Some("python"));
}

#[test]
fn unknown_or_empty_language_has_no_icon() {
    assert_eq!(language_icon_key(""), None);
    assert_eq!(language_icon_key("   "), None);
    assert_eq!(language_icon_key("klingon"), None);
}

#[test]
fn less_uses_the_css_icon_on_both_clients() {
    // The two clients disagreed (desktop `sass`, Android `css`). Less is not
    // Sass, so the shared answer is `css`.
    assert_eq!(language_icon_key("less"), Some("css"));
}

/// Every key the table can return must have an SVG behind it, or the icon
/// silently turns into the generic file glyph on that client.
#[test]
fn every_key_has_an_asset_on_both_clients() {
    let src = include_str!("../src/icons.rs");
    let mut keys: Vec<&str> = src
        .lines()
        .filter(|l| l.contains("=>"))
        .filter_map(|l| l.rsplit("=>").next())
        .filter_map(|r| r.trim().trim_end_matches(',').strip_prefix('"')?.strip_suffix('"'))
        .collect();
    keys.extend(
        src.lines()
            .filter_map(|l| l.trim().strip_prefix("Some(\""))
            .filter_map(|r| r.split('"').next()),
    );
    keys.sort_unstable();
    keys.dedup();
    assert!(keys.len() > 60, "key extraction broke: {} keys", keys.len());

    let manifest = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
    let desktop = manifest.join("../../assets/icons/file-types");
    let android = manifest.join("../../../android/app/src/main/assets/icons/file-types");
    for key in keys {
        for (client, dir) in [("desktop", &desktop), ("android", &android)] {
            assert!(
                dir.join(format!("{key}.svg")).exists(),
                "icon key `{key}` has no SVG for {client} ({})",
                dir.display()
            );
        }
    }
}
