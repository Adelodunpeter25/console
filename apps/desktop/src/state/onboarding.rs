//! First-run setup panel: shown once per server, dismissed forever.
//!
//! The distinction that makes this simple is between the two kinds of state:
//!
//!   - **Stored** — one bit per server: has this person seen the panel? That is
//!     a fact about the human, not the server, and no endpoint can tell us it.
//!   - **Derived** — is this server actually set up (has a project, a provider
//!     key, a GitHub key)? Read live from the server every time the panel
//!     opens.
//!
//! Nothing about server setup is persisted. If it were, the two would drift the
//! first time a user deleted their last project, and the panel would either
//! lie ("all done" over an empty server) or re-open itself at someone who
//! already did the work. Because only the dismissal is stored, deleting a
//! project correctly shows that step as unmet again while the panel stays shut.

use super::ConsoleDesktopApp;
use gpui::{Context, Window};
use std::rc::Rc;

/// Live setup status for the active server. Every field is a fact the server
/// just told us; none is remembered.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct SetupStatus {
    /// The server has at least one project.
    pub has_project: bool,
    /// At least one provider has an API key stored.
    pub has_provider: bool,
    /// A GitHub personal access token is stored.
    pub has_github: bool,
}

impl SetupStatus {
    /// Every step satisfied — the panel has nothing left to offer.
    pub fn is_complete(&self) -> bool {
        self.has_project && self.has_provider && self.has_github
    }
}

impl ConsoleDesktopApp {
    /// Show the setup panel if this server has never had it dismissed.
    ///
    /// Called from both places that change which server is active — the boot
    /// path in `init_environments` and `activate_environment` — because boot
    /// assigns `active_env_id` directly rather than going through the switch
    /// path, and "launch the app onto a new server" is the case that matters
    /// most.
    ///
    /// Cheap when the server is already dismissed: the set is read in memory
    /// and the function returns without a single request.
    pub fn maybe_show_onboarding(&mut self, env_id: Option<&str>, cx: &mut Context<Self>) {
        let Some(env_id) = env_id else {
            return;
        };
        if self.onboarding_dismissed.contains(env_id) {
            self.onboarding_env_id = None;
            return;
        }
        self.onboarding_env_id = Some(env_id.to_string());
        self.refresh_setup_status(cx);
        cx.notify();
    }

    /// Fetch the three live facts behind the panel's steps.
    ///
    /// Each is independent and a failure is confined to its own step: a server
    /// with no GitHub token still 200s `/api/auth/github/status`, but if that
    /// endpoint is unreachable the GitHub row shows as unmet rather than the
    /// whole panel failing to render. That is deliberate — an onboarding panel
    /// that errors out teaches the user nothing.
    pub fn refresh_setup_status(&mut self, cx: &mut Context<Self>) {
        self.setup_status_loading = true;
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            // Issued together: three sequential round trips would make the
            // panel feel slow for no reason.
            let (projects, providers, github) = futures::join!(
                client.projects.list(),
                client.auth.provider_credentials(),
                client.auth.github_status(),
            );
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let has_project = projects.map(|list| !list.is_empty()).unwrap_or(false);
                        let has_provider = providers
                            .map(|statuses| {
                                statuses
                                    .providers
                                    .values()
                                    .any(|status| status.configured == Some(true))
                            })
                            .unwrap_or(false);
                        let has_github = github
                            .map(|status| status.connected == Some(true))
                            .unwrap_or(false);
                        this.setup_status = SetupStatus {
                            has_project,
                            has_provider,
                            has_github,
                        };
                        this.setup_status_loading = false;
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Dismiss the panel for the active server.
    ///
    /// Latching, per server: other servers keep their own bit, so dismissing
    /// setup on a remote box does not silence the panel on the local one.
    pub fn dismiss_onboarding(&mut self, cx: &mut Context<Self>) {
        if let Some(env_id) = self.onboarding_env_id.take() {
            self.onboarding_dismissed.insert(env_id.clone());
            crate::persistence::dismiss_onboarding(&env_id);
        }
        cx.notify();
    }

    /// Close the panel after the user engages a step.
    ///
    /// Same effect as skipping, and for the same reason: the panel is a
    /// one-shot pointer, not a progress tracker. The steps themselves are all
    /// reachable from Settings afterwards.
    pub fn close_onboarding(&mut self, cx: &mut Context<Self>) {
        self.dismiss_onboarding(cx);
    }

    /// Build the panel, or `None` when no server is due one.
    ///
    /// Every step's action closes the panel first: engaging a step is the
    /// clearest possible signal that the user has read it, and leaving a panel
    /// floating over the palette it just opened would be worse than closing it.
    pub fn onboarding_panel_view(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Option<console_ui::OnboardingPanel> {
        let env_id = self.onboarding_env_id.clone()?;
        let server_name = self
            .environments
            .iter()
            .find(|e| e.id == env_id)
            .map(|e| e.name.clone())
            .unwrap_or_else(|| "This server".to_string());
        let entity = cx.entity().downgrade();

        // Each step closes the panel before running its action: engaging a step
        // is the clearest signal the user has read the panel, and leaving it
        // floating over the palette it just opened would be worse than closing.
        let close_then_open_settings = {
            let entity = entity.clone();
            Rc::new(move |window: &mut Window, cx: &mut gpui::App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.close_onboarding(cx));
                    // Providers and the GitHub key both live on Settings >
                    // Providers, which `open_settings` already opens (and which
                    // already refreshes both statuses on entry).
                    app.update(cx, |this, cx| this.open_settings(window, cx));
                }
            })
        };

        Some(console_ui::OnboardingPanel {
            server_name,
            status: console_ui::SetupStatus {
                has_project: self.setup_status.has_project,
                has_provider: self.setup_status.has_provider,
                has_github: self.setup_status.has_github,
            },
            loading: self.setup_status_loading,
            // Invokes the `AddProject` logic rather than synthesizing a
            // keystroke, so a rebound shortcut cannot break this step.
            on_add_project: {
                let entity = entity.clone();
                Rc::new(move |window: &mut Window, cx: &mut gpui::App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.close_onboarding(cx));
                        app.update(cx, |this, cx| this.open_project_browse(window, cx));
                    }
                })
            },
            on_open_providers: close_then_open_settings.clone(),
            on_open_github: close_then_open_settings,
            on_dismiss: Rc::new(move |_window: &mut Window, cx: &mut gpui::App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.dismiss_onboarding(cx));
                }
            }),
        })
    }
}