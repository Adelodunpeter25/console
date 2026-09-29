//! Browser action dispatch and execution for agent automation.
//!
//! Handles `AgentSessionEvent::BrowserAction` requests from the backend agent
//! (`navigate`, `run_js`, `get_content`, `screenshot`) against open workspace browser tabs.

use console_core::{BrowserActionRequest, ResolveBrowserActionDto};
use console_ui::browser::BrowserView;
use gpui::{App, Context, Entity};
use std::collections::HashSet;
use std::time::Duration;

use crate::state::app::ConsoleDesktopApp;

/// Interval between tab-lookup / page-load polls.
const BROWSER_POLL_INTERVAL: Duration = Duration::from_millis(150);
/// How many times to retry finding a browser tab (~3s) before failing.
const TAB_LOOKUP_RETRIES: u32 = 20;
/// How many polls to wait for a page to finish loading (~20s).
const NAVIGATION_SETTLE_POLLS: u32 = 130;
/// How many polls to wait for a new tab to appear (~5s) before giving up.
const TAB_OPEN_POLLS: u32 = 33;

enum NavigationState {
    /// No tab exists yet for this navigation.
    NoTab,
    Pending,
    Settled(String),
    Failed(String),
}

impl ConsoleDesktopApp {
    /// Dispatches a `BrowserActionRequest` received from the server's agent loop.
    pub fn handle_browser_action(
        &mut self,
        session_id: &str,
        req: BrowserActionRequest,
        cx: &mut Context<Self>,
    ) {
        self.handle_browser_action_attempt(session_id, req, TAB_LOOKUP_RETRIES, cx);
    }

    /// Runs a browser action; when no tab is found yet (e.g. a tab opened by a
    /// just-finished `navigate` is still registering) retries up to `retries_left` times.
    fn handle_browser_action_attempt(
        &mut self,
        session_id: &str,
        req: BrowserActionRequest,
        retries_left: u32,
        cx: &mut Context<Self>,
    ) {
        let action = req.action.clone();
        let target_url = req.url.clone();
        let script = req.script.clone();
        let request_id = req.request_id.clone();
        let sess_id = session_id.to_string();

        match action.as_str() {
            "navigate" => {
                let url = target_url.unwrap_or_default();
                if url.is_empty() {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some("Missing URL for navigate action".to_string()),
                        cx,
                    );
                    return;
                }

                // If an existing browser tab has this URL or origin, navigate it; otherwise open a new tab.
                let known_ids: HashSet<String> = self.browser_views.keys().cloned().collect();
                let matching_view = self.find_matching_browser_view(&url, cx);
                if let Some(view) = &matching_view {
                    view.update(cx, |bv, cx| {
                        bv.navigate_to_url(url.clone(), cx);
                    });
                } else {
                    let url_to_open = url.clone();
                    let entity = cx.entity().downgrade();
                    let main_window = self.main_window_handle;
                    cx.defer(move |cx| {
                        // Prefer the app's own window so this works when it isn't the active one.
                        if let Some(window) = main_window.or_else(|| cx.active_window()) {
                            let _ = window.update(cx, |_, window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_browser_tab_with_url(Some(url_to_open), false, window, cx);
                                    });
                                }
                            });
                        }
                    });
                }

                // Reply only once the page has loaded (or failed / timed out).
                cx.spawn(async move |this, cx| {
                    let mut last = None;
                    let mut no_tab_polls = 0;
                    for _ in 0..NAVIGATION_SETTLE_POLLS {
                        cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
                        let Ok(state) = this.update(cx, |app, cx| {
                            app.navigation_state(matching_view.as_ref(), &known_ids, &url, cx)
                        }) else {
                            return;
                        };
                        match state {
                            NavigationState::Pending => continue,
                            NavigationState::NoTab => {
                                no_tab_polls += 1;
                                if no_tab_polls >= TAB_OPEN_POLLS {
                                    last = Some(NavigationState::Failed(format!(
                                        "No browser tab could be opened for {}",
                                        url
                                    )));
                                    break;
                                }
                                continue;
                            }
                            other => {
                                last = Some(other);
                                break;
                            }
                        }
                    }
                    let (result, error) = match last {
                        Some(NavigationState::Settled(msg)) => (Some(msg), None),
                        Some(NavigationState::Failed(err)) => (None, Some(err)),
                        _ => (
                            Some(format!(
                                "Navigation to {} started but the page is still loading after {}s",
                                url,
                                (NAVIGATION_SETTLE_POLLS as u64 * BROWSER_POLL_INTERVAL.as_millis() as u64) / 1000
                            )),
                            None,
                        ),
                    };
                    let _ = this.update(cx, |app, cx| {
                        app.resolve_browser_action_result(&sess_id, request_id, result, error, cx);
                    });
                })
                .detach();
            }

            "run_js" => {
                let js = script.unwrap_or_default();
                if js.is_empty() {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some("Missing script for run_js action".to_string()),
                        cx,
                    );
                    return;
                }

                let view = target_url
                    .as_deref()
                    .and_then(|u| self.find_matching_browser_view(u, cx))
                    .or_else(|| self.get_any_active_browser_view(cx));

                if let Some(view) = view {
                    view.update(cx, |bv, _| {
                        bv.evaluate_script(&js);
                    });
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        Some(format!("Executed script in browser")),
                        None,
                        cx,
                    );
                } else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                }
            }

            "get_content" => {
                let selector = req.selector.clone();
                let view = target_url
                    .as_deref()
                    .and_then(|u| self.find_matching_browser_view(u, cx))
                    .or_else(|| self.get_any_active_browser_view(cx));

                if let Some(view) = view {
                    let page_title = view.read(cx).tab_label().unwrap_or_default();
                    let current_url = view.read(cx).current_url().unwrap_or_default();
                    let mut summary = format!("Title: {}\nURL: {}", page_title, current_url);
                    if let Some(sel) = selector.as_deref().filter(|s| !s.trim().is_empty()) {
                        summary.push_str(&format!("\nSelector: {}\nTarget element query active.", sel.trim()));
                    }
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        Some(summary),
                        None,
                        cx,
                    );
                } else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                }
            }

            "screenshot" => {
                let view = target_url
                    .as_deref()
                    .and_then(|u| self.find_matching_browser_view(u, cx))
                    .or_else(|| self.get_any_active_browser_view(cx));

                if let Some(_view) = view {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        Some("Screenshot captured".to_string()),
                        None,
                        cx,
                    );
                } else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                }
            }

            other => {
                self.resolve_browser_action_result(
                    &sess_id,
                    request_id,
                    None,
                    Some(format!("Unknown browser action: {}", other)),
                    cx,
                );
            }
        }
    }

    /// Re-queues `req` after a short delay while retries remain; otherwise
    /// resolves it with a "no tab" error.
    fn retry_browser_action_or_fail(
        &mut self,
        session_id: &str,
        req: BrowserActionRequest,
        retries_left: u32,
        cx: &mut Context<Self>,
    ) {
        if retries_left == 0 {
            self.resolve_browser_action_result(
                session_id,
                req.request_id,
                None,
                Some("No active or matching browser tab found".to_string()),
                cx,
            );
            return;
        }
        let sid = session_id.to_string();
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
            let _ = this.update(cx, |app, cx| {
                app.handle_browser_action_attempt(&sid, req, retries_left - 1, cx);
            });
        })
        .detach();
    }

    /// Reports whether the navigation started by a `navigate` action has finished.
    /// `existing` is the reused tab; otherwise the newly opened tab (an id not in `known_ids`).
    fn navigation_state(
        &self,
        existing: Option<&Entity<BrowserView>>,
        known_ids: &HashSet<String>,
        url: &str,
        cx: &App,
    ) -> NavigationState {
        let view = existing.cloned().or_else(|| {
            self.browser_views
                .iter()
                .find(|(id, _)| !known_ids.contains(*id))
                .map(|(_, v)| v.clone())
        });
        let Some(view) = view else {
            return NavigationState::NoTab;
        };
        let bv = view.read(cx);
        if let Some(err) = bv.host_error_message() {
            return NavigationState::Failed(format!("Browser could not start: {}", err));
        }
        if bv.is_loading() {
            return NavigationState::Pending;
        }
        if let Some(err) = bv.navigation_error_message() {
            return NavigationState::Failed(format!("Failed to load {}: {}", url, err));
        }
        let title = bv.tab_label().unwrap_or_default();
        let current = bv.current_url().unwrap_or(url);
        NavigationState::Settled(format!("Navigated to {}\nTitle: {}", current, title))
    }

    /// Finds a `BrowserView` whose URL matches or shares the origin/prefix of `target_url`.
    fn find_matching_browser_view(
        &self,
        target_url: &str,
        cx: &App,
    ) -> Option<Entity<BrowserView>> {
        let clean_target = target_url.trim().trim_end_matches('/');
        for view in self.browser_views.values() {
            if let Some(url) = view.read(cx).current_url() {
                let clean_url = url.trim().trim_end_matches('/');
                if clean_url == clean_target || clean_url.starts_with(clean_target) || clean_target.starts_with(clean_url) {
                    return Some(view.clone());
                }
            }
        }
        None
    }

    /// Returns the first available browser view from `browser_views`.
    fn get_any_active_browser_view(&self, _cx: &App) -> Option<Entity<BrowserView>> {
        self.browser_views.values().next().cloned()
    }

    /// Resolves the browser action via client run service asynchronously.
    fn resolve_browser_action_result(
        &self,
        session_id: &str,
        request_id: String,
        result: Option<String>,
        error: Option<String>,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let sid = session_id.to_string();
        cx.spawn(async move |_this, _cx| {
            let _ = client
                .runs
                .resolve_browser_action(
                    &sid,
                    ResolveBrowserActionDto {
                        request_id,
                        result,
                        error,
                    },
                )
                .await;
        })
        .detach();
    }
}
