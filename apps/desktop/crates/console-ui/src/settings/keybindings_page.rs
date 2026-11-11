use crate::input::ComposerInput;
use crate::markdown::render::MONO_FAMILY;
use crate::primitives::icons::{IconName, app_icon};
use crate::theme::Theme;
use gpui::{App, Entity, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};

/// One shortcut row as it will render: an action, the keystroke chips for the
/// running platform, and the key context it lives in.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct KeybindingEntry {
    pub action_name: String,
    pub description: String,
    pub keys: Vec<String>,
    pub context: String,
}

pub struct KeybindingCategory {
    pub title: String,
    pub entries: Vec<KeybindingEntry>,
}

/// A binding harvested from the live keymap, before descriptions and grouping
/// are applied.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ShortcutInfo {
    pub action_name: String,
    pub keys: Vec<String>,
    pub context: String,
}

struct ActionMeta {
    action: &'static str,
    category: &'static str,
    description: &'static str,
}

const CATEGORY_ORDER: &[&str] = &[
    "Global & Window",
    "Chat & Composer",
    "Autocomplete",
    "Panels & Overlays",
    "Browser Surface",
    "Terminal & Code Viewer",
    "Other",
];

/// Human metadata only — descriptions and grouping. Key combos live in the
/// keymap and are derived at runtime, so they can never drift from this table.
const ACTION_META: &[ActionMeta] = &[
    ActionMeta {
        action: "ToggleCommandPalette",
        category: "Global & Window",
        description: "Open command palette",
    },
    ActionMeta {
        action: "ToggleTabPalette",
        category: "Global & Window",
        description: "Switch open chat or terminal tab",
    },
    ActionMeta {
        action: "QuickOpenFile",
        category: "Global & Window",
        description: "Quick open file",
    },
    ActionMeta {
        action: "AddProject",
        category: "Global & Window",
        description: "Open project browser",
    },
    ActionMeta {
        action: "NewWindow",
        category: "Global & Window",
        description: "New desktop window",
    },
    ActionMeta {
        action: "NewChat",
        category: "Global & Window",
        description: "New chat session",
    },
    ActionMeta {
        action: "SettleSession",
        category: "Global & Window",
        description: "Settle the selected chat / move it back to active",
    },
    ActionMeta {
        action: "NewTerminal",
        category: "Global & Window",
        description: "New terminal tab",
    },
    ActionMeta {
        action: "NewBrowser",
        category: "Global & Window",
        description: "New browser tab",
    },
    ActionMeta {
        action: "CloseTab",
        category: "Global & Window",
        description: "Close active tab",
    },
    ActionMeta {
        action: "ToggleLeftSidebar",
        category: "Global & Window",
        description: "Toggle left sidebar",
    },
    ActionMeta {
        action: "ToggleRightSidebar",
        category: "Global & Window",
        description: "Toggle right sidebar",
    },
    ActionMeta {
        action: "OpenSettings",
        category: "Global & Window",
        description: "Open settings window",
    },
    ActionMeta {
        action: "FocusComposer",
        category: "Global & Window",
        description: "Focus composer input",
    },
    ActionMeta {
        action: "ToggleModelPicker",
        category: "Global & Window",
        description: "Toggle model picker",
    },
    ActionMeta {
        action: "ToggleThinkingPicker",
        category: "Global & Window",
        description: "Toggle thinking mode selector",
    },
    ActionMeta {
        action: "FocusModelSearch",
        category: "Global & Window",
        description: "Focus open picker search field",
    },
    ActionMeta {
        action: "CycleApprovalMode",
        category: "Global & Window",
        description: "Cycle approval mode",
    },
    ActionMeta {
        action: "SwitchSession",
        category: "Global & Window",
        description: "Switch session by number",
    },
    ActionMeta {
        action: "SwitchWorkspaceTab",
        category: "Global & Window",
        description: "Switch workspace tab by number",
    },
    ActionMeta {
        action: "Quit",
        category: "Global & Window",
        description: "Quit the app",
    },
    ActionMeta {
        action: "HideApp",
        category: "Global & Window",
        description: "Hide the app",
    },
    ActionMeta {
        action: "HideOthers",
        category: "Global & Window",
        description: "Hide other apps",
    },
    ActionMeta {
        action: "Minimize",
        category: "Global & Window",
        description: "Minimize window",
    },
    ActionMeta {
        action: "ShowNextTab",
        category: "Global & Window",
        description: "Show next tab",
    },
    ActionMeta {
        action: "ShowPreviousTab",
        category: "Global & Window",
        description: "Show previous tab",
    },
    ActionMeta {
        action: "NewProject",
        category: "Global & Window",
        description: "Open add-project dialog (menu only)",
    },
    ActionMeta {
        action: "CheckForUpdates",
        category: "Global & Window",
        description: "Check for updates (menu only)",
    },
    ActionMeta {
        action: "Enter",
        category: "Chat & Composer",
        description: "Submit prompt / send",
    },
    ActionMeta {
        action: "Newline",
        category: "Chat & Composer",
        description: "Insert newline",
    },
    ActionMeta {
        action: "SubmitSteer",
        category: "Chat & Composer",
        description: "Steer the running turn / queue follow-up",
    },
    ActionMeta {
        action: "SelectAll",
        category: "Chat & Composer",
        description: "Select all text",
    },
    ActionMeta {
        action: "Copy",
        category: "Chat & Composer",
        description: "Copy selection",
    },
    ActionMeta {
        action: "Cut",
        category: "Chat & Composer",
        description: "Cut selection",
    },
    ActionMeta {
        action: "Paste",
        category: "Chat & Composer",
        description: "Paste",
    },
    ActionMeta {
        action: "Undo",
        category: "Chat & Composer",
        description: "Undo edit",
    },
    ActionMeta {
        action: "Redo",
        category: "Chat & Composer",
        description: "Redo edit",
    },
    ActionMeta {
        action: "Backspace",
        category: "Chat & Composer",
        description: "Delete backwards",
    },
    ActionMeta {
        action: "Delete",
        category: "Chat & Composer",
        description: "Delete forwards",
    },
    ActionMeta {
        action: "DeleteToPreviousWord",
        category: "Chat & Composer",
        description: "Delete previous word",
    },
    ActionMeta {
        action: "DeleteToNextWord",
        category: "Chat & Composer",
        description: "Delete next word",
    },
    ActionMeta {
        action: "DeleteToStart",
        category: "Chat & Composer",
        description: "Delete to line start",
    },
    ActionMeta {
        action: "DeleteToEnd",
        category: "Chat & Composer",
        description: "Delete to line end",
    },
    ActionMeta {
        action: "Left",
        category: "Chat & Composer",
        description: "Move caret left",
    },
    ActionMeta {
        action: "Right",
        category: "Chat & Composer",
        description: "Move caret right",
    },
    ActionMeta {
        action: "Up",
        category: "Chat & Composer",
        description: "Move caret up",
    },
    ActionMeta {
        action: "Down",
        category: "Chat & Composer",
        description: "Move caret down",
    },
    ActionMeta {
        action: "Home",
        category: "Chat & Composer",
        description: "Move to line / text start",
    },
    ActionMeta {
        action: "End",
        category: "Chat & Composer",
        description: "Move to line / text end",
    },
    ActionMeta {
        action: "SelectLeft",
        category: "Chat & Composer",
        description: "Extend selection left",
    },
    ActionMeta {
        action: "SelectRight",
        category: "Chat & Composer",
        description: "Extend selection right",
    },
    ActionMeta {
        action: "SelectToStart",
        category: "Chat & Composer",
        description: "Select to line / text start",
    },
    ActionMeta {
        action: "SelectToEnd",
        category: "Chat & Composer",
        description: "Select to line / text end",
    },
    ActionMeta {
        action: "SelectToPreviousWord",
        category: "Chat & Composer",
        description: "Select previous word",
    },
    ActionMeta {
        action: "SelectToNextWord",
        category: "Chat & Composer",
        description: "Select next word",
    },
    ActionMeta {
        action: "MoveToPreviousWord",
        category: "Chat & Composer",
        description: "Move to previous word",
    },
    ActionMeta {
        action: "MoveToNextWord",
        category: "Chat & Composer",
        description: "Move to next word",
    },
    ActionMeta {
        action: "AutocompleteNext",
        category: "Autocomplete",
        description: "Next suggestion",
    },
    ActionMeta {
        action: "AutocompletePrevious",
        category: "Autocomplete",
        description: "Previous suggestion",
    },
    ActionMeta {
        action: "AutocompleteConfirm",
        category: "Autocomplete",
        description: "Confirm autocomplete",
    },
    ActionMeta {
        action: "AutocompleteDismiss",
        category: "Autocomplete",
        description: "Dismiss autocomplete",
    },
    ActionMeta {
        action: "FileMentionTrigger",
        category: "Autocomplete",
        description: "Trigger file mention (typed)",
    },
    ActionMeta {
        action: "SlashCommandTrigger",
        category: "Autocomplete",
        description: "Trigger slash command (typed)",
    },
    ActionMeta {
        action: "ChatRefTrigger",
        category: "Autocomplete",
        description: "Reference another chat (typed)",
    },
    ActionMeta {
        action: "Cancel",
        category: "Panels & Overlays",
        description: "Cancel / dismiss the focused panel",
    },
    ActionMeta {
        action: "ConfirmMatch",
        category: "Panels & Overlays",
        description: "Confirm highlighted result",
    },
    ActionMeta {
        action: "SelectNext",
        category: "Panels & Overlays",
        description: "Next result / entry",
    },
    ActionMeta {
        action: "SelectPrev",
        category: "Panels & Overlays",
        description: "Previous result / entry",
    },
    ActionMeta {
        action: "SelectNextEntry",
        category: "Panels & Overlays",
        description: "Next menu entry",
    },
    ActionMeta {
        action: "SelectPreviousEntry",
        category: "Panels & Overlays",
        description: "Previous menu entry",
    },
    ActionMeta {
        action: "SelectNextTab",
        category: "Panels & Overlays",
        description: "Next menu tab",
    },
    ActionMeta {
        action: "SelectPreviousTab",
        category: "Panels & Overlays",
        description: "Previous menu tab",
    },
    ActionMeta {
        action: "ConfirmEntry",
        category: "Panels & Overlays",
        description: "Confirm menu entry",
    },
    ActionMeta {
        action: "DismissMenu",
        category: "Panels & Overlays",
        description: "Dismiss open menu / palette",
    },
    ActionMeta {
        action: "CancelSidebarSearch",
        category: "Panels & Overlays",
        description: "Cancel sidebar search",
    },
    ActionMeta {
        action: "CommitSessionRename",
        category: "Panels & Overlays",
        description: "Commit session rename",
    },
    ActionMeta {
        action: "CancelSessionRename",
        category: "Panels & Overlays",
        description: "Cancel session rename",
    },
    ActionMeta {
        action: "FocusBrowserAddress",
        category: "Browser Surface",
        description: "Focus address bar",
    },
    ActionMeta {
        action: "BrowserReload",
        category: "Browser Surface",
        description: "Reload page",
    },
    ActionMeta {
        action: "BrowserHardReload",
        category: "Browser Surface",
        description: "Hard reload (bypass cache)",
    },
    ActionMeta {
        action: "BrowserBack",
        category: "Browser Surface",
        description: "Navigate back",
    },
    ActionMeta {
        action: "BrowserForward",
        category: "Browser Surface",
        description: "Navigate forward",
    },
    ActionMeta {
        action: "BrowserStop",
        category: "Browser Surface",
        description: "Stop loading",
    },
    ActionMeta {
        action: "BrowserDevtools",
        category: "Browser Surface",
        description: "Toggle web inspector / devtools",
    },
    ActionMeta {
        action: "BrowserToggleInspect",
        category: "Browser Surface",
        description: "Toggle element inspect mode",
    },
    ActionMeta {
        action: "BrowserAddressCancel",
        category: "Browser Surface",
        description: "Cancel address edit",
    },
    ActionMeta {
        action: "WebviewCopy",
        category: "Browser Surface",
        description: "Copy webview selection",
    },
    ActionMeta {
        action: "WebviewCut",
        category: "Browser Surface",
        description: "Cut webview selection",
    },
    ActionMeta {
        action: "WebviewPaste",
        category: "Browser Surface",
        description: "Paste into webview",
    },
    ActionMeta {
        action: "WebviewSelectAll",
        category: "Browser Surface",
        description: "Select all in webview",
    },
    ActionMeta {
        action: "TerminalTab",
        category: "Terminal & Code Viewer",
        description: "Send Tab to terminal (PTY)",
    },
    ActionMeta {
        action: "TerminalShiftTab",
        category: "Terminal & Code Viewer",
        description: "Send Shift+Tab to terminal (PTY)",
    },
    ActionMeta {
        action: "CopySelection",
        category: "Terminal & Code Viewer",
        description: "Copy selection",
    },
];

/// Friendly label for a key-context predicate string. Longer matches first —
/// several contexts nest `ComposerInput` inside a parent context.
fn friendly_context(raw: &str) -> String {
    let raw = raw.trim();
    if raw.is_empty() {
        return "Global".to_string();
    }
    for (needle, label) in [
        ("ComposerAutocomplete", "Autocomplete open"),
        ("ConsoleSidebarSearch", "Sidebar search"),
        ("ConsoleSessionRename", "Session rename"),
        ("FloatingComposer", "Floating composer"),
        ("GlobalSearchPanel", "Global search panel"),
        ("BrowserAddress", "Address bar"),
        ("ComposerInput", "Composer"),
        ("Browser", "Browser"),
        ("Terminal", "Terminal"),
        ("CodeViewer", "Code viewer"),
        ("ConsoleMenu", "Menu open"),
    ] {
        if raw.contains(needle) {
            return label.to_string();
        }
    }
    raw.to_string()
}

fn key_label(key: &str, mac: bool) -> String {
    match key {
        "escape" => "Esc".to_string(),
        "backspace" => {
            if mac {
                "⌫".to_string()
            } else {
                "Backspace".to_string()
            }
        }
        "delete" => {
            if mac {
                "⌦".to_string()
            } else {
                "Delete".to_string()
            }
        }
        "left" => "←".to_string(),
        "right" => "→".to_string(),
        "up" => "↑".to_string(),
        "down" => "↓".to_string(),
        "enter" => "Enter".to_string(),
        "tab" => "Tab".to_string(),
        "space" => "Space".to_string(),
        "home" => "Home".to_string(),
        "end" => "End".to_string(),
        "pageup" => "Page Up".to_string(),
        "pagedown" => "Page Down".to_string(),
        other => other.to_string(),
    }
}

fn keystroke_label(keystroke: &gpui::KeybindingKeystroke, mac: bool) -> String {
    let modifiers = keystroke.modifiers();
    let mut label = String::new();
    if modifiers.platform {
        label.push_str(if mac { "⌘" } else { "Cmd" });
    }
    if modifiers.control {
        label.push_str("Ctrl");
    }
    if modifiers.alt {
        label.push_str(if mac { "⌥" } else { "Alt" });
    }
    if modifiers.shift {
        label.push_str(if mac { "⇧" } else { "Shift" });
    }
    label.push_str(&key_label(keystroke.key(), mac));
    label
}

/// Harvest every registered binding from the live keymap. Platform duplicate
/// bindings (mac registers both `cmd-*` and `ctrl-*` variants so either works)
/// are collapsed to the preferred one, and same-action same-context bindings
/// (Tab / Enter confirm) merge into one row.
pub fn shortcuts_from_keymap(cx: &App) -> Vec<ShortcutInfo> {
    let mac = cfg!(target_os = "macos");
    let keymap = cx.key_bindings();
    let mut shortcuts: Vec<ShortcutInfo> = Vec::new();
    for binding in keymap.borrow().bindings() {
        let action_name = binding.action().name().to_string();
        if action_name.is_empty() {
            continue;
        }
        let keys = binding
            .keystrokes()
            .iter()
            .map(|keystroke| keystroke_label(keystroke, mac))
            .collect::<Vec<_>>();
        let context = friendly_context(
            &binding
                .predicate()
                .map(|predicate| predicate.to_string())
                .unwrap_or_default(),
        );
        // Prefer the platform modifier over a cross-platform ctrl duplicate.
        let uses_platform = keys
            .iter()
            .any(|key| key.starts_with("⌘") || key.starts_with("Cmd"));
        let uses_ctrl = keys.iter().any(|key| key.starts_with("Ctrl"));
        if (mac && uses_ctrl && !uses_platform || !mac && uses_platform)
            && shortcuts
                .iter()
                .any(|existing| existing.action_name == action_name && existing.context == context)
        {
            continue;
        }
        if let Some(existing) = shortcuts
            .iter_mut()
            .find(|existing| existing.action_name == action_name && existing.context == context)
        {
            for key in keys {
                if !existing.keys.contains(&key) {
                    existing.keys.push(key);
                }
            }
        } else {
            shortcuts.push(ShortcutInfo {
                action_name,
                keys,
                context,
            });
        }
    }
    shortcuts
}

/// Entries the keymap cannot express: menu-only actions and typed prefixes.
fn synthetic_shortcuts() -> Vec<ShortcutInfo> {
    [
        ("NewProject", vec!["Menu".to_string()]),
        ("CheckForUpdates", vec!["Menu".to_string()]),
        ("FileMentionTrigger", vec!["@".to_string()]),
        ("SlashCommandTrigger", vec!["/".to_string()]),
        ("ChatRefTrigger", vec!["$".to_string()]),
    ]
    .into_iter()
    .map(|(action, keys)| ShortcutInfo {
        action_name: action.to_string(),
        keys,
        context: if action == "FileMentionTrigger"
            || action == "SlashCommandTrigger"
            || action == "ChatRefTrigger"
        {
            "Typed in composer".to_string()
        } else {
            "Global".to_string()
        },
    })
    .collect()
}

fn meta_for(action_name: &str) -> Option<(&'static str, &'static str)> {
    // Numbered actions (SwitchSession1…9, SwitchWorkspaceTab1…9) share one
    // metadata row under the unnumbered name.
    let base = action_name.trim_end_matches(|c: char| c.is_ascii_digit());
    ACTION_META
        .iter()
        .find(|meta| meta.action == base)
        .map(|meta| (meta.category, meta.description))
}

/// Collapse numbered action families (SwitchSession1…9) into a single row so
/// the page shows one "by number" entry instead of nine.
fn collapse_numbered_families(shortcuts: Vec<ShortcutInfo>) -> Vec<ShortcutInfo> {
    let base_of = |name: &str| -> Option<String> {
        let base = name.trim_end_matches(|c: char| c.is_ascii_digit());
        (base != name && base.len() >= 2).then(|| base.to_string())
    };
    let mut collapsed: Vec<ShortcutInfo> = Vec::new();
    let mut family_sizes: std::collections::HashMap<String, usize> =
        std::collections::HashMap::new();
    for shortcut in shortcuts {
        let Some(base) = base_of(&shortcut.action_name) else {
            collapsed.push(shortcut);
            continue;
        };
        let merged = collapsed.iter_mut().any(|existing| {
            base_of(&existing.action_name).as_deref() == Some(base.as_str())
                && existing.context == shortcut.context
                && {
                    for key in &shortcut.keys {
                        if !existing.keys.contains(key) {
                            existing.keys.push(key.clone());
                        }
                    }
                    true
                }
        });
        if merged {
            *family_sizes.entry(base).or_insert(1) += 1;
        } else {
            collapsed.push(shortcut);
        }
    }
    for collapsed_shortcut in &mut collapsed {
        if let Some(base) = base_of(&collapsed_shortcut.action_name)
            && family_sizes.get(&base).copied().unwrap_or(1) > 1
        {
            collapsed_shortcut.action_name = base;
        }
    }
    collapsed
}

/// Derive the display catalog from harvested shortcuts. Every shortcut renders —
/// unknown actions fall back to their action name in an "Other" bucket.
pub fn keybinding_categories(shortcuts: &[ShortcutInfo]) -> Vec<KeybindingCategory> {
    let mut catalog: Vec<ShortcutInfo> = shortcuts.to_vec();
    for synthetic in synthetic_shortcuts() {
        if !catalog
            .iter()
            .any(|shortcut| shortcut.action_name == synthetic.action_name)
        {
            catalog.push(synthetic);
        }
    }
    let catalog = collapse_numbered_families(catalog);

    let mut categories: Vec<KeybindingCategory> = CATEGORY_ORDER
        .iter()
        .map(|title| KeybindingCategory {
            title: title.to_string(),
            entries: Vec::new(),
        })
        .collect();
    let category_index = |title: &str| {
        CATEGORY_ORDER
            .iter()
            .position(|candidate| *candidate == title)
    };

    for shortcut in &catalog {
        let (category, description) = match meta_for(&shortcut.action_name) {
            Some((category, description)) => (category.to_string(), description.to_string()),
            None => ("Other".to_string(), shortcut.action_name.clone()),
        };
        let index = category_index(&category).unwrap_or_else(|| category_index("Other").unwrap());
        categories[index].entries.push(KeybindingEntry {
            action_name: shortcut.action_name.clone(),
            description,
            keys: shortcut.keys.clone(),
            context: if shortcut.context.is_empty() {
                "Global".to_string()
            } else {
                shortcut.context.clone()
            },
        });
    }
    categories.retain(|category| !category.entries.is_empty());
    categories
}

fn key_search_aliases(key: &str) -> String {
    let mut parts: Vec<String> = Vec::new();
    let mut base = String::new();
    let (mut has_cmd, mut has_alt, mut has_shift) = (false, false, false);
    for c in key.chars() {
        match c {
            '⌘' => {
                parts.push("cmd command ctrl".to_string());
                has_cmd = true;
            }
            '⌥' => {
                parts.push("alt option opt".to_string());
                has_alt = true;
            }
            '⇧' => {
                parts.push("shift".to_string());
                has_shift = true;
            }
            '⌫' => parts.push("backspace".to_string()),
            '⌦' => parts.push("delete".to_string()),
            _ => base.push(c),
        }
    }
    let letter = base.to_lowercase();
    if !letter.is_empty() {
        parts.push(letter.clone());
    }
    if has_cmd && has_shift {
        parts.push(format!(
            "cmd+shift+{letter} shift+cmd+{letter} ctrl+shift+{letter} shift+ctrl+{letter}"
        ));
    } else if has_cmd && has_alt {
        parts.push(format!("cmd+alt+{letter} alt+cmd+{letter} cmd+opt+{letter} opt+cmd+{letter} ctrl+alt+{letter} alt+ctrl+{letter}"));
    } else if has_cmd {
        parts.push(format!("cmd+{letter} ctrl+{letter} command+{letter}"));
    } else if has_alt {
        parts.push(format!("alt+{letter} opt+{letter} option+{letter}"));
    } else if has_shift {
        parts.push(format!("shift+{letter}"));
    }
    parts.join(" ")
}

fn shortcut_search_text(entry: &KeybindingEntry) -> String {
    let key_blob = entry
        .keys
        .iter()
        .map(|key| key_search_aliases(key))
        .collect::<Vec<_>>()
        .join(" ");
    format!(
        "{} {} {} {} {}",
        entry.action_name,
        entry.description,
        entry.context,
        entry.keys.join("+"),
        key_blob,
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

/// Filter the catalog by description, action name, context, or shortcut text
/// (e.g. "reload", "BrowserReload", "cmd+r"). Empty categories are dropped so
/// the page only renders matching sections.
pub fn filter_keybinding_categories(
    query: &str,
    shortcuts: &[ShortcutInfo],
) -> Vec<KeybindingCategory> {
    filter_categories(query, &keybinding_categories(shortcuts))
}

fn filter_categories(query: &str, categories: &[KeybindingCategory]) -> Vec<KeybindingCategory> {
    categories
        .iter()
        .filter_map(|category| {
            let entries: Vec<KeybindingEntry> = category
                .entries
                .iter()
                .filter(|entry| entry_matches(entry, query))
                .cloned()
                .collect();
            if entries.is_empty() {
                None
            } else {
                Some(KeybindingCategory {
                    title: category.title.clone(),
                    entries,
                })
            }
        })
        .collect()
}

/// The derived catalog is immutable once the keymap is initialized, so build it
/// once and share it — re-deriving on every frame made scrolling stutter.
#[derive(Default)]
struct ShortcutCatalogCache {
    categories: Option<std::sync::Arc<Vec<KeybindingCategory>>>,
}

impl gpui::Global for ShortcutCatalogCache {}

fn cached_categories(cx: &mut App) -> std::sync::Arc<Vec<KeybindingCategory>> {
    if !cx.has_global::<ShortcutCatalogCache>() {
        let shortcuts = shortcuts_from_keymap(cx);
        let categories = keybinding_categories(&shortcuts);
        cx.set_global(ShortcutCatalogCache {
            categories: Some(std::sync::Arc::new(categories)),
        });
    }
    cx.global::<ShortcutCatalogCache>()
        .categories
        .clone()
        .unwrap_or_default()
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
        let cached = cached_categories(cx);
        let categories: std::sync::Arc<Vec<KeybindingCategory>> =
            if self.filter_query.trim().is_empty() {
                cached
            } else {
                std::sync::Arc::new(filter_categories(&self.filter_query, &cached))
            };
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
            for category in categories.iter() {
                let rows = category.entries.iter().map(|entry| {
                    let chips = entry
                        .keys
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
