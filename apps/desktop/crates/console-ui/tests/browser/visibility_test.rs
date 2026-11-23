use console_ui::browser::should_show_native as show;

// should_show_native(surface_visible, has_page, has_error)

#[test]
fn an_inactive_tab_is_never_shown() {
    // Even with a loaded page: a load or navigation finishing on an inactive
    // tab must not put the native view above the chat.
    assert!(!show(false, true, false));
}

#[test]
fn the_active_tab_with_a_page_is_shown() {
    assert!(show(true, true, false));
}

#[test]
fn a_tab_without_a_page_is_hidden() {
    assert!(!show(true, false, false));
}

#[test]
fn an_error_hides_the_native_view() {
    assert!(!show(true, true, true));
}
