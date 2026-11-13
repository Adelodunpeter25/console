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
            if let Some(url) = initial_url.clone()
                && !url.is_empty()
            {
                view.navigate_to_url(url, cx);
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
            // The annotation popover floats above the native webview in GPUI,
            // so an open popover (or address context menu) must hide the
            // webview via the occlusion sync, and closing it must restore
            // the live surface. The view notifies on every such change.
            this.sync_workspace_webviews(cx);
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
                        && id == &bid
                        && (*title != new_title || *url != new_url)
                    {
                        *title = new_title.clone();
                        *url = new_url.clone();
                        changed = true;
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
                            && id == &bid
                            && (*title != new_title || *url != new_url)
                        {
                            *title = new_title.clone();
                            *url = new_url.clone();
                            changed = true;
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
        let mut active_browser_ids: std::collections::HashSet<String> = self
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

        // The right sidebar's Browser tab hosts a webview too: it is the
        // visible surface while that tab is open, and hidden otherwise.
        if self.right_sidebar_visible
            && self.inspector_active_tab
                == console_ui::InspectorTab::Auxiliary(console_ui::AuxiliaryTab::Browser)
            && let Some(id) = &self.inspector_browser_tab_id
        {
            active_browser_ids.insert(id.clone());
        }

        // Settings take over the window; native webviews paint above GPUI, so
        // they must be hidden rather than left at their old bounds.
        if self.settings_open {
            active_browser_ids.clear();
        }

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
        let (browser_id, tab) = self.new_browser_tab(url.clone(), None, None, &pane_id);
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

    /// Whether agent browsing for `session_id` should land in the right
    /// sidebar's Browser tab: the sidebar is visible, the Browser tab is
    /// open, and it targets this chat's workspace. When true, agent-opened
    /// tabs render there instead of opening workspace tabs.
    pub fn sidebar_browser_targets(&self, session_id: &str) -> bool {
        self.right_sidebar_visible
            && self
                .inspector_open_auxiliary_tabs
                .contains(&console_ui::AuxiliaryTab::Browser)
            && self.active_inspector_target().0.as_deref() == Some(session_id)
    }

    /// Create a browser tab hosted in the right sidebar's Browser tab (no
    /// workspace tab entry), owned by `session_id`, and make it the one on
    /// screen. Existing tabs stay in the sidebar's tab strip; a chat with
    /// none gets its first.
    pub fn open_sidebar_browser_tab(
        &mut self,
        session_id: &str,
        url: Option<String>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Option<Entity<BrowserView>> {
        if cfg!(target_os = "windows") {
            if let Some(url) = url {
                cx.open_url(&url);
            }
            return None;
        }

        let browser_id = format!(
            "browser-{}-{}",
            chrono::Utc::now().timestamp_millis(),
            self.browser_views.len(),
        );
        self.sidebar_session_browsers
            .entry(session_id.to_string())
            .or_default()
            .push(browser_id.clone());
        self.inspector_browser_tab_id = Some(browser_id.clone());
        let view = self.get_or_create_browser_view(&browser_id, url, window, cx);
        // The view renders next frame; create the native browser now so the
        // page starts loading while the kernel's wait loop is polling.
        view.update(cx, |view, cx| view.ensure_host(window, cx));
        cx.notify();
        Some(view)
    }

    /// Drop a browser tab's workspace entries so it renders only in the
    /// right sidebar, keeping the view itself alive. A sidebar takeover of
    /// a tab the agent opened while the sidebar was closed.
    pub(crate) fn detach_browser_tab_from_workspaces(
        &mut self,
        browser_id: &str,
        cx: &mut Context<Self>,
    ) {
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
                let had_tab = self.project_workspace_roots.values().any(|root| {
                    root.leaves()
                        .iter()
                        .any(|leaf| leaf.tabs.iter().any(|t| t.id() == key))
                });
                if had_tab {
                    for root in self.project_workspace_roots.values_mut() {
                        console_ui::workspace::ops::close_matching_tabs(root, |t| t.id() == key);
                    }
                    self.persist_workspaces();
                }
            }
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

        // The sidebar's Browser tab owns agent browsing while it is open on
        // this chat's workspace: tabs open there, never as workspace tabs.
        if self.sidebar_browser_targets(session_id) {
            return self.open_sidebar_browser_tab(session_id, Some(url), window, cx);
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
            Active {
                pane_id: String,
            },
            Stashed {
                key: Option<String>,
                pane_id: String,
            },
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
                    let pane_id = self
                        .active_pane_id
                        .clone()
                        .unwrap_or_else(|| "pane-main".into());
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
                let root = self.project_workspace_roots.get_mut(key)?;
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
