//! WebTransport trust roots: an empty platform store (Android) must fall back
//! to the bundled roots instead of trusting nothing.

use console_core::net::wt_root_store;
use wtransport::tls::rustls::RootCertStore;

#[test]
fn empty_platform_store_falls_back_to_bundled_roots() {
    let roots = wt_root_store(RootCertStore::empty());
    assert!(
        !roots.is_empty(),
        "an empty platform store must not leave the client trusting nothing"
    );
}

#[test]
fn populated_platform_store_is_used_unchanged() {
    let mut native = RootCertStore::empty();
    native.extend(webpki_roots::TLS_SERVER_ROOTS.iter().take(3).cloned());
    let roots = wt_root_store(native);
    assert_eq!(roots.len(), 3, "platform roots must not be augmented");
}
