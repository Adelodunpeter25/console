use crate::types::terminal::{
    CursorPosition, TerminalBackend, TerminalCell, TerminalCellFlags, TerminalColor,
    TerminalGridSnapshot, TerminalLink, TerminalSize,
};
use termy_core::{Terminal, TerminalSize as TermySize};

pub struct TermyBackend {
    term: Terminal,
    size: TerminalSize,
}

fn to_termysize(size: TerminalSize) -> TermySize {
    TermySize {
        cols: size.cols,
        rows: size.rows,
        cell_width: 9.0,
        cell_height: 18.0,
    }
}

fn termycolor_to_terminal(c: termy_core::TermyColor) -> Option<TerminalColor> {
    // TermyColor has a, but console's TerminalColor is RGB; ignore alpha 0?
    // Treat fully transparent as None (default)?
    if c.a == 0 {
        return None;
    }
    Some(TerminalColor { r: c.r, g: c.g, b: c.b })
}

impl TerminalBackend for TermyBackend {
    fn new(size: TerminalSize) -> Self {
        let tsize = to_termysize(size);
        let term = Terminal::new_display(tsize, None);
        Self { term, size }
    }

    fn resize(&mut self, size: TerminalSize) {
        self.size = size;
        let tsize = to_termysize(size);
        self.term.resize(tsize);
    }

    fn advance(&mut self, data: &str) {
        // termy_core expects bytes
        self.term.feed_output(data.as_bytes());
    }

    fn snapshot(&self) -> TerminalGridSnapshot {
        let frame = self.term.snapshot();
        let cols = frame.cols as usize;
        let rows = frame.rows as usize;
        let mut out_rows = Vec::with_capacity(rows);
        for r in 0..rows {
            let mut row = Vec::with_capacity(cols);
            for c in 0..cols {
                let idx = r * cols + c;
                if let Some(cell) = frame.cells.get(idx) {
                    let fg = termycolor_to_terminal(cell.fg);
                    let bg = termycolor_to_terminal(cell.bg);
                    let flags = TerminalCellFlags {
                        bold: cell.bold,
                        dim: false,
                        italic: cell.italic,
                        underline: cell.underline,
                        inverse: false,
                        hidden: !cell.render_text && cell.char == ' ' && !cell.wide_character_spacer,
                        strike: cell.strikethrough,
                        blink: false,
                        wrapline: cell.line_wrapped,
                        wide_char: cell.wide_character_spacer,
                        wide_char_spacer: cell.wide_character_spacer,
                    };
                    row.push(TerminalCell {
                        c: cell.char,
                        fg,
                        bg,
                        flags,
                    });
                } else {
                    row.push(TerminalCell {
                        c: ' ',
                        fg: None,
                        bg: None,
                        flags: TerminalCellFlags::default(),
                    });
                }
            }
            out_rows.push(row);
        }

        // URL-only link detection, computed straight from the rendered grid
        // text. We deliberately do NOT call termy's `link_at` here: it walks
        // OSC 8 hyperlink metadata and runs file-path heuristics (including
        // filesystem canonicalization) per cell, which is dead weight for the
        // common case and has pathological behavior for links that soft-wrap
        // across rows. Only http/https/www URLs are surfaced — file paths and
        // OSC 8 targets are ignored on purpose.
        let mut detected_links = Vec::new();
        for (r, row) in out_rows.iter().enumerate() {
            detect_url_links(row, r, &mut detected_links);
        }

        let cursor = frame.cursor;
        let cursor_pos = if let Some(cur) = cursor {
            CursorPosition {
                col: cur.col as u16,
                row: cur.row as u16,
                visible: true,
            }
        } else {
            CursorPosition {
                col: 0,
                row: 0,
                visible: false,
            }
        };
        TerminalGridSnapshot {
            rows: out_rows,
            cursor: cursor_pos,
            size: self.size,
            links: detected_links,
            keyboard_mode: self.term.keyboard_mode(),
            bracketed_paste: self.term.bracketed_paste_mode(),
        }
    }

    fn size(&self) -> TerminalSize {
        self.size
    }

    fn scroll(&mut self, delta: i32) {
        // termy_core scroll_display expects delta_lines positive = up?
        // console's delta positive = scroll up. Pass through.
        let _ = self.term.scroll_display(delta);
    }

    fn is_alt_screen(&self) -> bool {
        self.term.alternate_screen_mode()
    }
}

impl TermyBackend {
    pub fn keyboard_mode(&self) -> termy_core::TerminalKeyboardMode {
        self.term.keyboard_mode()
    }

    pub fn bracketed_paste(&self) -> bool {
        self.term.bracketed_paste_mode()
    }
}

/// Case-insensitive prefix check for the ASCII URL schemes we detect.
fn starts_with_ignore_case(chars: &[char], prefix: &str) -> bool {
    chars.len() >= prefix.len()
        && chars[..prefix.len()]
            .iter()
            .zip(prefix.chars())
            .all(|(a, b)| a.to_ascii_lowercase() == b)
}

/// Scan one rendered grid row for URL tokens and append links for each.
///
/// Only `http://`, `https://`, and `www.` tokens count as links; file paths,
/// bare domains, and OSC 8 hyperlinks are deliberately ignored. Detection is
/// per-row: a URL soft-wrapped across the terminal edge is not linked (the
/// same tradeoff termy's own heuristics make).
fn detect_url_links(row: &[TerminalCell], row_idx: usize, links: &mut Vec<TerminalLink>) {
    const PREFIXES: [(&str, usize); 3] = [("https://", 8), ("http://", 7), ("www.", 4)];

    let chars: Vec<char> = row.iter().map(|cell| cell.c).collect();
    let mut c = 0usize;
    while c < chars.len() {
        let scheme_len = PREFIXES
            .iter()
            .find(|(prefix, _)| starts_with_ignore_case(&chars[c..], prefix))
            .map(|(_, len)| *len);
        let Some(scheme_len) = scheme_len else {
            c += 1;
            continue;
        };

        // Token ends at whitespace, quotes, or brackets.
        let mut end = c + scheme_len;
        while end < chars.len() {
            let ch = chars[end];
            if ch.is_whitespace() || matches!(ch, '"' | '\'' | '<' | '>' | '[' | ']' | '(' | ')') {
                break;
            }
            end += 1;
        }
        // Trim punctuation commonly glued to URLs in prose/shell output.
        while end > c + scheme_len
            && matches!(chars[end - 1], '.' | ',' | ';' | ':' | '!' | '?')
        {
            end -= 1;
        }

        // A lone "www." or "http://" is not a link.
        if end < c + scheme_len + 4 {
            c += 1;
            continue;
        }

        let mut target: String = chars[c..end].iter().collect();
        if target.len() >= 4 && target[..4].eq_ignore_ascii_case("www.") {
            target = format!("https://{target}");
        }
        links.push(TerminalLink {
            start_row: row_idx as u16,
            start_col: c as u16,
            end_row: row_idx as u16,
            end_col: (end - 1) as u16,
            target,
        });
        c = end;
    }
}
