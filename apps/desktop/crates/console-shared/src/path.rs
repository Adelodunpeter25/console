//! Path and folder display utilities.

use std::path::PathBuf;

// NOTE: there is deliberately no `base_name` here — the desktop UI uses
// `console_ui::primitives::base_name`, the single shared implementation.

/// The current user's home directory.
///
/// Windows does not set `HOME` outside MSYS/Git Bash, so fall back to
/// `USERPROFILE` and then the `HOMEDRIVE` + `HOMEPATH` pair.
pub fn home_dir() -> Option<PathBuf> {
    #[cfg(target_os = "windows")]
    {
        // `HOME` can point at an MSYS path (`/c/Users/...`) that native
        // Windows APIs cannot open, so prefer the native variables.
        std::env::var_os("USERPROFILE")
            .map(PathBuf::from)
            .or_else(|| {
                let drive = std::env::var_os("HOMEDRIVE")?;
                let path = std::env::var_os("HOMEPATH")?;
                Some(PathBuf::from(drive).join(path))
            })
            .or_else(|| std::env::var_os("HOME").map(PathBuf::from))
    }

    #[cfg(not(target_os = "windows"))]
    {
        std::env::var_os("HOME").map(PathBuf::from)
    }
}

/// True for dev installs: `CONSOLE_ENV=dev` wins, then the macOS bundle name
/// (a release-mode Dev bundle still reports dev), then debug builds.
pub fn is_dev_install() -> bool {
    if std::env::var("CONSOLE_ENV").as_deref() == Ok("dev") {
        return true;
    }
    if is_dev_bundle() {
        return true;
    }
    cfg!(debug_assertions)
}

#[cfg(target_os = "macos")]
fn is_dev_bundle() -> bool {
    std::env::current_exe()
        .map(|exe| exe.to_string_lossy().contains("Console Dev.app"))
        .unwrap_or(false)
}

#[cfg(not(target_os = "macos"))]
fn is_dev_bundle() -> bool {
    false
}

/// Display folder for this install: `Console Dev` vs `Console`. Single source
/// of truth for the desktop state dir, browser history, and log dir so dev
/// and prod never share (or split) files.
pub fn app_folder_name() -> &'static str {
    if is_dev_install() {
        "Console Dev"
    } else {
        "Console"
    }
}

/// Lowercase dashed form for Unix config/state homes (`console-dev`).
pub fn unix_folder_name() -> String {
    app_folder_name().to_lowercase().replace(' ', "-")
}

/// Desktop app state home: `CONSOLE_STATE_DIR` wins, then the platform store
/// (`~/Library/Application Support/<App>` on macOS).
pub fn storage_directory() -> PathBuf {
    if let Ok(path) = std::env::var("CONSOLE_STATE_DIR") {
        return PathBuf::from(path);
    }

    let folder = app_folder_name();

    #[cfg(target_os = "macos")]
    if let Some(home) = home_dir() {
        return home.join("Library").join("Application Support").join(folder);
    }

    #[cfg(target_os = "windows")]
    if let Ok(app_data) = std::env::var("APPDATA") {
        return PathBuf::from(app_data).join(folder);
    }

    let unix_folder = unix_folder_name();

    if let Ok(config_home) = std::env::var("XDG_CONFIG_HOME") {
        return PathBuf::from(config_home).join(&unix_folder);
    }

    home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".config")
        .join(&unix_folder)
}

/// Directory for `panic.log`: per-OS log home, per-install folder, so dev
/// and prod stop sharing one file and Linux stops getting `~/Library/Logs`.
pub fn log_directory() -> Option<PathBuf> {
    #[cfg(target_os = "macos")]
    return home_dir().map(|home| {
        home.join("Library")
            .join("Logs")
            .join(app_folder_name())
    });

    #[cfg(target_os = "windows")]
    return std::env::var_os("LOCALAPPDATA")
        .map(|dir| PathBuf::from(dir).join(app_folder_name()).join("logs"));

    #[cfg(not(any(target_os = "macos", target_os = "windows")))]
    {
        let base = std::env::var_os("XDG_STATE_HOME")
            .map(PathBuf::from)
            .or_else(|| home_dir().map(|home| home.join(".local").join("state")))?;
        return Some(base.join(unix_folder_name()));
    }
}

/// Parent directory of a path, handling both separator styles.
/// Returns `""` for bare filenames.
pub fn parent_dir(path: &str) -> &str {
    match path.rsplit_once(['/', '\\']) {
        Some((parent, _)) => parent,
        None => "",
    }
}

/// How aggressively `short_parent_dir` shortens a parent directory.
pub struct ParentDirDisplay {
    /// Trailing segments kept when shortening (`…/a/b/c`).
    pub keep_segments: usize,
    /// Parents at or below this length are returned whole.
    pub max_len: usize,
}

impl Default for ParentDirDisplay {
    fn default() -> Self {
        Self {
            keep_segments: 3,
            max_len: 28,
        }
    }
}

/// Shorten a parent dir for narrow rows: keep the trailing segments that
/// actually disambiguate same-name files.
/// `apps/desktop/crates/console-ui/src/markdown` → `…/console-ui/src/markdown`.
pub fn short_parent_dir(path: &str) -> String {
    short_parent_dir_with(path, &ParentDirDisplay::default())
}

/// [`short_parent_dir`] with explicit tuning. `keep_segments` is at least 1.
pub fn short_parent_dir_with(path: &str, display: &ParentDirDisplay) -> String {
    let parent = parent_dir(path);
    if parent.is_empty() {
        return String::new();
    }
    let keep = display.keep_segments.max(1);
    let segments: Vec<&str> = parent
        .split(['/', '\\'])
        .filter(|s| !s.is_empty())
        .collect();
    if segments.len() <= keep || parent.len() <= display.max_len {
        return parent.replace('\\', "/");
    }
    format!("…/{}", segments[segments.len() - keep..].join("/"))
}

/// Lexically join `base` + `relative`, resolving `.` / `..` without touching
/// the filesystem. Shared by file-link resolution and run-activity display.
pub fn join_path_lexical(base: &str, relative: &str) -> String {
    let base = base.trim_end_matches('/');
    let relative = relative.trim_start_matches('/');
    let mut parts: Vec<&str> = base.split('/').collect();
    for seg in relative.split('/') {
        match seg {
            "" | "." => {}
            ".." => {
                parts.pop();
            }
            s => parts.push(s),
        }
    }
    parts.join("/")
}

/// Extract the last path component from a directory path and capitalize its
/// first ASCII character for display (e.g. `"/Users/foo/my-project"` -> `"My-project"`).
/// Falls back to `"Workspace"` if the path is empty or has no valid component.
pub fn format_folder_display_name(path: &str) -> String {
    let mut name = path
        .rsplit(['/', '\\'])
        .find(|s| !s.is_empty())
        .unwrap_or("Workspace")
        .to_string();
    if let Some(first) = name.get_mut(..1) {
        first.make_ascii_uppercase();
    }
    name
}
