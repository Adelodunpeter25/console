//! The diff viewer: the editor's diff view (line-number gutter, change bars,
//! syntax highlighting). The Diff tab, the review tab, the chip hover popover
//! and the tool-call edit rows all render diffs through this one component,
//! so they look and behave the same.

use console_core::utils::diff::parse_unified_diff;
use editor_ui::{DiffLine, DiffLineKind, DiffResult, DiffState, FontConfig};
use gpui::{
    App, AppContext, Entity, FontWeight, IntoElement, RenderOnce, Window, div, prelude::*, px,
};

use crate::markdown::render::MONO_FAMILY;
use crate::primitives::{base_name, file_type_icon};
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
    editor_diff_from_parsed(&parse_unified_diff(raw))
}

/// Convert an already-parsed diff into the editor's diff model.
pub fn editor_diff_from_parsed(parsed: &console_core::DiffResult) -> DiffResult {
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

/// Build an editor diff view for a raw unified diff with the same settings
/// the Diff tab uses: the app's editor font, the theme's syntax preset, the
/// file's language, no wrapping.
pub fn build_editor_diff(raw: &str, path: &str, theme: &Theme, cx: &mut App) -> EditorDiff {
    build_from_result(editor_diff_result(raw), path, theme, cx)
}

/// [`build_editor_diff`] for a diff that is already parsed.
pub fn build_editor_diff_from_parsed(
    parsed: &console_core::DiffResult,
    path: &str,
    theme: &Theme,
    cx: &mut App,
) -> EditorDiff {
    build_from_result(editor_diff_from_parsed(parsed), path, theme, cx)
}

fn build_from_result(result: DiffResult, path: &str, theme: &Theme, cx: &mut App) -> EditorDiff {
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

/// One file's diff as a titled block: file icon, name and `+N -M` above the
/// editor diff view, which grows to its lines up to `max_height` and scrolls
/// beyond that. An empty `path` (a diff with no known file) drops the title.
pub fn file_diff_block(
    path: &str,
    added: usize,
    removed: usize,
    diff: EditorDiff,
    max_height: f32,
    theme: &Theme,
) -> impl IntoElement + use<> {
    let height = (diff.line_count as f32 * DIFF_LINE_HEIGHT + 8.0).min(max_height);
    let has_lines = diff.line_count > 0;
    let title = (!path.is_empty()).then(|| {
        div()
            .flex()
            .items_center()
            .justify_between()
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .min_w_0()
                    .child(file_type_icon(path, 13.0))
                    .child(
                        div()
                            .font_family(MONO_FAMILY)
                            .text_size(px(10.5))
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(theme.text_secondary)
                            .truncate()
                            .child(base_name(path).to_owned()),
                    ),
            )
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(8.0))
                    .text_size(px(10.0))
                    .font_weight(FontWeight::SEMIBOLD)
                    .child(div().text_color(theme.success).child(format!("+{added}")))
                    .child(div().text_color(theme.danger).child(format!("-{removed}"))),
            )
    });
    div()
        .flex()
        .flex_col()
        .gap(px(4.0))
        .children(title)
        .when(has_lines, |element| {
            element.child(
                div()
                    .w_full()
                    .h(px(height))
                    .rounded(px(5.0))
                    .overflow_hidden()
                    .child(DiffViewer::new(diff.view)),
            )
        })
}
