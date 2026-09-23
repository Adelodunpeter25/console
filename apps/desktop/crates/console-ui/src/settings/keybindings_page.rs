use crate::input::ComposerInput;
use crate::markdown::render::MONO_FAMILY;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use gpui::{App, Entity, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};

pub struct KeybindingEntry {
    pub action_name: &'static str,
    pub description: &'static str,
    pub macos_keys: &'static [&'static str],
    pub other_keys: &'static [&'static str],
    pub context: &'static str,
}

pub struct KeybindingCategory {
    pub title: &'static str,
    pub entries: Vec<KeybindingEntry>,
}

/// Static catalog mirroring the `bind_keys` calls in code. Update when bindings change:
/// - `apps/desktop/src/keybindings.rs` (`init`)
/// - `crates/console-ui/src/browser/actions.rs` (`init_browser_keybindings`)
/// - `crates/console-ui/src/terminal/actions.rs` (`init_terminal_keybindings`)
/// - `crates/console-ui/src/viewer/code_viewer.rs` (`init_code_viewer_keybindings`)
/// - `crates/console-ui/src/common/input/actions.rs` (`init_input_keybindings`)
/// - `crates/console-ui/src/common/autocomplete.rs` (`init`)
pub fn keybinding_categories() -> Vec<KeybindingCategory> {
    vec![
        KeybindingCategory {
            title: "Global & Window",
            entries: vec![
                KeybindingEntry {
                    action_name: "ToggleCommandPalette",
                    description: "Open command palette",
                    macos_keys: &["⌘", "K"],
                    other_keys: &["Ctrl", "K"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "ToggleTabPalette",
                    description: "Switch open chat or terminal tab",
                    macos_keys: &["⌘", "T"],
                    other_keys: &["Ctrl", "T"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "QuickOpenFile",
                    description: "Quick open file",
                    macos_keys: &["⌘", "P"],
                    other_keys: &["Ctrl", "P"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "AddProject",
                    description: "Open project browser",
                    macos_keys: &["⌘", "O"],
                    other_keys: &["Ctrl", "O"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "NewWindow",
                    description: "New desktop window",
                    macos_keys: &["⇧", "⌘", "N"],
                    other_keys: &["Ctrl", "Shift", "N"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "NewChat",
                    description: "New chat session",
                    macos_keys: &["⌘", "N"],
                    other_keys: &["Ctrl", "N"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "NewTerminal",
                    description: "New terminal tab",
                    macos_keys: &["⇧", "⌘", "T"],
                    other_keys: &["Ctrl", "Shift", "T"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "NewBrowser",
                    description: "New browser tab",
                    macos_keys: &["⇧", "⌘", "B"],
                    other_keys: &["Ctrl", "Shift", "B"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "CloseTab",
                    description: "Close active tab",
                    macos_keys: &["⌘", "W"],
                    other_keys: &["Ctrl", "W"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "ToggleLeftSidebar",
                    description: "Toggle left sidebar",
                    macos_keys: &["⌘", "B"],
                    other_keys: &["Ctrl", "B"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "ToggleRightSidebar",
                    description: "Toggle right sidebar",
                    macos_keys: &["⌥", "B"],
                    other_keys: &["Alt", "B"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "OpenSettings",
                    description: "Open settings window",
                    macos_keys: &["⌘", ","],
                    other_keys: &["Ctrl", ","],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "FocusComposer",
                    description: "Focus composer input",
                    macos_keys: &["⌘", "L"],
                    other_keys: &["Ctrl", "L"],
                    context: "Global (browser address bar wins on browser focus)",
                },
                KeybindingEntry {
                    action_name: "ToggleModelPicker",
                    description: "Toggle model picker",
                    macos_keys: &["⌘", "/"],
                    other_keys: &["Ctrl", "/"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "FocusModelSearch",
                    description: "Focus open picker search field",
                    macos_keys: &["/"],
                    other_keys: &["/"],
                    context: "Menu open",
                },
                KeybindingEntry {
                    action_name: "CycleApprovalMode",
                    description: "Cycle approval mode",
                    macos_keys: &["⇧", "Tab"],
                    other_keys: &["Shift", "Tab"],
                    context: "Composer focused",
                },
                KeybindingEntry {
                    action_name: "SwitchSession1..9",
                    description: "Switch session 1–9",
                    macos_keys: &["⌘", "1..9"],
                    other_keys: &["Ctrl", "1..9"],
                    context: "Global",
                },
                KeybindingEntry {
                    action_name: "SwitchWorkspaceTab1..9",
                    description: "Switch workspace tab 1–9",
                    macos_keys: &["⌥", "1..9"],
                    other_keys: &["Alt", "1..9"],
                    context: "Global",
                },
            ],
        },
        KeybindingCategory {
            title: "Chat & Composer",
            entries: vec![
                KeybindingEntry {
                    action_name: "Enter",
                    description: "Submit prompt / send",
                    macos_keys: &["Enter"],
                    other_keys: &["Enter"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Newline",
                    description: "Insert newline",
                    macos_keys: &["⇧", "Enter"],
                    other_keys: &["Shift", "Enter"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "SubmitSteer",
                    description: "Submit steer / queue follow-up while running",
                    macos_keys: &["⌘", "Enter"],
                    other_keys: &["Ctrl", "Enter"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "SelectAll",
                    description: "Select all composer text",
                    macos_keys: &["⌘", "A"],
                    other_keys: &["Ctrl", "A"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Copy",
                    description: "Copy composer selection",
                    macos_keys: &["⌘", "C"],
                    other_keys: &["Ctrl", "C"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Cut",
                    description: "Cut composer selection",
                    macos_keys: &["⌘", "X"],
                    other_keys: &["Ctrl", "X"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Paste",
                    description: "Paste into composer",
                    macos_keys: &["⌘", "V"],
                    other_keys: &["Ctrl", "V"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Undo",
                    description: "Undo composer edit",
                    macos_keys: &["⌘", "Z"],
                    other_keys: &["Ctrl", "Z"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Redo",
                    description: "Redo composer edit",
                    macos_keys: &["⇧", "⌘", "Z"],
                    other_keys: &["Ctrl", "Shift", "Z"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Move / Select",
                    description: "Move caret, extend selection (arrows, Shift, Home/End)",
                    macos_keys: &["←", "→", "↑", "↓"],
                    other_keys: &["←", "→", "↑", "↓"],
                    context: "Composer",
                },
                KeybindingEntry {
                    action_name: "Word / Line edit",
                    description: "Word jumps and deletes (Opt), line deletes (⌘⌫)",
                    macos_keys: &["⌥", "←/→/⌫"],
                    other_keys: &["Ctrl", "←/→"],
                    context: "Composer",
                },
            ],
        },
        KeybindingCategory {
            title: "Autocomplete",
            entries: vec![
                KeybindingEntry {
                    action_name: "AutocompleteNext",
                    description: "Next suggestion",
                    macos_keys: &["↓"],
                    other_keys: &["↓"],
                    context: "Autocomplete open",
                },
                KeybindingEntry {
                    action_name: "AutocompletePrevious",
                    description: "Previous suggestion",
                    macos_keys: &["↑"],
                    other_keys: &["↑"],
                    context: "Autocomplete open",
                },
                KeybindingEntry {
                    action_name: "AutocompleteConfirm",
                    description: "Confirm autocomplete",
                    macos_keys: &["Tab", "/ Enter"],
                    other_keys: &["Tab", "/ Enter"],
                    context: "Autocomplete open",
                },
                KeybindingEntry {
                    action_name: "AutocompleteDismiss",
                    description: "Dismiss autocomplete",
                    macos_keys: &["Esc"],
                    other_keys: &["Esc"],
                    context: "Autocomplete open",
                },
                KeybindingEntry {
                    action_name: "File mention trigger",
                    description: "Trigger file mention",
                    macos_keys: &["@"],
                    other_keys: &["@"],
                    context: "Typed in composer",
                },
                KeybindingEntry {
                    action_name: "Slash command trigger",
                    description: "Trigger slash command",
                    macos_keys: &["/"],
                    other_keys: &["/"],
                    context: "Typed in composer",
                },
            ],
        },
        KeybindingCategory {
            title: "Browser Surface",
            entries: vec![
                KeybindingEntry {
                    action_name: "FocusBrowserAddress",
                    description: "Focus address bar",
                    macos_keys: &["⌘", "L"],
                    other_keys: &["Ctrl", "L"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "BrowserReload",
                    description: "Reload page",
                    macos_keys: &["⌘", "R"],
                    other_keys: &["Ctrl", "R"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "BrowserHardReload",
                    description: "Hard reload (bypass cache)",
                    macos_keys: &["⇧", "⌘", "R"],
                    other_keys: &["Ctrl", "Shift", "R"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "BrowserBack",
                    description: "Navigate back",
                    macos_keys: &["⌘", "["],
                    other_keys: &["Ctrl", "["],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "BrowserForward",
                    description: "Navigate forward",
                    macos_keys: &["⌘", "]"],
                    other_keys: &["Ctrl", "]"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "BrowserStop",
                    description: "Stop loading",
                    macos_keys: &["Esc"],
                    other_keys: &["Esc"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "BrowserDevtools",
                    description: "Toggle web inspector / devtools",
                    macos_keys: &["⇧", "⌘", "I"],
                    other_keys: &["Ctrl", "Shift", "I"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "BrowserAddressCancel",
                    description: "Cancel address edit",
                    macos_keys: &["Esc"],
                    other_keys: &["Esc"],
                    context: "Address bar",
                },
                KeybindingEntry {
                    action_name: "WebviewCopy",
                    description: "Copy webview selection",
                    macos_keys: &["⌘", "C"],
                    other_keys: &["Ctrl", "C"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "WebviewCut",
                    description: "Cut webview selection",
                    macos_keys: &["⌘", "X"],
                    other_keys: &["Ctrl", "X"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "WebviewPaste",
                    description: "Paste into webview",
                    macos_keys: &["⌘", "V"],
                    other_keys: &["Ctrl", "V"],
                    context: "Browser",
                },
                KeybindingEntry {
                    action_name: "WebviewSelectAll",
                    description: "Select all in webview",
                    macos_keys: &["⌘", "A"],
                    other_keys: &["Ctrl", "A"],
                    context: "Browser",
                },
            ],
        },
        KeybindingCategory {
            title: "Terminal & Code Viewer",
            entries: vec![
                KeybindingEntry {
                    action_name: "TerminalTab",
                    description: "Send Tab to terminal (PTY)",
                    macos_keys: &["Tab"],
                    other_keys: &["Tab"],
                    context: "Terminal",
                },
                KeybindingEntry {
                    action_name: "TerminalShiftTab",
                    description: "Send Shift+Tab to terminal (PTY)",
                    macos_keys: &["⇧", "Tab"],
                    other_keys: &["Shift", "Tab"],
                    context: "Terminal",
                },
                KeybindingEntry {
                    action_name: "CopySelection",
                    description: "Copy code viewer selection",
                    macos_keys: &["⌘", "C"],
                    other_keys: &["Ctrl", "C"],
                    context: "Code viewer",
                },
                KeybindingEntry {
                    action_name: "SelectAll",
                    description: "Select all in code viewer",
                    macos_keys: &["⌘", "A"],
                    other_keys: &["Ctrl", "A"],
                    context: "Code viewer",
                },
            ],
        },
    ]
}

fn shortcut_search_text(entry: &KeybindingEntry) -> String {
    let macos = entry.macos_keys.join(" ");
    let other = entry.other_keys.join(" ");

    let has_shift = entry.macos_keys.contains(&"⇧") || entry.other_keys.contains(&"Shift");
    let has_cmd = entry.macos_keys.contains(&"⌘") || entry.other_keys.contains(&"Ctrl");
    let has_alt = entry.macos_keys.contains(&"⌥") || entry.other_keys.contains(&"Alt");
    let key_letter = entry
        .macos_keys
        .last()
        .copied()
        .unwrap_or("")
        .to_lowercase();

    let mut extra = String::new();
    if has_cmd && has_shift {
        extra.push_str(&format!(
            " cmd+shift+{key_letter} shift+cmd+{key_letter} ctrl+shift+{key_letter} shift+ctrl+{key_letter}"
        ));
    }
    if has_cmd && has_alt {
        extra.push_str(&format!(
            " cmd+alt+{key_letter} alt+cmd+{key_letter} cmd+opt+{key_letter} opt+cmd+{key_letter} cmd+option+{key_letter} option+cmd+{key_letter} ctrl+alt+{key_letter} alt+ctrl+{key_letter}"
        ));
    }
    if has_alt && !has_cmd && !has_shift {
        extra.push_str(&format!(
            " alt+{key_letter} opt+{key_letter} option+{key_letter}"
        ));
    }
    if has_cmd && !has_alt && !has_shift {
        extra.push_str(&format!(
            " cmd+{key_letter} ctrl+{key_letter} command+{key_letter}"
        ));
    }

    format!(
        "{} {} {} {} {} {} {} {}",
        entry.action_name,
        entry.description,
        entry.context,
        macos,
        other,
        other.to_lowercase().replace(' ', "+"),
        entry.macos_keys.join("+"),
        extra,
    )
    .to_lowercase()
}

fn entry_matches(entry: &KeybindingEntry, query: &str) -> bool {
    let query = query.trim().to_lowercase();
    if query.is_empty() {
        return true;
    }
    shortcut_search_text(entry).contains(&query)
}

/// Filter the static catalog by description, action name, context, or shortcut
/// text (e.g. "reload", "BrowserReload", "cmd+r", "ctrl+r"). Empty categories
/// are dropped so the page only renders matching sections.
pub fn filter_keybinding_categories(query: &str) -> Vec<KeybindingCategory> {
    keybinding_categories()
        .into_iter()
        .filter_map(|category| {
            let entries = category
                .entries
                .into_iter()
                .filter(|entry| entry_matches(entry, query))
                .collect::<Vec<_>>();
            if entries.is_empty() {
                None
            } else {
                Some(KeybindingCategory {
                    title: category.title,
                    entries,
                })
            }
        })
        .collect()
}

#[derive(IntoElement)]
pub struct KeybindingsPage {
    pub filter_query: String,
    pub search_input: Option<Entity<ComposerInput>>,
}

fn key_chip(key: &str, theme: &Theme) -> impl IntoElement {
    div()
        .px(px(6.0))
        .py(px(2.0))
        .rounded(px(5.0))
        .border_1()
        .border_color(theme.border)
        .bg(theme.surface)
        .text_size(px(11.5))
        .font_family(MONO_FAMILY)
        .text_color(theme.text)
        .child(key.to_string())
}

impl RenderOnce for KeybindingsPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let categories = filter_keybinding_categories(&self.filter_query);
        let is_empty = categories.is_empty();

        let mut content = div().flex().flex_col().gap(px(16.0)).child(
            div()
                .flex()
                .flex_col()
                .gap(px(4.0))
                .child(
                    div()
                        .text_size(px(16.0))
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .text_color(theme.text)
                        .child("Keyboard Shortcuts"),
                )
                .child(
                    div()
                        .text_size(px(12.5))
                        .text_color(theme.text_secondary)
                        .child(
                            "Every shortcut registered in the app, grouped by surface. Read-only.",
                        ),
                ),
        );

        if let Some(search) = self.search_input {
            content = content.child(
                div()
                    .flex()
                    .items_center()
                    .gap(px(6.0))
                    .h(px(30.0))
                    .px(px(8.0))
                    .rounded(px(7.0))
                    .border_1()
                    .border_color(theme.border_strong)
                    .bg(theme.inset)
                    .child(app_icon(IconName::Search, 13.0, theme.text_ghost))
                    .child(div().flex_1().min_w_0().child(search)),
            );
        }

        if is_empty {
            content = content.child(
                div()
                    .p(px(32.0))
                    .rounded(px(8.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.surface)
                    .flex()
                    .flex_col()
                    .items_center()
                    .justify_center()
                    .gap(px(8.0))
                    .child(app_icon(IconName::Search, 24.0, theme.text_ghost))
                    .child(
                        div()
                            .text_size(px(13.0))
                            .text_color(theme.text_secondary)
                            .child("No shortcuts match this filter."),
                    ),
            );
        } else {
            for category in categories {
                let rows = category.entries.into_iter().map(|entry| {
                    let chips = entry
                        .macos_keys
                        .iter()
                        .map(|key| key_chip(key, &theme))
                        .collect::<Vec<_>>();
                    div()
                        .py(px(7.0))
                        .border_b_1()
                        .border_color(theme.border)
                        .flex()
                        .items_center()
                        .justify_between()
                        .gap(px(12.0))
                        .child(
                            div()
                                .flex()
                                .flex_col()
                                .gap(px(2.0))
                                .min_w(px(0.0))
                                .flex_1()
                                .child(
                                    div()
                                        .text_size(px(13.0))
                                        .text_color(theme.text)
                                        .truncate()
                                        .child(entry.description.to_string()),
                                )
                                .child(
                                    div()
                                        .text_size(px(11.0))
                                        .font_family(MONO_FAMILY)
                                        .text_color(theme.text_tertiary)
                                        .child(format!(
                                            "{} · {}",
                                            entry.action_name, entry.context
                                        )),
                                ),
                        )
                        .child(div().flex().items_center().gap(px(4.0)).children(chips))
                });
                content = content.child(
                    div()
                        .flex()
                        .flex_col()
                        .gap(px(2.0))
                        .child(
                            div()
                                .text_size(px(12.0))
                                .font_weight(gpui::FontWeight::SEMIBOLD)
                                .text_color(theme.text_secondary)
                                .child(category.title.to_string()),
                        )
                        .child(
                            div()
                                .px(px(12.0))
                                .rounded(px(8.0))
                                .border_1()
                                .border_color(theme.border)
                                .bg(theme.raised)
                                .children(rows),
                        ),
                );
            }
        }

        content
    }
}
