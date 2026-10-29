//! Inline icons for file and web links in rendered markdown.
//!
//! The flattener reserves a one-em gap before the first run of a file or
//! http(s) link; the renderer paints the file-type icon or favicon over it.

use console_ui::markdown::parser::{InlineRun, InlineStyle};
use console_ui::markdown::render::{IconKind, Palette, flatten};
use console_ui::markdown::selection::ICON_GAP;
use console_ui::theme::Theme;
use gpui::FontWeight;

fn run(text: &str, link: Option<&str>) -> InlineRun {
    InlineRun {
        text: text.to_owned(),
        style: InlineStyle {
            link: link.map(str::to_owned),
            ..InlineStyle::default()
        },
    }
}

fn flat(runs: &[InlineRun]) -> console_ui::markdown::render::FlatText {
    let theme = Theme::dark();
    let palette = Palette::from_theme(&theme);
    flatten(runs, &palette, FontWeight::NORMAL, palette.text)
}

#[test]
fn file_link_reserves_a_gap_and_a_file_icon() {
    let flat = flat(&[run("see ", None), run("src/lib.rs", Some("src/lib.rs"))]);
    assert_eq!(flat.icons.len(), 1);
    assert_eq!(flat.icons[0].kind, IconKind::File("src/lib.rs".into()));
    assert_eq!(&flat.text[flat.icons[0].range.clone()], ICON_GAP);
    // The link covers the label only, never the gap.
    let (range, url) = &flat.links[0];
    assert_eq!(&flat.text[range.clone()], "src/lib.rs");
    assert_eq!(url, "src/lib.rs");
}

#[test]
fn web_link_gets_a_favicon() {
    let flat = flat(&[run("docs", Some("https://docs.rs/gpui"))]);
    assert_eq!(flat.icons.len(), 1);
    assert_eq!(
        flat.icons[0].kind,
        IconKind::Favicon("https://docs.rs/gpui".into())
    );
}

#[test]
fn runs_tile_the_text_exactly() {
    let flat = flat(&[
        run("a ", None),
        run("b/c.ts", Some("b/c.ts")),
        run(" z", None),
    ]);
    let total: usize = flat.runs.iter().map(|r| r.len).sum();
    assert_eq!(total, flat.text.len());
}

#[test]
fn one_link_split_across_runs_reserves_one_gap() {
    let mut bold = run("c.ts", Some("b/c.ts"));
    bold.style.bold = true;
    let flat = flat(&[run("b/", Some("b/c.ts")), bold]);
    assert_eq!(flat.icons.len(), 1);
}

#[test]
fn plain_text_and_other_schemes_get_no_icon() {
    let flat = flat(&[run("hello", None), run("mail", Some("mailto:a@b.com"))]);
    assert!(flat.icons.is_empty());
    assert!(!flat.text.contains(ICON_GAP));
}

#[test]
fn backticked_names_without_a_directory_become_file_links() {
    use console_ui::markdown::parser::{Block, parse};

    fn collect(blocks: &[Block], links: &mut Vec<String>) {
        for block in blocks {
            match block {
                Block::Paragraph { runs } => {
                    links.extend(runs.iter().filter_map(|run| run.style.link.clone()));
                }
                Block::List { items, .. } => {
                    for item in items {
                        collect(&item.blocks, links);
                    }
                }
                _ => {}
            }
        }
    }

    let tree = parse("- `README.md`\n- `Makefile`\n- `console.log`\n- `1.2.3`\n");
    let blocks: Vec<Block> = tree.blocks.into_iter().map(|top| top.block).collect();
    let mut links = Vec::new();
    collect(&blocks, &mut links);
    assert_eq!(links, vec!["README.md".to_string(), "Makefile".to_string()]);
}
