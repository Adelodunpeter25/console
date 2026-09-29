//! Wraps agent-supplied JavaScript so its result comes back over IPC.

/// Largest result (in characters) kept for one script run.
pub const MAX_RESULT_CHARS: usize = 30_000;

const TEMPLATE: &str = include_str!("agent_script.js");

/// Wrap `script` so the page evaluates it, awaits the value, and posts a
/// `script_result` IPC message tagged with `request_id`.
pub fn wrap_script(request_id: &str, script: &str) -> String {
    let quote = |s: &str| serde_json::to_string(s).unwrap_or_else(|_| "\"\"".to_string());
    TEMPLATE
        .replace("__REQUEST_ID__", &quote(request_id))
        .replace("__SOURCE__", &quote(script))
        .replace("__MAX_CHARS__", &MAX_RESULT_CHARS.to_string())
}

/// Cap `text` at `MAX_RESULT_CHARS` characters, marking the cut.
pub fn cap_result(text: String) -> String {
    let total = text.chars().count();
    if total <= MAX_RESULT_CHARS {
        return text;
    }
    let mut cut: String = text.chars().take(MAX_RESULT_CHARS).collect();
    cut.push_str(&format!("\n...[truncated: {total} chars total]"));
    cut
}
