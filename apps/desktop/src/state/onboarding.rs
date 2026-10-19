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
use gpui::{AnyElement, Context, IntoElement, ParentElement, Styled, Window, div};
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

// Deliberately no `is_complete` here: the panel computes it from its own
// mirror of these fields, since console-ui cannot depend on the app binary. One
// implementation lives in console-ui, in src/onboarding/panel.rs.

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

    /// Build the panel, or `None` when no server is due one.
    ///
    /// Every step's action closes the panel first: engaging a step is the
    /// clearest possible signal that the user has read it, and leaving a panel
    /// floating over the palette it just opened would be worse than closing it.
    pub fn onboarding_panel_view(&mut self, cx: &mut Context<Self>) -> Option<console_ui::OnboardingPanel> {
        let env_id = self.onboarding_env_id.clone()?;
        let server_name = self
            .environments
            .iter()
            .find(|e| e.id == env_id)
            .map(|e| e.name.clone())
            .unwrap_or_else(|| "This server".to_string());
        let entity = cx.entity().downgrade();

        // Engaging a step deliberately does NOT dismiss the panel.
        //
        // It used to, and that was wrong: opening the project palette also wrote
        // the dismissal, so pressing Escape to back out of the palette left the
        // user with neither onboarding nor a project — the panel had recorded
        // itself as seen at the exact moment they were mid-task.
        //
        // Only the explicit Skip button dismisses now. Escape from a step's
        // dialog returns to the panel, which is what pressing Escape means.
        let open_settings = {
            let entity = entity.clone();
            Rc::new(move |window: &mut Window, cx: &mut gpui::App| {
                if let Some(app) = entity.upgrade() {
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
                        app.update(cx, |this, cx| this.open_project_browse(window, cx));
                    }
                })
            },
            on_open_providers: open_settings.clone(),
            on_open_github: open_settings,
            on_dismiss: Rc::new(move |_window: &mut Window, cx: &mut gpui::App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.dismiss_onboarding(cx));
                }
            }),
        })
    }

    /// Render the setup panel as the entire main window.
    ///
    /// Returned from the window's render *instead of* the workspace, which is
    /// what keeps the title bar, sidebar and inspector out of the way. The
    /// overlay version left the sidebar showing through and read as an
    /// interruption rather than a first-run screen.
    ///
    /// The project-browse palette is re-parented in here so the "Add project"
    /// step still opens a dialog on top of the panel: the palette renders as
    /// part of the workspace root, so without this the step would do nothing at
    /// all while the panel had the window to itself.
    pub fn render_onboarding(&mut self, cx: &mut Context<Self>) -> AnyElement {
        let palette = self.project_browse_palette.clone();
        match self.onboarding_panel_view(cx) {
            Some(panel) => div()
                .size_full()
                .child(panel)
                .child(palette)
                .into_any_element(),
            // Raced with a dismissal between the check and this render.
            None => div().size_full().into_any_element(),
        }
    }
}