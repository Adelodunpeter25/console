use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::time::Duration;

use console_ui::browser::cef::agent_browser::{
    BuildError, Installer, PACKAGE_SPEC, RunError, build_invocation, extract_marked,
    installer_order, is_valid_target_id, parse_output, run,
};

const TARGET: &str = "294474B9957CEF672F9BFB2130A4B9D5";

fn cmd(parts: &[&str]) -> Vec<String> {
    parts.iter().map(|p| p.to_string()).collect()
}

fn fake_binary(name: &str, script: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("console-agent-browser-test-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    let path = dir.join(name);
    std::fs::write(&path, format!("#!/bin/sh\n{script}\n")).unwrap();
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).unwrap();
    path
}

fn read(path: &Path) -> String {
    std::fs::read_to_string(path).unwrap_or_default()
}

#[test]
fn target_id_must_be_32_hex_chars() {
    assert!(is_valid_target_id(TARGET));
    assert!(is_valid_target_id("294474b9957cef672f9bfb2130a4b9d5"));
    assert!(!is_valid_target_id("t1"));
    assert!(!is_valid_target_id(""));
    assert!(!is_valid_target_id("294474B9957CEF672F9BFB2130A4B9D"));
    assert!(!is_valid_target_id("294474B9957CEF672F9BFB2130A4B9DZ"));
}

#[test]
fn builds_an_atomic_batch_that_always_attaches_over_cdp() {
    let invocation = build_invocation(
        9333,
        "workspace-1",
        TARGET,
        &[cmd(&["snapshot", "-i"])],
    )
    .unwrap();
    assert_eq!(
        invocation.args,
        cmd(&[
            "--cdp", "9333", "--session", "workspace-1", "--json", "batch", "--bail"
        ])
    );
    let steps: Vec<Vec<String>> = serde_json::from_str(&invocation.stdin).unwrap();
    assert_eq!(steps, vec![cmd(&["tab", TARGET]), cmd(&["snapshot", "-i"])]);
}

#[test]
fn agent_text_stays_data_even_when_it_looks_like_flags_or_json() {
    let nasty = r#"--cdp 1 --session evil"]],[["close"]"#;
    let invocation = build_invocation(
        9333,
        "s",
        TARGET,
        &[cmd(&["fill", "@e1", nasty]), cmd(&["eval", "--help"])],
    )
    .unwrap();
    // The flag-looking text is an element of a JSON array, never an argv entry.
    assert!(!invocation.args.iter().any(|arg| arg.contains("evil")));
    let steps: Vec<Vec<String>> = serde_json::from_str(&invocation.stdin).unwrap();
    assert_eq!(steps.len(), 3);
    assert_eq!(steps[1], cmd(&["fill", "@e1", nasty]));
    assert_eq!(steps[2], cmd(&["eval", "--help"]));
}

#[test]
fn refuses_tab_lifecycle_close_and_install() {
    for verb in ["tab", "close", "install", "open", "connect", "launch", "upgrade", "--cdp"] {
        assert_eq!(
            build_invocation(9333, "s", TARGET, &[cmd(&[verb, "x"])]),
            Err(BuildError::DisallowedVerb(verb.to_string())),
            "{verb} must be refused"
        );
    }
    // A bad verb anywhere in the batch refuses the whole batch.
    assert!(build_invocation(9333, "s", TARGET, &[cmd(&["snapshot"]), cmd(&["close"])]).is_err());
}

#[test]
fn refuses_bad_target_session_and_empty_input() {
    assert!(matches!(
        build_invocation(9333, "s", "t2", &[cmd(&["snapshot"])]),
        Err(BuildError::InvalidTargetId(_))
    ));
    for session in ["", "has space", "semi;colon", "../x", &"a".repeat(65)] {
        assert!(
            matches!(
                build_invocation(9333, session, TARGET, &[cmd(&["snapshot"])]),
                Err(BuildError::InvalidSession(_))
            ),
            "{session:?}"
        );
    }
    assert_eq!(build_invocation(9333, "s", TARGET, &[]), Err(BuildError::NoCommands));
    assert_eq!(
        build_invocation(9333, "s", TARGET, &[vec![]]),
        Err(BuildError::EmptyCommand)
    );
}

#[test]
fn parse_output_drops_the_tab_step_and_keeps_results() {
    let out = format!(
        r#"[{{"command":["tab","{TARGET}"],"error":null,"result":{{"tabId":"t1"}}}},
            {{"command":["eval","1+1"],"error":null,"result":{{"value":2}}}},
            {{"command":["click","@e9"],"error":"Could not locate element","result":null}}]"#
    );
    let steps = parse_output(&out).unwrap();
    assert_eq!(steps.len(), 2);
    assert_eq!(steps[0].command, cmd(&["eval", "1+1"]));
    assert_eq!(steps[0].error, None);
    assert_eq!(steps[0].result["value"], 2);
    assert_eq!(steps[1].error.as_deref(), Some("Could not locate element"));
}

#[test]
fn parse_output_reports_a_vanished_target() {
    let out = r#"[{"command":["tab","294474B9957CEF672F9BFB2130A4B9D5"],"error":"No tab with label `x`","result":null}]"#;
    assert_eq!(
        parse_output(out),
        Err(RunError::TargetGone("No tab with label `x`".to_string()))
    );
}

#[test]
fn parse_output_rejects_garbage() {
    assert!(matches!(parse_output("not json"), Err(RunError::Failed(_))));
    assert!(matches!(parse_output("{}"), Err(RunError::Failed(_))));
    assert!(matches!(parse_output("[]"), Err(RunError::Failed(_))));
}

#[test]
fn run_pipes_the_batch_on_stdin_and_parses_stdout() {
    let capture = std::env::temp_dir().join(format!("ab-stdin-{}", std::process::id()));
    let _ = std::fs::remove_file(&capture);
    let bin = fake_binary(
        "ok",
        &format!(
            r#"cat > "{}"
echo "$@" >> "{}.args"
echo '[{{"command":["tab","x"],"error":null,"result":{{}}}},{{"command":["get","url"],"error":null,"result":{{"url":"https://example.com/"}}}}]'"#,
            capture.display(),
            capture.display()
        ),
    );
    let invocation = build_invocation(9333, "ws", TARGET, &[cmd(&["get", "url"])]).unwrap();
    let steps = run(&bin, &invocation, Duration::from_secs(10)).unwrap();
    assert_eq!(steps.len(), 1);
    assert_eq!(steps[0].result["url"], "https://example.com/");
    assert_eq!(read(&capture), invocation.stdin);
    let args = read(&PathBuf::from(format!("{}.args", capture.display())));
    assert!(args.contains("--cdp 9333 --session ws --json batch --bail"), "{args}");
}

#[test]
fn run_kills_a_hung_process_after_the_timeout() {
    let bin = fake_binary("hang", "sleep 30");
    let invocation = build_invocation(9333, "ws", TARGET, &[cmd(&["snapshot"])]).unwrap();
    let started = std::time::Instant::now();
    let err = run(&bin, &invocation, Duration::from_millis(300)).unwrap_err();
    assert!(started.elapsed() < Duration::from_secs(5));
    assert!(matches!(&err, RunError::Failed(m) if m.contains("timed out")), "{err:?}");
}

#[test]
fn run_surfaces_stderr_when_nothing_is_printed() {
    let bin = fake_binary("boom", "echo 'cannot reach browser' >&2; exit 1");
    let invocation = build_invocation(9333, "ws", TARGET, &[cmd(&["snapshot"])]).unwrap();
    let err = run(&bin, &invocation, Duration::from_secs(10)).unwrap_err();
    assert_eq!(err, RunError::Failed("cannot reach browser".to_string()));
}

#[test]
fn run_reports_a_missing_binary() {
    let invocation = build_invocation(9333, "ws", TARGET, &[cmd(&["snapshot"])]).unwrap();
    let err = run(Path::new("/nonexistent/agent-browser"), &invocation, Duration::from_secs(1)).unwrap_err();
    assert!(matches!(err, RunError::Failed(m) if m.contains("could not start")));
}

#[test]
fn installer_prefers_npm_then_falls_back_to_bun() {
    let npm = PathBuf::from("/usr/bin/npm");
    let bun = PathBuf::from("/usr/bin/bun");
    assert_eq!(
        installer_order(Some(npm.clone()), Some(bun.clone())),
        vec![Installer::Npm(npm.clone()), Installer::Bun(bun.clone())]
    );
    assert_eq!(installer_order(None, Some(bun.clone())), vec![Installer::Bun(bun)]);
    assert_eq!(installer_order(Some(npm.clone()), None), vec![Installer::Npm(npm)]);
    assert!(installer_order(None, None).is_empty());
}

#[test]
fn installers_install_the_pinned_package_globally_and_never_run_agent_browser_install() {
    assert_eq!(
        Installer::Npm("npm".into()).install_args(),
        cmd(&["install", "-g", PACKAGE_SPEC])
    );
    assert_eq!(
        Installer::Bun("bun".into()).install_args(),
        cmd(&["add", "-g", PACKAGE_SPEC])
    );
    assert!(PACKAGE_SPEC.contains('@') && PACKAGE_SPEC.starts_with("agent-browser@"));
}

#[test]
fn extract_marked_ignores_shell_noise() {
    let out = "Last login: today\nwelcome banner\n__M__/Users/me/.bun/bin/agent-browser__M__\nbye";
    assert_eq!(
        extract_marked(out, "__M__").as_deref(),
        Some("/Users/me/.bun/bin/agent-browser")
    );
    assert_eq!(extract_marked("__M____M__", "__M__"), None);
    assert_eq!(extract_marked("no markers here", "__M__"), None);
    assert_eq!(extract_marked("__M__only-one", "__M__"), None);
}
