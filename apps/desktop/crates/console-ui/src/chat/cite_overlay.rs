//! Floating Copy + Cite overlay and geometry probes for transcript text selection.

use gpui::{canvas, div, prelude::*, px, Bounds, IntoElement, Pixels};
use std::cell::RefCell;
use std::rc::Rc;

use crate::markdown::render::{self, TranscriptSelection};
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{app_icon, IconName};
use crate::theme::Theme;

/// Records the transcript container's window-space bounds every frame so the
/// Cite popup's window-space anchor can be converted to container-relative
/// coordinates. Paints beneath everything and registers no hitbox.
pub fn cite_origin_probe(cell: Rc<RefCell<Option<Bounds<Pixels>>>>) -> impl IntoElement {
    canvas(
        move |probe: Bounds<Pixels>, _, _| {
            *cell.borrow_mut() = Some(probe);
        },
        |_, _, _, _| (),
    )
    .absolute()
    .inset_0()
}

/// The floating Copy + Cite strip shown just above an active selection: one
/// card holding both icon buttons. It is purely visual: the selection mouse
/// handlers (render.rs) detect a press and release on the recorded hit rects
/// and fire the actions, so no click handler here can race the
/// selection-clearing mouse-down.
pub fn cite_popup(
    line: [f32; 4],
    container: Bounds<Pixels>,
    selection: TranscriptSelection,
    theme: Theme,
) -> impl IntoElement {
    const PILL_SIZE: f32 = 24.0;
    const PILL_GAP: f32 = 2.0;
    const STRIP_PAD: f32 = 3.0;
    const SELECTION_GAP: f32 = 2.0;
    const HIT_PAD: f32 = 6.0;
    let container_x = f32::from(container.origin.x);
    let container_y = f32::from(container.origin.y);
    let container_w = f32::from(container.size.width);
    let container_h = f32::from(container.size.height);
    let strip_w = STRIP_PAD * 2.0 + PILL_SIZE * 2.0 + PILL_GAP;
    let strip_h = STRIP_PAD * 2.0 + PILL_SIZE;
    // Right edge anchored past the selection's last line; flip below when
    // that would poke past the container's top edge.
    let row_right = line[0] + line[2] - container_x + SELECTION_GAP;
    let mut strip_x = row_right - strip_w;
    let mut y = line[1] - container_y - strip_h - 2.0;
    if y < 0.0 {
        y = line[1] + line[3] - container_y + 2.0;
    }
    strip_x = strip_x.clamp(0.0, (container_w - strip_w).max(0.0));
    y = y.clamp(0.0, (container_h - strip_h).max(0.0));
    let copy_x = strip_x + STRIP_PAD;
    let cite_x = copy_x + PILL_SIZE + PILL_GAP;
    // Record both hit rects at render time, from the same placement math
    // that positions the strip — deliberately generous so clicks near the
    // rounded edges still count. No paint-time probe: how Taffy sizes an
    // absolute probe inside a pill is exactly what we would be guessing at.
    {
        let mut state = selection.selection.borrow_mut();
        state.set_copy_button(Some([
            container_x + copy_x - HIT_PAD,
            container_y + y + STRIP_PAD - HIT_PAD,
            PILL_SIZE + 2.0 * HIT_PAD,
            PILL_SIZE + 2.0 * HIT_PAD,
        ]));
        state.set_cite_button(Some([
            container_x + cite_x - HIT_PAD,
            container_y + y + STRIP_PAD - HIT_PAD,
            PILL_SIZE + 2.0 * HIT_PAD,
            PILL_SIZE + 2.0 * HIT_PAD,
        ]));
    }
    div()
        .id("cite-popup-strip")
        .absolute()
        .left(px(strip_x))
        .top(px(y))
        .px(px(STRIP_PAD))
        .py(px(STRIP_PAD))
        .rounded(px(8.0))
        .border_1()
        .border_color(theme.border)
        .bg(theme.raised)
        .shadow_sm()
        .flex()
        .items_center()
        .gap(px(PILL_GAP))
        .child(popup_pill(
            "copy-popup",
            IconName::Copy,
            "Copy",
            theme.clone(),
        ))
        .child(popup_pill("cite-popup", IconName::Reply, "Quote selection", theme))
}

/// One icon button inside the popup strip. Activation lives in the selection
/// mouse handlers, so this is styling and hover only.
pub fn popup_pill(
    id: &'static str,
    icon: IconName,
    tooltip: &'static str,
    theme: Theme,
) -> impl IntoElement {
    div()
        .id(id)
        .size(px(24.0))
        .rounded(px(6.0))
        .flex()
        .items_center()
        .justify_center()
        .cursor_pointer()
        .hover(|element| element.bg(theme.overlay_strong))
        .tooltip(Tooltip::text(tooltip))
        .child(app_icon(icon, 12.0, theme.text_secondary))
}

pub fn selection_input(
    selection: TranscriptSelection,
    viewport: Rc<RefCell<Option<Bounds<Pixels>>>>,
) -> impl IntoElement {
    canvas(
        |_, _, _| (),
        move |_, _, window, _| {
            render::install_selection_input(window, &selection, Some(viewport.clone()));
        },
    )
    .absolute()
    .w(px(0.0))
    .h(px(0.0))
}
