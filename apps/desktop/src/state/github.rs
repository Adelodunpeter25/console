//! GitHub git-credential state for the Providers settings page.
//!
//! Separate from state/auth.rs, which is the OAuth flow for the LLM chat
//! providers (Codex / Claude / Antigravity). GitHub is not a chat provider —
//! its token exists so the git CLI on the *server host* can clone private
//! repositories — so it authenticates by pasted token, not by browser.
//!
//! That difference is why this cannot reuse `login_provider`: that path opens
//! a browser and waits on a loopback listener bound to this machine, which
//! never completes when the backend is a remote box.

use super::ConsoleDesktopApp;
use gpui::Context;

impl ConsoleDesktopApp {
    /// Ask the server for the stored GitHub credential state.
    ///
    /// A failure is reported, not silent: "Not connected" must mean "no
    /// token", and a failed fetch showing the same text made an unreachable
    /// backend indistinguishable from a disconnected account. The error
    /// renders under the row, where a form error would also appear.
    pub fn refresh_github_status(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.auth.github_status().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(status) => {
                                this.github_status = Some(status);
                                this.github_error = None;
                            }
                            Err(error) => {
                                this.github_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Validate and store a personal access token.
    ///
    /// The token is written straight into the request body and never retained
    /// in app state — the caller drops it as soon as this returns. On success
    /// the cached username/scopes replace the typed token, so the UI can clear
    /// the field without losing the row's state.
    pub fn connect_github(&mut self, token: String, cx: &mut Context<Self>) {
        let trimmed = token.trim().to_string();
        if trimmed.is_empty() {
            self.github_error = Some("Enter a personal access token.".to_string());
            cx.notify();
            return;
        }
        if self.github_busy {
            return;
        }

        self.github_busy = true;
        self.github_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.auth.connect_github(&trimmed).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(status) => {
                                this.github_status = Some(status);
                                this.github_error = None;
                            }
                            // The server's wording is already written to be
                            // shown as-is (it separates "bad token" from
                            // "GitHub is down"), so pass it through rather than
                            // rewriting it.
                            Err(error) => {
                                this.github_error = Some(error.to_string());
                            }
                        }
                        this.github_busy = false;
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Remove the stored token. Safe to call when already disconnected.
    pub fn disconnect_github(&mut self, cx: &mut Context<Self>) {
        if self.github_busy {
            return;
        }
        self.github_busy = true;
        self.github_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.auth.disconnect_github().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.github_busy = false;
                        // Drop the cached status either way: if the remove call
                        // failed but the token is gone, a stale "connected" row
                        // would be worse than showing disconnected.
                        this.github_status = None;
                        if let Err(error) = result {
                            this.github_error = Some(error.to_string());
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Show or hide the token field.
    pub fn set_github_form_open(&mut self, open: bool, cx: &mut Context<Self>) {
        self.github_form_open = open;
        // Closing the sheet must not leave the previous failure on screen.
        if !open {
            self.github_error = None;
        }
        cx.notify();
    }

    /// Fetch the connected account's repositories for the project menu's
    /// GitHub section. A failure is kept in `github_repos_error` so the menu
    /// can distinguish "no repos" from "could not load".
    pub fn refresh_github_repos(&mut self, cx: &mut Context<Self>) {
        if self.github_repos_loading {
            return;
        }
        self.github_repos_loading = true;
        self.github_repos_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.projects.list_github_repos().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.github_repos_loading = false;
                        match result {
                            Ok(repos) => {
                                this.github_repos = Some(std::rc::Rc::new(repos));
                                this.github_repos_error = None;
                            }
                            // Not-connected is a state, not a failure: the
                            // menu shows its own "connect" row for it, not
                            // an error line.
                            Err(error)
                                if console_core::is_github_not_connected(&error) =>
                            {
                                this.github_repos = None;
                                this.github_repos_error = None;
                                this.github_status = None;
                            }
                            Err(error) => {
                                this.github_repos_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// The one-tap repo-picker action: clone (or reuse) the repo and open
    /// the fresh session. The server owns the destination; see
    /// `POST /api/sessions/from-repo`.
    pub fn open_repo_task(
        &mut self,
        repo: console_core::types::GithubRepo,
        cx: &mut Context<Self>,
    ) {
        if self.repo_task_busy {
            return;
        }
        self.repo_task_busy = true;
        self.github_repos_error = None;
        cx.notify();

        let client = self.client.clone();
        let url = repo.clone_target();
        cx.spawn(async move |entity, cx| {
            let result = client.projects.create_session_from_repo(&url, None).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.repo_task_busy = false;
                        match result {
                            Ok(task) => this.adopt_repo_task(task, cx),
                            // Same handoff as the New-project dialog: a 409
                            // means the token is missing or dead, so send the
                            // user to Accounts instead of showing a git error.
                            Err(error)
                                if console_core::is_github_not_connected(&error) =>
                            {
                                this.github_status = None;
                                this.github_repos = None;
                                this.open_github_settings(cx);
                            }
                            Err(error) => {
                                this.github_repos_error = Some(error.to_string());
                                cx.notify();
                            }
                        }
                    });
                }
            });
        })
        .detach();
    }

    /// Open Settings on the Providers tab, where the GitHub token lives.
    /// Runs on the main window even when called from a pane menu, mirroring
    /// the New-project dialog's handoff.
    fn open_github_settings(&mut self, cx: &mut Context<Self>) {
        if let Some(main_handle) = self.main_window_handle {
            let app_entity = cx.entity().clone();
            cx.defer(move |cx| {
                let _ = main_handle.update(cx, |_root, main_window, cx| {
                    main_window.activate_window();
                    app_entity.update(cx, |this, cx| {
                        this.open_settings_tab(
                            console_ui::settings::SettingsTab::Providers,
                            main_window,
                            cx,
                        );
                    });
                });
            });
        }
    }
}
