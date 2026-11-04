//! The `$`/`/` prefix of a chat/skill chip is payload, not display: its
//! text run must paint in the pill background color so only the
//! slug/name shows.
use console_shared::composer::mentions::ComposerMention;
use console_ui::input::{SearchPaint, input_text_runs};
use gpui::{Hsla, hsla};

fn bg() -> Hsla {
    hsla(0.0, 0.0, 0.0, 0.0)
}

fn accent() -> Hsla {
    hsla(0.6, 0.5, 0.6, 1.0)
}

fn runs_for(mentions: &[ComposerMention], content_len: usize) -> Vec<gpui::TextRun> {
    input_text_runs(
        content_len,
        gpui::TextRun {
            len: content_len,
            font: gpui::Font::default(),
            color: accent(),
            background_color: None,
            underline: None,
            strikethrough: None,
        },
        None,
        None,
        bg(),
        &[],
        |_| accent(),
        SearchPaint::none(),
        mentions,
        accent(),
        bg(),
    )
}

#[test]
fn test_chat_chip_prefix_run_is_painted_in_pill_background() {
    let mentions = vec![ComposerMention::chat(4..18, "fix-login-bug")];
    let runs = runs_for(&mentions, 18);
    // Runs cover: "use " | "$" | "fix-login-bug"
    assert!(runs.len() >= 3, "unexpected runs: {runs:?}");
    assert_eq!(runs[1].len, 1, "prefix run must be exactly one char");
    assert_eq!(
        runs[1].color, bg(),
        "the $ must paint in the pill bg, got {:?}",
        runs[1].color
    );
    assert_eq!(runs[2].color, accent());
}

#[test]
fn test_skill_chip_prefix_run_is_painted_in_pill_background() {
    let mentions = vec![ComposerMention::skill(0..5, "plan")];
    let runs = runs_for(&mentions, 5);
    assert_eq!(runs[0].len, 1);
    assert_eq!(runs[0].color, bg());
    assert_eq!(runs[1].color, accent());
}

#[test]
fn test_file_chip_prefix_is_not_hidden() {
    let mentions = vec![ComposerMention {
        range: 0..4,
        path: "/a/b.rs".into(),
        label: "b.rs".into(),
    }];
    let runs = runs_for(&mentions, 4);
    assert_eq!(runs.len(), 1);
    assert_eq!(runs[0].color, accent());
}
