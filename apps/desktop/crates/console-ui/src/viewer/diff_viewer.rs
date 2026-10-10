//! The diff viewer: the editor's diff view (line-number gutter, change bars,
//! syntax highlighting). The Diff tab, the review tab and the chip hover
//! popover all render diffs through this one component, so they look and
//! behave the same.

use console_core::utils::diff::parse_unified_diff;
use editor_ui::{DiffLine, DiffLineKind, DiffResult, DiffState, FontConfig};
use gpui::{App, AppContext, Entity, IntoElement, RenderOnce, Window, div, prelude::*, px};

use crate::theme::Theme;

/// Row height of the editor diff view, for callers that size it to its lines.
pub const DIFF_LINE_HEIGHT: f32 = 20.0;

#[derive(IntoElement)]
pub struct DiffViewer {
    view: Entity<editor_ui::DiffView>,
}

impl DiffViewer {
    pub fn new(view: Entity<editor_ui::DiffView>) -> Self {
        Self { view }
    }
}

impl RenderOnce for DiffViewer {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        div().size_full().bg(gpui::rgb(0x000000)).child(self.view)
    }
}

/// A built editor diff view plus how many lines it holds, so a caller can
/// give it an exact height (the view fills whatever parent it is put in).
#[derive(Clone)]
pub struct EditorDiff {
    pub view: Entity<editor_ui::DiffView>,
    pub line_count: usize,
}

/// Convert a raw unified diff into the editor's diff model.
pub fn editor_diff_result(raw: &str) -> DiffResult {
    let parsed = parse_unified_diff(raw);
    let mut added = 0;
    let mut removed = 0;
    let lines = parsed
        .lines
        .iter()
        .map(|line| {
            let kind = match line.kind {
                console_core::DiffLineKind::Added => {
                    added += 1;
                    DiffLineKind::Added
                }
                console_core::DiffLineKind::Removed => {
                    removed += 1;
                    DiffLineKind::Removed
                }
                console_core::DiffLineKind::Context => DiffLineKind::Context,
            };
            DiffLine {
                kind,
                text: line.text.clone(),
                old_line: line.old_no.map(|n| n as u32),
                new_line: line.new_no.map(|n| n as u32),
            }
        })
        .collect();
    DiffResult {
        lines,
        added,
        removed,
    }
}

/// Build an editor diff view for `raw` with the same settings the Diff tab
/// uses: the app's editor font, the theme's syntax preset, the file's
/// language, no wrapping.
pub fn build_editor_diff(raw: &str, path: &str, theme: &Theme, cx: &mut App) -> EditorDiff {
    let language = syntax::LanguageRegistry::for_path(std::path::Path::new(path));
    let preset = if theme.is_dark {
        syntax::ThemePreset::GitHubDark
    } else {
        syntax::ThemePreset::GitHubLight
    };
    let font = FontConfig {
        family: "JetBrains Mono".into(),
        size: px(12.0),
        line_height: px(DIFF_LINE_HEIGHT),
    };
    let result = editor_diff_result(raw);
    let line_count = result.lines.len();
    let state = cx.new(|_| {
        let mut state = DiffState::from_result(result, language);
        state.set_theme(preset);
        state.set_wrap_enabled(false);
        state.set_font(font);
        state
    });
    let view = cx.new(|_| editor_ui::DiffView::new(&state));
    EditorDiff { view, line_count }
}
