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

fn color_to_terminal(color: alacritty_terminal::vte::ansi::Color) -> Option<TerminalColor> {
    use alacritty_terminal::vte::ansi::Color;
    match color {
        Color::Named(named) => named_color_to_terminal(named),
        Color::Indexed(idx) => Some(indexed_color_to_terminal(idx)),
        Color::Spec(rgb) => Some(TerminalColor {
            r: rgb.r,
            g: rgb.g,
            b: rgb.b,
        }),
    }
}

fn named_color_to_terminal(named: alacritty_terminal::vte::ansi::NamedColor) -> Option<TerminalColor> {
    use alacritty_terminal::vte::ansi::NamedColor;
    match named {
        NamedColor::Black => Some(TerminalColor { r: 0x1a, g: 0x1a, b: 0x1e }),
        NamedColor::Red => Some(TerminalColor { r: 0xef, g: 0x44, b: 0x44 }),
        NamedColor::Green => Some(TerminalColor { r: 0x22, g: 0xc5, b: 0x5e }),
        NamedColor::Yellow => Some(TerminalColor { r: 0xea, g: 0xb3, b: 0x08 }),
        NamedColor::Blue => Some(TerminalColor { r: 0x3b, g: 0x82, b: 0xf6 }),
        NamedColor::Magenta => Some(TerminalColor { r: 0xa8, g: 0x55, b: 0xf7 }),
        NamedColor::Cyan => Some(TerminalColor { r: 0x06, g: 0xb6, b: 0xd4 }),
        NamedColor::White => Some(TerminalColor { r: 0xe4, g: 0xe4, b: 0xe7 }),
        NamedColor::BrightBlack => Some(TerminalColor { r: 0x52, g: 0x52, b: 0x5b }),
        NamedColor::BrightRed => Some(TerminalColor { r: 0xf8, g: 0x71, b: 0x71 }),
        NamedColor::BrightGreen => Some(TerminalColor { r: 0x4a, g: 0xde, b: 0x80 }),
        NamedColor::BrightYellow => Some(TerminalColor { r: 0xfa, g: 0xcc, b: 0x15 }),
        NamedColor::BrightBlue => Some(TerminalColor { r: 0x60, g: 0xa5, b: 0xfa }),
        NamedColor::BrightMagenta => Some(TerminalColor { r: 0xc0, g: 0x84, b: 0xfc }),
        NamedColor::BrightCyan => Some(TerminalColor { r: 0x22, g: 0xd3, b: 0xee }),
        NamedColor::BrightWhite => Some(TerminalColor { r: 0xf4, g: 0xf4, b: 0xf5 }),
        _ => None,
    }
}

fn indexed_color_to_terminal(idx: u8) -> TerminalColor {
    match idx {
        0 => TerminalColor { r: 0x1a, g: 0x1a, b: 0x1e },
        1 => TerminalColor { r: 0xef, g: 0x44, b: 0x44 },
        2 => TerminalColor { r: 0x22, g: 0xc5, b: 0x5e },
        3 => TerminalColor { r: 0xea, g: 0xb3, b: 0x08 },
        4 => TerminalColor { r: 0x3b, g: 0x82, b: 0xf6 },
        5 => TerminalColor { r: 0xa8, g: 0x55, b: 0xf7 },
        6 => TerminalColor { r: 0x06, g: 0xb6, b: 0xd4 },
        7 => TerminalColor { r: 0xe4, g: 0xe4, b: 0xe7 },
        8 => TerminalColor { r: 0x52, g: 0x52, b: 0x5b },
        9 => TerminalColor { r: 0xf8, g: 0x71, b: 0x71 },
        10 => TerminalColor { r: 0x4a, g: 0xde, b: 0x80 },
        11 => TerminalColor { r: 0xfa, g: 0xcc, b: 0x15 },
        12 => TerminalColor { r: 0x60, g: 0xa5, b: 0xfa },
        13 => TerminalColor { r: 0xc0, g: 0x84, b: 0xfc },
        14 => TerminalColor { r: 0x22, g: 0xd3, b: 0xee },
        15 => TerminalColor { r: 0xf4, g: 0xf4, b: 0xf5 },
        16..=231 => {
            let i = idx - 16;
            let r_step = (i / 36) % 6;
            let g_step = (i / 6) % 6;
            let b_step = i % 6;
            let to_val = |v: u8| if v == 0 { 0 } else { 55 + v * 40 };
            TerminalColor {
                r: to_val(r_step),
                g: to_val(g_step),
                b: to_val(b_step),
            }
        }
        232..=255 => {
            let v = 8 + (idx - 232) * 10;
            TerminalColor { r: v, g: v, b: v }
        }
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
        blink: false,
        wrapline: flags.contains(Flags::WRAPLINE),
        wide_char: flags.contains(Flags::WIDE_CHAR),
        wide_char_spacer: flags.contains(Flags::WIDE_CHAR_SPACER)
            || flags.contains(Flags::LEADING_WIDE_CHAR_SPACER),
    }
}
