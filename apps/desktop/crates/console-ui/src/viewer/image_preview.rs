//! Image and SVG preview viewer component for workspace tabs.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, Styled, Window, div, img, prelude::*,
    px,
};

use crate::theme::Theme;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SvgViewMode {
    Preview,
    Source,
}

#[derive(Debug, Clone, Default)]
pub struct ImageMeta {
    pub width: Option<u32>,
    pub height: Option<u32>,
    pub size_bytes: Option<u64>,
    pub mime_type: Option<String>,
}

pub fn format_bytes(bytes: u64) -> String {
    if bytes < 1024 {
        format!("{bytes} B")
    } else if bytes < 1024 * 1024 {
        format!("{:.1} KB", bytes as f64 / 1024.0)
    } else {
        format!("{:.2} MB", bytes as f64 / (1024.0 * 1024.0))
    }
}

/// Sniff image dimensions from byte header (supports PNG, JPEG, GIF, WEBP, BMP, ICO).
pub fn image_dimensions(bytes: &[u8]) -> Option<(u32, u32)> {
    imagesize::blob_size(bytes)
        .ok()
        .map(|s| (s.width as u32, s.height as u32))
}

#[derive(IntoElement)]
pub struct ImagePreview {
    path: String,
    image: Arc<gpui::Image>,
    meta: Option<ImageMeta>,
    is_svg: bool,
    svg_mode: SvgViewMode,
    on_toggle_svg_mode: Option<Rc<dyn Fn(SvgViewMode, &mut Window, &mut App) + 'static>>,
    on_zoom: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
}

impl ImagePreview {
    pub fn new(path: impl Into<String>, image: Arc<gpui::Image>) -> Self {
        Self {
            path: path.into(),
            image,
            meta: None,
            is_svg: false,
            svg_mode: SvgViewMode::Preview,
            on_toggle_svg_mode: None,
            on_zoom: None,
        }
    }

    pub fn meta(mut self, meta: ImageMeta) -> Self {
        self.meta = Some(meta);
        self
    }

    pub fn is_svg(mut self, is_svg: bool) -> Self {
        self.is_svg = is_svg;
        self
    }

    pub fn svg_mode(mut self, mode: SvgViewMode) -> Self {
        self.svg_mode = mode;
        self
    }

    pub fn on_toggle_svg_mode(
        mut self,
        handler: impl Fn(SvgViewMode, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_toggle_svg_mode = Some(Rc::new(handler));
        self
    }

    pub fn on_zoom(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_zoom = Some(Rc::new(handler));
        self
    }
}

impl RenderOnce for ImagePreview {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);

        let mut meta_parts = Vec::new();
        if let Some(meta) = &self.meta {
            if let (Some(w), Some(h)) = (meta.width, meta.height) {
                meta_parts.push(format!("{w} × {h} px"));
            }
            if let Some(bytes) = meta.size_bytes {
                meta_parts.push(format_bytes(bytes));
            }
            if let Some(mime) = &meta.mime_type {
                meta_parts.push(mime.clone());
            }
        }
        let meta_text = meta_parts.join("  •  ");

        let preview_active = self.svg_mode == SvgViewMode::Preview;
        let on_toggle_preview = self.on_toggle_svg_mode.clone();
        let on_toggle_source = self.on_toggle_svg_mode.clone();

        div()
            .id(format!("image-preview-{}", self.path))
            .size_full()
            .flex()
            .flex_col()
            .bg(theme.canvas)
            .child(
                // Header bar: meta line + SVG toggle if applicable
                div()
                    .id("image-preview-header")
                    .h(px(32.0))
                    .px(px(12.0))
                    .bg(theme.surface)
                    .border_b_1()
                    .border_color(theme.border)
                    .flex()
                    .items_center()
                    .justify_between()
                    .gap(px(8.0))
                    .child(
                        div()
                            .text_size(px(11.0))
                            .text_color(theme.text_tertiary)
                            .truncate()
                            .child(meta_text),
                    )
                    .when(self.is_svg, |header| {
                        header.child(
                            div()
                                .id("svg-mode-toggle")
                                .flex()
                                .items_center()
                                .bg(theme.canvas)
                                .border_1()
                                .border_color(theme.border)
                                .rounded(px(5.0))
                                .p(px(1.5))
                                .gap(px(2.0))
                                .child(
                                    div()
                                        .id("svg-mode-preview")
                                        .px(px(8.0))
                                        .py(px(2.0))
                                        .rounded(px(3.0))
                                        .text_size(px(11.0))
                                        .font_weight(if preview_active {
                                            FontWeight::SEMIBOLD
                                        } else {
                                            FontWeight::NORMAL
                                        })
                                        .cursor_default()
                                        .when(preview_active, |s| {
                                            s.bg(theme.surface)
                                                .text_color(theme.text)
                                                .shadow_sm()
                                        })
                                        .when(!preview_active, |s| {
                                            s.text_color(theme.text_tertiary).hover(|h| {
                                                h.text_color(theme.text)
                                            })
                                        })
                                        .on_click(move |_, window, cx| {
                                            if let Some(on_toggle) = &on_toggle_preview {
                                                (on_toggle)(SvgViewMode::Preview, window, cx);
                                            }
                                        })
                                        .child("Preview"),
                                )
                                .child(
                                    div()
                                        .id("svg-mode-source")
                                        .px(px(8.0))
                                        .py(px(2.0))
                                        .rounded(px(3.0))
                                        .text_size(px(11.0))
                                        .font_weight(if !preview_active {
                                            FontWeight::SEMIBOLD
                                        } else {
                                            FontWeight::NORMAL
                                        })
                                        .cursor_default()
                                        .when(!preview_active, |s| {
                                            s.bg(theme.surface)
                                                .text_color(theme.text)
                                                .shadow_sm()
                                        })
                                        .when(preview_active, |s| {
                                            s.text_color(theme.text_tertiary).hover(|h| {
                                                h.text_color(theme.text)
                                            })
                                        })
                                        .on_click(move |_, window, cx| {
                                            if let Some(on_toggle) = &on_toggle_source {
                                                (on_toggle)(SvgViewMode::Source, window, cx);
                                            }
                                        })
                                        .child("Source"),
                                ),
                        )
                    }),
            )
            .child(
                // Centered image area with neutral background
                div()
                    .id("image-preview-viewport")
                    .flex_1()
                    .size_full()
                    .min_h_0()
                    .min_w_0()
                    .relative()
                    .flex()
                    .items_center()
                    .justify_center()
                    .overflow_hidden()
                    .bg(theme.chat_canvas)
                    .p(px(24.0))
                    .child(
                        div()
                            .id("image-preview-box")
                            .max_w_full()
                            .max_h_full()
                            .flex()
                            .items_center()
                            .justify_center()
                            .when_some(self.on_zoom.clone(), |s, on_zoom| {
                                s.cursor_pointer()
                                    .on_click(move |_, window, cx| (on_zoom)(window, cx))
                            })
                            .child(
                                img(self.image.clone())
                                    .id("image-preview-img")
                                    .max_w_full()
                                    .max_h_full()
                                    .object_fit(gpui::ObjectFit::ScaleDown)
                                    .rounded(px(4.0))
                                    .shadow_md(),
                            ),
                    ),
            )
    }
}

/// Standalone SVG header for when an SVG file is displayed in Source mode.
pub fn svg_source_header(
    theme: Theme,
    on_toggle_svg_mode: Option<Rc<dyn Fn(SvgViewMode, &mut Window, &mut App) + 'static>>,
) -> impl IntoElement {
    let on_toggle_preview = on_toggle_svg_mode.clone();
    let on_toggle_source = on_toggle_svg_mode;

    div()
        .id("svg-source-header")
        .h(px(32.0))
        .px(px(12.0))
        .bg(theme.surface)
        .border_b_1()
        .border_color(theme.border)
        .flex()
        .items_center()
        .justify_between()
        .gap(px(8.0))
        .child(
            div()
                .text_size(px(11.0))
                .text_color(theme.text_tertiary)
                .child("SVG Source"),
        )
        .child(
            div()
                .id("svg-mode-toggle-source")
                .flex()
                .items_center()
                .bg(theme.canvas)
                .border_1()
                .border_color(theme.border)
                .rounded(px(5.0))
                .p(px(1.5))
                .gap(px(2.0))
                .child(
                    div()
                        .id("svg-source-switch-preview")
                        .px(px(8.0))
                        .py(px(2.0))
                        .rounded(px(3.0))
                        .text_size(px(11.0))
                        .cursor_default()
                        .text_color(theme.text_tertiary)
                        .hover(|h| h.text_color(theme.text))
                        .on_click(move |_, window, cx| {
                            if let Some(on_toggle) = &on_toggle_preview {
                                (on_toggle)(SvgViewMode::Preview, window, cx);
                            }
                        })
                        .child("Preview"),
                )
                .child(
                    div()
                        .id("svg-source-switch-source")
                        .px(px(8.0))
                        .py(px(2.0))
                        .rounded(px(3.0))
                        .text_size(px(11.0))
                        .font_weight(FontWeight::SEMIBOLD)
                        .cursor_default()
                        .bg(theme.surface)
                        .text_color(theme.text)
                        .shadow_sm()
                        .on_click(move |_, window, cx| {
                            if let Some(on_toggle) = &on_toggle_source {
                                (on_toggle)(SvgViewMode::Source, window, cx);
                            }
                        })
                        .child("Source"),
                ),
        )
}
