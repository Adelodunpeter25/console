//! Driving the embedded CEF browser with [agent-browser](https://github.com/vercel-labs/agent-browser).
//!
//! agent-browser is a CLI/daemon that speaks CDP. We attach it to CEF's local
//! DevTools port (see [`super::runtime::debug_port`]) and run every request as
//! one `batch` on stdin: `[["tab", "<targetId>"], ["<verb>", ...args]]`. That
//! keeps the tab switch and the action atomic, needs no shell quoting (args
//! are JSON strings), and returns one structured result per step.
//!
//! Rules this module enforces (see `docs/plan/agent-browser-cef.md`):
//!
//! * `--cdp <port>` on every call. Without it a session with no live
//!   connection silently launches its own Chrome.
//! * Tabs are addressed by CDP target id, never by `tN` (reassigned on every
//!   connection).
//! * Only page-content verbs are allowed. Tab lifecycle (`tab`, `close`, ...)
//!   stays with Console, and `install` would download a browser we never use.
//!
//! Everything here is blocking (process spawn, shell lookups). Call it from a
//! background thread, never the UI thread.

use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use serde_json::Value;

/// Pinned agent-browser version (installed when missing, like a locked dep).
pub const VERSION: &str = "0.38.2";
/// npm/bun package spec used for auto-install.
pub const PACKAGE_SPEC: &str = "agent-browser@0.38.2";
/// Overrides binary discovery (tests, bundled builds, custom installs).
pub const BIN_ENV: &str = "CONSOLE_AGENT_BROWSER_BIN";

/// Page-content verbs Console may send. Everything else is refused.
const ALLOWED_VERBS: &[&str] = &[
    "snapshot",
    "click",
    "dblclick",
    "fill",
    "type",
    "press",
    "hover",
    "focus",
    "scroll",
    "select",
    "check",
    "uncheck",
    "eval",
    "get",
    "wait",
    "screenshot",
    "find",
    "is",
];

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

/// Why a request was refused before anything was spawned.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BuildError {
    /// Not a 32 character hex CDP target id.
    InvalidTargetId(String),
    /// Session names are `[A-Za-z0-9_-]{1,64}`.
    InvalidSession(String),
    NoCommands,
    EmptyCommand,
    /// The verb is not on the allow-list (tab lifecycle, `close`, `install`, ...).
    DisallowedVerb(String),
}

impl std::fmt::Display for BuildError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::InvalidTargetId(id) => write!(f, "invalid CDP target id {id:?}"),
            Self::InvalidSession(name) => write!(f, "invalid agent-browser session name {name:?}"),
            Self::NoCommands => write!(f, "no agent-browser command given"),
            Self::EmptyCommand => write!(f, "empty agent-browser command"),
            Self::DisallowedVerb(verb) => {
                write!(f, "agent-browser command {verb:?} is not allowed from Console")
            }
        }
    }
}

impl std::error::Error for BuildError {}

/// A fully built agent-browser call: argv (without the program) plus the JSON
/// batch to write on stdin.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Invocation {
    pub args: Vec<String>,
    pub stdin: String,
    /// Whether the batch starts with a `tab <targetId>` step. That step is
    /// consumed when parsing the output. It must be left out when the session
    /// is already on the target: every tab switch clears all element refs, so
    /// switching before each action would invalidate the refs from the
    /// previous snapshot (every `click @eN` after a snapshot would fail).
    pub has_tab_step: bool,
}

/// A CDP target id is 32 hex characters.
pub fn is_valid_target_id(id: &str) -> bool {
    id.len() == 32 && id.bytes().all(|b| b.is_ascii_hexdigit())
}

fn is_valid_session(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 64
        && name
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
}

/// Build the invocation for `commands` (each `[verb, args...]`) against one
/// CDP target of the browser listening on `port`, switching to it first.
pub fn build_invocation(
    port: u16,
    session: &str,
    target_id: &str,
    commands: &[Vec<String>],
) -> Result<Invocation, BuildError> {
    build_invocation_for(port, session, target_id, commands, true)
}

/// Like [`build_invocation`], but only switches to `target_id` when
/// `switch_tab` is set. Pass `false` when the session is already on the target
/// so element refs from the last snapshot stay valid.
pub fn build_invocation_for(
    port: u16,
    session: &str,
    target_id: &str,
    commands: &[Vec<String>],
    switch_tab: bool,
) -> Result<Invocation, BuildError> {
    if !is_valid_target_id(target_id) {
        return Err(BuildError::InvalidTargetId(target_id.to_string()));
    }
    if !is_valid_session(session) {
        return Err(BuildError::InvalidSession(session.to_string()));
    }
    if commands.is_empty() {
        return Err(BuildError::NoCommands);
    }
    let mut steps: Vec<Vec<String>> = Vec::with_capacity(commands.len() + 1);
    if switch_tab {
        steps.push(vec!["tab".to_string(), target_id.to_string()]);
    }
    for command in commands {
        let verb = command.first().ok_or(BuildError::EmptyCommand)?;
        if !ALLOWED_VERBS.contains(&verb.as_str()) {
            return Err(BuildError::DisallowedVerb(verb.clone()));
        }
        steps.push(command.clone());
    }
    let stdin = serde_json::to_string(&steps).expect("string arrays always serialize");
    let args = batch_args(port, session);
    Ok(Invocation {
        args,
        stdin,
        has_tab_step: switch_tab,
    })
}

fn batch_args(port: u16, session: &str) -> Vec<String> {
    vec![
        "--cdp".to_string(),
        port.to_string(),
        "--session".to_string(),
        session.to_string(),
        "--json".to_string(),
        "batch".to_string(),
        "--bail".to_string(),
    ]
}

/// A read-only call that lists the session's tabs, used to find which target
/// the daemon is currently on. Listing does not switch tabs or clear refs.
pub fn active_target_invocation(port: u16, session: &str) -> Result<Invocation, BuildError> {
    if !is_valid_session(session) {
        return Err(BuildError::InvalidSession(session.to_string()));
    }
    Ok(Invocation {
        args: batch_args(port, session),
        stdin: serde_json::to_string(&[["tab", "list"]]).expect("string arrays always serialize"),
        has_tab_step: false,
    })
}

/// The CDP target id of the active tab in the result of
/// [`active_target_invocation`].
pub fn parse_active_target(steps: &[StepResult]) -> Option<String> {
    steps
        .first()?
        .result
        .get("tabs")?
        .as_array()?
        .iter()
        .find(|tab| tab.get("active").and_then(Value::as_bool) == Some(true))?
        .get("targetId")?
        .as_str()
        .map(str::to_string)
}

/// Ask the daemon for the target it is currently on.
pub fn active_target(
    binary: &Path,
    port: u16,
    session: &str,
    timeout: Duration,
) -> Result<Option<String>, RunError> {
    let invocation = active_target_invocation(port, session)
        .map_err(|err| RunError::Failed(err.to_string()))?;
    Ok(parse_active_target(&run(binary, &invocation, timeout)?))
}

// ---------------------------------------------------------------------------
// Output parsing
// ---------------------------------------------------------------------------

/// One executed step of a batch (the leading `tab` step is not included).
#[derive(Debug, Clone, PartialEq)]
pub struct StepResult {
    pub command: Vec<String>,
    pub error: Option<String>,
    pub result: Value,
}

impl StepResult {
    /// The step's result as text for the agent: the snapshot tree, the `eval`
    /// value, page text, a saved file path, or a short confirmation.
    pub fn output(&self) -> String {
        let result = &self.result;
        let text_of = |value: &Value| match value {
            Value::String(text) => text.clone(),
            other => other.to_string(),
        };
        if let Some(snapshot) = result.get("snapshot") {
            return match snapshot.get("tree") {
                Some(tree) => text_of(tree),
                None => text_of(snapshot),
            };
        }
        if let Some(value) = result.get("result") {
            return text_of(value);
        }
        if let Some(waited) = result.get("waited").and_then(Value::as_str) {
            return format!("Wait for {waited} satisfied");
        }
        for key in ["text", "value", "html", "title", "url", "path"] {
            if let Some(Value::String(text)) = result.get(key) {
                return text.clone();
            }
        }
        for (key, verb) in [("clicked", "Clicked"), ("filled", "Filled"), ("typed", "Typed")] {
            if let Some(target) = result.get(key) {
                return format!("{verb} {}", text_of(target));
            }
        }
        match result {
            Value::Null => "Done".to_string(),
            Value::Object(map) => {
                let rest: serde_json::Map<String, Value> = map
                    .iter()
                    .filter(|(key, _)| !matches!(key.as_str(), "lifecycle" | "origin"))
                    .map(|(key, value)| (key.clone(), value.clone()))
                    .collect();
                if rest.is_empty() {
                    "Done".to_string()
                } else {
                    Value::Object(rest).to_string()
                }
            }
            other => text_of(other),
        }
    }
}

/// Why a run produced no usable results.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RunError {
    /// The CDP target is gone (tab closed or replaced); re-resolve its id.
    TargetGone(String),
    /// agent-browser could not be started, timed out, or printed garbage.
    Failed(String),
}

impl std::fmt::Display for RunError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::TargetGone(message) | Self::Failed(message) => f.write_str(message),
        }
    }
}

impl std::error::Error for RunError {}

/// Parse `--json batch` stdout (an array of `{command, error, result}`) of an
/// invocation that started with a `tab <targetId>` step. See
/// [`parse_output_for`].
pub fn parse_output(stdout: &str) -> Result<Vec<StepResult>, RunError> {
    parse_output_for(stdout, true)
}

/// Parse `--json batch` stdout. When `has_tab_step` is set the leading
/// `tab <targetId>` step is consumed: if it failed the target is gone.
pub fn parse_output_for(stdout: &str, has_tab_step: bool) -> Result<Vec<StepResult>, RunError> {
    let value: Value = serde_json::from_str(stdout.trim())
        .map_err(|err| RunError::Failed(format!("unreadable agent-browser output: {err}")))?;
    let entries = value
        .as_array()
        .ok_or_else(|| RunError::Failed("agent-browser output was not an array".to_string()))?;
    let mut steps = entries.iter().map(|entry| StepResult {
        command: entry
            .get("command")
            .and_then(Value::as_array)
            .map(|parts| {
                parts
                    .iter()
                    .filter_map(|part| part.as_str().map(str::to_string))
                    .collect()
            })
            .unwrap_or_default(),
        error: entry
            .get("error")
            .and_then(Value::as_str)
            .map(str::to_string),
        result: entry.get("result").cloned().unwrap_or(Value::Null),
    });
    if has_tab_step {
        match steps.next() {
            Some(tab_step) => {
                if let Some(error) = tab_step.error {
                    return Err(RunError::TargetGone(error));
                }
            }
            None => return Err(RunError::Failed("agent-browser returned no results".to_string())),
        }
    }
    let steps: Vec<StepResult> = steps.collect();
    if steps.is_empty() {
        return Err(RunError::Failed("agent-browser returned no results".to_string()));
    }
    Ok(steps)
}

// ---------------------------------------------------------------------------
// Running
// ---------------------------------------------------------------------------

/// Run `invocation` with `binary`, killing it after `timeout`. Returns the
/// parsed step results.
pub fn run(
    binary: &Path,
    invocation: &Invocation,
    timeout: Duration,
) -> Result<Vec<StepResult>, RunError> {
    let mut child = Command::new(binary)
        .args(&invocation.args)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|err| RunError::Failed(format!("could not start {}: {err}", binary.display())))?;

    if let Some(mut stdin) = child.stdin.take() {
        // A write error means the child already exited; its output explains.
        let _ = stdin.write_all(invocation.stdin.as_bytes());
    }
    let stdout_reader = drain(child.stdout.take());
    let stderr_reader = drain(child.stderr.take());

    let deadline = Instant::now() + timeout;
    let status = loop {
        match child.try_wait() {
            Ok(Some(status)) => break Some(status),
            Ok(None) if Instant::now() >= deadline => {
                let _ = child.kill();
                let _ = child.wait();
                break None;
            }
            Ok(None) => std::thread::sleep(Duration::from_millis(15)),
            Err(err) => return Err(RunError::Failed(format!("agent-browser wait failed: {err}"))),
        }
    };
    let stdout = stdout_reader.join().unwrap_or_default();
    let stderr = stderr_reader.join().unwrap_or_default();

    match status {
        None => Err(RunError::Failed(format!(
            "agent-browser timed out after {}s",
            timeout.as_secs()
        ))),
        Some(_) if stdout.trim().is_empty() => Err(RunError::Failed(if stderr.trim().is_empty() {
            "agent-browser produced no output".to_string()
        } else {
            stderr.trim().to_string()
        })),
        // A non-zero exit still carries per-step errors in the JSON.
        Some(_) => parse_output_for(&stdout, invocation.has_tab_step),
    }
}

fn drain<R: Read + Send + 'static>(stream: Option<R>) -> std::thread::JoinHandle<String> {
    std::thread::spawn(move || {
        let mut out = String::new();
        if let Some(mut stream) = stream {
            let _ = stream.read_to_string(&mut out);
        }
        out
    })
}

// ---------------------------------------------------------------------------
// Finding and installing the binary
// ---------------------------------------------------------------------------

/// Package manager used for auto-install.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Installer {
    Npm(PathBuf),
    Bun(PathBuf),
}

impl Installer {
    /// Argv for a global install of the pinned package.
    pub fn install_args(&self) -> Vec<String> {
        match self {
            Self::Npm(_) => vec!["install".into(), "-g".into(), PACKAGE_SPEC.into()],
            Self::Bun(_) => vec!["add".into(), "-g".into(), PACKAGE_SPEC.into()],
        }
    }

    fn program(&self) -> &Path {
        match self {
            Self::Npm(path) | Self::Bun(path) => path,
        }
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Npm(_) => "npm",
            Self::Bun(_) => "bun",
        }
    }
}

/// Installers to try, in order: npm first, then bun, whichever are available.
pub fn installer_order(npm: Option<PathBuf>, bun: Option<PathBuf>) -> Vec<Installer> {
    npm.map(Installer::Npm)
        .into_iter()
        .chain(bun.map(Installer::Bun))
        .collect()
}

/// Extract the value printed between `marker` pairs, ignoring any shell
/// start-up noise around it (interactive shells print banners and warnings).
pub fn extract_marked(output: &str, marker: &str) -> Option<String> {
    let start = output.find(marker)? + marker.len();
    let rest = &output[start..];
    let end = rest.find(marker)?;
    let value = rest[..end].trim();
    (!value.is_empty()).then(|| value.to_string())
}

fn is_executable(path: &Path) -> bool {
    use std::os::unix::fs::PermissionsExt;
    path.is_file()
        && std::fs::metadata(path)
            .map(|meta| meta.permissions().mode() & 0o111 != 0)
            .unwrap_or(false)
}

fn which_in(name: &str, path_var: &std::ffi::OsStr) -> Option<PathBuf> {
    std::env::split_paths(path_var)
        .map(|dir| dir.join(name))
        .find(|candidate| is_executable(candidate))
}

/// Ask the user's login shell where `name` lives. GUI apps launched from
/// Finder do not inherit the shell `PATH` (bun, mise, nvm and friends are set
/// up in shell rc files), so a plain `PATH` search misses them.
fn which_in_login_shell(name: &str) -> Option<PathBuf> {
    const MARKER: &str = "__CONSOLE_PATH__";
    let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/zsh".to_string());
    let script = format!("printf '{MARKER}%s{MARKER}' \"$(command -v {name})\"");
    let output = Command::new(shell)
        .args(["-ilc", &script])
        .stdin(Stdio::null())
        .stderr(Stdio::null())
        .output()
        .ok()?;
    let found = PathBuf::from(extract_marked(&String::from_utf8_lossy(&output.stdout), MARKER)?);
    is_executable(&found).then_some(found)
}

fn well_known_dirs() -> Vec<PathBuf> {
    let mut dirs = Vec::new();
    if let Some(home) = std::env::var_os("HOME").map(PathBuf::from) {
        dirs.push(home.join(".bun/bin"));
        dirs.push(home.join(".local/bin"));
        dirs.push(home.join(".npm-global/bin"));
        dirs.push(home.join(".cargo/bin"));
    }
    dirs.push(PathBuf::from("/opt/homebrew/bin"));
    dirs.push(PathBuf::from("/usr/local/bin"));
    dirs
}

/// Locate an executable by name: current `PATH`, then well-known install
/// directories, then the user's login shell.
fn find_tool(name: &str) -> Option<PathBuf> {
    std::env::var_os("PATH")
        .and_then(|path| which_in(name, &path))
        .or_else(|| {
            well_known_dirs()
                .into_iter()
                .map(|dir| dir.join(name))
                .find(|candidate| is_executable(candidate))
        })
        .or_else(|| which_in_login_shell(name))
}

/// Find an installed agent-browser: the [`BIN_ENV`] override, a copy bundled
/// next to the app, then the normal tool search. Never installs anything.
pub fn find_binary() -> Option<PathBuf> {
    if let Some(path) = std::env::var_os(BIN_ENV).map(PathBuf::from) {
        return is_executable(&path).then_some(path);
    }
    if let Some(dir) = std::env::current_exe()
        .ok()
        .and_then(|exe| exe.parent().map(Path::to_path_buf))
    {
        for candidate in [
            dir.join("agent-browser"),
            dir.join("../Resources/agent-browser"),
        ] {
            if is_executable(&candidate) {
                return Some(candidate);
            }
        }
    }
    find_tool("agent-browser")
}

static INSTALL_LOCK: Mutex<()> = Mutex::new(());

/// Return an installed agent-browser, installing the pinned version with npm
/// or bun (whichever is available) when none is found. This only installs the
/// CLI package; it never runs `agent-browser install` (a Chrome download that
/// attach mode does not need). Blocking: can take a while on first use.
pub fn ensure_binary() -> Result<PathBuf, String> {
    if let Some(path) = find_binary() {
        return Ok(path);
    }
    // One installer at a time; a second caller waits, then finds the result.
    let _guard = INSTALL_LOCK.lock().unwrap_or_else(|poisoned| poisoned.into_inner());
    if let Some(path) = find_binary() {
        return Ok(path);
    }
    let installers = installer_order(find_tool("npm"), find_tool("bun"));
    if installers.is_empty() {
        return Err(format!(
            "agent-browser is not installed and neither npm nor bun was found. \
             Install one of them or run `npm install -g {PACKAGE_SPEC}`."
        ));
    }
    let mut failures = Vec::new();
    for installer in &installers {
        log::info!("agent-browser not found; installing {PACKAGE_SPEC} with {}", installer.name());
        match run_installer(installer) {
            Ok(()) => match find_binary() {
                Some(path) => return Ok(path),
                None => failures.push(format!(
                    "{} reported success but agent-browser is still not on PATH",
                    installer.name()
                )),
            },
            Err(err) => failures.push(format!("{}: {err}", installer.name())),
        }
    }
    Err(format!(
        "could not install agent-browser ({})",
        failures.join("; ")
    ))
}

fn run_installer(installer: &Installer) -> Result<(), String> {
    let program = installer.program();
    let mut command = Command::new(program);
    command
        .args(installer.install_args())
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::piped());
    // npm/bun are often shims that need `node` next to them (mise, nvm, ...).
    if let Some(dir) = program.parent() {
        let mut paths = vec![dir.to_path_buf()];
        if let Some(existing) = std::env::var_os("PATH") {
            paths.extend(std::env::split_paths(&existing));
        }
        if let Ok(joined) = std::env::join_paths(paths) {
            command.env("PATH", joined);
        }
    }
    let output = command.output().map_err(|err| format!("could not run: {err}"))?;
    if output.status.success() {
        Ok(())
    } else {
        let stderr = String::from_utf8_lossy(&output.stderr);
        let tail: String = stderr.lines().rev().take(3).collect::<Vec<_>>().into_iter().rev().collect::<Vec<_>>().join(" ");
        Err(format!("exited with {}: {tail}", output.status))
    }
}
