//! Forwarded ports management and browser preview routing for workspace projects.

use std::rc::Rc;

use gpui::Context;

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Retrieve the forwarded ports for the currently active workspace/project.
    pub fn forwarded_ports_for_active_workspace(&self) -> Rc<Vec<console_core::ForwardedPort>> {
        let key = self.active_project_id_or_global();
        self.forwarded_ports_by_project
            .get(&key)
            .cloned()
            .unwrap_or_else(|| Rc::new(Vec::new()))
    }

    pub fn active_project_id_or_global(&self) -> String {
        self.active_pane_id
            .as_deref()
            .and_then(|pane_id| self.pane_project_id(pane_id))
            .or_else(|| self.selected_project_id.clone())
            .unwrap_or_else(|| "global".to_string())
    }

    /// Fetch latest forwarded ports from the server for the active workspace.
    pub fn fetch_forwarded_ports(&mut self, cx: &mut Context<Self>) {
        let project_id = self.active_project_id_or_global();
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        let pid_clone = project_id.clone();
        cx.spawn(async move |_, cx| {
            let pid_opt = if pid_clone == "global" {
                None
            } else {
                Some(pid_clone.as_str())
            };
            if let Ok(ports) = client.ports.list(pid_opt).await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let changed = this
                                .forwarded_ports_by_project
                                .get(&pid_clone)
                                .is_none_or(|current| current.as_ref() != &ports);
                            if changed {
                                this.forwarded_ports_by_project
                                    .insert(pid_clone, Rc::new(ports));
                                cx.notify();
                            }
                        });
                    }
                });
            }
        })
        .detach();
    }

    /// Unforward a port on the server.
    pub fn unforward_port(&mut self, port: u16, cx: &mut Context<Self>) {
        let project_id = self.active_project_id_or_global();
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        let pid_clone = project_id.clone();
        cx.spawn(async move |_, cx| {
            let pid_opt = if pid_clone == "global" {
                None
            } else {
                Some(pid_clone.as_str())
            };
            if let Ok(()) = client.ports.unforward(port, pid_opt).await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(list) = this.forwarded_ports_by_project.get_mut(&pid_clone)
                            {
                                let items = list
                                    .iter()
                                    .filter(|p| p.port != port)
                                    .cloned()
                                    .collect::<Vec<_>>();
                                *list = Rc::new(items);
                            }
                            cx.notify();
                            // Reconcile with the server immediately after the optimistic
                            // removal so externally managed metadata stays authoritative.
                            this.fetch_forwarded_ports(cx);
                        });
                    }
                });
            }
        })
        .detach();
    }

    /// Open a forwarded port URL directly inside the embedded browser.
    pub fn open_port_in_browser(
        &mut self,
        url: String,
        window: &mut gpui::Window,
        cx: &mut Context<Self>,
    ) {
        self.right_sidebar_visible = true;
        self.inspector_active_tab = console_ui::InspectorTab::Browser;
        let browser = self.browser_view_for_inspector(window, cx);
        browser.update(cx, |view, cx| {
            view.navigate_to_url(url, cx);
        });
        cx.notify();
    }
}
