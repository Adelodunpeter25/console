use gpui::{Hsla, rgb};

use crate::theme::Theme;

/// Ghostty-inspired dark palette for the desktop terminal.
/// Mirrors `apps/mobile/modules/console-terminal/src/terminal-theme.ts` but
/// as `Hsla` values for GPUI.
#[derive(Clone, Copy, Debug)]
pub struct TerminalTheme {
    pub background: Hsla,
    pub foreground: Hsla,
    pub cursor: Hsla,
    pub cursor_text: Hsla,
    pub selection: Hsla,
    pub black: Hsla,
    pub red: Hsla,
    pub green: Hsla,
    pub yellow: Hsla,
    pub blue: Hsla,
    pub magenta: Hsla,
    pub cyan: Hsla,
    pub white: Hsla,
    pub bright_black: Hsla,
    pub bright_red: Hsla,
    pub bright_green: Hsla,
    pub bright_yellow: Hsla,
    pub bright_blue: Hsla,
    pub bright_magenta: Hsla,
    pub bright_cyan: Hsla,
    pub bright_white: Hsla,
}

impl TerminalTheme {
    pub fn dark() -> Self {
        Self {
            background: rgb(0x0a0a0b).into(),
            foreground: rgb(0xe4e4e7).into(),
            cursor: rgb(0x009fff).into(),
            cursor_text: rgb(0x0a0a0b).into(),
            selection: hsla(240.0 / 360.0, 0.06, 0.20, 0.6),
            black: rgb(0x1a1a1e).into(),
            red: rgb(0xef4444).into(),
            green: rgb(0x22c55e).into(),
            yellow: rgb(0xeab308).into(),
            blue: rgb(0x3b82f6).into(),
            magenta: rgb(0xa855f7).into(),
            cyan: rgb(0x06b6d4).into(),
            white: rgb(0xe4e4e7).into(),
            bright_black: rgb(0x52525b).into(),
            bright_red: rgb(0xf87171).into(),
            bright_green: rgb(0x4ade80).into(),
            bright_yellow: rgb(0xfacc15).into(),
            bright_blue: rgb(0x60a5fa).into(),
            bright_magenta: rgb(0xc084fc).into(),
            bright_cyan: rgb(0x22d3ee).into(),
            bright_white: rgb(0xf4f4f5).into(),
        }
    }

    /// Map from the app theme's terminal surface when available.
    pub fn from_app_theme(theme: &Theme) -> Self {
        let mut t = Self::dark();
        // Keep the terminal background in sync with the app's terminal token
        t.background = theme.terminal;
        t
    }
}

fn hsla(h: f32, s: f32, l: f32, a: f32) -> Hsla {
    gpui::hsla(h, s, l, a)
}
