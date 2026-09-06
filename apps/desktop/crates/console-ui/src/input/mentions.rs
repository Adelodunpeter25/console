use std::ops::Range;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ComposerMention {
    pub range: Range<usize>,
    pub path: String,
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
        content.get(m.range.clone()) == Some(m.path.as_str())
    });
    mentions.sort_by_key(|m| m.range.start);
    mentions.dedup_by(|a, b| a.range == b.range);
}
