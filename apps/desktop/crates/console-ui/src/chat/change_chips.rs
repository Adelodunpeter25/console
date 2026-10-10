//! The changed-file chip row under a turn's final assistant message: one chip
//! per file (icon, name, `+N -N`, hover diff popover), and a fold toggle
//! ("+N more" / "Show less") once the turn touched more files than fit.

use std::rc::Rc;

use console_core::types::SessionFileChange;
use gpui::{
    App, ElementId, IntoElement, ParentElement, Stateful, StatefulInteractiveElement, Styled,
    Window, div, prelude::*, px,
};

use crate::primitives::{IconName, app_icon, base_name, file_type_icon};
use crate::theme::Theme;

use super::diff_popover::with_diff_popover;

/// A turn's chip row shows this many chips while collapsed; beyond
/// `COLLAPSED_CHIPS + 1` files the rest fold into one "+N more" chip (a
/// "+1 more" chip would hide no more than the file it replaces).
const COLLAPSED_CHIPS: usize = 3;

/// Opens the diff of one changed file: `(path, diff_text)`.
pub type OpenChangeHandler = Rc<dyn Fn(String, Option<String>, &mut Window, &mut App) + 'static>;

/// Expands or collapses the chip row.
pub type ToggleChangesHandler = Rc<dyn Fn(&mut Window, &mut App) + 'static>;

/// Build the chip row for one turn, or `None` when it changed no files.
pub fn turn_change_chips(
    key: &str,
    changes: &[SessionFileChange],
    expanded: bool,
    on_open: Option<OpenChangeHandler>,
    on_toggle: Option<ToggleChangesHandler>,
    theme: &Theme,
    window: &mut Window,
    cx: &mut App,
) -> Option<impl IntoElement + use<>> {
    if changes.is_empty() {
        return None;
    }
    let foldable = changes.len() > COLLAPSED_CHIPS + 1;
    let shown = if foldable && !expanded {
        COLLAPSED_CHIPS
    } else {
        changes.len()
    };
    let mut chips: Vec<Stateful<gpui::Div>> = changes
        .iter()
        .take(shown)
        .enumerate()
        .map(|(i, change)| {
            let chip_key = format!("{key}-{i}");
            let chip = file_change_chip(&chip_key, change, theme, on_open.clone());
            with_diff_popover(chip, &chip_key, change, theme, window, cx)
        })
        .collect();
    if foldable {
        chips.push(more_changes_chip(
            key,
            changes.len() - COLLAPSED_CHIPS,
            expanded,
            theme,
            on_toggle,
        ));
    }
    Some(
        div()
            .flex()
            .flex_wrap()
            .gap(px(6.0))
            .pt(px(4.0))
            .children(chips),
    )
}

/// One changed-file chip: file-type icon, name, `+N -N`. A deleted file's
/// name is struck through. Clicking opens that file's diff for the turn.
fn file_change_chip(
    key: &str,
    change: &SessionFileChange,
    theme: &Theme,
    on_open: Option<OpenChangeHandler>,
) -> Stateful<gpui::Div> {
    let name = base_name(&change.path).to_string();
    let deleted = change.status == "deleted";
    let path = change.path.clone();
    let diff_text = change.diff_text.clone();
    div()
        .id(ElementId::Name(format!("turn-change-{key}").into()))
        .flex()
        .items_center()
        .gap(px(6.0))
        .px(px(8.0))
        .py(px(3.0))
        .rounded(px(7.0))
        .border_1()
        .border_color(theme.border_strong)
        .cursor_pointer()
        .hover(|style| style.bg(theme.overlay))
        .child(file_type_icon(&change.path, 14.0))
        .child(
            div()
                .text_size(px(12.0))
                .text_color(if deleted {
                    theme.text_tertiary
                } else {
                    theme.text_secondary
                })
                .when(deleted, |element| element.line_through())
                .child(name),
        )
        .child(
            div()
                .flex()
                .items_center()
                .gap(px(4.0))
                .text_size(px(11.0))
                .font_weight(gpui::FontWeight::SEMIBOLD)
                .child(
                    div()
                        .text_color(theme.success)
                        .child(format!("+{}", change.additions)),
                )
                .child(
                    div()
                        .text_color(theme.danger)
                        .child(format!("-{}", change.deletions)),
                ),
        )
        .when_some(on_open, move |element, on_open| {
            element.on_click(move |_, window, cx| {
                cx.stop_propagation();
                (on_open)(path.clone(), diff_text.clone(), window, cx);
            })
        })
}

/// The fold toggle after a long chip row: "+N more" while collapsed, and the
/// same chip reading "Show less" once expanded. No hover popover.
fn more_changes_chip(
    key: &str,
    hidden: usize,
    expanded: bool,
    theme: &Theme,
    on_toggle: Option<ToggleChangesHandler>,
) -> Stateful<gpui::Div> {
    div()
        .id(ElementId::Name(format!("turn-change-more-{key}").into()))
        .flex()
        .items_center()
        .gap(px(6.0))
        .px(px(8.0))
        .py(px(3.0))
        .rounded(px(7.0))
        .border_1()
        .border_color(theme.border_strong)
        .cursor_pointer()
        .hover(|style| style.bg(theme.overlay))
        .child(app_icon(
            if expanded {
                IconName::Minus
            } else {
                IconName::Plus
            },
            12.0,
            theme.text_tertiary,
        ))
        .child(
            div()
                .text_size(px(12.0))
                .text_color(theme.text_secondary)
                .child(if expanded {
                    "Show less".to_string()
                } else {
                    format!("{hidden} more")
                }),
        )
        .when_some(on_toggle, |element, on_toggle| {
            element.on_click(move |_, window, cx| {
                cx.stop_propagation();
                (on_toggle)(window, cx);
            })
        })
}
