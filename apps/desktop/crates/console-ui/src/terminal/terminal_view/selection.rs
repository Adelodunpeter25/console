//! Text selection: word/line/all selection over the viewport grid.
use super::{TerminalCellPos, TerminalView};

impl TerminalView {
    pub fn selected_text(&self) -> Option<String> {
        let (start, end) = self.selection_range()?;
        let snapshot = self.snapshot.as_ref()?;
        let mut lines = Vec::new();

        for row_idx in start.row..=end.row {
            let Some(row) = snapshot.rows.get(row_idx as usize) else {
                continue;
            };
            let start_c = if row_idx == start.row { start.col } else { 0 };
            let end_c = if row_idx == end.row {
                end.col
            } else {
                (row.len() as u16).saturating_sub(1)
            };

            let mut line_str = String::new();
            for col_idx in start_c..=end_c {
                if let Some(cell) = row.get(col_idx as usize) {
                    if !cell.flags.wide_char_spacer {
                        line_str.push(cell.c);
                    }
                }
            }
            lines.push(line_str.trim_end().to_string());
        }

        if lines.is_empty() {
            None
        } else {
            Some(lines.join("\n"))
        }
    }

    pub fn selection_range(&self) -> Option<(TerminalCellPos, TerminalCellPos)> {
        let anchor = self.selection_anchor?;
        let head = self.selection_head?;
        if (head.row, head.col) < (anchor.row, anchor.col) {
            Some((head, anchor))
        } else {
            Some((anchor, head))
        }
    }

    pub fn clear_selection(&mut self) -> bool {
        let had = self.selection_anchor.is_some() || self.selection_head.is_some();
        self.selection_anchor = None;
        self.selection_head = None;
        self.selection_dragging = false;
        had
    }

    /// Select the whole viewport grid.
    pub fn select_all(&mut self) -> bool {
        let snapshot = match self.snapshot.as_ref() {
            Some(s) => s,
            None => return false,
        };
        if snapshot.rows.is_empty() {
            return false;
        }
        let last_row = (snapshot.rows.len() as u16).saturating_sub(1);
        let last_col = snapshot
            .rows
            .last()
            .map(|r| (r.len() as u16).saturating_sub(1))
            .unwrap_or(0);
        self.selection_anchor = Some(TerminalCellPos { col: 0, row: 0 });
        self.selection_head = Some(TerminalCellPos {
            col: last_col,
            row: last_row,
        });
        self.selection_dragging = false;
        true
    }

    /// Double-click: select the word token under a cell. Classes mirror
    /// termy (`whitespace | [alnum + _] | other`) with wide-spacer handling.
    /// Returns false when there is nothing selectable (e.g. trailing space),
    /// letting the caller fall through to a fresh single-click selection.
    pub fn select_token_at_cell(&mut self, pos: TerminalCellPos) -> bool {
        let snapshot = match self.snapshot.as_ref() {
            Some(s) => s,
            None => return false,
        };
        let row = match snapshot.rows.get(pos.row as usize) {
            Some(r) => r,
            None => return false,
        };
        if row.is_empty() || pos.col as usize >= row.len() {
            return false;
        }
        // Spacer cells stand for the wide glyph to their left.
        let line: Vec<Option<char>> = row
            .iter()
            .map(|c| {
                if c.flags.wide_char_spacer {
                    None
                } else {
                    Some(c.c)
                }
            })
            .collect();
        let mut col = pos.col as usize;
        if line[col].is_none() && col > 0 {
            col -= 1;
        }
        let Some(ch) = line[col] else {
            return false;
        };
        let class = selection_char_class(ch);
        if class == 0 {
            // Whitespace past the last printable cell is not selectable.
            let Some(last) = line
                .iter()
                .rposition(|c| c.is_some_and(|c| !c.is_whitespace()))
            else {
                return false;
            };
            if col > last {
                return false;
            }
        }
        let mut start = col;
        while start > 0 {
            match line[start - 1] {
                None if start >= 2
                    && line[start - 2].is_some_and(|c| selection_char_class(c) == class) =>
                {
                    start -= 1;
                }
                Some(c) if selection_char_class(c) == class => start -= 1,
                _ => break,
            }
        }
        let mut end = col;
        while end + 1 < line.len() {
            match line[end + 1] {
                None if end + 2 < line.len()
                    && line[end + 2].is_some_and(|c| selection_char_class(c) == class) =>
                {
                    end += 1;
                }
                Some(c) if selection_char_class(c) == class => end += 1,
                _ => break,
            }
        }
        // Cover the trailing spacer of a wide glyph at the token end.
        if end + 1 < line.len() && line[end + 1].is_none() {
            end += 1;
        }
        self.selection_anchor = Some(TerminalCellPos {
            col: start as u16,
            row: pos.row,
        });
        self.selection_head = Some(TerminalCellPos {
            col: end as u16,
            row: pos.row,
        });
        self.selection_dragging = false;
        true
    }

    /// Triple-click: select the full viewport row.
    pub fn select_line_at_row(&mut self, row: u16) -> bool {
        let snapshot = match self.snapshot.as_ref() {
            Some(s) => s,
            None => return false,
        };
        let row_cells = match snapshot.rows.get(row as usize) {
            Some(r) => r,
            None => return false,
        };
        let last_col = (row_cells.len() as u16).saturating_sub(1);
        self.selection_anchor = Some(TerminalCellPos { col: 0, row });
        self.selection_head = Some(TerminalCellPos {
            col: last_col,
            row,
        });
        self.selection_dragging = false;
        true
    }
}

/// Word-select character class: 0 = whitespace, 1 = word (`alnum | _`),
/// 2 = everything else (runs of punctuation select together).
fn selection_char_class(c: char) -> u8 {
    if c.is_whitespace() {
        0
    } else if c.is_alphanumeric() || c == '_' {
        1
    } else {
        2
    }
}
