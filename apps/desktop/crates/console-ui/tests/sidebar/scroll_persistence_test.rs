use console_ui::layout::sidebar::{SidebarRowKey, resolve_scroll_offset_after_row_change};
use gpui::{ListOffset, px};

#[test]
fn expanding_section_below_scroll_offset_preserves_position() {
    let prev = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::Session("s1".into()),
        SidebarRowKey::Session("s2".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
    ];
    // User is scrolled down to s2
    let old_offset = ListOffset {
        item_ix: 2,
        offset_in_item: px(25.0),
    };
    // Project 2 is expanded, adding sessions s3 and s4 below
    let current = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::Session("s1".into()),
        SidebarRowKey::Session("s2".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
        SidebarRowKey::Session("s3".into()),
        SidebarRowKey::Session("s4".into()),
    ];

    let resolved = resolve_scroll_offset_after_row_change(old_offset, &prev, &current);
    assert_eq!(resolved.item_ix, 2);
    assert_eq!(resolved.offset_in_item, px(25.0));
}

#[test]
fn expanding_section_above_scroll_offset_shifts_anchor_index() {
    let prev = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
        SidebarRowKey::Session("s3".into()),
    ];
    // User is scrolled to Project 2
    let old_offset = ListOffset {
        item_ix: 1,
        offset_in_item: px(12.0),
    };
    // Project 1 is expanded, inserting s1 and s2 above Project 2
    let current = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::Session("s1".into()),
        SidebarRowKey::Session("s2".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
        SidebarRowKey::Session("s3".into()),
    ];

    let resolved = resolve_scroll_offset_after_row_change(old_offset, &prev, &current);
    assert_eq!(resolved.item_ix, 3);
    assert_eq!(resolved.offset_in_item, px(12.0));
}

#[test]
fn collapsing_section_above_scroll_offset_shifts_anchor_index_up() {
    let prev = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::Session("s1".into()),
        SidebarRowKey::Session("s2".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
        SidebarRowKey::Session("s3".into()),
    ];
    // User is scrolled to Project 2
    let old_offset = ListOffset {
        item_ix: 3,
        offset_in_item: px(15.0),
    };
    // Project 1 collapsed (-2 items above Project 2)
    let current = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
        SidebarRowKey::Session("s3".into()),
    ];

    let resolved = resolve_scroll_offset_after_row_change(old_offset, &prev, &current);
    assert_eq!(resolved.item_ix, 1);
    assert_eq!(resolved.offset_in_item, px(15.0));
}

#[test]
fn collapsing_section_containing_current_anchor_falls_back_to_header() {
    let prev = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::Session("s1".into()),
        SidebarRowKey::Session("s2".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
    ];
    // User is scrolled into s2
    let old_offset = ListOffset {
        item_ix: 2,
        offset_in_item: px(20.0),
    };
    // Project 1 collapsed, leaving only headers
    let current = vec![
        SidebarRowKey::ProjectHeader("p1".into()),
        SidebarRowKey::ProjectHeader("p2".into()),
    ];

    let resolved = resolve_scroll_offset_after_row_change(old_offset, &prev, &current);
    // Nearest surviving predecessor is Project 1 header at index 0
    assert_eq!(resolved.item_ix, 0);
    assert_eq!(resolved.offset_in_item, px(0.0));
}

#[test]
fn empty_lists_handled_gracefully() {
    let old_offset = ListOffset {
        item_ix: 5,
        offset_in_item: px(10.0),
    };
    let resolved = resolve_scroll_offset_after_row_change(old_offset, &[], &[]);
    assert_eq!(resolved.item_ix, 0);
    assert_eq!(resolved.offset_in_item, px(0.0));
}
