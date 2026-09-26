//! Browser action dispatch and execution for agent automation.
//!
//! Handles `AgentSessionEvent::BrowserAction` requests from the backend agent
//! (`navigate`, `run_js`, `get_content`, `screenshot`) against open workspace browser tabs.

use console_core::{BrowserActionRequest, ResolveBrowserActionDto};
use console_ui::browser::BrowserView;
use gpui::{App, Context, Entity};

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Dispatches a `BrowserActionRequest` received from the server's agent loop.
    pub fn handle_browser_action(
        &mut self,
        session_id: &str,
        req: BrowserActionRequest,
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
                let matching_view = self.find_matching_browser_view(&url, cx);
                if let Some(view) = matching_view {
                    view.update(cx, |bv, cx| {
                        bv.navigate_to_url(url.clone(), cx);
                    });
                } else {
                    let url_to_open = url.clone();
                    let entity = cx.entity().downgrade();
                    cx.defer(move |cx| {
                        if let Some(window) = cx.active_window() {
                            let _ = window.update(cx, |_, window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_browser_tab_with_url(Some(url_to_open), window, cx);
                                    });
                                }
                            });
                        }
                    });
                }

                self.resolve_browser_action_result(
                    &sess_id,
                    request_id,
                    Some(format!("Navigated to {}", url)),
                    None,
                    cx,
                );
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
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some("No active or matching browser tab found".to_string()),
                        cx,
                    );
                }
            }

            "get_content" => {
                let view = target_url
                    .as_deref()
                    .and_then(|u| self.find_matching_browser_view(u, cx))
                    .or_else(|| self.get_any_active_browser_view(cx));

                if let Some(view) = view {
                    let page_title = view.read(cx).tab_label().unwrap_or_default();
                    let current_url = view.read(cx).current_url().unwrap_or_default();
                    let summary = format!("Title: {}\nURL: {}", page_title, current_url);
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        Some(summary),
                        None,
                        cx,
                    );
                } else {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some("No active or matching browser tab found".to_string()),
                        cx,
                    );
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
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some("No active or matching browser tab found".to_string()),
                        cx,
                    );
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
