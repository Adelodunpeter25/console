//! LaTeX in markdown: parsed into math runs/blocks and typeset by RaTeX.

use console_ui::markdown::math::{MathMode, gap_for_width, render};
use console_ui::markdown::parser::{Block, parse};
use console_ui::markdown::render::{IconKind, Palette, flatten_sized};
use console_ui::theme::Theme;
use gpui::FontWeight;

const WHITE: [u8; 3] = [255, 255, 255];

#[test]
fn typesets_a_formula_to_a_sized_image() {
    let formula = render(r"\frac{-b \pm \sqrt{b^2-4ac}}{2a}", MathMode::Display, 20.0, WHITE)
        .expect("a valid formula typesets");
    assert!(formula.width > 0.0 && formula.height > 0.0);
}

#[test]
fn display_math_is_taller_than_the_same_formula_inline() {
    let latex = r"\sum_{i=0}^{n} \frac{1}{i}";
    let display = render(latex, MathMode::Display, 16.0, WHITE).unwrap();
    let inline = render(latex, MathMode::Inline, 16.0, WHITE).unwrap();
    assert!(display.height > inline.height);
}

#[test]
fn unparseable_latex_degrades_instead_of_panicking() {
    // A formula still streaming in is the common case: an unclosed group.
    assert!(render(r"\frac{1", MathMode::Inline, 16.0, WHITE).is_none());
    assert!(render("   ", MathMode::Inline, 16.0, WHITE).is_none());
}

#[test]
fn gap_never_undershoots_the_formula() {
    for width in [3.0_f32, 9.0, 14.0, 21.5, 60.0] {
        let gap = gap_for_width(width, 14.0);
        let reserved: f32 = gap.chars().map(|c| if c == '\u{2003}' { 14.0 } else { 7.0 }).sum();
        assert!(reserved >= width, "{width} reserved {reserved}");
        assert!(reserved - width <= 7.0, "{width} reserved {reserved}: too much slack");
    }
}

#[test]
fn inline_math_is_a_math_run() {
    let tree = parse("Euler: $e^{i\\pi}+1=0$ done");
    let Block::Paragraph { runs } = &tree.blocks[0].block else {
        panic!("expected a paragraph");
    };
    let math: Vec<_> = runs.iter().filter(|run| run.style.math).collect();
    assert_eq!(math.len(), 1);
    assert_eq!(math[0].text, "e^{i\\pi}+1=0");
}

#[test]
fn display_math_is_its_own_block() {
    let tree = parse("before\n\n$$\n\\int_0^1 x\\,dx\n$$\n\nafter");
    let kinds: Vec<_> = tree.blocks.iter().map(|b| &b.block).collect();
    assert!(
        kinds.iter().any(|b| matches!(b, Block::Math { latex } if latex.contains("\\int"))),
        "{kinds:?}"
    );
}

#[test]
fn a_typeset_inline_formula_reserves_a_gap_and_a_slot() {
    let theme = Theme::dark();
    let palette = Palette::from_theme(&theme);
    let tree = parse("area $\\pi r^2$ ok");
    let Block::Paragraph { runs } = &tree.blocks[0].block else {
        panic!("expected a paragraph");
    };
    let flat = flatten_sized(runs, &palette, FontWeight::NORMAL, palette.text, 12.5);
    let slot = flat
        .icons
        .iter()
        .find(|slot| matches!(slot.kind, IconKind::Math { .. }))
        .expect("a math slot");
    // The reserved text is spaces only: the formula source never leaks into the layout.
    assert!(flat.text[slot.range.clone()].chars().all(|c| c == '\u{2003}' || c == '\u{2002}'));
    assert!(!flat.text.contains("\\pi"));
}

#[test]
fn an_unparseable_inline_formula_falls_back_to_readable_text() {
    let theme = Theme::dark();
    let palette = Palette::from_theme(&theme);
    // Valid inline math to the markdown parser, but an undefined command to RaTeX.
    let tree = parse("odd $x \\to \\notacommand$ typed");
    let Block::Paragraph { runs } = &tree.blocks[0].block else {
        panic!("expected a paragraph");
    };
    let flat = flatten_sized(runs, &palette, FontWeight::NORMAL, palette.text, 12.5);
    assert!(runs.iter().any(|run| run.style.math), "parsed as math: {runs:?}");
    assert!(flat.icons.iter().all(|slot| !matches!(slot.kind, IconKind::Math { .. })));
    assert!(flat.text.contains('→'), "{}", flat.text);
}
