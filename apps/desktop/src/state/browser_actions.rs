//! Browser action dispatch and execution for agent automation.
//!
//! Handles `AgentSessionEvent::BrowserAction` requests from the backend agent
//! (`navigate`, `run_js`, `get_content`, `tabs`, `switch_tab`, `close_tab`) against open workspace browser tabs.

use console_core::{BrowserActionRequest, ResolveBrowserActionDto};
use console_ui::browser::BrowserView;
use gpui::{App, AsyncApp, Context, Entity, WeakEntity};
use std::collections::HashSet;
use std::time::Duration;

use crate::state::app::ConsoleDesktopApp;

mod agent_cdp;

/// Interval between tab-lookup / page-load polls.
const BROWSER_POLL_INTERVAL: Duration = Duration::from_millis(150);
/// How many times to retry finding a browser tab (~3s) before failing.
const TAB_LOOKUP_RETRIES: u32 = 20;
/// How many polls to wait for a page to finish loading (~20s).
const NAVIGATION_SETTLE_POLLS: u32 = 130;
/// How many polls to wait for a new tab to appear (~5s) before giving up.
const TAB_OPEN_POLLS: u32 = 33;
/// How many polls to wait for a script result (~10s).
const SCRIPT_RESULT_POLLS: u32 = 67;

/// `wait_for` timeout when the agent gives none, and its upper bound.
const WAIT_FOR_DEFAULT_MS: u64 = 10_000;
const WAIT_FOR_MAX_MS: u64 = 60_000;
/// How many polls (~1.5s) a single `wait_for` condition check may take before
/// it is treated as "page busy" and retried.
const WAIT_FOR_CHECK_POLLS: u32 = 10;

enum NavigationState {
    /// No tab exists yet for this navigation.
    NoTab,
    Pending,
    Settled(String),
    Failed(String),
}

/// Waits for the result of a script started with `run_agent_script`.
async fn poll_script_result(
    this: &WeakEntity<ConsoleDesktopApp>,
    cx: &mut AsyncApp,
    view: &Entity<BrowserView>,
    script_id: &str,
) -> Result<String, String> {
    poll_script_result_within(this, cx, view, script_id, SCRIPT_RESULT_POLLS).await
}

/// Like `poll_script_result` but gives up after `polls` poll intervals.
async fn poll_script_result_within(
    this: &WeakEntity<ConsoleDesktopApp>,
    cx: &mut AsyncApp,
    view: &Entity<BrowserView>,
    script_id: &str,
    polls: u32,
) -> Result<String, String> {
    for _ in 0..polls {
        cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
        let Ok(found) = this.update(cx, |_, cx| {
            view.update(cx, |bv, _| bv.take_script_result(script_id))
        }) else {
            return Err("Console window closed".to_string());
        };
        if let Some(result) = found {
            return result;
        }
    }
    Err(format!(
        "Script produced no result after {}s (the page may be blocking script evaluation, or the script never finished)",
        (polls as u64 * BROWSER_POLL_INTERVAL.as_millis() as u64) / 1000
    ))
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
        let request_id = req.request_id.clone();
        let sess_id = session_id.to_string();

        match action.as_str() {
            "tabs" => {
                let listing = self.describe_browser_tabs(cx);
                self.resolve_browser_action_result(&sess_id, request_id, Some(listing), None, cx);
            }

            "navigate" => {
                let url = req.url.clone().unwrap_or_default();
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

                // An explicit tabId wins; otherwise reuse a tab already on this
                // URL/origin; otherwise open a new tab.
                let target = match self.pick_browser_view(req.tab_id.as_deref(), None, false, cx) {
                    Ok(Some(entry)) if req.tab_id.is_some() => Some(entry),
                    Ok(_) => self.find_matching_browser_view(&url, cx),
                    Err(err) => {
                        self.resolve_browser_action_result(&sess_id, request_id, None, Some(err), cx);
                        return;
                    }
                };
                let known_ids: HashSet<String> = self.browser_views.keys().cloned().collect();
                let matching_view = target.map(|(_, view)| view);
                if let Some(view) = &matching_view {
                    // A reused tab may never have been rendered (opened in the
                    // background earlier), so it can lack a native browser:
                    // create it so the page actually loads.
                    let view = view.clone();
                    let url_to_load = url.clone();
                    self.defer_in_main_window(cx, move |_this, window, cx| {
                        view.update(cx, |bv, cx| {
                            bv.navigate_to_url(url_to_load, cx);
                            bv.ensure_host(window, cx);
                        });
                    });
                } else {
                    let url_to_open = url.clone();
                    self.defer_in_main_window(cx, move |this, window, cx| {
                        this.open_browser_tab_with_url(Some(url_to_open), false, window, cx);
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
                        _ => {
                            // Timed out: report what has loaded so far.
                            let partial = this
                                .update(cx, |app, cx| {
                                    let entry = app.navigation_view(matching_view.as_ref(), &known_ids);
                                    entry.map(|(id, view)| {
                                        let rid = format!("{request_id}-partial");
                                        view.update(cx, |bv, _| {
                                            bv.run_agent_script(
                                                &rid,
                                                "(document.body ? document.body.innerText : '')",
                                            )
                                        });
                                        let bv = view.read(cx);
                                        let header = format!(
                                            "Tab: {}\nTitle: {}\nURL: {}",
                                            id,
                                            bv.tab_label().unwrap_or_default(),
                                            bv.current_url().unwrap_or(&url)
                                        );
                                        (view.clone(), rid, header)
                                    })
                                })
                                .ok()
                                .flatten();
                            let secs = (NAVIGATION_SETTLE_POLLS as u64
                                * BROWSER_POLL_INTERVAL.as_millis() as u64)
                                / 1000;
                            let mut msg = format!(
                                "Navigation to {url} is still loading after {secs}s."
                            );
                            if let Some((view, rid, header)) = partial {
                                msg.push_str(&format!("\n{header}"));
                                if let Ok(text) = poll_script_result(&this, cx, &view, &rid).await {
                                    msg.push_str(&format!("\nText loaded so far:\n{text}"));
                                }
                            }
                            (Some(msg), None)
                        }
                    };
                    let _ = this.update(cx, |app, cx| {
                        if let Some((id, _)) = app.navigation_view(matching_view.as_ref(), &known_ids) {
                            app.agent_browser_tab = Some(id);
                        }
                        app.resolve_browser_action_result(&sess_id, request_id, result, error, cx);
                    });
                })
                .detach();
            }

            "snapshot" | "click" | "type" => {
                let element_ref = req.element_ref.clone().filter(|s| !s.trim().is_empty());
                if action != "snapshot" && element_ref.is_none() {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some(format!("'{action}' needs a ref from a snapshot")),
                        cx,
                    );
                    return;
                }
                if action == "type" && req.text.is_none() {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some("'type' needs text".to_string()),
                        cx,
                    );
                    return;
                }
                let entry = match self.pick_browser_view(req.tab_id.as_deref(), None, false, cx) {
                    Ok(entry) => entry,
                    Err(err) => {
                        self.resolve_browser_action_result(&sess_id, request_id, None, Some(err), cx);
                        return;
                    }
                };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };
                let note = self.tab_choice_note(req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tab = Some(tab_id.clone());
                if self.try_agent_browser(&sess_id, &req, &tab_id, &view, note.clone(), retries_left, cx) {
                    return;
                }

                let js = console_ui::browser::agent_script::element_script(
                    &action,
                    element_ref.as_deref(),
                    req.text.as_deref(),
                    req.submit.unwrap_or(false),
                    req.selector.as_deref(),
                );
                let is_snapshot = action == "snapshot";
                let script_id = request_id.clone();
                view.update(cx, |bv, _| bv.run_agent_script(&script_id, &js));
                cx.spawn(async move |this, cx| {
                    let outcome = poll_script_result(&this, cx, &view, &script_id).await;
                    let (result, error) = match outcome {
                        Ok(value) => {
                            let header = this
                                .update(cx, |_, cx| {
                                    let bv = view.read(cx);
                                    format!(
                                        "Tab: {}\nTitle: {}\nURL: {}",
                                        tab_id,
                                        bv.tab_label().unwrap_or_default(),
                                        bv.current_url().unwrap_or_default()
                                    )
                                })
                                .unwrap_or_default();
                            let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                            let hint = if is_snapshot {
                                ""
                            } else {
                                "\n(Take a new snapshot if the page changed.)"
                            };
                            (Some(format!("{note}{header}\n\n{value}{hint}")), None)
                        }
                        Err(err) => (None, Some(format!("[tab {tab_id}] {err}"))),
                    };
                    let _ = this.update(cx, |app, cx| {
                        app.resolve_browser_action_result(&sess_id, request_id, result, error, cx);
                    });
                })
                .detach();
            }

            "switch_tab" | "close_tab" => {
                let Some(requested) = req.tab_id.as_deref().map(str::trim).filter(|s| !s.is_empty()) else {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some(format!(
                            "'{action}' needs a tabId. Open tabs:\n{}",
                            self.describe_browser_tabs(cx)
                        )),
                        cx,
                    );
                    return;
                };
                // Unknown ids error out with the tab listing; never fall back to another tab.
                let (tab_id, view) = match self.pick_browser_view(Some(requested), None, false, cx) {
                    Ok(Some(entry)) => entry,
                    Ok(None) => return,
                    Err(err) => {
                        self.resolve_browser_action_result(&sess_id, request_id, None, Some(err), cx);
                        return;
                    }
                };
                let (title, url) = {
                    let bv = view.read(cx);
                    (
                        bv.tab_label().unwrap_or_default(),
                        bv.current_url().unwrap_or("(blank)").to_string(),
                    )
                };

                let result = if action == "switch_tab" {
                    // Only retargets the agent; the tab the user is looking at is untouched.
                    self.agent_browser_tab = Some(tab_id.clone());
                    format!("Tab: {tab_id}\nTitle: {title}\nURL: {url}\nSwitched agent target to this tab.")
                } else {
                    self.close_agent_browser_tab(&tab_id, cx);
                    if self.agent_browser_tab.as_deref() == Some(tab_id.as_str()) {
                        self.agent_browser_tab = self.most_recent_browser_tab();
                    }
                    let next = match &self.agent_browser_tab {
                        Some(id) => format!("Agent target is now tab '{id}'."),
                        None => "No browser tabs are open.".to_string(),
                    };
                    format!(
                        "Tab: {tab_id}\nClosed: {url}\n{next}\nOpen tabs:\n{}",
                        self.describe_browser_tabs(cx)
                    )
                };
                self.resolve_browser_action_result(&sess_id, request_id, Some(result), None, cx);
            }

            "wait_for" => {
                let selector = req.selector.clone().filter(|s| !s.trim().is_empty());
                let url_contains = req.url_contains.clone().filter(|s| !s.is_empty());
                let text = req.text.clone().filter(|s| !s.is_empty());
                if selector.is_none() && url_contains.is_none() && text.is_none() {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some("wait_for needs at least one of selector, urlContains, or text".to_string()),
                        cx,
                    );
                    return;
                }
                let entry = match self.pick_browser_view(req.tab_id.as_deref(), None, false, cx) {
                    Ok(entry) => entry,
                    Err(err) => {
                        self.resolve_browser_action_result(&sess_id, request_id, None, Some(err), cx);
                        return;
                    }
                };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };
                let note = self.tab_choice_note(req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tab = Some(tab_id.clone());
                if self.try_agent_browser(&sess_id, &req, &tab_id, &view, note.clone(), retries_left, cx) {
                    return;
                }

                let timeout = Duration::from_millis(
                    req.timeout_ms.unwrap_or(WAIT_FOR_DEFAULT_MS).clamp(100, WAIT_FOR_MAX_MS),
                );
                let script = wait_for_script(selector.as_deref(), url_contains.as_deref(), text.as_deref());
                cx.spawn(async move |this, cx| {
                    let started = std::time::Instant::now();
                    let mut attempt = 0u32;
                    let mut last_problem: Option<String> = None;
                    let mut met = false;
                    // Short one-shot checks (rather than one long in-page promise) so the
                    // wait survives full page loads that destroy the JS context.
                    loop {
                        attempt += 1;
                        let check_id = format!("{request_id}-wf-{attempt}");
                        let started_check = this.update(cx, |_, cx| {
                            view.update(cx, |bv, _| bv.run_agent_script(&check_id, &script))
                        });
                        if started_check.is_err() {
                            return;
                        }
                        match poll_script_result_within(&this, cx, &view, &check_id, WAIT_FOR_CHECK_POLLS).await {
                            Ok(value) if value.contains("WF_OK") => {
                                met = true;
                                break;
                            }
                            Ok(value) if value.contains("WF_ERR") => {
                                last_problem = Some(value);
                                break;
                            }
                            Ok(_) => {}
                            // No result usually means the page is mid-navigation; keep waiting.
                            Err(_) => {}
                        }
                        if started.elapsed() >= timeout {
                            break;
                        }
                        cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
                    }
                    let elapsed_ms = started.elapsed().as_millis();
                    let (result, error) = this
                        .update(cx, |_, cx| {
                            let bv = view.read(cx);
                            let header = format!(
                                "Tab: {}\nTitle: {}\nURL: {}",
                                tab_id,
                                bv.tab_label().unwrap_or_default(),
                                bv.current_url().unwrap_or_default()
                            );
                            let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                            if met {
                                (Some(format!("{note}Condition met after {elapsed_ms}ms\n{header}")), None)
                            } else if let Some(problem) = last_problem {
                                (None, Some(format!("{note}{problem}\n{header}")))
                            } else {
                                (
                                    None,
                                    Some(format!(
                                        "{note}Timed out after {}ms waiting for the condition\n{header}",
                                        timeout.as_millis()
                                    )),
                                )
                            }
                        })
                        .unwrap_or((None, Some("Console window closed".to_string())));
                    let _ = this.update(cx, |app, cx| {
                        app.resolve_browser_action_result(&sess_id, request_id, result, error, cx);
                    });
                })
                .detach();
            }

            "run_js" | "get_content" => {
                let is_js = action == "run_js";
                let js = if is_js {
                    let js = req.script.clone().unwrap_or_default();
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
                    js
                } else {
                    content_script(req.selector.as_deref())
                };

                let entry = match self.pick_browser_view(req.tab_id.as_deref(), req.url.as_deref(), true, cx) {
                    Ok(entry) => entry,
                    Err(err) => {
                        self.resolve_browser_action_result(&sess_id, request_id, None, Some(err), cx);
                        return;
                    }
                };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };

                let note = self.tab_choice_note(req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tab = Some(tab_id.clone());
                if self.try_agent_browser(&sess_id, &req, &tab_id, &view, note.clone(), retries_left, cx) {
                    return;
                }
                let script_id = request_id.clone();
                view.update(cx, |bv, _| bv.run_agent_script(&script_id, &js));
                cx.spawn(async move |this, cx| {
                    let outcome = poll_script_result(&this, cx, &view, &script_id).await;
                    let (result, error) = match outcome {
                        Ok(value) if is_js => {
                            let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                            (Some(format!("{note}Tab: {tab_id}\n{value}")), None)
                        }
                        Ok(value) => {
                            let header = this
                                .update(cx, |_, cx| {
                                    let bv = view.read(cx);
                                    format!(
                                        "Tab: {}\nTitle: {}\nURL: {}",
                                        tab_id,
                                        bv.tab_label().unwrap_or_default(),
                                        bv.current_url().unwrap_or_default()
                                    )
                                })
                                .unwrap_or_default();
                            let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                            (Some(format!("{note}{header}\n\n{value}")), None)
                        }
                        Err(err) => (None, Some(format!("[tab {tab_id}] {err}"))),
                    };
                    let _ = this.update(cx, |app, cx| {
                        app.resolve_browser_action_result(&sess_id, request_id, result, error, cx);
                    });
                })
                .detach();
            }

            "screenshot" => {
                let entry = match self.pick_browser_view(req.tab_id.as_deref(), req.url.as_deref(), true, cx) {
                    Ok(entry) => entry,
                    Err(err) => {
                        self.resolve_browser_action_result(&sess_id, request_id, None, Some(err), cx);
                        return;
                    }
                };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };
                let note = self.tab_choice_note(req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tab = Some(tab_id.clone());
                if self.try_agent_browser(&sess_id, &req, &tab_id, &view, note.clone(), retries_left, cx) {
                    return;
                }
                let Some(slot) = view.read(cx).start_snapshot() else {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some(format!("[tab {tab_id}] Tab has no live webview to capture (open it first)")),
                        cx,
                    );
                    return;
                };
                cx.spawn(async move |this, cx| {
                    let mut outcome = Err("Snapshot timed out".to_string());
                    for _ in 0..SCRIPT_RESULT_POLLS {
                        cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
                        if let Some(done) = slot.borrow_mut().take() {
                            outcome = done;
                            break;
                        }
                    }
                    let _ = this.update(cx, |app, cx| match outcome {
                        Ok(png) => {
                            use base64::Engine;
                            let b64 = base64::engine::general_purpose::STANDARD.encode(&png);
                            let title = view.read(cx).tab_label().unwrap_or_default();
                            let url = view.read(cx).current_url().unwrap_or_default().to_string();
                            let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                            app.resolve_browser_action_with_image(
                                &sess_id,
                                request_id,
                                Some(format!("{note}Tab: {tab_id}\nTitle: {title}\nURL: {url}\nScreenshot attached.")),
                                None,
                                Some(b64),
                                cx,
                            );
                        }
                        Err(err) => app.resolve_browser_action_result(
                            &sess_id,
                            request_id,
                            None,
                            Some(format!("[tab {tab_id}] {err}")),
                            cx,
                        ),
                    });
                })
                .detach();
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

    /// Runs `f` in the app's main window (falling back to the active one),
    /// deferred until the current update finishes. Prefers the app's own window
    /// so this works when it isn't the active one.
    fn defer_in_main_window(
        &self,
        cx: &mut Context<Self>,
        f: impl FnOnce(&mut Self, &mut gpui::Window, &mut Context<Self>) + 'static,
    ) {
        let entity = cx.entity().downgrade();
        let main_window = self.main_window_handle;
        cx.defer(move |cx| {
            if let Some(window) = main_window.or_else(|| cx.active_window()) {
                let _ = window.update(cx, |_, window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| f(this, window, cx));
                    }
                });
            }
        });
    }

    /// Re-queues `req` after a short delay while retries remain; otherwise
    /// resolves it with a "no tab" error that lists the open tabs.
    fn retry_browser_action_or_fail(
        &mut self,
        session_id: &str,
        req: BrowserActionRequest,
        retries_left: u32,
        cx: &mut Context<Self>,
    ) {
        if retries_left == 0 {
            let error = format!(
                "No active or matching browser tab found. Open tabs:\n{}",
                self.describe_browser_tabs(cx)
            );
            self.resolve_browser_action_result(session_id, req.request_id, None, Some(error), cx);
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

    /// Closes a browser tab through the normal workspace close path so the tab
    /// strip, persistence, and the webview (and its per-page ref store) go with it.
    fn close_agent_browser_tab(&mut self, browser_id: &str, cx: &mut Context<Self>) {
        let key = format!("browser:{browser_id}");
        let pane_id = self
            .workspace_root
            .leaves()
            .iter()
            .find(|leaf| leaf.tabs.iter().any(|t| t.id() == key))
            .map(|leaf| leaf.id.clone());
        match pane_id {
            Some(pane_id) => self.close_tab_and_sync_pane(&pane_id, &key, cx),
            None => {
                // Tab lives in a stashed workspace: drop it there and free the view.
                for root in self.project_workspace_roots.values_mut() {
                    console_ui::workspace::ops::close_matching_tabs(root, |t| t.id() == key);
                }
                self.persist_workspaces();
            }
        }
        // Guarantee the view is gone from the registry even in the stashed case.
        if let Some(view) = self.browser_views.remove(browser_id) {
            view.update(cx, |bv, cx| bv.close(cx));
        }
    }

    /// The most recently used open browser tab (by tab `last_active_at_ms`),
    /// else the lowest id; `None` when no browser tabs remain.
    fn most_recent_browser_tab(&self) -> Option<String> {
        let mut best: Option<(i64, String)> = None;
        for leaf in self.workspace_root.leaves() {
            for tab in &leaf.tabs {
                if let console_core::WorkspaceTabConfig::Browser { browser_id, last_active_at_ms, .. } = tab {
                    if !self.browser_views.contains_key(browser_id) {
                        continue;
                    }
                    let ts = last_active_at_ms.unwrap_or(0);
                    if best.as_ref().map_or(true, |(b, _)| ts > *b) {
                        best = Some((ts, browser_id.clone()));
                    }
                }
            }
        }
        best.map(|(_, id)| id).or_else(|| {
            let mut ids: Vec<&String> = self.browser_views.keys().collect();
            ids.sort();
            ids.first().map(|id| (*id).clone())
        })
    }

    /// One line per open browser tab: id, title, URL, and which one is active.
    fn describe_browser_tabs(&self, cx: &App) -> String {
        if self.browser_views.is_empty() {
            return "No browser tabs are open. Use navigate to open one.".to_string();
        }
        let active = self.active_browser_entry().map(|(id, _)| id);
        let mut ids: Vec<&String> = self.browser_views.keys().collect();
        ids.sort();
        ids.iter()
            .map(|id| {
                let bv = self.browser_views[*id].read(cx);
                format!(
                    "- {}{}: \"{}\" {}",
                    id,
                    if active.as_deref() == Some(id.as_str()) { " (active)" } else { "" },
                    bv.tab_label().unwrap_or_default(),
                    bv.current_url().unwrap_or("(blank)")
                )
            })
            .collect::<Vec<_>>()
            .join("\n")
    }

    /// Picks the tab an action should run against.
    /// - `tab_id` given: exactly that tab, or an error listing the open tabs.
    /// - otherwise a tab matching `url` (when `allow_url_match`), else the
    ///   active browser tab, else any tab.
    /// `Ok(None)` means no tab exists yet.
    fn pick_browser_view(
        &self,
        tab_id: Option<&str>,
        url: Option<&str>,
        allow_url_match: bool,
        cx: &App,
    ) -> Result<Option<(String, Entity<BrowserView>)>, String> {
        if let Some(id) = tab_id.map(str::trim).filter(|s| !s.is_empty()) {
            return match self.browser_views.get(id) {
                Some(view) => Ok(Some((id.to_string(), view.clone()))),
                None => Err(format!(
                    "No browser tab with id '{}'. Open tabs:\n{}",
                    id,
                    self.describe_browser_tabs(cx)
                )),
            };
        }
        if allow_url_match {
            if let Some(entry) = url.and_then(|u| self.find_matching_browser_view(u, cx)) {
                return Ok(Some(entry));
            }
        }
        // Prefer the tab the agent last worked in: `navigate` opens tabs in the
        // background, so the visible tab is often not the one the agent means.
        let last = self.agent_browser_tab.as_ref().and_then(|id| {
            self.browser_views.get(id).map(|view| (id.clone(), view.clone()))
        });
        if last.is_some() {
            return Ok(last);
        }
        Ok(self.active_browser_entry())
    }

    /// Warning shown when several tabs are open and the agent named none, so a
    /// wrong-tab default is visible at the top of the result.
    fn tab_choice_note(&self, explicit: bool, chosen: &str, cx: &App) -> Option<String> {
        if explicit || self.browser_views.len() < 2 {
            return None;
        }
        Some(format!(
            "Note: {} tabs are open and no tabId was given, so tab '{}' was used (the tab you last navigated or acted on). Pass tabId to target another. Open tabs:\n{}",
            self.browser_views.len(),
            chosen,
            self.describe_browser_tabs(cx)
        ))
    }

    /// The tab a navigation is acting on: the reused one, or the newly opened one.
    fn navigation_view(
        &self,
        existing: Option<&Entity<BrowserView>>,
        known_ids: &HashSet<String>,
    ) -> Option<(String, Entity<BrowserView>)> {
        if let Some(view) = existing {
            let id = self
                .browser_views
                .iter()
                .find(|(_, v)| v.entity_id() == view.entity_id())
                .map(|(id, _)| id.clone())
                .unwrap_or_default();
            return Some((id, view.clone()));
        }
        self.browser_views
            .iter()
            .find(|(id, _)| !known_ids.contains(*id))
            .map(|(id, v)| (id.clone(), v.clone()))
    }

    /// Reports whether the navigation started by a `navigate` action has finished.
    fn navigation_state(
        &self,
        existing: Option<&Entity<BrowserView>>,
        known_ids: &HashSet<String>,
        url: &str,
        cx: &App,
    ) -> NavigationState {
        let Some((id, view)) = self.navigation_view(existing, known_ids) else {
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
        NavigationState::Settled(format!(
            "Navigated to {}\nTab: {}\nTitle: {}",
            current, id, title
        ))
    }

    /// Finds a tab whose URL matches or shares the origin/prefix of `target_url`.
    fn find_matching_browser_view(
        &self,
        target_url: &str,
        cx: &App,
    ) -> Option<(String, Entity<BrowserView>)> {
        let clean_target = target_url.trim().trim_end_matches('/');
        let mut ids: Vec<&String> = self.browser_views.keys().collect();
        ids.sort();
        for id in ids {
            let view = &self.browser_views[id];
            if let Some(url) = view.read(cx).current_url() {
                let clean_url = url.trim().trim_end_matches('/');
                if clean_url == clean_target
                    || clean_url.starts_with(clean_target)
                    || clean_target.starts_with(clean_url)
                {
                    return Some((id.clone(), view.clone()));
                }
            }
        }
        None
    }

    /// The browser tab that is active in a visible pane, else the first by id.
    fn active_browser_entry(&self) -> Option<(String, Entity<BrowserView>)> {
        for leaf in self.workspace_root.leaves() {
            let Some(active_id) = leaf.active_tab_id.as_deref() else { continue };
            for tab in &leaf.tabs {
                if tab.id() != active_id {
                    continue;
                }
                if let console_core::WorkspaceTabConfig::Browser { browser_id, .. } = tab {
                    if let Some(view) = self.browser_views.get(browser_id) {
                        return Some((browser_id.clone(), view.clone()));
                    }
                }
            }
        }
        let mut ids: Vec<&String> = self.browser_views.keys().collect();
        ids.sort();
        ids.first().map(|id| ((*id).clone(), self.browser_views[*id].clone()))
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
        self.resolve_browser_action_with_image(session_id, request_id, result, error, None, cx);
    }

    fn resolve_browser_action_with_image(
        &self,
        session_id: &str,
        request_id: String,
        result: Option<String>,
        error: Option<String>,
        image_base64: Option<String>,
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
                        image_base64,
                    },
                )
                .await;
        })
        .detach();
    }
}

/// Page-side one-shot check for `wait_for`. Returns WF_OK when every given
/// condition holds, WF_NO otherwise, or WF_ERR on an invalid selector.
fn wait_for_script(selector: Option<&str>, url_contains: Option<&str>, text: Option<&str>) -> String {
    let q = |v: Option<&str>| {
        serde_json::to_string(&v.unwrap_or("")).unwrap_or_else(|_| "\"\"".to_string())
    };
    format!(
        "(() => {{ const sel = {}; const u = {}; const t = {}; \
         try {{ if (sel && !document.querySelector(sel)) return 'WF_NO'; }} \
         catch (e) {{ return 'WF_ERR: Invalid selector: ' + e.message; }} \
         if (u && !location.href.includes(u)) return 'WF_NO'; \
         if (t && !(document.body && document.body.innerText.includes(t))) return 'WF_NO'; \
         return 'WF_OK'; }})()",
        q(selector),
        q(url_contains),
        q(text)
    )
}

/// Page-side script for `get_content`: visible text, or the text of every
/// element matching `selector` (or a "no match" message).
fn content_script(selector: Option<&str>) -> String {
    match selector.map(str::trim).filter(|s| !s.is_empty()) {
        None => "(document.body ? document.body.innerText : '')".to_string(),
        Some(sel) => {
            let quoted = serde_json::to_string(sel).unwrap_or_else(|_| "\"\"".to_string());
            format!(
                "(() => {{ const sel = {quoted}; let els; \
                 try {{ els = Array.from(document.querySelectorAll(sel)); }} \
                 catch (e) {{ return 'Invalid selector: ' + e.message; }} \
                 if (!els.length) return 'No match for selector: ' + sel; \
                 const out = els.slice(0, 20).map((el, i) => '[' + i + '] ' + (el.innerText || el.textContent || '').trim()); \
                 if (els.length > 20) out.push('...(' + els.length + ' matches total, showing 20)'); \
                 return out.join('\\n\\n'); }})()"
            )
        }
    }
}

