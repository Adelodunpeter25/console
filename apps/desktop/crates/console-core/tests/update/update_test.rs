use console_core::services::update::{asset_name, parse_manifest};

#[test]
fn only_a_strictly_higher_build_is_an_update() {
    let manifest = parse_manifest(r#"{"build":42,"sha":"abc"}"#).expect("parse");
    assert!(manifest.is_newer_than(41));
    assert!(!manifest.is_newer_than(42));
    assert!(!manifest.is_newer_than(43));
}

#[test]
fn asset_matches_the_published_zips() {
    assert_eq!(
        asset_name("macos", "aarch64"),
        Some("Console-macos-arm64.zip")
    );
    assert_eq!(asset_name("macos", "x86_64"), Some("Console-macos-x64.zip"));
    assert_eq!(
        asset_name("windows", "x86_64"),
        Some("Console-windows-x64.zip")
    );
    assert_eq!(asset_name("linux", "x86_64"), None);
}
