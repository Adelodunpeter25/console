use std::time::Duration;
use gpui::Context;
use serde::{Deserialize, Serialize};
use console_ui::settings::{EnvironmentRow, ProbeState};
use crate::persistence::store::{
    PersistedEnvironment, PersistedEnvironmentsState, load_environments, save_environments,
};
use super::ConsoleDesktopApp;

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct Environment {
    pub id: String,
    pub name: String,
    pub url: String,
}

impl Environment {
    pub fn default_local() -> Self {
        Self {
            id: "default-local".to_string(),
            name: "Local Daemon".to_string(),
            url: "http://127.0.0.1:4040".to_string(),
        }
    }
}

impl ConsoleDesktopApp {
    pub fn init_environments(&mut self) {
        if let Some(persisted) = load_environments() {
            if !persisted.environments.is_empty() {
                self.environments = persisted.environments.into_iter().map(|e| Environment {
                    id: e.id,
                    name: e.name,
                    url: e.url,
                }).collect();
                self.active_env_id = persisted.active_id.or_else(|| self.environments.first().map(|e| e.id.clone()));
                return;
            }
        }
        let def = Environment::default_local();
        self.active_env_id = Some(def.id.clone());
        self.environments = vec![def];
        self.save_persisted_environments();
    }

    pub fn save_persisted_environments(&self) {
        let state = PersistedEnvironmentsState {
            environments: self.environments.iter().map(|e| PersistedEnvironment {
                id: e.id.clone(),
                name: e.name.clone(),
                url: e.url.clone(),
            }).collect(),
            active_id: self.active_env_id.clone(),
        };
        save_environments(state);
    }

    pub fn environment_rows(&self) -> Vec<EnvironmentRow> {
        self.environments.iter().map(|e| {
            let is_active = self.active_env_id.as_deref() == Some(&e.id);
            let probe_state = self.env_probes.get(&e.id).copied().unwrap_or(ProbeState::Unknown);
            EnvironmentRow {
                id: e.id.clone(),
                name: e.name.clone(),
                url: e.url.clone(),
                is_active,
                probe_state,
            }
        }).collect()
    }

    pub fn probe_environment(&mut self, env_id: String, cx: &mut Context<Self>) {
        let Some(env) = self.environments.iter().find(|e| e.id == env_id) else { return; };
        let url = env.url.clone();
        self.env_probes.insert(env_id.clone(), ProbeState::Probing);
        cx.notify();

        cx.spawn(async move |entity, cx| {
            let ok = console_core::utils::probe_backend(&url, Duration::from_secs(3)).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.env_probes.insert(env_id, if ok.is_ok() { ProbeState::Ok } else { ProbeState::Failed });
                        cx.notify();
                    });
                }
            });
        }).detach();
    }

    pub fn add_environment(&mut self, name: String, url: String, cx: &mut Context<Self>) {
        let id = format!("env-{}", std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap_or_default().as_millis());
        let env = Environment { id, name, url };
        self.environments.push(env);
        self.save_persisted_environments();
        cx.notify();
    }

    pub fn remove_environment(&mut self, env_id: String, cx: &mut Context<Self>) {
        if self.active_env_id.as_deref() == Some(&env_id) {
            return; // Cannot remove active
        }
        self.environments.retain(|e| e.id != env_id);
        self.env_probes.remove(&env_id);
        self.save_persisted_environments();
        cx.notify();
    }

    pub fn activate_environment(&mut self, env_id: String, cx: &mut Context<Self>) {
        let Some(env) = self.environments.iter().find(|e| e.id == env_id).cloned() else { return; };
        self.active_env_id = Some(env_id);
        self.save_persisted_environments();

        let client = self.client.clone();
        let url = env.url.clone();

        cx.spawn(async move |entity, cx| {
            client.set_base_url(&url).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // Reset in-memory session and workspace caches
                        this.sessions = std::rc::Rc::new(Vec::new());
                        this.selected_session_id = None;
                        this.projects = std::rc::Rc::new(Vec::new());
                        this.selected_project_id = None;
                        this.branches = std::rc::Rc::new(Vec::new());
                        this.branch_loaded = false;
                        this.transcript_scroll_positions.clear();

                        // Reload data from new server
                        this.load_sessions(cx);
                        this.load_providers(cx);
                        this.load_projects(cx);
                        this.refresh_auth_status(cx);
                        cx.notify();
                    });
                }
            });
        }).detach();
    }
}
