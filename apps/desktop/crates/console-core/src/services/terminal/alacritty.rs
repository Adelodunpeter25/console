use crate::types::terminal::{
    CursorPosition, TerminalBackend, TerminalCell, TerminalCellFlags, TerminalColor,
    TerminalGridSnapshot, TerminalSize,
};
use alacritty_terminal::event::{Event, EventListener};
use alacritty_terminal::grid::Dimensions;
use alacritty_terminal::term::Config as TermConfig;
use alacritty_terminal::term::Term;
use alacritty_terminal::vte::ansi::Processor;

#[derive(Clone)]
struct DummyListener;

impl EventListener for DummyListener {
    fn send_event(&self, _event: Event) {}
}

#[derive(Clone, Copy, Debug)]
struct AlacSize {
    cols: u16,
    rows: u16,
}

impl Dimensions for AlacSize {
    fn total_lines(&self) -> usize {
        self.rows as usize
    }
    fn screen_lines(&self) -> usize {
        self.rows as usize
    }
    fn columns(&self) -> usize {
        self.cols as usize
    }
}

pub struct AlacrittyBackend {
    term: Term<DummyListener>,
    processor: Processor,
    size: TerminalSize,
}

impl TerminalBackend for AlacrittyBackend {
    fn new(size: TerminalSize) -> Self {
        let listener = DummyListener;
        let config = TermConfig::default();
        let term_size = AlacSize {
            cols: size.cols,
            rows: size.rows,
        };
        let term = Term::new(config, &term_size, listener.clone());
        let processor = Processor::new();
        Self {
            term,
            processor,
            size,
        }
    }

    fn resize(&mut self, size: TerminalSize) {
        self.size = size;
        let term_size = AlacSize {
            cols: size.cols,
            rows: size.rows,
        };
        self.term.resize(term_size);
    }

    fn advance(&mut self, data: &str) {
        self.processor.advance(&mut self.term, data.as_bytes());
    }

    fn snapshot(&self) -> TerminalGridSnapshot {
        let grid = self.term.grid();
        let display_offset = grid.display_offset();
        let screen_lines = self.size.rows as usize;
        let cols = self.size.cols as usize;

        let mut rows = Vec::with_capacity(screen_lines);
        for line_idx in 0..screen_lines {
            let line = line_idx as i32 - display_offset as i32;
            let grid_line = &grid[alacritty_terminal::index::Line(line)];
            let mut row = Vec::with_capacity(cols);
            for col_idx in 0..cols {
                let cell = &grid_line[alacritty_terminal::index::Column(col_idx)];
                let c = cell.c;
                let fg = color_to_terminal(cell.fg);
                let bg = color_to_terminal(cell.bg);
                let flags = flags_to_terminal(cell.flags);
                row.push(TerminalCell { c, fg, bg, flags });
            }
            rows.push(row);
        }

        let cursor = grid.cursor.point;
        let visible = self
            .term
            .mode()
            .contains(alacritty_terminal::term::TermMode::SHOW_CURSOR);
        let cursor_pos = CursorPosition {
            col: cursor.column.0 as u16,
            row: (cursor.line.0 + display_offset as i32) as u16,
            visible,
        };

        TerminalGridSnapshot {
            rows,
            cursor: cursor_pos,
            size: self.size,
        }
    }

    fn size(&self) -> TerminalSize {
        self.size
    }
}

fn color_to_terminal(color: alacritty_terminal::term::color::CellColor) -> Option<TerminalColor> {
    use alacritty_terminal::term::color::CellColor;
    match color {
        CellColor::Named(_) | CellColor::Indexed(_) => None,
        CellColor::Spec(rgb) => Some(TerminalColor {
            r: rgb.r,
            g: rgb.g,
            b: rgb.b,
        }),
    }
}

fn flags_to_terminal(flags: alacritty_terminal::term::cell::Flags) -> TerminalCellFlags {
    use alacritty_terminal::term::cell::Flags;
    TerminalCellFlags {
        bold: flags.contains(Flags::BOLD),
        dim: flags.contains(Flags::DIM),
        italic: flags.contains(Flags::ITALIC),
        underline: flags.contains(Flags::UNDERLINE),
        inverse: flags.contains(Flags::INVERSE),
        hidden: flags.contains(Flags::HIDDEN),
        strike: flags.contains(Flags::STRIKEOUT),
        blink: flags.contains(Flags::BLINK),
        wrapline: flags.contains(Flags::WRAPLINE),
    }
}
