use gpui::{
    AnyElement, App, Div, ElementId, Hsla, Img, InteractiveElement, Interactivity, ParentElement,
    PathBuilder, Pixels, RenderOnce, ScrollHandle, SharedString, Stateful, StyleRefinement, Styled,
    Svg, Window, canvas, div, img, point, prelude::*, px, rgb, svg,
};

pub mod context_menu;
pub mod file_icons;
pub mod icons;
pub mod menu;
pub mod motion;
pub mod scrollbar;
pub mod text_field;
pub mod tooltip;

use crate::theme::Theme;
use console_core::SessionStatus;

pub use context_menu::{draft_context_menu, session_context_menu};
pub use file_icons::{
    base_name, file_icon_for_language, file_icon_for_name, file_icon_for_path, file_type_icon,
    lang_tag_for_path,
};
pub use icons::{FileTypeIcon, IconName, ProviderIcon, app_icon, provider_app_icon};
pub use menu::{ContextMenuHandle, MenuAlign, MenuItem, dropdown_menu, popover};
pub use scrollbar::ScrollbarState;
pub use text_field::TextField;

/// A monochrome icon from the embedded set, tinted via text color.
pub fn icon(path: &'static str, size: f32, color: Hsla) -> Svg {
    svg()
        .path(path)
        .w(px(size))
        .h(px(size))
        .flex_none()
        .text_color(color)
}

/// A polychrome file icon rendered as an image so the SVG's authored colors are preserved.
pub fn file_icon(path: &'static str, size: f32) -> Img {
    img(path).w(px(size)).h(px(size)).flex_none()
}

/// A compact ghost icon button.
pub fn icon_button(id: impl Into<ElementId>, path: &'static str, theme: Theme) -> Stateful<Div> {
    div()
        .id(id)
        .size(px(22.0))
        .rounded(px(6.0))
        .flex()
        .items_center()
        .justify_center()
        .cursor_default()
        .hover(|element| element.bg(theme.overlay))
        .active(|element| element.bg(theme.overlay_strong))
        .child(icon(path, 13.0, theme.text_tertiary))
}

/// Keeps a wheel gesture inside a nested scrollable.
pub fn contain_scroll(handle: &ScrollHandle, cx: &mut App) {
    if handle.max_offset().y > px(0.5) {
        cx.stop_propagation();
    }
}

/// Brand hue for each provider.
pub fn provider_color(theme: &Theme, provider: &str) -> Hsla {
    match provider {
        "antigravity" => rgb(0xE2795B).into(),
        "claude" | "anthropic" => rgb(0xD97757).into(),
        "deepseek" => rgb(0x4D6BFE).into(),
        _ => {
            if theme.is_dark {
                rgb(0xF3F3F3).into()
            } else {
                rgb(0x34363B).into()
            }
        }
    }
}

/// Recognizable provider SVG marks.
pub fn provider_icon(provider: &str) -> &'static str {
    let provider = provider.to_ascii_lowercase();
    match provider.as_str() {
        "antigravity" | "google" | "opencode" | "codex" | "openai"
        | "chatgpt" | "claude" | "anthropic" | "deepseek" | "grok" | "cursor" | "amp" | "pi" => {
            IconName::provider(&provider).path()
        }
        _ => IconName::Bot.path(),
    }
}

pub fn status_color(theme: &Theme, status: &SessionStatus) -> Hsla {
    match status {
        SessionStatus::Idle => theme.text_ghost,
        SessionStatus::Working => theme.accent,
        SessionStatus::NeedsAttention => theme.warning,
        SessionStatus::Done => theme.success,
    }
}

pub fn activity_icon(tool_name: &str) -> &'static str {
    let icon = match tool_name {
        "reasoning" | "think" => IconName::Sparkle,
        "bash" | "shell" | "command" => IconName::Terminal,
        "edit_file" | "write_file" | "str_replace" => IconName::Pencil,
        "read_file" => IconName::File,
        "list_files" | "ls" => IconName::Folder,
        "search_files" | "grep" => IconName::Search,
        "web_search" => IconName::Globe,
        "plan" => IconName::List,
        _ => IconName::Wrench,
    };
    icon.path()
}

/// A compact chip used as a dropdown-menu trigger.
#[derive(IntoElement)]
pub struct MenuChip {
    base: Stateful<Div>,
    icon: Option<(&'static str, Hsla)>,
    label: SharedString,
    caret: bool,
    outlined: bool,
    selected: bool,
    disabled: bool,
    height: Option<Pixels>,
    background: Option<Hsla>,
}

impl MenuChip {
    pub fn new(id: impl Into<ElementId>) -> Self {
        Self {
            base: div().id(id),
            icon: None,
            label: SharedString::default(),
            caret: true,
            outlined: false,
            selected: false,
            disabled: false,
            height: None,
            background: None,
        }
    }
    pub fn height(mut self, height: Pixels) -> Self {
        self.height = Some(height);
        self
    }
    pub fn background(mut self, background: Hsla) -> Self {
        self.background = Some(background);
        self
    }
    pub fn icon(mut self, path: &'static str, color: Hsla) -> Self {
        self.icon = Some((path, color));
        self
    }
    pub fn label(mut self, label: impl Into<SharedString>) -> Self {
        self.label = label.into();
        self
    }
    pub fn outlined(mut self) -> Self {
        self.outlined = true;
        self
    }
    pub fn caret(mut self, caret: bool) -> Self {
        self.caret = caret;
        self
    }
    pub fn disabled(mut self, disabled: bool) -> Self {
        self.disabled = disabled;
        self
    }
    pub fn selected(mut self, selected: bool) -> Self {
        self.selected = selected;
        self
    }
}

impl Styled for MenuChip {
    fn style(&mut self) -> &mut StyleRefinement {
        self.base.style()
    }
}
impl InteractiveElement for MenuChip {
    fn interactivity(&mut self) -> &mut Interactivity {
        self.base.interactivity()
    }
}
impl ParentElement for MenuChip {
    fn extend(&mut self, elements: impl IntoIterator<Item = AnyElement>) {
        self.base.extend(elements);
    }
}

impl RenderOnce for MenuChip {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        self.base
            .h(self
                .height
                .unwrap_or(if self.outlined { px(30.0) } else { px(24.0) }))
            .px(if self.outlined { px(10.0) } else { px(7.0) })
            .rounded(if self.outlined { px(7.0) } else { px(6.0) })
            .flex()
            .items_center()
            .gap(px(6.0))
            .text_size(px(11.5))
            .line_height(px(14.0))
            .cursor_default()
            .focus_visible(|s| s.border_1().border_color(theme.accent))
            .when(self.outlined, |e| {
                e.border_1()
                    .border_color(theme.border_strong)
                    .bg(self.background.unwrap_or(theme.raised))
            })
            .when(self.selected, |e| e.bg(theme.overlay))
            .when(!self.disabled, |e| e.hover(|e| e.bg(theme.overlay)))
            .when(self.disabled, |e| e.opacity(0.7))
            .when_some(self.icon, |e, (path, color)| {
                e.child(icon(path, 10.5, color))
            })
            .child(
                div()
                    .min_w_0()
                    .truncate()
                    .text_color(theme.text_secondary)
                    .child(self.label),
            )
            .when(self.caret, |e| {
                e.child(app_icon(IconName::ChevronDown, 9.0, theme.text_ghost))
            })
    }
}

/// Inline link-like dropdown trigger for project name selectors.
#[derive(IntoElement)]
pub struct ProjectNameSelector {
    base: Stateful<Div>,
    label: SharedString,
    selected: bool,
}

impl ProjectNameSelector {
    pub fn new(id: impl Into<ElementId>, label: impl Into<SharedString>) -> Self {
        Self {
            base: div().id(id),
            label: label.into(),
            selected: false,
        }
    }
    pub fn selected(mut self, selected: bool) -> Self {
        self.selected = selected;
        self
    }
}

impl Styled for ProjectNameSelector {
    fn style(&mut self) -> &mut StyleRefinement {
        self.base.style()
    }
}
impl InteractiveElement for ProjectNameSelector {
    fn interactivity(&mut self) -> &mut Interactivity {
        self.base.interactivity()
    }
}
impl ParentElement for ProjectNameSelector {
    fn extend(&mut self, elements: impl IntoIterator<Item = AnyElement>) {
        self.base.extend(elements);
    }
}

impl RenderOnce for ProjectNameSelector {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let underline_color = if self.selected {
            theme.text_secondary
        } else {
            theme.text_tertiary
        };
        self.base
            .relative()
            .flex_none()
            .cursor_default()
            .focus_visible(|s| s.border_1().border_color(theme.accent))
            .child(self.label)
            .child(
                canvas(
                    |_, _, _| {},
                    move |bounds, _, window, _| {
                        let y = bounds.origin.y + bounds.size.height - px(0.5);
                        let mut builder =
                            PathBuilder::stroke(px(1.0)).dash_array(&[px(1.0), px(2.0)]);
                        builder.move_to(point(bounds.origin.x, y));
                        builder.line_to(point(bounds.origin.x + bounds.size.width, y));
                        if let Ok(line) = builder.build() {
                            window.paint_path(line, underline_color);
                        }
                    },
                )
                .absolute()
                .inset_0(),
            )
    }
}
