use std::ops::Range;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ComposerMention {
    pub range: Range<usize>,
    /// Full path sent to the agent.
    pub path: String,
    /// Filename shown in the chip (may differ from path).
    pub label: String,
}

pub fn adjust_mentions(
    mentions: &mut Vec<ComposerMention>,
    edit_range: &Range<usize>,
    new_text_len: usize,
) {
    let old_len = edit_range.len();
    let delta = (new_text_len as isize) - (old_len as isize);
    let mut kept = Vec::with_capacity(mentions.len());

    for mut mention in mentions.drain(..) {
        if edit_range.end <= mention.range.start {
            let start = ((mention.range.start as isize) + delta).max(0) as usize;
            let end = ((mention.range.end as isize) + delta).max(0) as usize;
            mention.range = start..end;
            kept.push(mention);
        } else if edit_range.start >= mention.range.end {
            kept.push(mention);
        }
    }
    *mentions = kept;
}

pub fn reconcile_mentions(mentions: &mut Vec<ComposerMention>, content: &str) {
    mentions.retain(|m| {
        if m.range.end > content.len() || m.range.start >= m.range.end {
            return false;
        }
        if !content.is_char_boundary(m.range.start) || !content.is_char_boundary(m.range.end) {
            return false;
        }
        content.get(m.range.clone()) == Some(m.label.as_str())
    });
    mentions.sort_by_key(|m| m.range.start);
    mentions.dedup_by(|a, b| a.range == b.range);
}

/// Rebuild mention ranges by locating each context-file's basename as a
/// whitespace-delimited token in `content`. Used when text and paths are
/// known but editor ranges are not — prompt-history recall and queued-prompt
/// restore both start from just `(text, paths)`.
pub fn mentions_from_context_files(content: &str, context_files: &[String]) -> Vec<ComposerMention> {
    let mut mentions = Vec::new();
    let mut search_from = 0;
    for path in context_files {
        let label = std::path::Path::new(path)
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or(path)
            .to_string();
        let Some(relative_start) = content[search_from..].find(&label) else {
            continue;
        };
        let start = search_from + relative_start;
        let end = start + label.len();
        let before_ok = start == 0
            || content[..start]
                .chars()
                .next_back()
                .is_some_and(|character| character.is_whitespace());
        let after_ok = end >= content.len()
            || content[end..]
                .chars()
                .next()
                .is_some_and(|character| character.is_whitespace());
        if before_ok && after_ok {
            mentions.push(ComposerMention {
                range: start..end,
                path: path.clone(),
                label,
            });
            search_from = end;
        }
    }
    mentions
}
