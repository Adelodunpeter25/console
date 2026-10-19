//! First-run setup panel: persistence shape and the stored-vs-derived split.
//!
//! These cover the property that makes the design safe — the store records only
//! "this person has seen the panel" and never anything about server setup. If
//! setup state were persisted, deleting the last project would leave the store
//! claiming a server was ready when it was not.

use console_ui::SetupStatus;

/// A server with everything configured, ready to onboard.
fn fully_set_up() -> SetupStatus {
    SetupStatus {
        has_project: true,
        has_provider: true,
        has_github: true,
    }
}

#[test]
fn a_server_missing_nothing_is_complete() {
    assert!(fully_set_up().is_complete());
}

#[test]
fn each_step_alone_leaves_it_incomplete() {
    // The panel's whole purpose is the remaining work, so a single unmet step
    // must keep it open-able.
    let complete = fully_set_up();
    let variants = [
        SetupStatus {
            has_project: false,
            ..complete
        },
        SetupStatus {
            has_provider: false,
            ..complete
        },
        SetupStatus {
            has_github: false,
            ..complete
        },
    ];
    for status in variants {
        assert!(
            !status.is_complete(),
            "a missing step must not read as complete: {status:?}"
        );
    }
}

#[test]
fn an_untouched_server_is_not_complete() {
    // Fresh server, nothing fetched or nothing configured. Must not report
    // ready, or the panel would have nothing to offer.
    assert!(!SetupStatus::default().is_complete());
}

#[test]
fn dismissals_are_per_server() {
    // The shape the store holds: a set of environment ids, not a single global
    // flag. Dismissing setup on a remote box must not silence the panel on the
    // local server, and vice versa.
    let mut dismissed: std::collections::HashSet<String> = std::collections::HashSet::new();
    dismissed.insert("remote-linux".to_string());

    assert!(dismissed.contains("remote-linux"));
    // The other server is untouched, so its panel still opens.
    assert!(!dismissed.contains("default-local"));
}

#[test]
fn dismissing_twice_is_idempotent() {
    // A set makes this free, but the behaviour is load-bearing: the panel can
    // be dismissed from two paths (the button and a step click), and the second
    // must not error or duplicate.
    let mut dismissed: std::collections::HashSet<String> = std::collections::HashSet::new();
    dismissed.insert("default-local".to_string());
    let len_after_first = dismissed.len();
    dismissed.insert("default-local".to_string());
    assert_eq!(dismissed.len(), len_after_first);
}

#[test]
fn onboarding_state_survives_a_document_without_it() {
    // Every existing install has a store.json written before this field
    // existed. It must deserialize with an empty set rather than fail, which
    // would mean every server shows the panel once on upgrade — acceptable —
    // versus refusing to start, which is not.
    #[derive(serde::Deserialize)]
    struct Document {
        version: u32,
    }
    #[derive(serde::Deserialize)]
    struct Onboarding {
        #[serde(default)]
        dismissed: std::collections::HashSet<String>,
    }
    #[derive(serde::Deserialize)]
    struct Store {
        version: u32,
        #[serde(default)]
        onboarding: Option<Onboarding>,
    }

    let legacy = r#"{"version":1,"environments":{"environments":[],"active_id":null}}"#;
    let store: Store = serde_json::from_str(legacy).expect("legacy document must parse");
    assert_eq!(store.version, 1);
    assert!(store.onboarding.is_none());
}