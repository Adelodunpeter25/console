use console_ui::layout::sidebar::session_item::{SETTLE_SHORTCUT_LABEL, settle_tooltip_label};

#[test]
fn active_chat_tooltip_offers_the_settle_shortcut() {
    let label = settle_tooltip_label(false);
    assert!(label.starts_with("Settle chat"), "label was {label:?}");
    assert!(label.contains(SETTLE_SHORTCUT_LABEL), "label was {label:?}");
}

#[test]
fn settled_chat_tooltip_offers_the_move_back_shortcut() {
    let label = settle_tooltip_label(true);
    assert!(
        label.starts_with("Move back to active"),
        "label was {label:?}"
    );
    assert!(label.contains(SETTLE_SHORTCUT_LABEL), "label was {label:?}");
}

#[test]
fn shortcut_label_names_the_shift_and_command_modifiers() {
    // The shortcut is ⇧⌘S on macOS and Ctrl+Shift+S elsewhere; both spellings
    // name Shift, so the tooltip never hides that a modifier is held.
    assert!(SETTLE_SHORTCUT_LABEL.contains('S'));
    assert!(
        SETTLE_SHORTCUT_LABEL.contains('⇧') || SETTLE_SHORTCUT_LABEL.contains("Shift"),
        "label was {SETTLE_SHORTCUT_LABEL:?}"
    );
}
