use console_core::WorkspaceTabConfig;
use console_ui::browser::{BrowserView, default_browser_title};
use console_ui::workspace::ops as workspace_ops;
use gpui::{AppContext, Context, Entity, Focusable as _, Window};

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open a new Browser tab in the active pane. No URL yet: the view shows
    /// its start page until the user submits an address or a port-open /
    /// chat-link navigates it via `open_browser_smart`.
    pub fn open_browser_tab(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.open_browser_tab_with_url(None, true, window, cx);
    }

    /// Retrieve an existing `BrowserView` entity for `browser_id` or instantiate a
    /// new one, observing its URL and page title updates to synchronize back to the
    /// workspace tab strip.
    pub fn get_or_create_browser_view(
        &mut self,
        browser_id: &str,
        initial_url: Option<String>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Entity<BrowserView> {
        if let Some(view) = self.browser_views.get(browser_id) {
            return view.clone();
        }

        let view = cx.new(|cx| {
            let mut view = BrowserView::new(window, cx);
            if let Some(url) = initial_url.clone() {
                if !url.is_empty() {
                    view.navigate_to_url(url, cx);
                }
            }
            view
        });
        let handler_bid = browser_id.to_string();
        let app_entity = cx.entity().downgrade();
        view.update(cx, |v, _cx| {
            v.set_annotation_handler(move |annotation, cx| {
                let _ = app_entity.update(cx, |this, cx| {
                    this.stage_browser_annotation(&handler_bid, annotation, cx);
                });
            });
        });

        let bid = browser_id.to_string();
        let sub = cx.observe(&view, move |this, view, cx| {
            let (new_title, new_url) = {
                let bv = view.read(cx);
                let title = bv.tab_label().unwrap_or_else(|| "New Tab".to_string());
                let url = bv.current_url().unwrap_or("").to_string();
                (title, url)
            };
            let mut changed = false;
            for leaf in this.workspace_root.leaves_mut() {
                for tab in &mut leaf.tabs {
                    if let WorkspaceTabConfig::Browser {
                        browser_id: id,
                        title,
                        url,
                        ..
                    } = tab
                    {
                        if id == &bid {
                            if *title != new_title || *url != new_url {
                                *title = new_title.clone();
                                *url = new_url.clone();
                                changed = true;
                            }
                        }
                    }
                }
            }
            for root in this.project_workspace_roots.values_mut() {
                for leaf in root.leaves_mut() {
                    for tab in &mut leaf.tabs {
                        if let WorkspaceTabConfig::Browser {
                            browser_id: id,
                            title,
                            url,
                            ..
                        } = tab
                        {
                            if id == &bid {
                                if *title != new_title || *url != new_url {
                                    *title = new_title.clone();
                                    *url = new_url.clone();
                                    changed = true;
                                }
                            }
                        }
                    }
                }
            }
            if changed {
                this.persist_workspaces();
                cx.notify();
            }
        });
        self._subscriptions.push(sub);

        self.browser_views
            .insert(browser_id.to_string(), view.clone());
        view
    }

    /// Synchronize the visibility and occlusion state of all workspace browser webviews.
    /// Only browser tabs active in currently visible panes stay visible; inactive ones are hidden.
    pub fn sync_workspace_webviews(&self, cx: &mut Context<Self>) {
        let active_browser_ids: std::collections::HashSet<String> = self
            .workspace_root
            .leaves()
            .into_iter()
            .filter_map(|leaf| {
                let active_id = leaf.active_tab_id.as_deref()?;
                leaf.tabs.iter().find(|t| t.id() == active_id)
            })
            .filter_map(|tab| match tab {
                WorkspaceTabConfig::Browser { browser_id, .. } => Some(browser_id.clone()),
                _ => None,
            })
            .collect();

        let palettes_open = self.any_palette_open(cx);

        for (browser_id, view) in &self.browser_views {
            let is_active = active_browser_ids.contains(browser_id);
            let occluded = palettes_open || view.read(cx).overlay_open(cx);
            view.update(cx, |v, cx| {
                v.sync_native_state(is_active, occluded, cx);
            });
        }
    }

    /// Open a Browser tab in the active pane, optionally seeded with a URL.
    /// Title defaults to host + port and updates from the page title when the
    /// view reports one.
    pub fn open_browser_tab_with_url(
        &mut self,
        url: Option<String>,
        focus: bool,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        // Browser tabs are deferred on Windows. A URL still opens in the
        // system browser so forwarded ports and chat links stay useful.
        if cfg!(target_os = "windows") {
            if let Some(url) = url {
                cx.open_url(&url);
            }
            return;
        }

        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        let (browser_id, tab) =
            self.new_browser_tab(url.clone(), None, None, &pane_id);
        if focus {
            workspace_ops::open_tab(&mut self.workspace_root, &pane_id, tab);
        } else {
            // Background open (agent-initiated): add the tab but leave the
            // user's current tab and pane focus alone.
            workspace_ops::open_tab_in_background(&mut self.workspace_root, &pane_id, tab);
        }
        self.active_pane_id = Some(pane_id);
        let view = self.finish_browser_tab_open(&browser_id, url, !focus, window, cx);
        if focus {
            // Focus the browser so keyboard input works immediately (address bar via ⌘L, etc.)
            window.focus(&view.read(cx).focus_handle(cx), cx);
        }
    }

    /// Open a chat's agent browser tab (always in the background) inside the
    /// workspace that chat lives in, tagged with the owning session so the
    /// chat's browser actions can only ever reach its own tabs. It never
    /// steals focus: the user keeps their current tab and pane. Returns the
    /// view for the navigation wait loop.
    pub fn open_agent_browser_tab(
        &mut self,
        session_id: &str,
        url: String,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Option<Entity<BrowserView>> {
        if cfg!(target_os = "windows") {
            cx.open_url(&url);
            return None;
        }

        let chat_tab_id = format!("chat:{session_id}");
        // The pane showing this chat in the active (visible) workspace, if any.
        let active_chat_pane = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.tabs.iter().any(|t| t.id() == chat_tab_id))
            .map(|leaf| leaf.id.clone());

        // Which tree/pane the tab opens in: the chat's own workspace when one
        // can be located, else the active pane as before.
        enum Target {
            Active { pane_id: String },
            Stashed { key: Option<String>, pane_id: String },
        }
        let target = if let Some(pane_id) = active_chat_pane {
            Target::Active { pane_id }
        } else {
            let mut stashed = None;
            for (key, root) in self.project_workspace_roots.iter() {
                let leaves = root.leaves();
                let chat_pane = leaves
                    .iter()
                    .find(|leaf| leaf.tabs.iter().any(|t| t.id() == chat_tab_id))
                    .map(|leaf| leaf.id.clone());
                let remembered_pane = self
                    .project_active_panes
                    .get(key)
                    .cloned()
                    .filter(|pane| leaves.iter().any(|leaf| &leaf.id == pane));
                let pane = chat_pane
                    .or(remembered_pane)
                    .or_else(|| root.first_leaf().map(|leaf| leaf.id.clone()));
                if let Some(pane) = pane {
                    stashed = Some((key.clone(), pane));
                    break;
                }
            }
            match stashed {
                Some((key, pane_id)) => Target::Stashed { key, pane_id },
                // Chat rendered nowhere: fall back to the active tree/pane.
                None => {
                    let pane_id =
                        self.active_pane_id.clone().unwrap_or_else(|| "pane-main".into());
                    Target::Active { pane_id }
                }
            }
        };
        let (tree_key, pane_id) = match &target {
            Target::Active { pane_id } => (None, pane_id.clone()),
            Target::Stashed { key, pane_id } => (key.clone(), pane_id.clone()),
        };

        let (browser_id, tab) = self.new_browser_tab(
            Some(url.clone()),
            Some(session_id.to_string()),
            tree_key,
            &pane_id,
        );
        match &target {
            Target::Active { .. } => {
                workspace_ops::open_tab_in_background(&mut self.workspace_root, &pane_id, tab);
            }
            Target::Stashed { key, .. } => {
                let Some(root) = self.project_workspace_roots.get_mut(key) else {
                    return None;
                };
                workspace_ops::open_tab_in_background(root, &pane_id, tab);
            }
        }
        self.agent_browser_tabs
            .insert(session_id.to_string(), browser_id.clone());
        let view = self.finish_browser_tab_open(&browser_id, Some(url), true, window, cx);
        Some(view)
    }

    /// Build the id and tab config for the next browser tab in `pane_id`.
    /// `project_key` wins when given (agent tabs know their workspace); user
    /// tabs fall back to the pane's project selection.
    fn new_browser_tab(
        &self,
        url: Option<String>,
        owner_session: Option<String>,
        project_key: Option<String>,
        pane_id: &str,
    ) -> (String, WorkspaceTabConfig) {
        let browser_id = format!(
            "browser-{}-{}",
            chrono::Utc::now().timestamp_millis(),
            self.browser_views.len(),
        );
        let title = url
            .as_deref()
            .map(default_browser_title)
            .unwrap_or_else(|| "New Tab".into());
        let tab = WorkspaceTabConfig::Browser {
            browser_id: browser_id.clone(),
            url: url.unwrap_or_default(),
            title,
            session_id: owner_session,
            project_id: project_key.or_else(|| self.pane_project_id(pane_id)),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        (browser_id, tab)
    }

    /// Post-open bookkeeping once a browser tab is in a workspace tree:
    /// create/fetch the webview, ensure the host for background tabs, sync
    /// visibility, persist, and notify. Returns the view for focusing/results.
    fn finish_browser_tab_open(
        &mut self,
        browser_id: &str,
        url: Option<String>,
        background: bool,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Entity<BrowserView> {
        let view = self.get_or_create_browser_view(browser_id, url, window, cx);
        if background {
            // A background tab is never rendered, so create its native browser
            // now (hidden) so the page loads without the user clicking it. A
            // focused tab is rendered next frame, which creates it as usual.
            view.update(cx, |view, cx| view.ensure_host(window, cx));
        }
        self.sync_workspace_webviews(cx);
        self.persist_workspaces();
        cx.notify();
        view
    }

    /// Open a chat transcript web link inside the embedded browser (same path
    /// as forwarded-port previews). File links stay on workspace tabs.
    pub fn open_chat_url_in_browser(
        &mut self,
        url: String,
        window: &mut gpui::Window,
        cx: &mut Context<Self>,
    ) {
        self.open_port_in_browser(url, window, cx);
    }
}
