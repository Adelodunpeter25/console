//! Repo-task state: the GitHub section of the project pickers.
//!
//! One file for everything a repo tap does — list the connected account's
//! repositories, clone one (or reuse an existing project), and hand the
//! result to the surface that asked. The token itself is managed elsewhere
//! (state/github.rs); this file only consumes its "connected" state.
//!
//! Two surfaces share it and differ in one step:
//! - The docked footer's project menu opens the fresh session immediately
//!   (`open_repo_task` → `adopt_repo_task`) — there is no prompt typed yet.
//! - The floating composer card (`floating_clone_repo`) only selects the
//!   cloned project for its submit, so the prompt the user is typing stays
//!   in the card and becomes the session's first run.

use std::rc::Rc;

use gpui::Context;

use super::ConsoleDesktopApp;
use console_core::GithubRepo;

impl ConsoleDesktopApp {
    /// Fetch the connected account's repositories for the pickers' GitHub
    /// section. A failure is kept in `github_repos_error` so the menus can
    /// distinguish "no repos" from "could not load".
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
                        // An open floating composer paints its own copy of
                        // this list — keep it in step.
                        let mirror = this.floating_composer.open;
                        match result {
                            Ok(repos) => {
                                this.github_repos = Some(std::rc::Rc::new(repos.clone()));
                                this.github_repos_error = None;
                                if mirror {
                                    this.floating_composer.github_repos =
                                        Some(std::rc::Rc::new(repos));
                                    this.floating_composer.github_repos_loading = false;
                                    this.floating_composer.github_repos_error = None;
                                }
                            }
                            // Not-connected is a state, not a failure: the
                            // menus show their own "connect" row for it, not
                            // an error line.
                            Err(error) if console_core::is_github_not_connected(&error) => {
                                this.github_repos = None;
                                this.github_repos_error = None;
                                this.github_status = None;
                                if mirror {
                                    this.floating_composer.github_repos = None;
                                    this.floating_composer.github_repos_loading = false;
                                    this.floating_composer.github_repos_error = None;
                                }
                            }
                            Err(error) => {
                                log::warn!("github repo list error: {error}");
                                this.github_repos_error = Some(error.to_string());
                                if mirror {
                                    this.floating_composer.github_repos_loading = false;
                                    this.floating_composer.github_repos_error =
                                        Some(error.to_string());
                                }
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// The footer's one-tap action: clone (or reuse) the repo and open the
    /// fresh session. The server owns the destination; see
    /// `POST /api/sessions/from-repo`.
    pub fn open_repo_task(
        &mut self,
        repo: GithubRepo,
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
                            Err(error) if console_core::is_github_not_connected(&error) => {
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

    /// The floating composer's action: clone the repo and select the
    /// resulting project for the card's submit. No session is opened here —
    /// the card creates the session on submit, so the prompt the user is
    /// typing survives the tap.
    pub fn floating_clone_repo(
        &mut self,
        repo: GithubRepo,
        cx: &mut Context<Self>,
    ) {
        if self.repo_task_busy {
            return;
        }
        self.repo_task_busy = true;
        self.floating_composer.error = None;
        cx.notify();

        let client = self.client.clone();
        let url = repo.clone_target();
        cx.spawn(async move |entity, cx| {
            let result = client.projects.clone_project(&url, "", None).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.repo_task_busy = false;
                        match result {
                            Ok(project) => {
                                if !this.projects.iter().any(|p| p.id == project.id) {
                                    Rc::make_mut(&mut this.projects).push(project.clone());
                                }
                                // The card paints its own copy of the list.
                                if !this
                                    .floating_composer
                                    .projects
                                    .iter()
                                    .any(|p| p.id == project.id)
                                {
                                    Rc::make_mut(&mut this.floating_composer.projects)
                                        .push(project.clone());
                                }
                                this.floating_project_changed(project.id, cx);
                            }
                            Err(error) if console_core::is_github_not_connected(&error) => {
                                this.github_status = None;
                                this.github_repos = None;
                                this.open_github_settings(cx);
                            }
                            Err(error) => {
                                this.floating_composer.error = Some(error.to_string());
                                cx.notify();
                            }
                        }
                    });
                }
            });
        })
        .detach();
    }

    /// Mirror a repo task's results into local state and open the fresh
    /// session. The server registered the project and created the session;
    /// this only mirrors them, then `select_and_open_session` handles the
    /// workspace switch (it reads the session's project).
    pub fn adopt_repo_task(
        &mut self,
        task: console_core::types::RepoTask,
        cx: &mut Context<Self>,
    ) {
        if !self.projects.iter().any(|p| p.id == task.project.id) {
            Rc::make_mut(&mut self.projects).push(task.project.clone());
        }
        if !self.sessions.iter().any(|s| s.id == task.session.id) {
            Rc::make_mut(&mut self.sessions).push(task.session.clone());
        }
        crate::window::broadcast_settings_refresh(cx);
        self.select_and_open_session(task.session.id.clone(), cx);
    }

    /// Open Settings on the Providers tab, where the GitHub token lives.
    /// Runs on the main window even when called from a pane menu, mirroring
    /// the New-project dialog's handoff.
    pub(crate) fn open_github_settings(&mut self, cx: &mut Context<Self>) {
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
