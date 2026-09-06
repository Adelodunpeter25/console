use crate::types::terminal::{
    CursorPosition, TerminalBackend, TerminalCell, TerminalCellFlags, TerminalColor,
    TerminalGridSnapshot, TerminalSize,
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

        let mut detected_links = Vec::new();
        for r in 0..rows {
            let mut c = 0;
            while c < cols {
                if let Some(link) = self.term.link_at(r, c) {
                    // `end_col` is relative to the link's FINAL row: a link
                    // whose text soft-wraps onto later rows reports the end
                    // column of its last row (termy clips the range to the
                    // viewport, not to this row). On this row the link runs
                    // to the line end. Never let the cursor rewind — a
                    // rewind spins this loop forever, wedges the backend
                    // mutex, and kills the terminal (see the wrapped-OSC-8
                    // regression test).
                    let end_col = if link.end_row > r {
                        cols.saturating_sub(1)
                    } else {
                        link.end_col.min(cols.saturating_sub(1))
                    };
                    detected_links.push(crate::types::terminal::TerminalLink {
                        start_row: link.start_row as u16,
                        start_col: link.start_col as u16,
                        end_row: link.end_row as u16,
                        end_col: end_col as u16,
                        target: link.target,
                    });
                    c = c.max(end_col) + 1;
                } else {
                    c += 1;
                }
            }
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

    pub fn link_at(&self, row: usize, col: usize) -> Option<termy_core::DetectedViewportLink> {
        self.term.link_at(row, col)
    }
}
