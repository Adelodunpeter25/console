use console_ui::layout::sidebar::{
    SIDEBAR_SEARCH_FIELD_CONTEXT, SIDEBAR_SEARCH_PARENT_CONTEXT, search_results_group_header,
};
use console_ui::theme::Theme;

#[test]
fn test_search_context_constants() {
    assert_eq!(SIDEBAR_SEARCH_PARENT_CONTEXT, "ConsoleSidebarSearch");
    assert_eq!(
        SIDEBAR_SEARCH_FIELD_CONTEXT,
        "ConsoleSidebarSearch > ComposerInput"
    );
}

#[test]
fn test_search_results_group_header_instantiation() {
    let theme = Theme::dark();
    let _header = search_results_group_header(theme, "Search Results");
}
