//! Per-server device credentials on disk.
//!
//! Pairing is a property of a *server*, not of the app: the same install can
//! be paired to several Console servers, and a token issued by one must never
//! be sent to another. The store is keyed by the server's normalized base
//! URL, and the file holds a long-lived signed device token, so it is written
//! with mode 0600 and replaced atomically.

use console_core::utils::server_auth_store::{self, StoredServerAuth};
use std::path::Path;

fn temp_dir(name: &str) -> std::path::PathBuf {
    let dir = std::env::temp_dir().join(format!("console-server-auth-{name}"));
    let _ = std::fs::remove_dir_all(&dir);
    dir
}

fn credential(token: &str) -> StoredServerAuth {
    StoredServerAuth {
        token: token.to_string(),
        device_id: "d1".to_string(),
        device_name: "Desktop".to_string(),
        expires_at: 1762592000000,
    }
}

#[test]
fn an_unpaired_server_has_no_credential() {
    let dir = temp_dir("unpaired");
    assert!(server_auth_store::load_for(&dir, "https://a.example").is_none());
    assert!(server_auth_store::token_for(&dir, "https://a.example").is_none());
}

#[test]
fn a_credential_is_returned_for_its_own_server() {
    let dir = temp_dir("roundtrip");
    server_auth_store::save(&dir, "https://a.example", credential("ct1_a")).unwrap();
    let stored = server_auth_store::load_for(&dir, "https://a.example").unwrap();
    assert_eq!(stored.token, "ct1_a");
    assert_eq!(stored.device_id, "d1");
    assert_eq!(server_auth_store::token_for(&dir, "https://a.example").as_deref(), Some("ct1_a"));
}

#[test]
fn a_credential_is_never_served_to_another_server() {
    let dir = temp_dir("isolation");
    server_auth_store::save(&dir, "https://a.example", credential("ct1_a")).unwrap();
    assert!(server_auth_store::token_for(&dir, "https://b.example").is_none());
}

#[test]
fn the_same_server_spelled_differently_is_the_same_server() {
    let dir = temp_dir("normalize");
    server_auth_store::save(&dir, "https://a.example", credential("ct1_a")).unwrap();
    // A trailing slash and different casing are spelling, not a new server.
    assert_eq!(
        server_auth_store::token_for(&dir, "https://A.example/").as_deref(),
        Some("ct1_a"),
    );
}

#[test]
fn pairing_two_servers_keeps_both() {
    let dir = temp_dir("two");
    server_auth_store::save(&dir, "https://a.example", credential("ct1_a")).unwrap();
    server_auth_store::save(&dir, "https://b.example", credential("ct1_b")).unwrap();
    assert_eq!(
        server_auth_store::token_for(&dir, "https://a.example").as_deref(),
        Some("ct1_a"),
    );
    assert_eq!(
        server_auth_store::token_for(&dir, "https://b.example").as_deref(),
        Some("ct1_b"),
    );
}

#[test]
fn clearing_one_server_leaves_the_other() {
    let dir = temp_dir("clear");
    server_auth_store::save(&dir, "https://a.example", credential("ct1_a")).unwrap();
    server_auth_store::save(&dir, "https://b.example", credential("ct1_b")).unwrap();
    server_auth_store::clear(&dir, "https://a.example").unwrap();
    assert!(server_auth_store::token_for(&dir, "https://a.example").is_none());
    assert_eq!(
        server_auth_store::token_for(&dir, "https://b.example").as_deref(),
        Some("ct1_b"),
    );
}

#[test]
fn the_token_file_is_not_world_readable() {
    let dir = temp_dir("permissions");
    server_auth_store::save(&dir, "https://a.example", credential("ct1_a")).unwrap();
    let file = dir.join("server-auth.json");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mode = std::fs::metadata(&file).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, 0o600, "the token file must not be readable by others");
    }
    let _ = Path::new(&file);
}

#[test]
fn a_corrupt_file_reads_as_not_paired_rather_than_failing() {
    let dir = temp_dir("corrupt");
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(dir.join("server-auth.json"), b"not json").unwrap();
    assert!(server_auth_store::load_for(&dir, "https://a.example").is_none());
}
