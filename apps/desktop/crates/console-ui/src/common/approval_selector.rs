use std::rc::Rc;

use console_core::ApprovalMode;
use gpui::{
    App, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

/// UI-only presentation for the domain [`ApprovalMode`]: the icon shown next
/// to each mode in the selector. The enum itself lives in `console-core`.
pub trait ApprovalModeIconExt {
    fn icon(self) -> IconName;
}

impl ApprovalModeIconExt for ApprovalMode {
    fn icon(self) -> IconName {
        match self {
            Self::AlwaysAsk => IconName::TriangleAlert,
            Self::AcceptEdits => IconName::Pencil,
            Self::PlanMode => IconName::List,
            Self::FullAccess => IconName::Zap,
        }
    }
}

pub struct ApprovalModeSelector {
    pub mode: ApprovalMode,
    pub open: bool,
}

impl ApprovalModeSelector {
    pub fn new(mode: ApprovalMode) -> Self {
        Self { mode, open: false }
    }

    pub fn set_mode(&mut self, mode: ApprovalMode) {
        self.mode = mode;
        self.open = false;
    }

    pub fn toggle_open(&mut self) {
        self.open = !self.open;
    }
}

#[derive(Clone, IntoElement)]
pub struct ApprovalModeDropdown {
    selected: ApprovalMode,
    on_select: Rc<dyn Fn(ApprovalMode, &mut Window, &mut App) + 'static>,
}

impl ApprovalModeDropdown {
    pub fn new(
        selected: ApprovalMode,
        on_select: impl Fn(ApprovalMode, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            selected,
            on_select: Rc::new(on_select),
        }
    }
}

impl RenderOnce for ApprovalModeDropdown {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let selected_mode = self.selected;
        let on_select = self.on_select;

        div()
            .id("approval-dropdown-card")
            .w(px(270.0))
            .rounded(px(12.0))
            .bg(theme.canvas)
            .border_1()
            .border_color(theme.border)
            .shadow_xl()
            .overflow_hidden()
            .flex()
            .flex_col()
            .on_click(|_, _, cx| {
                cx.stop_propagation();
            })
            // Options
            .child(div().p(px(6.0)).flex().flex_col().gap_y(px(2.0)).children(
                ApprovalMode::ALL.into_iter().map(|mode| {
                    let is_active = mode == selected_mode;
                    let on_sel = on_select.clone();

                    div()
                        .id(ElementId::Name(mode.value().into()))
                        .px(px(10.0))
                        .py(px(7.0))
                        .rounded(px(6.0))
                        .overflow_hidden()
                        .cursor_default()
                        .when(is_active, |s| {
                            s.bg(theme.accent.opacity(0.18))
                                .border_1()
                                .border_color(theme.accent.opacity(0.40))
                        })
                        .when(!is_active, |s| s.hover(|h| h.bg(theme.overlay)))
                        .on_click(move |_, window, cx| {
                            (on_sel)(mode, window, cx);
                        })
                        .flex()
                        .items_center()
                        .justify_between()
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .flex()
                                .flex_col()
                                .gap_y(px(1.0))
                                .child(
                                    div()
                                        .text_size(px(12.0))
                                        .font_weight(if is_active {
                                            FontWeight::SEMIBOLD
                                        } else {
                                            FontWeight::MEDIUM
                                        })
                                        .text_color(theme.text)
                                        .child(mode.label()),
                                )
                                .child(
                                    div()
                                        .text_size(px(10.0))
                                        .text_color(theme.text_tertiary)
                                        .truncate()
                                        .child(mode.description()),
                                ),
                        )
                        .when(is_active, |s| {
                            s.child(app_icon(IconName::Check, 11.0, theme.accent))
                        })
                }),
            ))
    }
}
