//! The suggestion popup opens above its anchor unless a surface opts in to
//! opening below it (the floating composer).

use console_ui::{AutocompletePlacement, AutocompleteView};

#[test]
fn popup_opens_above_by_default() {
    let view = AutocompleteView::new(Vec::new(), 0, false);
    assert_eq!(view.placement(), AutocompletePlacement::Above);
}

#[test]
fn below_switches_the_popup_under_its_anchor() {
    let view = AutocompleteView::new(Vec::new(), 0, false).below();
    assert_eq!(view.placement(), AutocompletePlacement::Below);
}
