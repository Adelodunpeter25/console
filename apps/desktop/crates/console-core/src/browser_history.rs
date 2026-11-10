//! The built-in browser's own history: imported Chrome entries plus every
//! site visited inside Console, one deduplicated store with suggestions.
//!
//! Chrome's own database stays on the server side; the desktop holds the
//! merged copy here as a plain JSON file under the app storage directory and
//! feeds the omnibox suggestions from it.

use std::fs;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

/// One history entry: URL, title, how often it was visited.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct BrowserHistoryEntry {
    pub url: String,
    pub title: String,
    pub visit_count: i64,
}

#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct BrowserHistory {
    pub entries: Vec<BrowserHistoryEntry>,
}

/// The cap the cmux store used: plenty for suggestions, small enough that a
/// rewrite of the file stays trivial.
const MAX_ENTRIES: usize = 5000;

impl BrowserHistory {
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    /// Record one visit: the entry's count and title are updated in place,
    /// or a new entry is added. `about:` and empty URLs are ignored.
    pub fn record_visit(&mut self, url: &str, title: &str) {
        let url = url.trim().to_string();
        if url.is_empty() || url.starts_with("about:") {
            return;
        }
        let title = title.trim().to_string();
        match self.index_of(&url) {
            Some(index) => {
                let entry = &mut self.entries[index];
                entry.visit_count += 1;
                if !title.is_empty() {
                    entry.title = title;
                }
            }
            None => self.entries.push(BrowserHistoryEntry {
                url,
                title,
                visit_count: 1,
            }),
        }
        self.sort_and_cap();
    }

    /// Merge imported Chrome entries: a hit merges (additive counts, the
    /// imported title fills gaps), a miss appends.
    pub fn merge_chrome(&mut self, imported: &[BrowserHistoryEntry]) {
        for item in imported {
            if item.url.trim().is_empty() {
                continue;
            }
            match self.index_of(&item.url) {
                Some(index) => {
                    let entry = &mut self.entries[index];
                    entry.visit_count += item.visit_count;
                    if entry.title.trim().is_empty() && !item.title.trim().is_empty() {
                        entry.title = item.title.clone();
                    }
                }
                None => self.entries.push(item.clone()),
            }
        }
        self.sort_and_cap();
    }

    /// Record a title-only change for a URL already in the store.
pub fn set_title(&mut self, url: &str, title: &str) {
    let trimmed = title.trim();
    if trimmed.is_empty() {
        return;
    }
    if let Some(index) = self.index_of(url) {
        self.entries[index].title = trimmed.to_string();
    }
}

/// Top matches for an omnibox query: URL or title contains the query,
    /// unique by normalized URL, most visited first.
    pub fn suggest(&self, query: &str, limit: usize) -> Vec<BrowserHistoryEntry> {
        let query = query.trim().to_lowercase();
        if query.is_empty() {
            return Vec::new();
        }
        let mut out: Vec<BrowserHistoryEntry> = Vec::new();
        let mut seen: Vec<String> = Vec::new();
        for entry in &self.entries {
            if !entry.url.to_lowercase().contains(&query)
                && !entry.title.to_lowercase().contains(&query)
            {
                continue;
            }
            let normalized = normalize(&entry.url);
            if seen.contains(&normalized) {
                continue;
            }
            seen.push(normalized);
            out.push(entry.clone());
            if out.len() >= limit {
                break;
            }
        }
        out
    }

    fn index_of(&self, url: &str) -> Option<usize> {
        let wanted = normalize(url);
        self.entries
            .iter()
            .position(|entry| normalize(&entry.url) == wanted)
    }

    fn sort_and_cap(&mut self) {
        self.entries.sort_by_key(|b| std::cmp::Reverse(b.visit_count));
        if self.entries.len() > MAX_ENTRIES {
            self.entries.truncate(MAX_ENTRIES);
        }
    }
}

/// Lowercased, scheme and trailing slash gone — the merge/suggestion key.
fn normalize(url: &str) -> String {
    let stripped = url
        .trim()
        .trim_start_matches("http://")
        .trim_start_matches("https://");
    let without_query = stripped.split(['?', '#']).next().unwrap_or(stripped);
    without_query.trim_end_matches('/').to_lowercase()
}

/// Storage path: `CONSOLE_STATE_DIR` wins, else the app support folder the
/// desktop state file already uses — dev installs keep their own history.
pub fn storage_path() -> PathBuf {
    if let Ok(dir) = std::env::var("CONSOLE_STATE_DIR") {
        return PathBuf::from(dir).join("browser-history.json");
    }
    console_shared::path::storage_directory().join("browser-history.json")
}

/// Load from disk; a missing or unreadable file is an empty store.
pub fn load() -> BrowserHistory {
    from_path(&storage_path())
}

/// Same over a path the caller picks (tests, custom storage).
pub fn from_path(path: &std::path::Path) -> BrowserHistory {
    fs::read_to_string(path)
        .ok()
        .and_then(|text| serde_json::from_str(&text).ok())
        .unwrap_or_default()
}

/// Persist atomically; a failure is logged and left to the next save.
pub fn save(history: &BrowserHistory) {
    save_to_path(&storage_path(), history);
}

pub fn save_to_path(path: &std::path::Path, history: &BrowserHistory) {
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    let Ok(json) = serde_json::to_string_pretty(history) else {
        return;
    };
    let temp = sibling(path, ".tmp");
    match fs::write(&temp, json).and_then(|_| fs::rename(&temp, path)) {
        Ok(()) => {}
        Err(error) => log::warn!("browser history save failed: {error}"),
    }
}

fn sibling(path: &std::path::Path, suffix: &str) -> PathBuf {
    let name = path
        .file_name()
        .map(|n| n.to_string_lossy().to_string())
        .unwrap_or_else(|| "browser-history.json".to_string());
    path.with_file_name(format!("{name}{suffix}"))
}
