//! Binary integration state for the Integrations settings page.
//!
//! Separate from state/github.rs, which is the PAT credential itself. This is
//! the *binary* side: whether the server box has tools like the `gh` CLI
//! installed for the agent to shell out to. The credential can be connected
//! while the binary is missing (and vice versa), so the two states must not
//! be merged.

use super::ConsoleDesktopApp;
use gpui::Context;

impl ConsoleDesktopApp {
    /// Ask the server for the current integration rows.
    ///
    /// Runs on every Integrations tab open: detection is live server-side, so
    /// installing the binary then re-opening the tab flips the row with no
    /// restart. A failure keeps the last known rows and records the error
    /// beside them — blanking the page on a transient failure would read as
    /// "everything uninstalled".
    pub fn refresh_integrations(&mut self, cx: &mut Context<Self>) {
        if self.integrations_loading {
            return;
        }
        self.integrations_loading = true;
        self.integrations_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.integrations.list().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.integrations_loading = false;
                        match result {
                            Ok(rows) => {
                                this.integrations = rows;
                                this.integrations_error = None;
                            }
                            Err(error) => {
                                this.integrations_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }
}
