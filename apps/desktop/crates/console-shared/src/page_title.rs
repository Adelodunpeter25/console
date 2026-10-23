//! The human title of a web page, for showing in place of a bare URL.
//!
//! Pure extraction only; fetching the page lives in `console-core`.

/// How much of the response body is searched for `<title>`.
pub const MAX_BODY_BYTES: usize = 96 * 1024;
/// Titles longer than this are cut with an ellipsis.
const MAX_TITLE_CHARS: usize = 90;

/// The page title from an HTML document, cleaned for display: entities
/// decoded, whitespace collapsed, length capped. `None` when there is none.
pub fn extract_html_title(html: &str) -> Option<String> {
    let lower = html.to_ascii_lowercase();
    let open = lower.find("<title")?;
    let content_start = open + lower[open..].find('>')? + 1;
    let content_end = content_start + lower[content_start..].find("</title")?;
    let raw = html.get(content_start..content_end)?;
    let decoded = decode_entities(raw);
    let collapsed = decoded.split_whitespace().collect::<Vec<_>>().join(" ");
    if collapsed.is_empty() {
        return None;
    }
    if collapsed.chars().count() > MAX_TITLE_CHARS {
        let cut: String = collapsed.chars().take(MAX_TITLE_CHARS - 1).collect();
        return Some(format!("{}…", cut.trim_end()));
    }
    Some(collapsed)
}

fn decode_entities(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let mut rest = text;
    while let Some(amp) = rest.find('&') {
        out.push_str(&rest[..amp]);
        let tail = &rest[amp..];
        match tail.find(';').filter(|end| *end <= 10) {
            Some(end) => {
                let entity = &tail[1..end];
                match decode_entity(entity) {
                    Some(c) => out.push(c),
                    None => out.push_str(&tail[..=end]),
                }
                rest = &tail[end + 1..];
            }
            None => {
                out.push('&');
                rest = &tail[1..];
            }
        }
    }
    out.push_str(rest);
    out
}

fn decode_entity(entity: &str) -> Option<char> {
    match entity {
        "amp" => Some('&'),
        "lt" => Some('<'),
        "gt" => Some('>'),
        "quot" => Some('"'),
        "apos" => Some('\''),
        "nbsp" => Some(' '),
        _ => {
            let number = entity.strip_prefix('#')?;
            let code = match number.strip_prefix(['x', 'X']) {
                Some(hex) => u32::from_str_radix(hex, 16).ok()?,
                None => number.parse().ok()?,
            };
            char::from_u32(code)
        }
    }
}
