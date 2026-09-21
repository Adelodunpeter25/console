use std::cell::RefCell;
use std::collections::HashSet;
use std::path::Path;
use std::rc::Rc;
use std::sync::Arc;

use super::markdown_helpers::assistant_ctx;
pub(crate) use super::markdown_helpers::render_selectable_markdown;
use crate::common::{attachment_image, copy_button};
use crate::markdown::render::{
    LinkHandler, MarkdownView, Palette, TranscriptSelection,
};
use crate::theme::Theme;
use crate::utils::format_message_time;

use super::ThinkingBlock;
use base64::Engine as _;
use console_core::{AssistantContentPart, ImageAttachment};
use gpui::{
    App, ElementId, IntoElement, ParentElement, RenderOnce, Styled, Window, div, img,
    prelude::*, px,
};

/// A segment of a user message — either a plain-text run or an inline file pill.
enum MessageSegment {
    Text(String),
    FilePill { path: String, label: String },
}

/// Split `content` into alternating text/pill segments.
///
/// The composer inserts mentions as bare filenames (no `@`) surrounded by
/// whitespace padding, e.g. `"     .gitignore  "`. We find each label as a
/// whole-word token (preceded and followed by whitespace or string boundaries)
/// and absorb the surrounding whitespace into the pill so no stray gaps appear.
fn split_message_segments(content: &str, context_files: &[String]) -> Vec<MessageSegment> {
    // Build (label, path) pairs.
    let pills: Vec<(String, String)> = context_files
        .iter()
        .map(|path| {
            let label = Path::new(path)
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or(path)
                .to_string();
            (label, path.clone())
        })
        .collect();

    let mut segments: Vec<MessageSegment> = Vec::new();
    let mut remaining = content;

    'outer: while !remaining.is_empty() {
        // Find the earliest label occurrence among all pills.
        let mut earliest: Option<(usize, usize, usize)> = None; // (match_start, match_end, pill_idx)
        for (pill_idx, (label, _)) in pills.iter().enumerate() {
            if let Some(pos) = remaining.find(label.as_str()) {
                // Must be surrounded by whitespace or string boundaries.
                let before_ok = pos == 0
                    || remaining[..pos]
                        .chars()
                        .next_back()
                        .is_some_and(|c| c.is_whitespace());
                let after_pos = pos + label.len();
                let after_ok = after_pos >= remaining.len()
                    || remaining[after_pos..]
                        .chars()
                        .next()
                        .is_some_and(|c| c.is_whitespace());

                if before_ok && after_ok {
                    // Absorb surrounding whitespace so no padding gaps remain.
                    let match_start = remaining[..pos]
                        .rfind(|c: char| !c.is_whitespace())
                        .map(|i| i + remaining[i..].chars().next().map(|c| c.len_utf8()).unwrap_or(1))
                        .unwrap_or(0);
                    let match_end = after_pos
                        + remaining[after_pos..]
                            .find(|c: char| !c.is_whitespace())
                            .unwrap_or(remaining[after_pos..].len());

                    if earliest.is_none() || match_start < earliest.unwrap().0 {
                        earliest = Some((match_start, match_end, pill_idx));
                    }
                }
            }
        }

        match earliest {
            None => {
                if !remaining.is_empty() {
                    segments.push(MessageSegment::Text(remaining.to_string()));
                }
                break 'outer;
            }
            Some((start, end, pill_idx)) => {
                if start > 0 {
                    segments.push(MessageSegment::Text(remaining[..start].to_string()));
                }
                let (label, path) = pills[pill_idx].clone();
                segments.push(MessageSegment::FilePill { path, label });
                remaining = &remaining[end..];
            }
        }
    }

    segments
}

/// Invoked with the decoded image when the user clicks an image in a message,
/// opening the app's image preview modal.
type PreviewImageHandler = Rc<dyn Fn(Arc<gpui::Image>, &mut Window, &mut App) + 'static>;

fn selection_row_for_part(row: &str, part_index: usize) -> String {
    format!("{row}-part-{part_index}")
}

// `render_selectable_markdown` lives in `markdown_helpers` (A+B); kept
// re-exported here so existing `crate::chat::message_bubble::…` imports keep working.

// ─── User message ─────────────────────────────────────────────────────────────

#[derive(IntoElement)]
pub struct UserMessageBubble {
    pub content: String,
    pub attachments: Vec<ImageAttachment>,
    pub context_files: Vec<String>,
    pub created_at: Option<i64>,
    selection: Option<TranscriptSelection>,
    selection_row: String,
    on_preview_image: Option<PreviewImageHandler>,
}

impl UserMessageBubble {
    pub fn new(content: impl Into<String>) -> Self {
        Self {
            content: content.into(),
            attachments: Vec::new(),
            context_files: Vec::new(),
            created_at: None,
            selection: None,
            selection_row: "user-message".to_owned(),
            on_preview_image: None,
        }
    }

    pub fn selection(mut self, selection: TranscriptSelection, row: impl Into<String>) -> Self {
        self.selection = Some(selection);
        self.selection_row = row.into();
        self
    }

    pub fn attachments(mut self, attachments: Vec<ImageAttachment>) -> Self {
        self.attachments = attachments;
        self
    }

    pub fn context_files(mut self, files: Vec<String>) -> Self {
        self.context_files = files;
        self
    }

    pub fn created_at(mut self, created_at: Option<i64>) -> Self {
        self.created_at = created_at;
        self
    }

    pub fn on_preview_image(mut self, handler: PreviewImageHandler) -> Self {
        self.on_preview_image = Some(handler);
        self
    }
}

impl RenderOnce for UserMessageBubble {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let group_name = format!("user-message-{}", self.selection_row);
        let copy_content = self.content.clone();
        let timestamp = format_message_time(self.created_at);

        let preview_handler = self.on_preview_image.clone();
        let context_files = self.context_files;

        // Build inline segments: text runs interleaved with file pills.
        // When there are no context files the content renders as-is.
        let segments = if context_files.is_empty() {
            vec![MessageSegment::Text(self.content.clone())]
        } else {
            split_message_segments(&self.content, &context_files)
        };
        let has_content = !self.content.is_empty() || !context_files.is_empty();

        div()
            .w_full()
            .flex()
            .flex_col()
            .items_end()
            .gap(px(3.0))
            .group(group_name.clone())
            // Message: image(s) pinned to the top, user bubble card below.
            .child(
                div()
                    .max_w(px(540.0))
                    .flex()
                    .flex_col()
                    .items_end()
                    .gap(px(8.0))
                    .when(!self.attachments.is_empty(), |element| {
                        element.child(
                            div()
                                .flex()
                                .flex_row()
                                .flex_wrap()
                                .justify_end()
                                .gap(px(8.0))
                                .children(self.attachments.iter().cloned().enumerate().filter_map(
                                    |(index, attachment)| {
                                        // Decode to bytes so gpui actually renders it (a
                                        // `data:` URI string would be fetched as a URL).
                                        let image = attachment_image(&attachment)?;
                                        let on_preview = preview_handler.clone();
                                        Some(
                                            div()
                                                .id(ElementId::Name(
                                                    format!("user-attachment-{index}").into(),
                                                ))
                                                .size(px(80.0))
                                                .rounded(px(9.0))
                                                .overflow_hidden()
                                                .border_1()
                                                .border_color(theme.user_bubble_border)
                                                .bg(gpui::rgb(0x000000))
                                                .cursor_default()
                                                .when_some(on_preview, |tile, on_preview| {
                                                    let image = image.clone();
                                                    tile.on_click(move |_, window, cx| {
                                                        (on_preview)(image.clone(), window, cx);
                                                        cx.stop_propagation();
                                                    })
                                                })
                                                .child(img(image).size_full()),
                                        )
                                    },
                                )),
                        )
                    })
                    .when(has_content, |element| {
                        element.child(
                            div()
                                .max_w(px(540.0))
                                .px(px(14.0))
                                .py(px(9.0))
                                .rounded(px(12.0))
                                .bg(theme.user_bubble)
                                .border_1()
                                .border_color(theme.user_bubble_border)
                                .child(
                                    // Inline flow: text runs + file pills in a wrapping row.
                                    div()
                                        .flex()
                                        .flex_wrap()
                                        .items_baseline()
                                        .gap_x(px(4.0))
                                        .gap_y(px(4.0))
                                        .text_size(px(14.0))
                                        .line_height(px(20.0))
                                        .text_color(theme.text)
                                        .children(segments.into_iter().enumerate().map(
                                            |(seg_idx, segment)| match segment {
                                                MessageSegment::Text(text) => div()
                                                    .id(ElementId::Name(
                                                        format!("msg-text-{seg_idx}").into(),
                                                    ))
                                                    .child(text)
                                                    .into_any_element(),
                                                MessageSegment::FilePill { path, label } => div()
                                                    .id(ElementId::Name(
                                                        format!("msg-pill-{seg_idx}").into(),
                                                    ))
                                                    .flex()
                                                    .items_center()
                                                    .gap(px(4.0))
                                                    .px(px(6.0))
                                                    .py(px(1.0))
                                                    .rounded(px(5.0))
                                                    .border_1()
                                                    .border_color(theme.accent.opacity(0.45))
                                                    .bg(theme.accent.opacity(0.10))
                                                    .child(
                                                        crate::primitives::file_type_icon(
                                                            &path, 11.0,
                                                        ),
                                                    )
                                                    .child(
                                                        div()
                                                            .text_size(px(12.5))
                                                            .text_color(theme.accent)
                                                            .child(label),
                                                    )
                                                    .into_any_element(),
                                            },
                                        )),
                                ),
                        )
                    }),
            )
            .child(
                div()
                    .h(px(27.0))
                    .flex()
                    .items_center()
                    .gap(px(1.0))
                    .invisible()
                    .group_hover(group_name, |element| element.visible())
                    .when_some(timestamp, |element, timestamp| {
                        element.child(
                            div()
                                .px(px(4.0))
                                .text_size(px(11.5))
                                .text_color(theme.text_ghost)
                                .child(timestamp),
                        )
                    })
                    .child(copy_button(
                        format!("copy-user-message-{}", self.selection_row),
                        copy_content,
                        theme,
                        cx,
                    )),
            )
    }
}

// ─── Assistant message ────────────────────────────────────────────────────────

#[derive(IntoElement)]
pub struct AssistantMessageBubble {
    pub content_parts: Rc<Vec<AssistantContentPart>>,
    pub is_streaming: bool,
    pub created_at: Option<i64>,
    content_for_copy: Option<gpui::SharedString>,
    markdown_views: Vec<Option<Rc<RefCell<MarkdownView>>>>,
    selection: Option<TranscriptSelection>,
    selection_row: String,
    on_preview_image: Option<PreviewImageHandler>,
    thinking_expanded: Option<Rc<RefCell<HashSet<String>>>>,
    on_thinking_toggle: Option<Rc<dyn Fn(String, bool, &mut Window, &mut App) + 'static>>,
    link_handler: Option<LinkHandler>,
}

impl AssistantMessageBubble {
    pub fn new(content_parts: Rc<Vec<AssistantContentPart>>) -> Self {
        Self {
            content_parts,
            is_streaming: false,
            created_at: None,
            content_for_copy: None,
            markdown_views: Vec::new(),
            selection: None,
            selection_row: "assistant-message".to_owned(),
            on_preview_image: None,
            thinking_expanded: None,
            on_thinking_toggle: None,
            link_handler: None,
        }
    }

    pub fn selection(mut self, selection: TranscriptSelection, row: impl Into<String>) -> Self {
        self.selection = Some(selection);
        self.selection_row = row.into();
        self
    }

    pub fn streaming(mut self, is_streaming: bool) -> Self {
        self.is_streaming = is_streaming;
        self
    }

    pub fn copy_content(mut self, content: gpui::SharedString) -> Self {
        self.content_for_copy = Some(content);
        self
    }

    pub fn markdown_views(
        mut self,
        markdown_views: Vec<Option<Rc<RefCell<MarkdownView>>>>,
    ) -> Self {
        self.markdown_views = markdown_views;
        self
    }

    pub fn created_at(mut self, created_at: Option<i64>) -> Self {
        self.created_at = created_at;
        self
    }

    pub fn on_preview_image(mut self, handler: PreviewImageHandler) -> Self {
        self.on_preview_image = Some(handler);
        self
    }

    pub fn thinking_expanded(mut self, expanded: Rc<RefCell<HashSet<String>>>) -> Self {
        self.thinking_expanded = Some(expanded);
        self
    }

    pub fn on_thinking_toggle(
        mut self,
        handler: impl Fn(String, bool, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_thinking_toggle = Some(Rc::new(handler));
        self
    }

    pub fn link_handler(mut self, handler: Option<LinkHandler>) -> Self {
        self.link_handler = handler;
        self
    }
}

impl RenderOnce for AssistantMessageBubble {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let content_for_copy = self.content_for_copy.unwrap_or_else(|| {
            self.content_parts
                .iter()
                .filter_map(|part| match part {
                    AssistantContentPart::Text { text, .. }
                    | AssistantContentPart::Thinking { text } => Some(text.as_str()),
                    AssistantContentPart::ToolCall { .. } | AssistantContentPart::Image { .. } => {
                        None
                    }
                })
                .collect::<Vec<_>>()
                .join("\n\n")
                .into()
        });
        let timestamp = format_message_time(self.created_at);
        let group_name = format!("assistant-message-{}", self.selection_row);
        let palette = Palette::from_theme(&theme);
        let selection = self.selection.clone().unwrap_or_default();
        let markdown_views = self.markdown_views;
        let content_parts = self.content_parts;
        let is_streaming = self.is_streaming;
        let preview_handler = self.on_preview_image.clone();
        let thinking_expanded = self.thinking_expanded.clone();
        let on_thinking_toggle = self.on_thinking_toggle.clone();
        let link_handler = self.link_handler.clone();

        div()
            .w_full()
            .flex()
            .flex_col()
            .gap(px(6.0))
            .group(group_name.clone())
            .children(
                content_parts
                    .iter()
                    .enumerate()
                    .map(|(index, part)| match part {
                        AssistantContentPart::Thinking { text } => {
                            let thinking_id = selection_row_for_part(&self.selection_row, index);
                            let collapsed = !thinking_expanded
                                .as_ref()
                                .is_some_and(|state| state.borrow().contains(&thinking_id));
                            let action_id = thinking_id.clone();
                            let on_toggle = on_thinking_toggle.clone();
                            let markdown_view = markdown_views
                                .get(index)
                                .and_then(Option::as_ref)
                                .cloned()
                                .unwrap_or_else(|| Rc::new(RefCell::new(MarkdownView::new())));
                            ThinkingBlock::new(thinking_id, text.clone(), collapsed)
                                .markdown_view(markdown_view)
                                .selection(selection.clone())
                                .streaming(is_streaming)
                                .link_handler(link_handler.clone())
                                .on_toggle(move |expanded, window, cx| {
                                    if let Some(on_toggle) = &on_toggle {
                                        on_toggle(action_id.clone(), expanded, window, cx);
                                    }
                                })
                                .into_any_element()
                        }
                        AssistantContentPart::Text { text, .. } => {
                            let markdown_ctx = assistant_ctx(
                                selection_row_for_part(&self.selection_row, index),
                                &palette,
                                selection.clone(),
                                link_handler.clone(),
                            );
                            render_selectable_markdown(
                                text,
                                markdown_views.get(index).and_then(Option::as_ref),
                                &markdown_ctx,
                                is_streaming,
                            )
                        }
                        // Tool calls are rendered in the user's grouped run activity,
                        // not inline in the assistant bubble. Keeping this branch empty
                        // also prevents the same call from appearing twice while a
                        // streamed turn is replaced by its persisted form.
                        AssistantContentPart::ToolCall { .. } => div().into_any_element(),
                        AssistantContentPart::Image { data, mime_type } => {
                            // Decode the payload; skip the part when the format is not
                            // supported by gpui rather than rendering a blank tile.
                            let image =
                                gpui::ImageFormat::from_mime_type(&mime_type).and_then(|format| {
                                    base64::engine::general_purpose::STANDARD
                                        .decode(&data)
                                        .ok()
                                        .map(|bytes| {
                                            Arc::new(gpui::Image::from_bytes(format, bytes))
                                        })
                                });
                            let Some(image) = image else {
                                return div().into_any_element();
                            };
                            let on_preview = preview_handler.clone();
                            div()
                                .id(ElementId::Name(format!("assistant-image-{index}").into()))
                                .max_w(px(400.0))
                                .rounded(px(8.0))
                                .overflow_hidden()
                                .border_1()
                                .border_color(theme.border)
                                .cursor_default()
                                .when_some(on_preview, |tile, on_preview| {
                                    let image = image.clone();
                                    tile.on_click(move |_, window, cx| {
                                        (on_preview)(image.clone(), window, cx);
                                        cx.stop_propagation();
                                    })
                                })
                                .child(img(image).size_full())
                                .into_any_element()
                        }
                    }),
            )
            // The live working indicator (dots + "Working for Ns") is the
            // single streaming signal, rendered as the transcript's trailing
            // row — no per-bubble "Working…" here.
            .child(
                div()
                    .h(px(27.0))
                    .flex()
                    .items_center()
                    .gap(px(1.0))
                    .invisible()
                    .group_hover(group_name, |element| element.visible())
                    .when_some(timestamp, |element, timestamp| {
                        element.child(
                            div()
                                .px(px(4.0))
                                .text_size(px(11.5))
                                .text_color(theme.text_ghost)
                                .child(timestamp),
                        )
                    })
                    .child(copy_button(
                        format!("copy-assistant-message-{}", self.selection_row),
                        content_for_copy,
                        theme,
                        cx,
                    )),
            )
    }
}
