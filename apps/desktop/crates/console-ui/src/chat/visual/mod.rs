//! Renderer for workspace image visuals (`read_file`). Legacy chart/metric/table
//! specs show a compact fallback.

use console_core::{VisualArtifact, VisualBlock, VisualParse, parse_visual};
use gpui::{AnyElement, App, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};

use crate::markdown::render::TranscriptSelection;
use crate::theme::Theme;

/// One card for a `VisualArtifact` (workspace image from `read_file`).
#[derive(IntoElement)]
pub struct VisualCard {
    artifact: VisualArtifact,
    #[allow(dead_code)]
    row: String,
    #[allow(dead_code)]
    selection: TranscriptSelection,
}

impl VisualCard {
    pub fn new(
        row: impl Into<String>,
        artifact: VisualArtifact,
        selection: TranscriptSelection,
    ) -> Self {
        Self {
            artifact,
            row: row.into(),
            selection,
        }
    }
}

impl RenderOnce for VisualCard {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let body = match parse_visual(&self.artifact) {
            VisualParse::Ok(spec) => {
                let mut col = div().w_full().flex().flex_col().gap(px(12.0));
                for block in spec.blocks {
                    match block {
                        VisualBlock::Image { path, alt } => {
                            col =
                                col.child(crate::chat::image::ChatImage::new(path, None).alt(alt));
                        }
                    }
                }
                col.into_any_element()
            }
            VisualParse::Unsupported | VisualParse::Invalid => {
                fallback("Visual unavailable", theme)
            }
        };
        div()
            .w_full()
            .py(px(6.0))
            .px(px(4.0))
            .flex()
            .flex_col()
            .gap(px(12.0))
            .child(body)
    }
}

fn fallback(message: &str, theme: Theme) -> AnyElement {
    div()
        .text_size(px(12.5))
        .text_color(theme.text_tertiary)
        .child(message.to_string())
        .into_any_element()
}
