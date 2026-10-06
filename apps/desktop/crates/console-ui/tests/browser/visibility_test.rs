use console_ui::browser::view::should_show_native as show;

// should_show_native(surface_visible, has_page, occluded, snapshot_pending, has_error)

#[test]
fn an_inactive_tab_is_never_shown() {
    // Even with a loaded page and no overlay: a load or navigation finishing on
    // an inactive tab must not put the native view above the chat.
    assert!(!show(false, true, false, false, false));
    assert!(!show(false, true, false, true, false));
}

#[test]
fn the_active_tab_with_a_page_is_shown() {
    assert!(show(true, true, false, false, false));
}

#[test]
fn a_tab_without_a_page_is_hidden() {
    assert!(!show(true, false, false, false, false));
}

#[test]
fn an_error_hides_the_native_view() {
    assert!(!show(true, true, false, false, true));
}

#[test]
fn an_overlay_hides_it_unless_a_snapshot_stands_in() {
    assert!(!show(true, true, true, false, false));
    assert!(show(true, true, true, true, false));
}
