//! Unit tests for embedded browser address resolution, snapshot repacking, and file naming.
//!
//! Mirrors the test specifications from `waku/src/browser.rs`.

use console_ui::browser::{
    AddressTarget, bgra_from_bitmap, display_url, download_destination, is_secure_url,
    resolve_address, search_url,
};

#[test]
fn test_addresses_resolve_like_an_omnibox() {
    assert_eq!(
        resolve_address("https://example.com"),
        Some(AddressTarget::Url("https://example.com".into()))
    );
    assert_eq!(
        resolve_address("localhost:3000"),
        Some(AddressTarget::Url("http://localhost:3000".into()))
    );
    assert_eq!(
        resolve_address("127.0.0.1:8080/api"),
        Some(AddressTarget::Url("http://127.0.0.1:8080/api".into()))
    );
    assert_eq!(
        resolve_address("example.com/docs?q=1"),
        Some(AddressTarget::Url("https://example.com/docs?q=1".into()))
    );
    assert_eq!(
        resolve_address("about:blank"),
        Some(AddressTarget::Url("about:blank".into()))
    );
    assert_eq!(
        resolve_address("rust borrow checker"),
        Some(AddressTarget::Search("rust borrow checker".into()))
    );
    assert_eq!(
        resolve_address("what is wry"),
        Some(AddressTarget::Search("what is wry".into()))
    );
    assert_eq!(
        resolve_address("readme"),
        Some(AddressTarget::Search("readme".into()))
    );
    assert_eq!(resolve_address("   "), None);
}

#[test]
fn test_search_urls_encode_queries() {
    assert_eq!(
        search_url("rust borrow checker"),
        "https://www.google.com/search?q=rust+borrow+checker"
    );
    assert_eq!(
        search_url("a&b=c"),
        "https://www.google.com/search?q=a%26b%3Dc"
    );
}

#[test]
fn test_address_bar_hides_only_the_https_scheme() {
    assert_eq!(display_url("https://example.com/x"), "example.com/x");
    assert_eq!(
        display_url("http://localhost:3000"),
        "http://localhost:3000"
    );
    assert!(is_secure_url("https://example.com"));
    assert!(!is_secure_url("http://localhost:3000"));
}

#[test]
fn test_bitmap_repacking_reaches_bgra_from_every_snapshot_layout() {
    // One red pixel then one green pixel, expressed in each channel
    // layout NSBitmapImageRep can return for an 8-bit snapshot.
    let bgra = [0u8, 0, 255, 255, 0, 255, 0, 255];
    let rgba = [255u8, 0, 0, 255, 0, 255, 0, 255];
    let argb = [255u8, 255, 0, 0, 255, 0, 255, 0];
    let abgr = [255u8, 0, 0, 255, 255, 0, 255, 0];
    let rgb = [255u8, 0, 0, 0, 255, 0];
    let expected = vec![0u8, 0, 255, 255, 0, 255, 0, 255];

    assert_eq!(
        bgra_from_bitmap(&bgra, 2, 1, 8, 4, true, true),
        Some(expected.clone())
    );
    assert_eq!(
        bgra_from_bitmap(&rgba, 2, 1, 8, 4, false, false),
        Some(expected.clone())
    );
    assert_eq!(
        bgra_from_bitmap(&argb, 2, 1, 8, 4, true, false),
        Some(expected.clone())
    );
    assert_eq!(
        bgra_from_bitmap(&abgr, 2, 1, 8, 4, false, true),
        Some(expected.clone())
    );
    assert_eq!(
        bgra_from_bitmap(&rgb, 2, 1, 6, 3, false, false),
        Some(expected)
    );
}

#[test]
fn test_bitmap_repacking_honors_row_padding_and_rejects_bad_layouts() {
    // Two rows of one RGBA pixel with 4 bytes of row padding.
    let padded = [
        255u8, 0, 0, 255, 9, 9, 9, 9, //
        0, 255, 0, 255, 9, 9, 9, 9,
    ];
    assert_eq!(
        bgra_from_bitmap(&padded, 1, 2, 8, 4, false, false),
        Some(vec![0, 0, 255, 255, 0, 255, 0, 255])
    );
    assert_eq!(bgra_from_bitmap(&[0; 8], 2, 1, 8, 2, false, false), None);
    assert_eq!(bgra_from_bitmap(&[0; 7], 2, 1, 8, 4, false, false), None);
    assert_eq!(bgra_from_bitmap(&[], 0, 0, 0, 4, false, false), None);
}

#[test]
fn test_download_names_do_not_overwrite() {
    let temp_dir = std::env::temp_dir().join(format!("console-download-test-{}", std::process::id()));
    let _ = std::fs::create_dir_all(&temp_dir);
    let initial_file = temp_dir.join("report.pdf");
    std::fs::write(&initial_file, "content").unwrap();

    let dest = download_destination("https://example.com/report.pdf", initial_file, Some(temp_dir.clone()));
    assert!(dest.is_some());
    assert_eq!(dest.unwrap().file_name().unwrap(), "report (2).pdf");

    let _ = std::fs::remove_dir_all(&temp_dir);
}
