//! Unit tests for the keyboard shortcuts settings page filter. The catalog is
//! derived from the live keymap at runtime, so these tests exercise the
//! derivation-independent parts — grouping, metadata lookup, and filtering —
//! against fixture shortcuts shaped like the real keymap output.

use console_ui::settings::{ShortcutInfo, filter_keybinding_categories, keybinding_categories};

fn fixture_shortcuts() -> Vec<ShortcutInfo> {
    [
        ("ToggleCommandPalette", vec!["⌘K"], ""),
        ("BrowserReload", vec!["⌘R"], "Browser"),
        ("NewTerminal", vec!["⇧⌘T"], ""),
        ("NewBrowser", vec!["⇧⌘B"], ""),
        ("ToggleRightSidebar", vec!["⌥B"], ""),
        ("SettleSession", vec!["⇧⌘S"], ""),
        ("BrowserDevtools", vec!["⇧⌘I"], "Browser"),
        ("WebviewCopy", vec!["⌘C"], "Browser"),
        (
            "AutocompleteConfirm",
            vec!["Tab", "Enter"],
            "Autocomplete open",
        ),
        ("UnknownAction", vec!["⌘?"], ""),
    ]
    .into_iter()
    .map(|(action, keys, context)| ShortcutInfo {
        action_name: action.to_string(),
        keys: keys.iter().map(|key| key.to_string()).collect(),
        context: context.to_string(),
    })
    .collect()
}

#[test]
fn test_empty_query_returns_all_categories() {
    let categories = filter_keybinding_categories("", &fixture_shortcuts());
    assert!(!categories.is_empty());
    assert!(categories.iter().all(|c| !c.entries.is_empty()));
}

#[test]
fn test_filter_matches_description_and_action_name() {
    let by_description = filter_keybinding_categories("reload", &fixture_shortcuts());
    assert!(
        by_description
            .iter()
            .any(|c| c.entries.iter().any(|e| e.action_name == "BrowserReload"))
    );

    let by_action = filter_keybinding_categories("browserreload", &fixture_shortcuts());
    assert!(
        by_action
            .iter()
            .any(|c| c.entries.iter().any(|e| e.action_name == "BrowserReload"))
    );
}

#[test]
fn test_filter_matches_shortcut_text() {
    for query in ["cmd+r", "ctrl+r"] {
        let categories = filter_keybinding_categories(query, &fixture_shortcuts());
        assert!(
            categories
                .iter()
                .any(|c| c.entries.iter().any(|e| e.action_name == "BrowserReload")),
            "query {query:?} should match BrowserReload"
        );
    }

    for query in ["cmd+shift+t", "ctrl+shift+t"] {
        let categories = filter_keybinding_categories(query, &fixture_shortcuts());
        assert!(
            categories
                .iter()
                .any(|c| c.entries.iter().any(|e| e.action_name == "NewTerminal")),
            "query {query:?} should match NewTerminal"
        );
    }

    for query in ["cmd+shift+b", "ctrl+shift+b"] {
        let categories = filter_keybinding_categories(query, &fixture_shortcuts());
        assert!(
            categories
                .iter()
                .any(|c| c.entries.iter().any(|e| e.action_name == "NewBrowser")),
            "query {query:?} should match NewBrowser"
        );
    }

    for query in ["alt+b", "option+b"] {
        let categories = filter_keybinding_categories(query, &fixture_shortcuts());
        assert!(
            categories.iter().any(|c| c
                .entries
                .iter()
                .any(|e| e.action_name == "ToggleRightSidebar")),
            "query {query:?} should match ToggleRightSidebar"
        );
    }
}

#[test]
fn test_settle_shortcut_is_listed_and_searchable() {
    let categories = keybinding_categories(&fixture_shortcuts());
    let entry = categories
        .iter()
        .flat_map(|c| c.entries.iter())
        .find(|e| e.action_name == "SettleSession")
        .expect("SettleSession should be listed in the shortcut catalog");
    assert_eq!(entry.context, "Global");
    assert_eq!(entry.keys, vec!["⇧⌘S"]);

    for query in ["cmd+shift+s", "ctrl+shift+s", "settle"] {
        assert!(
            filter_keybinding_categories(query, &fixture_shortcuts())
                .iter()
                .flat_map(|c| c.entries.iter())
                .any(|e| e.action_name == "SettleSession"),
            "query {query:?} should match SettleSession"
        );
    }
}

#[test]
fn test_filter_drops_empty_categories() {
    let categories = filter_keybinding_categories("devtools", &fixture_shortcuts());
    assert_eq!(categories.len(), 1);
    assert_eq!(categories[0].title, "Browser Surface");
}

#[test]
fn test_filter_with_no_match_returns_empty() {
    assert!(filter_keybinding_categories("zzz-no-such-shortcut", &fixture_shortcuts()).is_empty());
}

#[test]
fn test_synthetic_and_unknown_entries_render() {
    let categories = keybinding_categories(&fixture_shortcuts());
    let entries = || categories.iter().flat_map(|c| c.entries.iter());
    // Menu-only action surfaces via the synthetic list.
    assert!(entries().any(|e| e.action_name == "NewProject"));
    // Typed prefixes surface even though they are not keymap bindings.
    assert!(entries().any(|e| e.action_name == "ChatRefTrigger" && e.keys == vec!["$"]));
    // Unmapped actions land in Other with their action name as description.
    let unknown = entries()
        .find(|e| e.action_name == "UnknownAction")
        .expect("unknown actions should still render");
    assert_eq!(unknown.description, "UnknownAction");
}

#[test]
fn test_numbered_action_family_collapses() {
    let mut shortcuts = Vec::new();
    for n in 1..=9 {
        shortcuts.push(ShortcutInfo {
            action_name: format!("SwitchSession{n}"),
            keys: vec![format!("⌘{n}")],
            context: "Global".to_string(),
        });
    }
    let categories = keybinding_categories(&shortcuts);
    let family = categories
        .iter()
        .flat_map(|c| c.entries.iter())
        .find(|e| e.action_name.starts_with("SwitchSession"))
        .expect("numbered family should render");
    assert_eq!(family.keys.len(), 9);
    assert_eq!(family.description, "Switch session by number");
}
