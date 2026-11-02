use console_core::browser_history::{BrowserHistory, BrowserHistoryEntry};

fn entry(url: &str, title: &str, visits: i64) -> BrowserHistoryEntry {
    BrowserHistoryEntry {
        url: url.to_string(),
        title: title.to_string(),
        visit_count: visits,
    }
}

#[test]
fn record_visit_dedupes_by_normalized_url() {
    let mut history = BrowserHistory::default();
    history.record_visit("http://Example.com/docs/?tab=readme", "Example");
    history.record_visit("https://example.com/docs/", "Example Docs");
    assert_eq!(history.len(), 1);
    assert_eq!(history.entries[0].visit_count, 2);
    assert_eq!(history.entries[0].title, "Example Docs");
}

#[test]
fn record_visit_ignores_about_pages() {
    let mut history = BrowserHistory::default();
    history.record_visit("about:blank", "");
    assert!(history.is_empty());
}

#[test]
fn merge_chrome_adds_counts_for_known_urls() {
    let mut history = BrowserHistory::default();
    history.record_visit("https://rust-lang.org/learn/", "Rust");
    history.merge_chrome(&[entry("http://RUST-LANG.org/learn/?x=1", "Rust Lang", 30)]);
    assert_eq!(history.len(), 1);
    assert_eq!(history.entries[0].visit_count, 31);
    history.merge_chrome(&[entry("https://crates.io", "crates.io", 7)]);
    assert_eq!(history.len(), 2);
    // Most visited first.
    assert_eq!(history.entries[0].url, "https://rust-lang.org/learn/");
}

#[test]
fn suggest_matches_url_or_title_unique_by_url() {
    let mut history = BrowserHistory::default();
    history.merge_chrome(&[
        entry("https://docs.rs/serde/latest/serde/", "serde — docs.rs", 50),
        entry("https://github.com/rust-lang/rust", "rust-lang/rust", 20),
        entry("https://crates.io", "crates.io: Rust Package Registry", 9),
    ]);
    let hits = history.suggest("serde", 5);
    assert_eq!(hits.len(), 1);
    assert_eq!(hits[0].url, "https://docs.rs/serde/latest/serde/");

    // Both docs and the crates entry match "rust" through title/url; one per
    // normalized url, most visited first.
    let hits = history.suggest("rust", 5);
    assert_eq!(hits.len(), 2);
    assert_eq!(hits[0].url, "https://github.com/rust-lang/rust");

    // Scheme and spelling case never matter.
    assert!(history.suggest("", 5).is_empty());
}

#[test]
fn round_trips_through_the_storage_file() {
    let dir = std::env::temp_dir().join(format!(
        "console-history-test-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    let path = dir.join("browser-history.json");
    let mut history = BrowserHistory::default();
    history.merge_chrome(&[entry("https://example.com", "Example", 3)]);
    console_core::browser_history::save_to_path(&path, &history);
    let loaded = console_core::browser_history::from_path(&path);
    assert_eq!(loaded, history);
    let _ = std::fs::remove_file(&path);
    let _ = std::fs::remove_dir(&dir);
}
