//! File line comments carried inside a prompt.
//!
//! A staged comment travels as a tagged block appended to the message text, so
//! the agent reads `path`, line range and note, while clients can strip the
//! blocks back out and paint them as chips instead of raw text.

const OPEN: &str = "<file_comment path=\"";
const CLOSE: &str = "</file_comment>";

/// One comment recovered from a message.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FileCommentRef {
    pub path: String,
    pub lines: String,
    pub text: String,
}

/// Render one comment as the block the agent receives.
pub fn format_file_comment(path: &str, lines: &str, text: &str) -> String {
    format!(
        "{OPEN}{}\" lines=\"{lines}\">\n{text}\n{CLOSE}",
        path.replace('"', "&quot;")
    )
}

/// Split `content` into the user's own text and the comment blocks in it.
pub fn split_file_comments(content: &str) -> (String, Vec<FileCommentRef>) {
    let mut text = String::new();
    let mut comments = Vec::new();
    let mut rest = content;
    while let Some(start) = rest.find(OPEN) {
        let after_open = &rest[start + OPEN.len()..];
        let parsed = (|| {
            let (path, tail) = after_open.split_once("\" lines=\"")?;
            let (lines, tail) = tail.split_once("\">")?;
            let (body, tail) = tail.split_once(CLOSE)?;
            Some((path, lines, body, tail))
        })();
        let Some((path, lines, body, tail)) = parsed else {
            break;
        };
        text.push_str(&rest[..start]);
        comments.push(FileCommentRef {
            path: path.replace("&quot;", "\""),
            lines: lines.to_string(),
            text: body.trim_matches('\n').to_string(),
        });
        rest = tail;
    }
    text.push_str(rest);
    (text.trim().to_string(), comments)
}
