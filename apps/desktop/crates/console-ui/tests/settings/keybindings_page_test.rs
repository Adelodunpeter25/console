//! Unit tests for the keyboard shortcuts settings page filter.

use console_ui::settings::filter_keybinding_categories;

#[test]
fn test_empty_query_returns_all_categories() {
    let categories = filter_keybinding_categories("");
    assert_eq!(categories.len(), 5);
    assert!(categories.iter().all(|c| !c.entries.is_empty()));
}

#[test]
fn test_filter_matches_description_and_action_name() {
    let by_description = filter_keybinding_categories("reload");
    assert!(
        by_description
            .iter()
            .any(|c| c.entries.iter().any(|e| e.action_name == "BrowserReload"))
    );

    let by_action = filter_keybinding_categories("browserreload");
    assert!(
        by_action
            .iter()
            .any(|c| c.entries.iter().any(|e| e.action_name == "BrowserReload"))
    );
}

#[test]
fn test_filter_matches_shortcut_text() {
    for query in ["cmd+r", "ctrl+r"] {
        let categories = filter_keybinding_categories(query);
        assert!(
            categories
                .iter()
                .any(|c| c.entries.iter().any(|e| e.action_name == "BrowserReload")),
            "query {query:?} should match BrowserReload"
        );
    }
}

#[test]
fn test_filter_drops_empty_categories() {
    let categories = filter_keybinding_categories("devtools");
    assert_eq!(categories.len(), 1);
    assert_eq!(categories[0].title, "Browser Surface");
}

#[test]
fn test_filter_with_no_match_returns_empty() {
    assert!(filter_keybinding_categories("zzz-no-such-shortcut").is_empty());
}
