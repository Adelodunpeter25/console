//! Startup backend bootstrap & background polling routines.

use std::rc::Rc;
use console_core::{ModelFavorite, SelectedModel};
use console_ui::PickerTab;
use gpui::Context;

use super::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    pub(crate) fn wire_palette_callbacks(&mut self, cx: &mut Context<Self>) {
        let entity = cx.entity().downgrade();
        self.quick_open_palette.update(cx, |palette, cx| {
            palette.set_on_open_file(
                move |path, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let pane_id = this
                                .active_pane_id
                                .clone()
                                .unwrap_or_else(|| "pane-main".to_string());
                            this.open_file_tab_in_pane(&pane_id, path, cx);
                        });
                    }
                },
                cx,
            );
        });

        let entity = cx.entity().downgrade();
        self.project_browse_palette.update(cx, |palette, cx| {
            palette.set_on_select_project(
                move |path, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.add_project_from_path(path, cx));
                    }
                },
                cx,
            );
        });
    }

    pub(crate) fn bootstrap_backend(
        &mut self,
        target: crate::window::WindowLaunchTarget,
        cx: &mut Context<Self>,
    ) {
        let client_clone = self.client.clone();
        let target_for_bootstrap = target.clone();

        cx.spawn(async move |entity, cx| {
            // 1. Fetch the session list for the sidebar.
            match client_clone.sessions.list(None, None).await {
                Ok(sessions) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.sessions = Rc::new(sessions);
                                if matches!(
                                    target_for_bootstrap,
                                    crate::window::WindowLaunchTarget::RestorePersisted
                                ) {
                                    let mut to_load = Vec::new();
                                    for leaf in this.workspace_root.leaves() {
                                        if let Some(session_id) =
                                            this.active_session_for_pane(&leaf.id)
                                        {
                                            to_load.push((leaf.id.clone(), session_id));
                                        }
                                    }
                                    for (pane_id, session_id) in to_load {
                                        if this.active_pane_id.as_deref() == Some(&pane_id)
                                            || this.selected_session_id.is_none()
                                        {
                                            this.selected_session_id = Some(session_id.clone());
                                        }
                                        this.load_session_messages_for_pane(
                                            pane_id, session_id, cx,
                                        );
                                    }
                                }
                                match target_for_bootstrap {
                                    crate::window::WindowLaunchTarget::Fresh { .. } => {
                                        // Blank independent canvas, no session opened by default.
                                    }
                                    crate::window::WindowLaunchTarget::Session(session_id) => {
                                        this.select_and_open_session(session_id, cx);
                                    }
                                    _ => {}
                                }
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to load sessions: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            }

            // 2. Fetch providers and models concurrently
            let providers_client = client_clone.clone();
            let providers_entity = entity.clone();
            cx.spawn(async move |cx| {
                match providers_client.providers.list().await {
                    Ok(providers) => {
                        cx.update(|cx| {
                            if let Some(app) = providers_entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    let first_model = providers.first().and_then(|p| {
                                        p.models.first().map(|m| SelectedModel {
                                            provider: p.name.clone(),
                                            model_id: m.id.clone(),
                                        })
                                    });
                                    this.providers = Rc::new(providers);
                                    if let Some(first) = this.providers.first() {
                                        let first_name = first.name.clone();
                                        let needs_init = match &this.active_picker_tab {
                                            PickerTab::Favorites => true,
                                            PickerTab::Provider(name) => {
                                                !this.providers.iter().any(|p| &p.name == name)
                                            }
                                        };
                                        if needs_init {
                                            this.active_picker_tab =
                                                PickerTab::Provider(first_name.clone());
                                            if let Some(state) =
                                                this.workspace_pane_states.get_mut("pane-main")
                                            {
                                                state.active_picker_tab =
                                                    PickerTab::Provider(first_name);
                                            }
                                        }
                                    }
                                    if this.selected_model.is_none() {
                                        this.selected_model = first_model.clone();
                                    }
                                    if let Some(state) =
                                        this.workspace_pane_states.get_mut("pane-main")
                                    {
                                        if state.selected_model.is_none() {
                                            state.selected_model = first_model;
                                        }
                                    }
                                    cx.notify();
                                });
                            }
                        });
                    }
                    Err(error) => {
                        let message = format!("Unable to load providers and models: {error}");
                        cx.update(|cx| {
                            if let Some(app) = providers_entity.upgrade() {
                                app.update(cx, |this, cx| this.set_error(message, cx));
                            }
                        });
                    }
                }
            })
            .detach();

            // 3. Load model favorites
            let favorites_client = client_clone.clone();
            let favorites_entity = entity.clone();
            cx.spawn(async move |cx| match favorites_client.model_favorites.list().await {
                Ok(model_favorites) => {
                    cx.update(|cx| {
                        if let Some(app) = favorites_entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.favorites = Rc::new(
                                    model_favorites
                                        .into_iter()
                                        .map(|favorite: ModelFavorite| {
                                            format!(
                                                "{}:{}",
                                                favorite.provider, favorite.model_id
                                            )
                                        })
                                        .collect(),
                                );
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to load model favorites: {error}");
                    cx.update(|cx| {
                        if let Some(app) = favorites_entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            })
            .detach();

            // 4. Load projects and git branches
            match client_clone.projects.list().await {
                Ok(projects) => {
                    let project_path = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.projects = Rc::new(projects);
                                if this.selected_project_id.is_none() {
                                    this.selected_project_id = this
                                        .sessions
                                        .iter()
                                        .find(|session| {
                                            Some(&session.id) == this.selected_session_id.as_ref()
                                        })
                                        .and_then(|session| {
                                            session.project_id.clone().or_else(|| {
                                                this.projects
                                                    .iter()
                                                    .find(|p| {
                                                        !session.cwd.is_empty()
                                                            && p.path == session.cwd
                                                    })
                                                    .map(|p| p.id.clone())
                                            })
                                        });
                                }
                                let active_id = this
                                    .active_pane_id
                                    .clone()
                                    .unwrap_or_else(|| "pane-main".into());
                                if let Some(state) = this.workspace_pane_states.get_mut(&active_id)
                                {
                                    if state.selected_project_id.is_none() {
                                        state.selected_project_id =
                                            this.selected_project_id.clone();
                                    }
                                }
                                let path = this
                                    .selected_project_for_pane(&active_id)
                                    .map(|project| project.path.clone());
                                cx.notify();
                                path.map(|p| (active_id, p))
                            })
                        } else {
                            None
                        }
                    });
                    if let Some((active_id, path)) = project_path {
                        let client = client_clone.clone();
                        let entity = entity.clone();
                        cx.spawn(async move |cx| {
                            match client.git.list_branches(Some(&path)).await {
                                Ok(branches) => cx.update(|cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.branches = Rc::new(branches.branches.clone());
                                            this.branch_loaded = true;
                                            this.branch_is_git_repository =
                                                branches.is_git_repository;
                                            if let Some(state) =
                                                this.workspace_pane_states.get_mut(&active_id)
                                            {
                                                state.branches = Rc::new(branches.branches);
                                                state.branch_loaded = true;
                                                state.branch_is_git_repository =
                                                    branches.is_git_repository;
                                            }
                                            cx.notify();
                                        });
                                    }
                                }),
                                Err(_) => {}
                            }
                        })
                        .detach();
                    }
                }
                Err(error) => {
                    let message = format!("Unable to load projects: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            }
        })
        .detach();

        // 5. Periodic polling for session list / working status sync across surfaces.
        let poll_client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            loop {
                cx.background_executor()
                    .timer(std::time::Duration::from_secs(30))
                    .await;
                if let Ok(sessions) = poll_client.sessions.list(None, None).await {
                    let _ = cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.sessions = Rc::new(sessions);
                                cx.notify();
                            });
                        }
                    });
                }
            }
        })
        .detach();
    }
}
