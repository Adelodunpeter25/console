//! What a provider API-key row shows, derived from the server's status.
//!
//! The interesting claim is not the wording — it is that "a key is configured"
//! and "a key is configured *here*" are different. A key supplied by the
//! server's environment survives a remove, so the row must not offer a
//! disconnect that silently does nothing.

use console_core::types::ProviderCredentialStatus;
use console_ui::settings::KeyRowState;

fn status(configured: Option<bool>, source: Option<&str>) -> ProviderCredentialStatus {
    ProviderCredentialStatus {
        configured,
        source: source.map(str::to_string),
        updated_at: None,
    }
}

#[test]
fn no_status_reads_as_the_free_tier() {
    // Before the status request lands, or when the server reported nothing:
    // the provider still works on its free tier, so this must not look broken.
    let state = KeyRowState::from_status(None, false);
    assert!(!state.configured);
    assert!(!state.from_env);
    assert!(!state.can_remove);
    assert_eq!(state.status_text, "Using the free tier");
}

#[test]
fn an_unreported_configured_field_is_not_true() {
    // `configured` is explicit-presence on the wire, so absent means the server
    // did not say — which must not be read as "yes, a key exists".
    let state = KeyRowState::from_status(Some(&status(None, None)), false);
    assert!(!state.configured);
    assert_eq!(state.status_text, "Using the free tier");
}

#[test]
fn an_explicit_false_is_not_configured() {
    let state = KeyRowState::from_status(Some(&status(Some(false), None)), false);
    assert!(!state.configured);
    assert_eq!(state.status_text, "Using the free tier");
}

#[test]
fn a_stored_key_is_removable() {
    let state = KeyRowState::from_status(Some(&status(Some(true), Some("stored"))), false);
    assert!(state.configured);
    assert!(!state.from_env);
    assert!(state.can_remove);
    assert_eq!(state.status_text, "Using your API key");
}

#[test]
fn an_environment_key_is_configured_but_not_removable() {
    // The whole reason `source` is on the wire: removing a stored key cannot
    // remove an env-supplied one, so no remove button may be offered.
    let state = KeyRowState::from_status(Some(&status(Some(true), Some("env"))), false);
    assert!(state.configured);
    assert!(state.from_env);
    assert!(!state.can_remove, "an env key must not offer a remove");
    assert_eq!(state.status_text, "Using API key from server environment");
}

#[test]
fn busy_wins_over_the_other_states() {
    // Inside the sheet's "no key" state the row still says the free tier, but
    // a save in flight is the more urgent fact.
    let state = KeyRowState::from_status(Some(&status(Some(true), Some("stored"))), true);
    assert_eq!(state.status_text, "Saving...");
    // Busy must not change what the row claims about the key itself.
    assert!(state.configured);
    assert!(state.can_remove);
}
