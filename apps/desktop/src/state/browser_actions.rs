//! Browser action dispatch and execution for agent automation.
//!
//! Handles `AgentSessionEvent::BrowserAction` requests from the backend agent
//! (`navigate`, `run_js`, `get_content`, `query`, `tabs`, `switch_tab`, `close_tab`) against open workspace browser tabs,
//! scoped to the requesting chat: only the browser tabs that chat's agent
//! created are visible, targetable, or listed to it.

use console_core::{BrowserActionRequest, ResolveBrowserActionDto};
use console_ui::browser::driver::Request as DriverRequest;
use console_ui::browser::{BrowserView, ScriptOutcome};
use gpui::{App, AsyncApp, Context, Entity, WeakEntity};
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

/// Waits for the result of a script or driver request.
async fn poll_script_result(
    this: &WeakEntity<ConsoleDesktopApp>,
    cx: &mut AsyncApp,
    view: &Entity<BrowserView>,
    script_id: &str,
) -> Result<ScriptOutcome, String> {
    poll_script_result_within(this, cx, view, script_id, SCRIPT_RESULT_POLLS).await
}

/// Like `poll_script_result` but gives up after `polls` poll intervals.
async fn poll_script_result_within(
    this: &WeakEntity<ConsoleDesktopApp>,
    cx: &mut AsyncApp,
    view: &Entity<BrowserView>,
    script_id: &str,
    polls: u32,
) -> Result<ScriptOutcome, String> {
    for _ in 0..polls {
        cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
        let Ok(found) = this.update(cx, |_, cx| {
            view.update(cx, |bv, _| bv.take_script_result(script_id))
        }) else {
            return Err("Console window closed".to_string());
        };
        if let Some(result) = found {
            return Ok(result);
        }
    }
    Err(format!(
        "Script produced no result after {}s (the page may be blocking script evaluation, or the script never finished)",
        (polls as u64 * BROWSER_POLL_INTERVAL.as_millis() as u64) / 1000
    ))
}

/// Prefer the page identity the driver reported when the script finished.
/// The committed tab URL lags behind `pushState` routes, so it is only a
/// fallback for older scripts that do not send `url`/`title`.
fn format_tab_header(
    view: &Entity<BrowserView>,
    tab_id: &str,
    url: Option<&str>,
    title: Option<&str>,
    cx: &App,
) -> String {
    let bv = view.read(cx);
    let url = url
        .filter(|u| !u.is_empty())
        .map(str::to_owned)
        .or_else(|| bv.current_url().map(str::to_owned))
        .unwrap_or_default();
    let title = title
        .filter(|t| !t.trim().is_empty())
        .map(str::to_owned)
        .or_else(|| bv.tab_label())
        .unwrap_or_default();
    format!("Tab: {tab_id}\nTitle: {title}\nURL: {url}")
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
                let listing = self.describe_browser_tabs(&sess_id, cx);
                self.resolve_browser_action_result(&sess_id, request_id, Some(listing), None, cx);
            }

            // Internal, used by the server's /browser-use kernel: the same
            // tab list as `tabs` but as JSON rows, so the kernel reads
            // structured tab identity instead of parsing the display text.
            // Not part of the low-level tool's schema.
            "tabs_json" => {
                let listing = self.browser_tabs_json(&sess_id, cx);
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
                // URL/origin; otherwise open a new tab — all within this chat's
                // own tabs. A `new_tab` request (the server's browser kernel
                // opening a tab it will own) skips the reuse entirely: reset
                // later closes what the kernel opened, which must never be a
                // tab someone already had.
                let forced_new = req.new_tab.unwrap_or(false);
                let target = if forced_new {
                    None
                } else {
                    match self.pick_browser_view(&sess_id, req.tab_id.as_deref(), None, false, cx) {
                        Ok(Some(entry)) if req.tab_id.is_some() => Some(entry),
                        Ok(_) => self.find_matching_browser_view(&sess_id, &url, cx),
                        Err(err) => {
                            self.resolve_browser_action_result(
                                &sess_id,
                                request_id,
                                None,
                                Some(err),
                                cx,
                            );
                            return;
                        }
                    }
                };
                let known_ids: HashSet<String> =
                    self.session_browser_ids(&sess_id).into_iter().collect();
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
                    let chat_to_open = sess_id.clone();
                    self.defer_in_main_window(cx, move |this, window, cx| {
                        this.open_agent_browser_tab(&chat_to_open, url_to_open, window, cx);
                    });
                }

                // Reply only once the page has loaded (or failed / timed out).
                cx.spawn(async move |this, cx| {
                    let mut last = None;
                    let mut no_tab_polls = 0;
                    for _ in 0..NAVIGATION_SETTLE_POLLS {
                        cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
                        let Ok(state) = this.update(cx, |app, cx| {
                            app.navigation_state(
                                &sess_id,
                                matching_view.as_ref(),
                                &known_ids,
                                &url,
                                cx,
                            )
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
                                    let entry = app.navigation_view(
                                        &sess_id,
                                        matching_view.as_ref(),
                                        &known_ids,
                                    );
                                    entry.map(|(id, view)| {
                                        let rid = format!("{request_id}-partial");
                                        let request = DriverRequest::content(None);
                                        view.update(cx, |bv, _| {
                                            bv.run_driver_script(&rid, &request)
                                        });
                                        let header = format_tab_header(&view, &id, None, None, cx);
                                        (view.clone(), rid, header)
                                    })
                                })
                                .ok()
                                .flatten();
                            let secs = (NAVIGATION_SETTLE_POLLS as u64
                                * BROWSER_POLL_INTERVAL.as_millis() as u64)
                                / 1000;
                            let mut msg =
                                format!("Navigation to {url} is still loading after {secs}s.");
                            if let Some((view, rid, header)) = partial {
                                msg.push_str(&format!("\n{header}"));
                                if let Ok(outcome) =
                                    poll_script_result(&this, cx, &view, &rid).await
                                {
                                    if let Ok(text) = outcome.value {
                                        msg.push_str(&format!("\nText loaded so far:\n{text}"));
                                    }
                                }
                            }
                            (Some(msg), None)
                        }
                    };
                    let _ = this.update(cx, |app, cx| {
                        if let Some((id, _)) =
                            app.navigation_view(&sess_id, matching_view.as_ref(), &known_ids)
                        {
                            app.agent_browser_tabs.insert(sess_id.clone(), id);
                        }
                        app.resolve_browser_action_result(&sess_id, request_id, result, error, cx);
                    });
                })
                .detach();
            }

            "snapshot" | "click" | "type" => {
                let element_ref = req.element_ref.clone().filter(|s| !s.trim().is_empty());
                // A CSS selector is a fallback target for click/type: the
                // driver resolves the first visible match when no ref is
                // given (used by the server's browser kernel, whose scripts
                // address elements without snapshotting first).
                let has_selector = req
                    .selector
                    .as_deref()
                    .is_some_and(|s| !s.trim().is_empty());
                if action != "snapshot" && element_ref.is_none() && !has_selector {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some(format!("'{action}' needs a ref from a snapshot or a selector")),
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
                let entry =
                    match self.pick_browser_view(&sess_id, req.tab_id.as_deref(), None, false, cx) {
                        Ok(entry) => entry,
                        Err(err) => {
                            self.resolve_browser_action_result(
                                &sess_id,
                                request_id,
                                None,
                                Some(err),
                                cx,
                            );
                            return;
                        }
                    };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };
                let note = self.tab_choice_note(&sess_id, req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tabs.insert(sess_id.clone(), tab_id.clone());

                let request = DriverRequest::action(
                    &action,
                    element_ref.as_deref(),
                    req.text.as_deref(),
                    req.submit.unwrap_or(false),
                    req.selector.as_deref(),
                );
                let is_snapshot = action == "snapshot";
                let script_id = request_id.clone();
                view.update(cx, |bv, _| bv.run_driver_script(&script_id, &request));
                cx.spawn(async move |this, cx| {
                    let outcome = poll_script_result(&this, cx, &view, &script_id).await;
                    let (result, error) = match outcome {
                        Ok(outcome) => {
                            let header = this
                                .update(cx, |_, cx| {
                                    format_tab_header(
                                        &view,
                                        &tab_id,
                                        outcome.url.as_deref(),
                                        outcome.title.as_deref(),
                                        cx,
                                    )
                                })
                                .unwrap_or_default();
                            let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                            match outcome.value {
                                Ok(value) => {
                                    let hint = if is_snapshot {
                                        ""
                                    } else {
                                        "\n(Take a new snapshot if the page changed.)"
                                    };
                                    (Some(format!("{note}{header}\n\n{value}{hint}")), None)
                                }
                                Err(err) => (None, Some(format!("[tab {tab_id}] {err}"))),
                            }
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
                let Some(requested) = req
                    .tab_id
                    .as_deref()
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                else {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some(format!(
                            "'{action}' needs a tabId. Open tabs:\n{}",
                            self.describe_browser_tabs(&sess_id, cx)
                        )),
                        cx,
                    );
                    return;
                };
                // Unknown or other-chat ids error out with this chat's tab
                // listing; never fall back to another tab.
                let (tab_id, view) = match self.pick_browser_view(
                    &sess_id,
                    Some(requested),
                    None,
                    false,
                    cx,
                ) {
                    Ok(Some(entry)) => entry,
                    Ok(None) => return,
                    Err(err) => {
                        self.resolve_browser_action_result(
                            &sess_id,
                            request_id,
                            None,
                            Some(err),
                            cx,
                        );
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
                    self.agent_browser_tabs.insert(sess_id.clone(), tab_id.clone());
                    format!(
                        "Tab: {tab_id}\nTitle: {title}\nURL: {url}\nSwitched agent target to this tab."
                    )
                } else {
                    self.close_agent_browser_tab(&tab_id, cx);
                    let pointer = self.agent_browser_tabs.get(&sess_id).cloned();
                    if pointer.as_deref() == Some(tab_id.as_str()) {
                        match self.most_recent_session_tab(&sess_id) {
                            Some(next) => {
                                self.agent_browser_tabs.insert(sess_id.clone(), next);
                            }
                            None => {
                                self.agent_browser_tabs.remove(&sess_id);
                            }
                        }
                    }
                    let next = match self.agent_browser_tabs.get(&sess_id) {
                        Some(id) => format!("Agent target is now tab '{id}'."),
                        None => "No browser tabs are open in this chat.".to_string(),
                    };
                    format!(
                        "Tab: {tab_id}\nClosed: {url}\n{next}\nOpen tabs:\n{}",
                        self.describe_browser_tabs(&sess_id, cx)
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
                        Some(
                            "wait_for needs at least one of selector, urlContains, or text"
                                .to_string(),
                        ),
                        cx,
                    );
                    return;
                }
                let entry =
                    match self.pick_browser_view(&sess_id, req.tab_id.as_deref(), None, false, cx) {
                        Ok(entry) => entry,
                        Err(err) => {
                            self.resolve_browser_action_result(
                                &sess_id,
                                request_id,
                                None,
                                Some(err),
                                cx,
                            );
                            return;
                        }
                    };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };
                let note = self.tab_choice_note(&sess_id, req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tabs.insert(sess_id.clone(), tab_id.clone());

                let timeout = Duration::from_millis(
                    req.timeout_ms
                        .unwrap_or(WAIT_FOR_DEFAULT_MS)
                        .clamp(100, WAIT_FOR_MAX_MS),
                );
                cx.spawn(async move |this, cx| {
                    let started = std::time::Instant::now();
                    let mut attempt = 0u32;
                    let mut last_problem: Option<String> = None;
                    let mut last_url: Option<String> = None;
                    let mut last_title: Option<String> = None;
                    let mut hidden = false;
                    let mut met = false;
                    // Short one-shot checks (rather than one long in-page promise) so the
                    // wait survives full page loads that destroy the JS context.
                    loop {
                        attempt += 1;
                        let check_id = format!("{request_id}-wf-{attempt}");
                        let request = DriverRequest::wait(
                            selector.as_deref(),
                            url_contains.as_deref(),
                            text.as_deref(),
                        );
                        let started_check = this.update(cx, |_, cx| {
                            view.update(cx, |bv, _| bv.run_driver_script(&check_id, &request))
                        });
                        if started_check.is_err() {
                            return;
                        }
                        match poll_script_result_within(&this, cx, &view, &check_id, WAIT_FOR_CHECK_POLLS).await {
                            Ok(outcome) => {
                                let ScriptOutcome { value, url, title } = outcome;
                                last_url = url;
                                last_title = title;
                                match value {
                                    Ok(value) if value.contains("WF_OK") => {
                                        met = true;
                                        break;
                                    }
                                    Ok(value) if value.contains("WF_ERR") => {
                                        last_problem = Some(value);
                                        break;
                                    }
                                    Ok(value) if value.contains("WF_HIDDEN") => {
                                        hidden = true;
                                    }
                                    Ok(value) if value.contains("WF_NO") => {
                                        hidden = false;
                                    }
                                    Ok(_) => {}
                                    // No result usually means the page is mid-navigation; keep waiting.
                                    Err(_) => {}
                                }
                            }
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
                            let header = format_tab_header(
                                &view,
                                &tab_id,
                                last_url.as_deref(),
                                last_title.as_deref(),
                                cx,
                            );
                            let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                            if met {
                                (Some(format!("{note}Condition met after {elapsed_ms}ms\n{header}")), None)
                            } else if let Some(problem) = last_problem {
                                (None, Some(format!("{note}{problem}\n{header}")))
                            } else if hidden {
                                (
                                    None,
                                    Some(format!(
                                        "{note}Timed out after {}ms: the selector matched an element that is not visible\n{header}",
                                        timeout.as_millis()
                                    )),
                                )
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

            "run_js" | "get_content" | "query" | "extract" | "scroll" => {
                let is_js = action == "run_js";
                let request = if is_js {
                    let js = req.script.as_deref().unwrap_or("");
                    if js.trim().is_empty() {
                        self.resolve_browser_action_result(
                            &sess_id,
                            request_id,
                            None,
                            Some("Missing script for run_js action".to_string()),
                            cx,
                        );
                        return;
                    }
                    DriverRequest::eval(js)
                } else if action == "query" {
                    let selector = req.selector.as_deref().unwrap_or("");
                    if selector.trim().is_empty() {
                        self.resolve_browser_action_result(
                            &sess_id,
                            request_id,
                            None,
                            Some("Missing selector for query action".to_string()),
                            cx,
                        );
                        return;
                    }
                    DriverRequest::query(selector)
                } else if action == "extract" || action == "scroll" {
                    // Internal, used by the server's browser kernel: the JSON
                    // spec rides the script wire field; the driver parses it.
                    let spec = req.script.as_deref().unwrap_or("");
                    if spec.trim().is_empty() {
                        self.resolve_browser_action_result(
                            &sess_id,
                            request_id,
                            None,
                            Some(format!("Missing spec for {action} action")),
                            cx,
                        );
                        return;
                    }
                    if action == "extract" {
                        DriverRequest::extract(spec)
                    } else {
                        DriverRequest::scroll(spec)
                    }
                } else {
                    DriverRequest::content(req.selector.as_deref())
                };

                let entry =
                    match self.pick_browser_view(&sess_id, req.tab_id.as_deref(), req.url.as_deref(), true, cx)
                    {
                        Ok(entry) => entry,
                        Err(err) => {
                            self.resolve_browser_action_result(
                                &sess_id,
                                request_id,
                                None,
                                Some(err),
                                cx,
                            );
                            return;
                        }
                    };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };

                let note = self.tab_choice_note(&sess_id, req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tabs.insert(sess_id.clone(), tab_id.clone());
                let script_id = request_id.clone();
                view.update(cx, |bv, _| bv.run_driver_script(&script_id, &request));
                // The kernel reads tab identity itself (tabs_json) and parses
                // query/extract/scroll results as JSON, so those come back
                // bare; get_content keeps the header for the model.
                let with_header = action == "get_content";
                cx.spawn(async move |this, cx| {
                    let outcome = poll_script_result(&this, cx, &view, &script_id).await;
                    let (result, error) = match outcome {
                        Ok(outcome) => match outcome.value {
                            Ok(value) if is_js => {
                                let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                                (Some(format!("{note}Tab: {tab_id}\n{value}")), None)
                            }
                            Ok(value) => {
                                let note = note.map(|n| format!("{n}\n")).unwrap_or_default();
                                let body = if with_header {
                                    let header = this
                                        .update(cx, |_, cx| {
                                            format_tab_header(
                                                &view,
                                                &tab_id,
                                                outcome.url.as_deref(),
                                                outcome.title.as_deref(),
                                                cx,
                                            )
                                        })
                                        .unwrap_or_default();
                                    format!("{header}\n\n{value}")
                                } else {
                                    value
                                };
                                (Some(format!("{note}{body}")), None)
                            }
                            Err(err) => (None, Some(format!("[tab {tab_id}] {err}"))),
                        },
                        Err(err) => (None, Some(format!("[tab {tab_id}] {err}"))),
                    };
                    let _ = this.update(cx, |app, cx| {
                        app.resolve_browser_action_result(&sess_id, request_id, result, error, cx);
                    });
                })
                .detach();
            }

            "screenshot" => {
                let entry = match self.pick_browser_view(
                    &sess_id,
                    req.tab_id.as_deref(),
                    req.url.as_deref(),
                    true,
                    cx,
                ) {
                    Ok(entry) => entry,
                    Err(err) => {
                        self.resolve_browser_action_result(
                            &sess_id,
                            request_id,
                            None,
                            Some(err),
                            cx,
                        );
                        return;
                    }
                };
                let Some((tab_id, view)) = entry else {
                    self.retry_browser_action_or_fail(&sess_id, req, retries_left, cx);
                    return;
                };
                let note = self.tab_choice_note(&sess_id, req.tab_id.is_some(), &tab_id, cx);
                self.agent_browser_tabs.insert(sess_id.clone(), tab_id.clone());
                let Some(slot) = view.read(cx).start_snapshot() else {
                    self.resolve_browser_action_result(
                        &sess_id,
                        request_id,
                        None,
                        Some(format!(
                            "[tab {tab_id}] Tab has no live webview to capture (open it first)"
                        )),
                        cx,
                    );
                    return;
                };
                cx.spawn(async move |this, cx| {
                    let mut outcome = Err("Snapshot timed out".to_string());
                    for _ in 0..SCRIPT_RESULT_POLLS {
                        cx.background_executor().timer(BROWSER_POLL_INTERVAL).await;
                        if let Some(done) = slot.lock().unwrap().take() {
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
                "No active or matching browser tab in this chat. Open tabs:\n{}",
                self.describe_browser_tabs(session_id, cx)
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

    /// The most recently used open browser tab owned by this chat (by tab
    /// `last_active_at_ms`), else the lowest owned id; `None` when the chat
    /// has no browser tabs. Scoped to the chat so a never-owned tab can
    /// never become the agent's default target.
    fn most_recent_session_tab(&self, session_id: &str) -> Option<String> {
        let owned = self.session_browser_ids(session_id);
        let mut best: Option<(i64, String)> = None;
        for id in &owned {
            let ts = self
                .browser_tab_last_active_ms(id)
                .unwrap_or(0);
            let has_view = self.browser_views.contains_key(id);
            if !has_view {
                continue;
            }
            if best.as_ref().map_or(true, |(b, _)| ts > *b) {
                best = Some((ts, id.clone()));
            }
        }
        best.map(|(_, id)| id).or_else(|| {
            owned
                .iter()
                .find(|id| self.browser_views.contains_key(*id))
                .cloned()
        })
    }

    /// A tab's `last_active_at_ms` from its workspace tab config.
    fn browser_tab_last_active_ms(&self, browser_id: &str) -> Option<i64> {
        let key = format!("browser:{browser_id}");
        let mut found = None;
        for leaf in self.workspace_root.leaves() {
            for tab in &leaf.tabs {
                if tab.id() != key {
                    continue;
                }
                if let console_core::WorkspaceTabConfig::Browser {
                    last_active_at_ms, ..
                } = tab
                {
                    found = *last_active_at_ms;
                }
            }
        }
        for root in self.project_workspace_roots.values() {
            for leaf in root.leaves() {
                for tab in &leaf.tabs {
                    if tab.id() != key {
                        continue;
                    }
                    if let console_core::WorkspaceTabConfig::Browser {
                        last_active_at_ms, ..
                    } = tab
                    {
                        found = *last_active_at_ms;
                    }
                }
            }
        }
        found
    }

    /// Ids of the browser tabs this chat's agent created, across the active
    /// workspace and every stashed workspace. Sorted, deduplicated.
    fn session_browser_ids(&self, session_id: &str) -> Vec<String> {
        let mut ids = Vec::new();
        let mut record = |tab: &console_core::WorkspaceTabConfig| {
            if let console_core::WorkspaceTabConfig::Browser {
                session_id: Some(owner),
                browser_id,
                ..
            } = tab
            {
                if owner == session_id {
                    ids.push(browser_id.clone());
                }
            }
        };
        for leaf in self.workspace_root.leaves() {
            for tab in &leaf.tabs {
                record(tab);
            }
        }
        for root in self.project_workspace_roots.values() {
            for leaf in root.leaves() {
                for tab in &leaf.tabs {
                    record(tab);
                }
            }
        }
        ids.sort();
        ids.dedup();
        ids
    }

    /// The same tab list `describe_browser_tabs` renders, as JSON rows
    /// ({id, url, title, active}) for the server's browser kernel.
    fn browser_tabs_json(&self, session_id: &str, cx: &App) -> String {
        let ids = self.session_browser_ids(session_id);
        let target = self.agent_browser_tabs.get(session_id);
        let rows: Vec<serde_json::Value> = ids
            .iter()
            .map(|id| {
                let (title, url) = match self.browser_views.get(id) {
                    Some(view) => {
                        let bv = view.read(cx);
                        (
                            bv.tab_label().unwrap_or_default(),
                            bv.current_url().unwrap_or("(blank)").to_string(),
                        )
                    }
                    None => ("New Tab".to_string(), "(blank)".to_string()),
                };
                serde_json::json!({
                    "id": id,
                    "url": url,
                    "title": title,
                    "active": target.is_some_and(|t| t == id),
                })
            })
            .collect();
        serde_json::to_string(&rows).unwrap_or_else(|_| "[]".to_string())
    }

    /// One line per browser tab owned by this chat: id, title, URL, and which
    /// one is the agent's current target.
    fn describe_browser_tabs(&self, session_id: &str, cx: &App) -> String {
        let ids = self.session_browser_ids(session_id);
        if ids.is_empty() {
            return "No browser tabs are open in this chat. Use navigate to open one.".to_string();
        }
        let target = self.agent_browser_tabs.get(session_id);
        ids.iter()
            .map(|id| {
                let (label, url) = match self.browser_views.get(id) {
                    Some(view) => {
                        let bv = view.read(cx);
                        (
                            bv.tab_label().unwrap_or_default(),
                            bv.current_url().unwrap_or("(blank)").to_string(),
                        )
                    }
                    None => ("New Tab".to_string(), "(blank)".to_string()),
                };
                format!(
                    "- {}{}: \"{}\" {}",
                    id,
                    if target.is_some_and(|t| t == id) {
                        " (agent target)"
                    } else {
                        ""
                    },
                    label,
                    url
                )
            })
            .collect::<Vec<_>>()
            .join("\n")
    }

    /// Picks the tab an action should run against, from this chat's tabs only.
    /// - `tab_id` given: exactly that tab if the chat owns it, else an error
    ///   listing this chat's tabs (owned-but-not-yet-rendered falls through).
    /// - otherwise a tab matching `url` (when `allow_url_match`), else the
    ///   chat's agent target, else the chat's most recent own tab.
    /// `Ok(None)` means no tab exists yet in this chat.
    fn pick_browser_view(
        &self,
        session_id: &str,
        tab_id: Option<&str>,
        url: Option<&str>,
        allow_url_match: bool,
        cx: &App,
    ) -> Result<Option<(String, Entity<BrowserView>)>, String> {
        let owned = self.session_browser_ids(session_id);
        if let Some(id) = tab_id.map(str::trim).filter(|s| !s.is_empty()) {
            if !owned.iter().any(|o| o == id) {
                return Err(format!(
                    "No browser tab with id '{id}' in this chat. Open tabs:\n{}",
                    self.describe_browser_tabs(session_id, cx)
                ));
            }
            return match self.browser_views.get(id) {
                Some(view) => Ok(Some((id.to_string(), view.clone()))),
                None => Ok(None),
            };
        }
        if allow_url_match {
            if let Some(entry) =
                url.and_then(|u| self.find_matching_browser_view(session_id, u, cx))
            {
                return Ok(Some(entry));
            }
        }
        // Prefer the tab the agent last worked in: `navigate` opens tabs in the
        // background, so the visible tab is often not the one the agent means.
        if let Some(id) = self.agent_browser_tabs.get(session_id) {
            if owned.iter().any(|o| o == id) {
                if let Some(view) = self.browser_views.get(id) {
                    return Ok(Some((id.clone(), view.clone())));
                }
            }
        }
        // Fall back to the chat's most recent own tab.
        Ok(self
            .most_recent_session_tab(session_id)
            .and_then(|id| self.browser_views.get(&id).cloned().map(|view| (id, view))))
    }

    /// Warning shown when several of the chat's tabs are open and the agent
    /// named none, so a wrong-tab default is visible at the top of the result.
    fn tab_choice_note(&self, session_id: &str, explicit: bool, chosen: &str, cx: &App) -> Option<String> {
        let count = self.session_browser_ids(session_id).len();
        if explicit || count < 2 {
            return None;
        }
        Some(format!(
            "Note: {} tabs are open in this chat and no tabId was given, so tab '{}' was used (the tab this chat last navigated or acted on). Pass tabId to target another. Open tabs:\n{}",
            count,
            chosen,
            self.describe_browser_tabs(session_id, cx)
        ))
    }

    /// The tab a navigation is acting on: the reused one, or the newly opened
    /// one — detected as a browser tab owned by this chat that wasn't known
    /// before the navigation started.
    fn navigation_view(
        &self,
        session_id: &str,
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
        self.session_browser_ids(session_id)
            .into_iter()
            .find(|id| !known_ids.contains(id))
            .and_then(|id| self.browser_views.get(&id).cloned().map(|view| (id, view)))
    }

    /// Reports whether the navigation started by a `navigate` action has finished.
    fn navigation_state(
        &self,
        session_id: &str,
        existing: Option<&Entity<BrowserView>>,
        known_ids: &HashSet<String>,
        url: &str,
        cx: &App,
    ) -> NavigationState {
        let Some((id, view)) = self.navigation_view(session_id, existing, known_ids) else {
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

    /// Finds a chat-owned tab whose URL matches or shares the origin/prefix of
    /// `target_url`.
    fn find_matching_browser_view(
        &self,
        session_id: &str,
        target_url: &str,
        cx: &App,
    ) -> Option<(String, Entity<BrowserView>)> {
        let clean_target = target_url.trim().trim_end_matches('/');
        for id in self.session_browser_ids(session_id) {
            let Some(view) = self.browser_views.get(&id) else {
                continue;
            };
            if let Some(url) = view.read(cx).current_url() {
                let clean_url = url.trim().trim_end_matches('/');
                if clean_url == clean_target
                    || clean_url.starts_with(clean_target)
                    || clean_target.starts_with(clean_url)
                {
                    return Some((id, view.clone()));
                }
            }
        }
        None
    }

    /// Close every browser tab the deleted chat's agent created, wherever it
    /// lives (active workspace tree or a stashed workspace tree), dispose the
    /// now-unreferenced webviews, and drop the chat's agent target pointer.
    pub(crate) fn close_session_browser_tabs(&mut self, session_id: &str, cx: &mut Context<Self>) {
        let owned = self.session_browser_ids(session_id);
        if owned.is_empty() {
            self.agent_browser_tabs.remove(session_id);
            return;
        }
        // Active workspace: the normal close path so pane active-tab fixes and
        // sync happen there. Cloned first: closing mutates the tree.
        let mut active_closes: Vec<(String, String)> = Vec::new();
        for leaf in self.workspace_root.leaves() {
            for tab in &leaf.tabs {
                let owned_here = matches!(
                    tab,
                    console_core::WorkspaceTabConfig::Browser {
                        session_id: Some(owner),
                        ..
                    } if owner == session_id
                );
                if owned_here {
                    active_closes.push((leaf.id.clone(), tab.id()));
                }
            }
        }
        for (pane_id, tab_id) in active_closes {
            self.close_tab_and_sync_pane(&pane_id, &tab_id, cx);
        }
        // Stashed workspaces: drop the tabs there; disposal happens below.
        for root in self.project_workspace_roots.values_mut() {
            console_ui::workspace::ops::close_matching_tabs(root, |tab| {
                matches!(
                    tab,
                    console_core::WorkspaceTabConfig::Browser {
                        session_id: Some(owner),
                        ..
                    } if owner == session_id
                )
            });
        }
        for id in &owned {
            if let Some(view) = self.browser_views.remove(id) {
                view.update(cx, |bv, cx| bv.close(cx));
            }
        }
        self.agent_browser_tabs.remove(session_id);
        self.persist_workspaces();
        self.sync_workspace_webviews(cx);
        cx.notify();
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
