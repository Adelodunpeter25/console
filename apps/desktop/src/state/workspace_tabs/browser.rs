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
        self.open_browser_tab_with_url(None, window, cx);
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
            if let Some(url) = initial_url {
                if !url.is_empty() {
                    view.navigate_to_url(url, cx);
                }
            }
            view
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

        let is_overlay_open = self.any_palette_open(cx);

        for (browser_id, view) in &self.browser_views {
            let is_active = active_browser_ids.contains(browser_id);
            view.update(cx, |v, cx| {
                v.sync_native_state(is_active, is_overlay_open, cx);
            });
        }
    }

    /// Open a Browser tab in the active pane, optionally seeded with a URL.
    /// Title defaults to host + port and updates from the page title when the
    /// view reports one.
    pub fn open_browser_tab_with_url(
        &mut self,
        url: Option<String>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        let browser_id = format!(
            "browser-{}-{}",
            chrono::Utc::now().timestamp_millis(),
            self.browser_views.len(),
        );
        let title = url
            .as_deref()
            .map(default_browser_title)
            .unwrap_or_else(|| "New Tab".into());
        let browser_view = self.get_or_create_browser_view(&browser_id, url.clone(), window, cx);
        let tab = WorkspaceTabConfig::Browser {
            browser_id: browser_id.clone(),
            url: url.unwrap_or_default(),
            title,
            project_id: self.pane_project_id(&pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        workspace_ops::open_tab(&mut self.workspace_root, &pane_id, tab);
        self.active_pane_id = Some(pane_id);
        self.sync_workspace_webviews(cx);
        self.persist_workspaces();
        // Focus the browser so keyboard input works immediately (address bar via ⌘L, etc.)
        window.focus(&browser_view.read(cx).focus_handle(cx), cx);
        cx.notify();
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
