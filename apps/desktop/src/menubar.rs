//! In-window menu bar for platforms without a native app menu.
//!
//! macOS keeps the native AppKit menu bar (`app_menu.rs`); Windows and Linux
//! get this GPUI-rendered strip under the title bar, built from the same
//! primitives as every other popover and dispatching the same actions the
//! keymap and (on macOS) the native menu use.
//!
//! Note: Edit-menu items dispatch the composer's actions, which only take
//! effect when a text input has focus — same semantics as the keystrokes
//! themselves.

use gpui::{
    Action, AnyElement, App, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, div, px,
};

use console_ui::primitives::ContextMenuHandle;
use console_ui::primitives::menu::{MenuAlign, popover};
use console_ui::theme::Theme;

use crate::app_menu::Quit;
use crate::keybindings::{
    AddProject, NewBrowser, NewChat, NewProject, NewTerminal, NewWindow, OpenSettings,
    QuickOpenFile,
};

/// One dropdown entry: label, right-aligned shortcut hint, and the action to
/// dispatch.
struct Entry {
    label: &'static str,
    shortcut: &'static str,
    action: Box<dyn Action>,
}

fn entry(label: &'static str, shortcut: &'static str, action: impl Action + 'static) -> Entry {
    Entry {
        label,
        shortcut,
        action: Box::new(action),
    }
}

fn separator() -> AnyElement {
    div()
        .h(px(1.0))
        .my(px(3.0))
        .bg(gpui::black().opacity(0.12))
        .into_any_element()
}

fn menu_row(theme: &Theme, id: String, entry: Entry, handle: &ContextMenuHandle) -> AnyElement {
    let handle = handle.clone();
    let shortcut_color = theme.text_ghost;
    div()
        .id(gpui::ElementId::Name(id.into()))
        .h(px(26.0))
        .px(px(8.0))
        .rounded(px(4.0))
        .flex()
        .items_center()
        .justify_between()
        .gap(px(24.0))
        .cursor_pointer()
        .hover(|s| s.bg(theme.overlay))
        .on_click(move |_, window, cx| {
            handle.close(window, cx);
            window.dispatch_action(entry.action.boxed_clone(), cx);
        })
        .child(
            div()
                .text_size(px(12.5))
                .text_color(theme.text)
                .child(entry.label),
        )
        .children((!entry.shortcut.is_empty()).then(|| {
            div()
                .text_size(px(11.0))
                .text_color(shortcut_color)
                .child(entry.shortcut)
        }))
        .into_any_element()
}

/// Indices into `entries` that render as separators instead of rows.
const FILE_SEPARATORS: &[usize] = &[5, 6, 7];
const EDIT_SEPARATORS: &[usize] = &[2];

fn dropdown_card(
    handle: &ContextMenuHandle,
    entries: Vec<Entry>,
    separators: &[usize],
    cx: &mut App,
) -> AnyElement {
    let theme = Theme::current(cx);
    let handle = handle.clone();
    div()
        .w(px(240.0))
        .p(px(6.0))
        .rounded(px(8.0))
        .bg(theme.surface)
        .border_1()
        .border_color(theme.border)
        .shadow_md()
        .flex()
        .flex_col()
        .gap(px(1.0))
        .children(entries.into_iter().enumerate().map(|(index, item)| {
            if separators.contains(&index) {
                separator()
            } else {
                menu_row(&theme, format!("menu-item-{index}"), item, &handle)
            }
        }))
        .into_any_element()
}

fn menu_trigger(id: &'static str, label: &'static str, theme: &Theme) -> gpui::Stateful<gpui::Div> {
    div()
        .id(id)
        .h(px(22.0))
        .px(px(8.0))
        .rounded(px(5.0))
        .flex()
        .items_center()
        .text_size(px(12.5))
        .text_color(theme.text_secondary)
        .cursor_pointer()
        .hover(|s| s.bg(theme.overlay).text_color(theme.text))
        .child(label)
}

fn file_entries() -> Vec<Entry> {
    let mut entries = vec![
        entry("New Window", "Ctrl+Shift+N", NewWindow),
        entry("New Chat", "Ctrl+N", NewChat),
        entry("New Terminal", "Ctrl+Shift+T", NewTerminal),
    ];
    // Browser tabs are deferred on Windows, matching the native menu.
    if !cfg!(target_os = "windows") {
        entries.push(entry("New Browser Tab", "Ctrl+Shift+B", NewBrowser));
    }
    entries.extend([
        entry("Add Project…", "Ctrl+O", AddProject),
        entry("New Project…", "", NewProject),
        entry("Quick Open File…", "Ctrl+P", QuickOpenFile),
        entry("Settings…", "Ctrl+,", OpenSettings),
        entry("Exit", "Ctrl+Q", Quit),
    ]);
    entries
}

fn edit_entries() -> Vec<Entry> {
    vec![
        entry("Undo", "Ctrl+Z", console_ui::Undo),
        entry("Redo", "Ctrl+Shift+Z", console_ui::Redo),
        entry("Cut", "Ctrl+X", console_ui::Cut),
        entry("Copy", "Ctrl+C", console_ui::Copy),
        entry("Paste", "Ctrl+V", console_ui::Paste),
        entry("Select All", "Ctrl+A", console_ui::SelectAll),
    ]
}

/// The menu bar strip. `None` on macOS, which keeps its native menu bar.
pub fn menu_bar(
    file: &ContextMenuHandle,
    edit: &ContextMenuHandle,
    cx: &App,
) -> Option<AnyElement> {
    if cfg!(target_os = "macos") {
        return None;
    }
    let theme = Theme::current(cx);
    let file = file.clone();
    let edit = edit.clone();

    Some(
        div()
            .flex()
            .items_center()
            .h(px(30.0))
            .px(px(6.0))
            .gap(px(2.0))
            .border_b_1()
            .border_color(theme.border)
            .child(popover(
                menu_trigger("menubar-file", "File", &theme),
                &file,
                MenuAlign::BelowLeft,
                move |handle, _window, cx| {
                    dropdown_card(handle, file_entries(), FILE_SEPARATORS, cx)
                },
            ))
            .child(popover(
                menu_trigger("menubar-edit", "Edit", &theme),
                &edit,
                MenuAlign::BelowLeft,
                move |handle, _window, cx| {
                    dropdown_card(handle, edit_entries(), EDIT_SEPARATORS, cx)
                },
            ))
            .into_any_element(),
    )
}
