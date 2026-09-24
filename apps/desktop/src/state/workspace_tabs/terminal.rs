use console_core::WorkspaceTabConfig;
use console_ui::terminal::TerminalView;
use console_ui::workspace::ops as workspace_ops;
use gpui::{AppContext, Context, Entity, Focusable as _, Window};

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Open a new Terminal tab in the active pane. The PTY cwd is the pane's
    /// selected project path. No fallback: without an explicit project
    /// selection there is no safe cwd to send to the server (process cwd or
    /// "." may not exist there, especially after switching servers).
    pub fn open_terminal_tab(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        let Some(cwd) = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path.clone())
        else {
            self.set_error("Select a project before opening a terminal.", cx);
            return;
        };
        self.open_terminal_tab_in_pane(&pane_id, cwd, window, cx);
    }

    /// Open a Terminal tab running at `cwd` in a specific pane.
    pub fn open_terminal_tab_in_pane(
        &mut self,
        pane_id: &str,
        cwd: String,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let terminal_id = format!(
            "term-{}-{}",
            chrono::Utc::now().timestamp_millis(),
            self.terminals.len(),
        );
        let view = self.get_or_create_terminal_view(&terminal_id, cwd, window, cx);
        let tab = WorkspaceTabConfig::Terminal {
            terminal_id: terminal_id.clone(),
            title: "Terminal".into(),
            project_id: self.pane_project_id(pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };
        workspace_ops::open_tab(&mut self.workspace_root, pane_id, tab);
        self.active_pane_id = Some(pane_id.to_string());
        self.sync_workspace_webviews(cx);
        self.persist_workspaces();
        // Focus the terminal so keyboard input works immediately
        window.focus(&view.read(cx).focus_handle(cx), cx);
        cx.notify();
    }

    /// Retrieve an existing `TerminalView` entity for `terminal_id` or instantiate a
    /// new one, observing its dynamic title updates to synchronize back to the
    /// workspace tab strip.
    pub fn get_or_create_terminal_view(
        &mut self,
        terminal_id: &str,
        cwd: String,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Entity<TerminalView> {
        if let Some(view) = self.terminals.get(terminal_id) {
            return view.clone();
        }

        let view = cx.new(|cx| TerminalView::with_cwd(cwd, self.client.clone(), window, cx));

        let tid = terminal_id.to_string();
        let sub = cx.observe(&view, move |this, view, cx| {
            let tv = view.read(cx);
            let new_title = if tv.status() == console_core::types::terminal::TerminalStatus::Exited {
                "Terminal".to_string()
            } else {
                tv.title()
                    .filter(|t| !t.trim().is_empty())
                    .unwrap_or("Terminal")
                    .to_string()
            };
            let mut changed = false;
            for leaf in this.workspace_root.leaves_mut() {
                for tab in &mut leaf.tabs {
                    if let WorkspaceTabConfig::Terminal {
                        terminal_id: id,
                        title,
                        ..
                    } = tab
                    {
                        if id == &tid {
                            if *title != new_title {
                                *title = new_title.clone();
                                changed = true;
                            }
                        }
                    }
                }
            }
            for root in this.project_workspace_roots.values_mut() {
                for leaf in root.leaves_mut() {
                    for tab in &mut leaf.tabs {
                        if let WorkspaceTabConfig::Terminal {
                            terminal_id: id,
                            title,
                            ..
                        } = tab
                        {
                            if id == &tid {
                                if *title != new_title {
                                    *title = new_title.clone();
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

        self.terminals
            .insert(terminal_id.to_string(), view.clone());
        view
    }
}
