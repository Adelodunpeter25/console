use console_ui::browser::cef::runtime::resolve_debug_port;

#[test]
fn valid_override_wins() {
    assert_eq!(resolve_debug_port(Some("9333")), Some(9333));
    assert_eq!(resolve_debug_port(Some(" 9444 ")), Some(9444));
}

#[test]
fn invalid_override_falls_back_to_a_free_port() {
    for raw in ["", "abc", "80", "99999", "-5"] {
        let port = resolve_debug_port(Some(raw)).expect("a free port");
        assert!(port >= 1024, "{raw:?} gave {port}");
    }
}

#[test]
fn no_override_picks_a_bindable_loopback_port() {
    let port = resolve_debug_port(None).expect("a free port");
    assert!(port >= 1024);
    // The reserved port was released, so it can be bound again.
    std::net::TcpListener::bind(("127.0.0.1", port)).expect("port is free");
}
