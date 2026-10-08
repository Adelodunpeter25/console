//! Which live watch streams each inspector tab needs.

use console_ui::{AuxiliaryTab, InspectorTab, PrimaryTab};

#[test]
fn files_tab_needs_only_fs_watch() {
    let w = InspectorTab::Primary(PrimaryTab::AllFiles).watches();
    assert!(w.fs && !w.git);
}

#[test]
fn changes_tab_needs_only_git_watch() {
    let w = InspectorTab::Primary(PrimaryTab::Changes).watches();
    assert!(!w.fs && w.git);
}

#[test]
fn auxiliary_tabs_need_no_watch() {
    for tab in AuxiliaryTab::ALL {
        let w = InspectorTab::Auxiliary(tab).watches();
        assert!(!w.fs && !w.git);
    }
}

#[test]
fn default_tab_is_files() {
    assert!(InspectorTab::default().watches().fs);
}
