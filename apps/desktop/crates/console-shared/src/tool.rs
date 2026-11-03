//! Pure formatting and parsing helpers for tool-call UI.

use serde_json::Value;

const PATH_KEYS: [&str; 4] = ["path", "filePath", "targetFile", "absolutePath"];

pub fn single_line(value: &str) -> String {
    value.split_whitespace().collect::<Vec<_>>().join(" ")
}

pub fn truncate(value: &str, max_chars: usize) -> String {
    let mut chars = value.chars();
    let output: String = chars.by_ref().take(max_chars).collect();
    if chars.next().is_some() {
        format!("{output}…")
    } else {
        output
    }
}

pub fn to_relative_path(path: &str, cwd: Option<&str>) -> String {
    let path = path.replace('\\', "/");
    let Some(cwd) = cwd.map(|value| value.replace('\\', "/")) else {
        return path;
    };
    let cwd = cwd.trim_end_matches('/');
    if !cwd.is_empty() {
        if let Some(rest) = path.strip_prefix(cwd) {
            return rest.trim_start_matches('/').to_owned();
        }
    }
    path
}

pub fn argument_path<'a>(arguments: &'a Value) -> Option<&'a str> {
    let object = arguments.as_object()?;
    PATH_KEYS
        .into_iter()
        .find_map(|key| object.get(key).and_then(Value::as_str))
}

pub fn argument_summary(name: &str, arguments: &Value, cwd: Option<&str>) -> Option<String> {
    let object = arguments.as_object()?;
    let string = |key: &str| object.get(key).and_then(Value::as_str);
    let cwd = cwd.or_else(|| string("cwd"));
    if let Some(path) = argument_path(arguments) {
        return Some(truncate(&to_relative_path(path, cwd), 72));
    }
    if name == "bashJob" || name == "bash_job" {
        let action = string("action").unwrap_or("status");
        return Some(truncate(
            &match string("jobId") {
                Some(id) => format!("{action} {id}"),
                None => action.to_owned(),
            },
            72,
        ));
    }
    if name == "loadTools" {
        return string("group").map(|value| truncate(value, 72));
    }
    // The console and computer tools take a script in `code`. The script is
    // noise in a one-line row; the calls it makes say what it did.
    if matches!(name, "console" | "computer") {
        return string("code").and_then(|code| script_calls(name, code));
    }
    if name == "memory" {
        let op = string("op").unwrap_or("memory");
        if let Some(query) = string("query") {
            return Some(truncate(&format!(r#"{op} "{query}""#), 72));
        }
        if let Some(content) = string("content") {
            return Some(truncate(&format!(r#"{op} "{}""#, single_line(content)), 72));
        }
        if let Some(tags) = object.get("tags").and_then(Value::as_array) {
            let tags = tags
                .iter()
                .filter_map(Value::as_str)
                .collect::<Vec<_>>()
                .join(", ");
            return Some(truncate(&format!("{op} [{tags}]"), 72));
        }
        if let Some(id) = string("id") {
            return Some(truncate(&format!("{op} {id}"), 72));
        }
        return Some(truncate(op, 72));
    }
    for key in ["command", "pattern", "query", "url", "directory"] {
        if let Some(value) = string(key) {
            let value = if key == "directory" {
                to_relative_path(value, cwd)
            } else {
                value.to_owned()
            };
            return Some(truncate(&single_line(&value), 72));
        }
    }
    for key in ["paths", "files", "operations"] {
        if let Some(values) = object
            .get(key)
            .and_then(Value::as_array)
            .filter(|values| !values.is_empty())
        {
            return Some(format!(
                "{} {}",
                values.len(),
                if key == "operations" {
                    "operations"
                } else {
                    "files"
                }
            ));
        }
    }
    None
}

/// The distinct `root.method(` calls a script makes, in first-use order, as
/// `sessions.list, scripts.run`. `root` is the object the script calls (`console`
/// or `computer`); only names that look like calls on it are listed, so a string
/// or comment that merely mentions one is not mistaken for a call.
///
/// Returns None when the script calls nothing on `root` (pure JavaScript), so the
/// row falls back to showing no summary rather than inventing one.
pub fn script_calls(root: &str, code: &str) -> Option<String> {
    let bytes = code.as_bytes();
    let root_b = root.as_bytes();
    let mut calls: Vec<String> = Vec::new();
    let mut i = 0;
    let mut quote: Option<u8> = None;
    while i < bytes.len() {
        let c = bytes[i];
        // Skip string literals and comments so `"console.x()"` is not a call.
        if let Some(q) = quote {
            if c == b'\\' {
                i += 2;
                continue;
            }
            if c == q {
                quote = None;
            }
            i += 1;
            continue;
        }
        match c {
            b'"' | b'\'' | b'`' => {
                quote = Some(c);
                i += 1;
                continue;
            }
            b'/' if bytes.get(i + 1) == Some(&b'/') => {
                while i < bytes.len() && bytes[i] != b'\n' {
                    i += 1;
                }
                continue;
            }
            b'/' if bytes.get(i + 1) == Some(&b'*') => {
                i += 2;
                while i + 1 < bytes.len() && !(bytes[i] == b'*' && bytes[i + 1] == b'/') {
                    i += 1;
                }
                i += 2;
                continue;
            }
            _ => {}
        }
        let at_boundary = i == 0 || !is_ident_byte(bytes[i - 1]) && bytes[i - 1] != b'.';
        if at_boundary && bytes[i..].starts_with(root_b) && bytes.get(i + root_b.len()) == Some(&b'.') {
            let mut j = i + root_b.len() + 1;
            let start = j;
            while j < bytes.len() && (is_ident_byte(bytes[j]) || bytes[j] == b'.') {
                j += 1;
            }
            let path = &code[start..j];
            // A call has an opening parenthesis next (allowing whitespace).
            let mut k = j;
            while k < bytes.len() && bytes[k].is_ascii_whitespace() {
                k += 1;
            }
            let path = path.trim_end_matches('.');
            if bytes.get(k) == Some(&b'(') && !path.is_empty() && !calls.iter().any(|c| c == path) {
                calls.push(path.to_owned());
            }
            i = j;
            continue;
        }
        i += 1;
    }
    if calls.is_empty() {
        return None;
    }
    Some(truncate(&calls.join(", "), 72))
}

fn is_ident_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_' || b == b'$'
}

pub fn parse_read_file_output(raw: &str) -> (String, String) {
    let lines: Vec<&str> = raw.lines().collect();
    let header_end = if lines.first().is_some_and(|line| line.starts_with("File:")) {
        lines
            .iter()
            .skip(1)
            .position(|line| line.is_empty())
            .filter(|end| *end > 0 && *end <= 6)
            .map(|end| end + 1)
    } else {
        None
    };
    let code = &lines[header_end.unwrap_or(0)..];
    let parsed: Vec<Option<(&str, &str)>> = code
        .iter()
        .map(|line| parse_line_number_and_code(line))
        .collect();
    if parsed.iter().filter(|line| line.is_some()).count() * 2 > code.len() {
        let numbers = parsed
            .iter()
            .map(|line| line.map_or("", |(number, _)| number))
            .collect::<Vec<_>>()
            .join("\n");
        let body = parsed
            .iter()
            .map(|line| line.map_or("", |(_, body)| body))
            .collect::<Vec<_>>()
            .join("\n");
        (numbers, body)
    } else {
        (
            (1..=code.len())
                .map(|n| n.to_string())
                .collect::<Vec<_>>()
                .join("\n"),
            code.join("\n"),
        )
    }
}

fn parse_line_number_and_code(line: &str) -> Option<(&str, &str)> {
    let start = line.len() - line.trim_start().len();
    let bytes = line.as_bytes();
    let mut end = start;
    while end < line.len() && bytes[end].is_ascii_digit() {
        end += 1;
    }
    if end == start || bytes.get(end) != Some(&b':') {
        return None;
    }
    let number = &line[start..end];
    end += 1;
    if bytes.get(end) == Some(&b' ') {
        end += 1;
    }
    Some((number, &line[end..]))
}
