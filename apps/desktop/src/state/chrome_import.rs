//! Chrome import (Browser settings tab): profile fetch, one-shot import, and
//! the local follow-through — history into the browser store, cookies into
//! the webview.

use gpui::Context;

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Load Chrome profiles from the server (the machine the daemon runs
    /// on). Called when the Browser tab is selected.
    pub fn fetch_chrome_profiles(&mut self, cx: &mut Context<Self>) {
        if self.chrome_profiles_loading || self.chrome_importing {
            return;
        }
        self.chrome_profiles_loading = true;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let profiles = client.chrome.profiles().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.chrome_profiles_loading = false;
                        match profiles {
                            Ok(list) => {
                                // Keep the selection valid across refetches.
                                let previous = this
                                    .chrome_selected_profile
                                    .and_then(|index| this.chrome_profiles.get(index).cloned());
                                this.chrome_profiles = list;
                                this.chrome_selected_profile = this
                                    .chrome_profiles
                                    .iter()
                                    .position(|p| Some(p) == previous.as_ref())
                                    .or(if this.chrome_profiles.is_empty() {
                                        None
                                    } else {
                                        Some(0)
                                    });
                            }
                            Err(error) => {
                                log::warn!("Failed to fetch Chrome profiles: {error}");
                                this.chrome_status = Some(format!("Could not load profiles: {error}"));
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    pub fn select_chrome_profile(&mut self, index: usize, cx: &mut Context<Self>) {
        self.chrome_selected_profile = Some(index);
        cx.notify();
    }

    /// Run the import for the selected profile: server fetch, then on the
    /// main thread — where WKWebView lives — merge the history into the
    /// browser store and inject the cookies into the webview jar.
    pub fn import_chrome_profile(&mut self, cx: &mut Context<Self>) {
        if self.chrome_importing {
            return;
        }
        let Some(index) = self.chrome_selected_profile else { return };
        let Some(profile) = self.chrome_profiles.get(index) else { return };
        let profile_id = profile.id.clone();
        let profile_name = profile.name.clone();

        self.chrome_importing = true;
        self.chrome_status = Some(format!(
            "Importing from \"{}\"… (history, then cookies; macOS may ask for keychain access)",
            profile_name
        ));
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let imported = client.chrome.import(&profile_id, true, true).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.chrome_importing = false;
                        match imported {
                            Ok(data) => {
                                let visit_count = data.history.len();
                                let (cookie_count, cookie_note) =
                                    crate::state::chrome_import::install(
                                        &data.history,
                                        &data.cookies,
                                    );
                                this.chrome_status = Some(match cookie_note {
                                    None => format!(
                                        "Imported: {visit_count} Chrome visits ({} sites), {cookie_count} cookies.",
                                        console_ui::browser::browser_state::history_len()
                                    ),
                                    Some(error) => format!(
                                        "Imported {visit_count} Chrome visits ({} sites), but cookies could not be injected: {error}",
                                        console_ui::browser::browser_state::history_len()
                                    ),
                                });
                            }
                            Err(error) => {
                                this.chrome_status = Some(format!(
                                    "Import failed: {error}"
                                ));
                            }
                        }
                        cx.notify();
                    });
                }
            });
            // The cookie store settles asynchronously; the read-back that
            // says what WebKit actually kept runs a couple of seconds later.
            // WebKit-only: the cookies module is macOS gated.
            #[cfg(target_os = "macos")]
            cx.background_executor()
                .timer(std::time::Duration::from_secs(2))
                .await;
            #[cfg(target_os = "macos")]
            let _ = cx.update(|_cx| {
                console_ui::browser::browser_state::cookies::log_cookie_jar();
            });
        })
        .detach();
    }
}

/// The main-thread half: history merges into the persistent store (also
/// feeding omnibox suggestions), cookies inject into the webview jar.
/// `None` note means injection succeeded (or there was nothing to inject).
pub(crate) fn install(
    history: &[console_core::browser_history::BrowserHistoryEntry],
    cookies: &[console_core::types::ChromeCookie],
) -> (usize, Option<String>) {
    console_ui::browser::browser_state::merge_history(history);
    if cookies.is_empty() {
        log::warn!("chrome import: server returned no cookies (history entries: {})", history.len());
        return (0, None);
    }
    match console_ui::browser::browser_state::inject_cookies(cookies) {
        Ok(count) => {
            log::info!("chrome import: injected {count} cookies (history entries: {})", history.len());
            (count, None)
        }
        Err(error) => (0, Some(error)),
    }
}

/// Startup: point the browser's suggestion source at the persisted history.
pub(crate) fn init_from_disk() {
    console_ui::browser::browser_state::reload_from_disk();
}
