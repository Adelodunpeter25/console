//! Recursive workspace renderer: a leaf pane renders its tab bar plus the
//! active tab's content; a split renders two panes with a draggable divider.
//! Mirrors the desktop app's `WorkspacePane`.

use std::rc::Rc;

use console_core::{LeafPaneNode, WorkspaceNode};
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    Styled, Window, div, prelude::FluentBuilder, px,
};

use crate::theme::Theme;

use super::{WorkspaceDrag, WorkspaceDropAction, WorkspaceTabBar};

/// Renders the content area for a leaf's active tab. The app shell supplies
/// this (chat transcript + composer today); file/terminal/diff views can be
/// added here later, matching the desktop app's `WorkspaceContent`.
pub type ContentRenderer = Rc<
    dyn Fn(
        &str,
        Option<&console_core::WorkspaceTabConfig>,
        &mut Window,
        &mut App,
    ) -> gpui::AnyElement,
>;

/// The whole workspace: tree + which pane currently holds focus.
#[derive(IntoElement)]
pub struct WorkspacePane {
    pub root: WorkspaceNode,
    pub active_pane_id: Option<String>,
    pub render_content: ContentRenderer,
    on_select_tab: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_close_tab: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_drop_tab:
        Rc<dyn Fn(String, WorkspaceDrag, WorkspaceDropAction, &mut Window, &mut App) + 'static>,
    on_close_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_focus_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_resize_split: Option<
        Rc<
            dyn Fn(
                    String,
                    console_core::SplitDirection,
                    gpui::Point<gpui::Pixels>,
                    &mut Window,
                    &mut App,
                ) + 'static,
        >,
    >,
}

impl WorkspacePane {
    pub fn new(
        root: WorkspaceNode,
        active_pane_id: Option<String>,
        render_content: ContentRenderer,
        on_select_tab: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
        on_close_tab: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
        on_drop_tab: Rc<
            dyn Fn(String, WorkspaceDrag, WorkspaceDropAction, &mut Window, &mut App) + 'static,
        >,
        on_close_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
        on_focus_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    ) -> Self {
        Self {
            root,
            active_pane_id,
            render_content,
            on_select_tab,
            on_close_tab,
            on_drop_tab,
            on_close_pane,
            on_focus_pane,
            on_resize_split: None,
        }
    }

    pub fn with_resize_split(
        mut self,
        on_resize_split: impl Fn(
            String,
            console_core::SplitDirection,
            gpui::Point<gpui::Pixels>,
            &mut Window,
            &mut App,
        ) + 'static,
    ) -> Self {
        self.on_resize_split = Some(Rc::new(on_resize_split));
        self
    }
}

fn drop_zone(
    target_pane_id: String,
    action: WorkspaceDropAction,
    on_drop_tab: &Rc<
        dyn Fn(String, WorkspaceDrag, WorkspaceDropAction, &mut Window, &mut App) + 'static,
    >,
    theme: Theme,
) -> gpui::AnyElement {
    let label = match action {
        WorkspaceDropAction::SplitLeft => "Split left",
        WorkspaceDropAction::AddTab => "Add tab",
        WorkspaceDropAction::SplitRight => "Split right",
    };
    let on_drop = on_drop_tab.clone();
    div()
        .id(ElementId::Name(
            format!("workspace-drop-zone-{}-{action:?}", target_pane_id).into(),
        ))
        .flex_1()
        .h_full()
        .flex()
        .items_center()
        .justify_center()
        .opacity(0.0)
        .drag_over::<WorkspaceDrag>(move |style, _, _, _| {
            style
                .opacity(1.0)
                .bg(theme.accent.opacity(0.12))
                .border_2()
                .border_color(theme.accent.opacity(0.65))
        })
        .can_drop(|value, _, _| value.is::<WorkspaceDrag>())
        .on_drop(move |drag: &WorkspaceDrag, window, cx| {
            (on_drop)(target_pane_id.clone(), drag.clone(), action, window, cx);
        })
        .child(
            div()
                .px(px(12.0))
                .py(px(8.0))
                .rounded(px(6.0))
                .bg(theme.surface)
                .border_1()
                .border_color(theme.accent)
                .text_size(px(12.0))
                .text_color(theme.text)
                .child(label),
        )
        .into_any_element()
}

fn render_leaf(
    leaf: &LeafPaneNode,
    is_focused: bool,
    render_content: &ContentRenderer,
    on_select_tab: &Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_close_tab: &Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_drop_tab: &Rc<
        dyn Fn(String, WorkspaceDrag, WorkspaceDropAction, &mut Window, &mut App) + 'static,
    >,
    on_close_pane: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_focus_pane: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    can_close_pane: bool,
    window: &mut Window,
    cx: &mut App,
) -> gpui::AnyElement {
    let active_tab = leaf
        .active_tab_id
        .as_deref()
        .and_then(|id| leaf.tabs.iter().find(|tab| tab.id() == id));

    let on_sel = on_select_tab.clone();
    let on_cls = on_close_tab.clone();
    let on_drop = on_drop_tab.clone();
    let on_close_pane = on_close_pane.clone();
    let on_focus_pane = on_focus_pane.clone();
    let bar = WorkspaceTabBar::new(
        leaf.clone(),
        move |pane_id, tab_id, window, cx| (on_sel)(pane_id, tab_id, window, cx),
        move |pane_id, tab_id, window, cx| (on_cls)(pane_id, tab_id, window, cx),
        can_close_pane,
        move |pane_id, window, cx| (on_close_pane)(pane_id, window, cx),
    );

    div()
        .id(ElementId::Name(
            format!("workspace-pane-{}", leaf.id).into(),
        ))
        .flex_1()
        .min_w_0()
        .min_h_0()
        .flex()
        .flex_col()
        .overflow_hidden()
        .on_mouse_down(MouseButton::Left, {
            let pane_id = leaf.id.clone();
            move |_, window, cx| (on_focus_pane)(pane_id.clone(), window, cx)
        })
        // Dim inactive panes so the focused pane reads clearly.
        .when(!is_focused, |el| el.opacity(0.92))
        .child(bar)
        .child(
            div()
                .id(ElementId::Name(
                    format!("workspace-pane-content-{}", leaf.id).into(),
                ))
                .flex_1()
                .min_h_0()
                .w_full()
                .flex()
                .flex_col()
                .overflow_hidden()
                .child((render_content)(leaf.id.as_str(), active_tab, window, cx)),
        )
        // Three equal drop zones distinguish splitting left, adding a tab to
        // this pane, and splitting right.
        .child(
            div()
                .id(ElementId::Name(
                    format!("workspace-drop-overlay-{}", leaf.id).into(),
                ))
                .absolute()
                .inset_0()
                .flex()
                .child(drop_zone(
                    leaf.id.clone(),
                    WorkspaceDropAction::SplitLeft,
                    &on_drop,
                    Theme::current(cx),
                ))
                .child(drop_zone(
                    leaf.id.clone(),
                    WorkspaceDropAction::AddTab,
                    &on_drop,
                    Theme::current(cx),
                ))
                .child(drop_zone(
                    leaf.id.clone(),
                    WorkspaceDropAction::SplitRight,
                    &on_drop,
                    Theme::current(cx),
                )),
        )
        .into_any_element()
}

fn render_node(
    node: &WorkspaceNode,
    active_pane_id: Option<&str>,
    render_content: &ContentRenderer,
    on_select_tab: &Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_close_tab: &Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_drop_tab: &Rc<
        dyn Fn(String, WorkspaceDrag, WorkspaceDropAction, &mut Window, &mut App) + 'static,
    >,
    on_close_pane: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_focus_pane: &Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    on_resize_split: &Option<
        Rc<
            dyn Fn(
                    String,
                    console_core::SplitDirection,
                    gpui::Point<gpui::Pixels>,
                    &mut Window,
                    &mut App,
                ) + 'static,
        >,
    >,
    can_close_pane: bool,
    window: &mut Window,
    cx: &mut App,
) -> gpui::AnyElement {
    match node {
        WorkspaceNode::Leaf(leaf) => render_leaf(
            leaf,
            active_pane_id == Some(leaf.id.as_str()),
            render_content,
            on_select_tab,
            on_close_tab,
            on_drop_tab,
            on_close_pane,
            on_focus_pane,
            can_close_pane,
            window,
            cx,
        ),
        WorkspaceNode::Split(split) => {
            let theme = Theme::current(cx);
            let is_horizontal = matches!(split.direction, console_core::SplitDirection::Horizontal);
            let size_0 = split.sizes[0].clamp(5.0, 95.0);
            let size_1 = split.sizes[1].clamp(5.0, 95.0);
            let on_resize = on_resize_split.clone();
            let split_id = split.id.clone();
            let direction = split.direction;

            div()
                .id(ElementId::Name(
                    format!("workspace-split-{}", split.id).into(),
                ))
                .flex_1()
                .min_w_0()
                .min_h_0()
                .flex()
                .when(is_horizontal, |el| el.flex_row())
                .when(!is_horizontal, |el| el.flex_col())
                .child(
                    div()
                        .min_w_0()
                        .min_h_0()
                        .flex_grow(size_0)
                        .flex_shrink(1.0)
                        .flex_basis(gpui::relative(size_0 / 100.0))
                        .flex()
                        .flex_col()
                        .child(render_node(
                            &split.children[0],
                            active_pane_id,
                            render_content,
                            on_select_tab,
                            on_close_tab,
                            on_drop_tab,
                            on_close_pane,
                            on_focus_pane,
                            on_resize_split,
                            can_close_pane,
                            window,
                            cx,
                        )),
                )
                // Divider between panes.
                .child(
                    div()
                        .id(ElementId::Name(
                            format!("workspace-divider-{}", split.id).into(),
                        ))
                        .flex_none()
                        .when(is_horizontal, |el| {
                            el.w(px(6.0)).mx(px(-2.0)).h_full().cursor_col_resize()
                        })
                        .when(!is_horizontal, |el| {
                            el.h(px(6.0)).my(px(-2.0)).w_full().cursor_row_resize()
                        })
                        .flex()
                        .items_center()
                        .justify_center()
                        .child(
                            div()
                                .when(is_horizontal, |el| el.w(px(1.0)).h_full())
                                .when(!is_horizontal, |el| el.h(px(1.0)).w_full())
                                .bg(theme.border),
                        )
                        .hover(|s| s.bg(theme.accent.opacity(0.4)))
                        .when_some(on_resize, |el, on_resize| {
                            let split_id = split_id.clone();
                            el.on_mouse_down(
                                gpui::MouseButton::Left,
                                move |event: &gpui::MouseDownEvent, window, cx| {
                                    (on_resize)(
                                        split_id.clone(),
                                        direction,
                                        event.position,
                                        window,
                                        cx,
                                    );
                                },
                            )
                        }),
                )
                .child(
                    div()
                        .min_w_0()
                        .min_h_0()
                        .flex_grow(size_1)
                        .flex_shrink(1.0)
                        .flex_basis(gpui::relative(size_1 / 100.0))
                        .flex()
                        .flex_col()
                        .child(render_node(
                            &split.children[1],
                            active_pane_id,
                            render_content,
                            on_select_tab,
                            on_close_tab,
                            on_drop_tab,
                            on_close_pane,
                            on_focus_pane,
                            on_resize_split,
                            can_close_pane,
                            window,
                            cx,
                        )),
                )
                .into_any_element()
        }
    }
}

impl RenderOnce for WorkspacePane {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let can_close_pane = self.root.leaves().len() > 1;
        render_node(
            &self.root,
            self.active_pane_id.as_deref(),
            &self.render_content,
            &self.on_select_tab,
            &self.on_close_tab,
            &self.on_drop_tab,
            &self.on_close_pane,
            &self.on_focus_pane,
            &self.on_resize_split,
            can_close_pane,
            window,
            cx,
        )
    }
}
