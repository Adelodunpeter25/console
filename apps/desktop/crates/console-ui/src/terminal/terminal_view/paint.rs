//! Canvas painting for the terminal grid: row paint cache, run shaping,
//! cursor overlay.
use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

use gpui::{App, Pixels, SharedString, Window, px};

use crate::terminal::theme::TerminalTheme;

use super::TerminalCellPos;

/// Drop-in terminal pane. Owns its `AlacrittyBackend` + WS `TerminalHandle`,
/// feeds server output → grid → snapshot, and forwards keyboard → `input`.
///
/// One terminal row, pre-shaped. Repainting it costs a few quads plus cached
/// glyph paint instead of a full text-shaping pass — this is what keeps
/// constantly-redrawing TUIs (btop, editors) cheap: unchanged rows never
/// re-shape, no matter how often the frame repaints.
pub(super) struct CachedRowPaint {
    hash: u64,
    bg_runs: Vec<(usize, usize, gpui::Hsla)>,
    text_runs: Vec<CachedTextRun>,
}

struct CachedTextRun {
    start_col: usize,
    shaped: gpui::ShapedLine,
}

fn hash_cell_runs(runs: &[CellRun]) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    for run in runs {
        run.text.hash(&mut h);
        run.count.hash(&mut h);
        for f in [
            run.fg.h, run.fg.s, run.fg.l, run.fg.a, run.bg.h, run.bg.s, run.bg.l, run.bg.a,
        ] {
            f.to_bits().hash(&mut h);
        }
        run.bold.hash(&mut h);
        run.italic.hash(&mut h);
        run.underline.hash(&mut h);
    }
    h.finish()
}

struct CellRun {
    start_col: usize,
    count: usize,
    fg: gpui::Hsla,
    bg: gpui::Hsla,
    bold: bool,
    italic: bool,
    underline: bool,
    text: String,
}

fn color_to_hsla(c: console_core::types::terminal::TerminalColor) -> gpui::Hsla {
    let r = c.r as f32 / 255.0;
    let g = c.g as f32 / 255.0;
    let b = c.b as f32 / 255.0;
    gpui::Rgba { r, g, b, a: 1.0 }.into()
}

pub(super) fn render_canvas_grid(
    bounds: gpui::Bounds<gpui::Pixels>,
    cols: u16,
    rows: u16,
    cell_w: gpui::Pixels,
    cell_h: gpui::Pixels,
    snapshot: Option<&console_core::types::terminal::TerminalGridSnapshot>,
    selection_range: Option<(TerminalCellPos, TerminalCellPos)>,
    render_cursor: bool,
    paint_cache: &Rc<RefCell<HashMap<u16, CachedRowPaint>>>,
    theme: TerminalTheme,
    window: &mut Window,
    cx: &mut App,
) {
    let pad_x = px(8.0);
    let pad_y = px(8.0);
    let origin = bounds.origin + gpui::point(pad_x, pad_y);

    window.paint_quad(gpui::fill(bounds, theme.background));

    let Some(snap) = snapshot else {
        return;
    };

    let cursor = if render_cursor {
        snap.cursor
    } else {
        console_core::types::terminal::CursorPosition {
            col: 0,
            row: 0,
            visible: false,
        }
    };

    // Cache validity is content-addressed: every row rebuilds its runs and
    // hashes them, and a hash match repaints the stored shaped line without
    // re-shaping. This deliberately ignores termy's damage spans — damage is
    // consumed per snapshot_full, but GPUI can drop a frame's snapshot when
    // the watch loop takes two snapshots between paints (output bursts). A
    // damage-gated fast path then repainted stale rows ("disappearing text");
    // comparing hashes against the actual grid cannot go stale. Run-building
    // + hashing a row costs microseconds; shaping is what we're avoiding.
    for (row_idx, row) in snap.rows.iter().enumerate() {
        if row_idx as u16 >= rows {
            break;
        }
        let y = origin.y + cell_h * row_idx as f32;
        let row_u16 = row_idx as u16;

        let selection_here = match selection_range {
            Some((start, end)) => row_u16 >= start.row && row_u16 <= end.row,
            None => false,
        };
        // Cursor no longer poisons the cache: rows shape without cursor
        // styling and the cursor paints as a one-cell overlay below, so
        // full-screen TUIs that move the cursor every frame still hit the
        // cache. Selection still bypasses (that state isn't in the hash).
        let cacheable = !selection_here;
        let cursor_here =
            cursor.visible && cursor.row == row_u16 && (cursor.col as usize) < row.len();

        // Collect the links overlapping this row once per row instead of
        // scanning the full link list for every cell (O(cells × links) →
        // O(cells × row_links)).
        let row_links: Vec<&console_core::types::terminal::TerminalLink> = snap
            .links
            .iter()
            .filter(|l| row_idx as u16 >= l.start_row && row_idx as u16 <= l.end_row)
            .collect();

        let mut runs: Vec<CellRun> = Vec::new();

        for (col_idx, cell) in row.iter().enumerate() {
            if col_idx as u16 >= cols {
                break;
            }

            let mut fg = cell.fg.map(color_to_hsla).unwrap_or(theme.foreground);
            let mut bg = cell.bg.map(color_to_hsla).unwrap_or(theme.background);

            let is_selected = if let Some((start, end)) = selection_range {
                let r = row_idx as u16;
                let c = col_idx as u16;
                if r > start.row && r < end.row {
                    true
                } else if r == start.row && r == end.row {
                    c >= start.col && c <= end.col
                } else if r == start.row {
                    c >= start.col
                } else if r == end.row {
                    c <= end.col
                } else {
                    false
                }
            } else {
                false
            };

            if is_selected {
                bg = theme.selection;
            }

            if cell.flags.inverse {
                std::mem::swap(&mut fg, &mut bg);
            }
            if cell.flags.dim {
                fg.a *= 0.6;
            }
            if cell.flags.hidden {
                fg = bg;
            }

            // NOTE: no cursor override here — the cursor paints as an overlay
            // after the row so cursor motion never invalidates the row cache.

            let c = if cell.flags.wide_char_spacer {
                ' '
            } else {
                cell.c
            };

            let mut is_link = false;
            for l in &row_links {
                let r = row_idx as u16;
                let c = col_idx as u16;
                let in_start = r > l.start_row || c >= l.start_col;
                let in_end = r < l.end_row || c <= l.end_col;
                if in_start && in_end {
                    is_link = true;
                    break;
                }
            }

            if is_link {
                fg = gpui::blue();
            }

            let cell_underline = cell.flags.underline || is_link;

            let can_merge = if let Some(last) = runs.last() {
                last.fg == fg
                    && last.bg == bg
                    && last.bold == cell.flags.bold
                    && last.italic == cell.flags.italic
                    && last.underline == cell_underline
            } else {
                false
            };

            if can_merge {
                let last = runs.last_mut().unwrap();
                last.count += 1;
                last.text.push(c);
            } else {
                runs.push(CellRun {
                    start_col: col_idx,
                    count: 1,
                    fg,
                    bg,
                    bold: cell.flags.bold,
                    italic: cell.flags.italic,
                    underline: cell_underline,
                    text: c.to_string(),
                });
            }
        }

        // Shape cache: unchanged rows repaint stored quads + shaped lines
        // without shaping. Selection rows bypass (that state isn't in
        // the hash); cursor rows stay cached via the overlay below.
        let row_hash: Option<u64> = if cacheable {
            Some(hash_cell_runs(&runs))
        } else {
            None
        };
        if let Some(hash) = row_hash {
            let hit = paint_cache
                .borrow()
                .get(&row_u16)
                .map(|entry| entry.hash == hash)
                .unwrap_or(false);
            if hit {
                if let Some(entry) = paint_cache.borrow().get(&row_u16) {
                    paint_cached_entry(origin, cell_w, cell_h, y, entry, theme.background, window, cx);
                }
                paint_cursor_overlay(
                    origin, cell_w, cell_h, y, snap, row_u16, cursor_here, cursor, theme,
                    window, cx,
                );
                continue;
            }
        }

        let mut cached_bg: Vec<(usize, usize, gpui::Hsla)> = Vec::new();
        let mut cached_text: Vec<CachedTextRun> = Vec::new();

        for run in runs {
            let run_x = origin.x + cell_w * run.start_col as f32;
            let run_w = cell_w * run.count as f32;

            if run.bg != theme.background {
                let bg_quad = gpui::Bounds {
                    origin: gpui::point(run_x, y),
                    size: gpui::size(run_w, cell_h),
                };
                window.paint_quad(gpui::fill(bg_quad, run.bg));
                if cacheable {
                    cached_bg.push((run.start_col, run.count, run.bg));
                }
            }

            if !run.text.trim().is_empty() || run.underline {
                let font_weight = if run.bold {
                    gpui::FontWeight::BOLD
                } else {
                    gpui::FontWeight::NORMAL
                };
                let font_style = if run.italic {
                    gpui::FontStyle::Italic
                } else {
                    gpui::FontStyle::Normal
                };

                let text_run = gpui::TextRun {
                    len: run.text.len(),
                    font: gpui::Font {
                        family: SharedString::from(crate::markdown::render::MONO_FAMILY),
                        weight: font_weight,
                        style: font_style,
                        features: Default::default(),
                        fallbacks: None,
                    },
                    color: run.fg,
                    background_color: None,
                    underline: if run.underline {
                        Some(gpui::UnderlineStyle {
                            color: Some(run.fg),
                            thickness: px(1.0),
                            wavy: false,
                        })
                    } else {
                        None
                    },
                    strikethrough: None,
                };

                let shaped = window.text_system().shape_line(
                    SharedString::from(run.text),
                    px(12.0),
                    &[text_run],
                    None,
                );
                if cacheable {
                    cached_text.push(CachedTextRun {
                        start_col: run.start_col,
                        shaped: shaped.clone(),
                    });
                }
                let _ = shaped.paint(
                    gpui::point(run_x, y),
                    cell_h,
                    gpui::TextAlign::Left,
                    None,
                    window,
                    cx,
                );
            }
        }

        if cacheable {
            paint_cache.borrow_mut().insert(
                row_u16,
                CachedRowPaint {
                    hash: row_hash.unwrap_or(0),
                    bg_runs: cached_bg,
                    text_runs: cached_text,
                },
            );
        }
        paint_cursor_overlay(
            origin, cell_w, cell_h, y, snap, row_u16, cursor_here, cursor, theme,
            window, cx,
        );
    }
}

/// Repaint a cache-hit row without shaping.
fn paint_cached_entry(
    origin: gpui::Point<Pixels>,
    cell_w: Pixels,
    cell_h: Pixels,
    y: Pixels,
    entry: &CachedRowPaint,
    background: gpui::Hsla,
    window: &mut Window,
    cx: &mut App,
) {
    for (start_col, count, bg) in &entry.bg_runs {
        if *bg != background {
            let bg_quad = gpui::Bounds {
                origin: gpui::point(origin.x + cell_w * *start_col as f32, y),
                size: gpui::size(cell_w * *count as f32, cell_h),
            };
            window.paint_quad(gpui::fill(bg_quad, *bg));
        }
    }
    for text in &entry.text_runs {
        let _ = text.shaped.paint(
            gpui::point(origin.x + cell_w * text.start_col as f32, y),
            cell_h,
            gpui::TextAlign::Left,
            None,
            window,
            cx,
        );
    }
}

/// Paint the cursor as a one-cell overlay so cursor motion never invalidates
/// the row paint cache. Shapes only the cursor cell per frame instead of the
/// whole row.
fn paint_cursor_overlay(
    origin: gpui::Point<Pixels>,
    cell_w: Pixels,
    cell_h: Pixels,
    y: Pixels,
    snap: &console_core::types::terminal::TerminalGridSnapshot,
    row: u16,
    cursor_here: bool,
    cursor: console_core::types::terminal::CursorPosition,
    theme: TerminalTheme,
    window: &mut Window,
    cx: &mut App,
) {
    if !cursor_here {
        return;
    }
    let Some(cells) = snap.rows.get(row as usize) else {
        return;
    };
    let Some(cell) = cells.get(cursor.col as usize) else {
        return;
    };
    let ch = if cell.flags.wide_char_spacer {
        ' '
    } else {
        cell.c
    };
    let cursor_x = origin.x + cell_w * cursor.col as f32;
    window.paint_quad(gpui::fill(
        gpui::Bounds {
            origin: gpui::point(cursor_x, y),
            size: gpui::size(cell_w, cell_h),
        },
        theme.cursor,
    ));
    // Shape just the cursor cell with the cursor colors. An empty/blank cell
    // still needs the bg quad above; skip shaping whitespace.
    if ch == ' ' && !cell.flags.underline {
        return;
    }
    let text_run = gpui::TextRun {
        len: ch.len_utf8(),
        font: gpui::Font {
            family: SharedString::from(crate::markdown::render::MONO_FAMILY),
            weight: if cell.flags.bold {
                gpui::FontWeight::BOLD
            } else {
                gpui::FontWeight::NORMAL
            },
            style: if cell.flags.italic {
                gpui::FontStyle::Italic
            } else {
                gpui::FontStyle::Normal
            },
            features: Default::default(),
            fallbacks: None,
        },
        color: theme.cursor_text,
        background_color: None,
        underline: None,
        strikethrough: None,
    };
    let shaped = window.text_system().shape_line(
        SharedString::from(ch.to_string()),
        px(12.0),
        &[text_run],
        None,
    );
    let _ = shaped.paint(
        gpui::point(cursor_x, y),
        cell_h,
        gpui::TextAlign::Left,
        None,
        window,
        cx,
    );
}
