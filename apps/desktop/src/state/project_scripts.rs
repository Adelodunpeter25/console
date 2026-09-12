//! Project run-script panel state.
//!
//! Script definitions and execution live on the server (`console.toml`); this
//! module keeps the desktop's view of them, scoped per project so switching
//! projects can never show another project's commands or runs.

use std::collections::{HashMap, HashSet};

use console_core::{
    ProjectScript, ScriptRun, ScriptRunEvent, ScriptRunStatus, compute_shortcut_state,
};
use console_ui::run::{push_run_output, truncate_run_output_head};
use gpui::{Context, Keystroke};

use super::ConsoleDesktopApp;

/// Latest known run for one script row, plus its rendered output tail.
#[derive(Clone, Debug, Default)]
pub struct ProjectScriptRunView {
    pub run: Option<ScriptRun>,
    pub output: String,
    pub starting: bool,
}

/// Everything the Run tab needs for one project.
#[derive(Clone, Debug, Default)]
pub struct ProjectScriptsPanelState {
    pub scripts: Vec<ProjectScript>,
    /// `"console.toml"` or `"missing"`, straight from the list response.
    pub source: String,
    pub loaded: bool,
    pub loading: bool,
    pub error: Option<String>,
    /// Latest run per script id.
    pub runs: HashMap<String, ProjectScriptRunView>,
    /// Script ids with their output expanded.
    pub expanded: HashSet<String>,
    /// Canonical shortcuts claimed by more than one script. Rows showing one
    /// of these get a warning badge and the shortcut stays unbound — every
    /// other script's shortcut keeps working.
    pub shortcut_conflicts: HashSet<String>,
    pub generation: u64,
}

fn snapshot_output(run: &ScriptRun) -> String {
    let mut output = String::with_capacity(run.stdout.len() + run.stderr.len() + 1);
    output.push_str(&run.stdout);
    output.push_str(&run.stderr);
    truncate_run_output_head(&mut output);
    output
}

impl ConsoleDesktopApp {
    /// Project id behind the active pane: the active tab's project wins,
    /// falling back to the pane's selected project.
    pub fn active_scripts_project_id(&self) -> Option<String> {
        let pane_id = self.active_pane_id.as_deref().unwrap_or("pane-main");
        let leaf = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|l| l.id == pane_id);
        let active_tab = leaf.as_ref().and_then(|l| {
            l.active_tab_id
                .as_deref()
                .and_then(|tab_id| l.tabs.iter().find(|t| t.id() == tab_id))
        });
        let from_tab = match active_tab {
            Some(console_core::WorkspaceTabConfig::Chat {
                session_id,
                project_id,
                ..
            }) => self
                .sessions
                .iter()
                .find(|s| &s.id == session_id)
                .and_then(|s| s.project_id.clone())
                .or(project_id.clone()),
            Some(console_core::WorkspaceTabConfig::File { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Diff { project_id, .. })
            | Some(console_core::WorkspaceTabConfig::Terminal { project_id, .. }) => {
                project_id.clone()
            }
            None => None,
        };
        from_tab.or_else(|| {
            self.selected_project_for_pane(pane_id)
                .map(|p| p.id.clone())
        })
    }

    /// Fetch script definitions (once per project) and reconnect to any runs
    /// the server still holds. Cheap guards make this safe to call on every
    /// render while the bottom panel is visible.
    pub fn ensure_project_scripts(&mut self, cx: &mut Context<Self>) {
        let Some(project_id) = self.active_scripts_project_id() else {
            return;
        };
        let loaded = self
            .project_scripts_by_project
            .get(&project_id)
            .is_some_and(|state| state.loading || state.loaded);
        if loaded {
            return;
        }
        self.fetch_project_scripts(project_id, cx);
    }

    /// Re-read `console.toml` even when already loaded, so edits never need
    /// an app restart. Runs and expanded rows merge by script id, so a
    /// refresh never collapses open output or drops a live tail.
    pub fn refresh_project_scripts(&mut self, cx: &mut Context<Self>) {
        let Some(project_id) = self.active_scripts_project_id() else {
            return;
        };
        let loading = self
            .project_scripts_by_project
            .get(&project_id)
            .is_some_and(|state| state.loading);
        if loading {
            return;
        }
        self.fetch_project_scripts(project_id, cx);
    }

    fn fetch_project_scripts(&mut self, project_id: String, cx: &mut Context<Self>) {
        let state = self
            .project_scripts_by_project
            .entry(project_id.clone())
            .or_default();
        state.loading = true;
        state.generation += 1;
        let generation = state.generation;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let list = client.scripts.list(&project_id).await;
            let runs = client
                .scripts
                .list_runs(&project_id)
                .await
                .unwrap_or_default();
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // Snapshot the active-project id before any mutable
                        // borrows land — both readers below are immutable
                        // but the borrow checker can't see that across the
                        // later `state.runs.get_mut`.
                        let active_pid = this
                            .selected_project_id
                            .clone()
                            .or_else(|| this.active_scripts_project_id());

                        let running: Vec<(String, String, String)> = {
                            let Some(state) = this.project_scripts_by_project.get_mut(&project_id)
                            else {
                                return;
                            };
                            if state.generation != generation {
                                return;
                            }
                            match list {
                                Ok(result) => {
                                    state.scripts = result.scripts;
                                    state.source = result.source;
                                    state.error = None;
                                }
                                Err(err) => {
                                    state.error =
                                        Some(format!("Couldn't load run scripts: {err:#}"));
                                }
                            }
                            // Refresh the dispatch state now that we have the
                            // latest script list. Cheap: O(scripts). Unique
                            // shortcuts stay live; only duplicated ones are
                            // disabled, and every row reads its badge from the
                            // stored set below — never from a run view, so
                            // never-run scripts warn too.
                            let shortcut_state = compute_shortcut_state(&state.scripts);
                            state.shortcut_conflicts =
                                shortcut_state.conflicts.keys().cloned().collect();
                            // Only repopulate the active shortcut map when
                            // this project is the one currently driving
                            // keyboard dispatch.
                            if active_pid.as_deref() == Some(project_id.as_str()) {
                                this.active_project_shortcuts = shortcut_state.bindings;
                            }
                            for run in runs {
                                // A live stream already owns running views —
                                // never clobber its tail with a snapshot.
                                let owned = state.runs.get(&run.script_id).is_some_and(|view| {
                                    view.run.as_ref().is_some_and(|current| {
                                        current.run_id == run.run_id
                                            && current.status == ScriptRunStatus::Running
                                    })
                                });
                                if !owned {
                                    let output = snapshot_output(&run);
                                    state.runs.insert(
                                        run.script_id.clone(),
                                        ProjectScriptRunView {
                                            run: Some(run),
                                            output,
                                            starting: false,
                                        },
                                    );
                                }
                            }
                            state.loading = false;
                            state.loaded = true;
                            state
                                .runs
                                .iter()
                                .filter(|(_, view)| {
                                    view.run
                                        .as_ref()
                                        .is_some_and(|run| run.status == ScriptRunStatus::Running)
                                })
                                .map(|(script_id, view)| {
                                    (
                                        project_id.clone(),
                                        script_id.clone(),
                                        view.run.as_ref().map(|run| run.run_id.clone()),
                                    )
                                })
                                .filter_map(|(pid, sid, rid)| rid.map(|rid| (pid, sid, rid)))
                                .collect()
                        };
                        for (pid, sid, rid) in running {
                            this.watch_script_run(&pid, &sid, &rid, cx);
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Start a script by id and follow its run. The command comes from the
    /// server's `console.toml` — only the id travels over the wire.
    pub fn run_project_script(&mut self, script_id: &str, cx: &mut Context<Self>) {
        let Some(project_id) = self.active_scripts_project_id() else {
            return;
        };
        // The row shows Stop while a run is active or starting — a second
        // play press is always a no-op, never a duplicate run.
        if let Some(state) = self.project_scripts_by_project.get(&project_id)
            && let Some(view) = state.runs.get(script_id)
            && (view.starting
                || view
                    .run
                    .as_ref()
                    .is_some_and(|run| run.status == ScriptRunStatus::Running))
        {
            return;
        }
        if let Some(state) = self.project_scripts_by_project.get_mut(&project_id) {
            state
                .runs
                .entry(script_id.to_string())
                .or_default()
                .starting = true;
            state.expanded.insert(script_id.to_string());
            state.error = None;
        }
        cx.notify();

        let client = self.client.clone();
        let project_id_clone = project_id.clone();
        let script_id_owned = script_id.to_string();
        cx.spawn(async move |entity, cx| {
            let started = client
                .scripts
                .start_run(&project_id_clone, &script_id_owned)
                .await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match started {
                            Ok(run) => {
                                let output = snapshot_output(&run);
                                let run_id = run.run_id.clone();
                                if let Some(state) =
                                    this.project_scripts_by_project.get_mut(&project_id_clone)
                                {
                                    state.runs.insert(
                                        script_id_owned.clone(),
                                        ProjectScriptRunView {
                                            run: Some(run),
                                            output,
                                            starting: false,
                                        },
                                    );
                                }
                                this.watch_script_run(
                                    &project_id_clone,
                                    &script_id_owned,
                                    &run_id,
                                    cx,
                                );
                            }
                            Err(err) => {
                                if let Some(state) =
                                    this.project_scripts_by_project.get_mut(&project_id_clone)
                                {
                                    if let Some(view) = state.runs.get_mut(&script_id_owned) {
                                        view.starting = false;
                                    }
                                    state.error = Some(format!("Couldn't start script: {err:#}"));
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

    /// Ask the server to stop a script's latest run. The final status still
    /// arrives through the run's stream.
    pub fn stop_project_script(&mut self, script_id: &str, cx: &mut Context<Self>) {
        let Some(project_id) = self.active_scripts_project_id() else {
            return;
        };
        let run_id = self
            .project_scripts_by_project
            .get(&project_id)
            .and_then(|state| state.runs.get(script_id))
            .and_then(|view| view.run.as_ref())
            .filter(|run| run.status == ScriptRunStatus::Running)
            .map(|run| run.run_id.clone());
        let Some(run_id) = run_id else {
            return;
        };
        // A stopped run restarts fresh: drop the old output now so the next
        // start never shows stale logs beneath the new ones.
        if let Some(state) = self.project_scripts_by_project.get_mut(&project_id)
            && let Some(view) = state.runs.get_mut(script_id)
        {
            view.output.clear();
        }
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if let Err(err) = client.scripts.stop_run(&project_id, &run_id).await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let Some(state) =
                                this.project_scripts_by_project.get_mut(&project_id)
                            {
                                state.error = Some(format!("Couldn't stop script: {err:#}"));
                            }
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

    pub fn toggle_project_script_expanded(&mut self, script_id: &str, cx: &mut Context<Self>) {
        if let Some(project_id) = self.active_scripts_project_id()
            && let Some(state) = self.project_scripts_by_project.get_mut(&project_id)
        {
            if !state.expanded.remove(script_id) {
                state.expanded.insert(script_id.to_string());
            }
            cx.notify();
        }
    }

    /// Rebuild `active_project_shortcuts` from the scripts of whatever
    /// project is currently active. Unique shortcuts stay dispatchable;
    /// duplicated ones stay out of the map (their rows warn instead). Call
    /// this whenever the active project may have changed so root key
    /// dispatch targets the right shortcuts.
    pub fn sync_active_shortcuts_to_active_project(&mut self) {
        let active_pid = self
            .selected_project_id
            .clone()
            .or_else(|| self.active_scripts_project_id());
        let Some(active_pid) = active_pid else {
            self.active_project_shortcuts.clear();
            return;
        };
        let Some(state) = self.project_scripts_by_project.get(&active_pid) else {
            self.active_project_shortcuts.clear();
            return;
        };
        self.active_project_shortcuts = compute_shortcut_state(&state.scripts).bindings;
    }

    /// Match a live keystroke against the active project's script
    /// shortcuts. Returns the script id to run, if any. Duplicated
    /// shortcuts never enter the map, so they can never dispatch.
    pub fn match_script_shortcut(&self, keystroke: &Keystroke) -> Option<String> {
        if self.active_project_shortcuts.is_empty() {
            return None;
        }
        let canonical = crate::keybindings::normalize_script_keystroke(keystroke)?;
        self.active_project_shortcuts.get(&canonical).cloned()
    }

    /// Palettes and pickers own the keyboard while open — script shortcuts
    /// never steal a keystroke from under them.
    pub fn any_palette_open(&self, cx: &gpui::App) -> bool {
        self.command_palette.read(cx).is_open(cx)
            || self.quick_open_palette.read(cx).is_open(cx)
            || self.project_browse_palette.read(cx).is_open(cx)
    }

    pub fn select_bottom_run_tab(&mut self, cx: &mut Context<Self>) {
        if self.right_sidebar_bottom_collapsed {
            self.right_sidebar_bottom_collapsed = false;
            self.persist_layout();
        }
        self.right_sidebar_bottom_run_selected = true;
        // Selecting the tab re-reads console.toml, so edits show up without
        // an app restart. Merge-by-id keeps open output and live tails.
        self.refresh_project_scripts(cx);
        cx.notify();
    }

    /// Follow one run: backfill its record, stream live events, then
    /// reconcile once the stream ends. Replacing the stored task cancels a
    /// previous watch, so reconnects never accumulate duplicate streams.
    pub(crate) fn watch_script_run(
        &mut self,
        project_id: &str,
        script_id: &str,
        run_id: &str,
        cx: &mut Context<Self>,
    ) {
        use futures_util::StreamExt;

        self.project_script_stream_seq += 1;
        let seq = self.project_script_stream_seq;
        let client = self.client.clone();
        let project_id_owned = project_id.to_string();
        let script_id_owned = script_id.to_string();
        let run_id_owned = run_id.to_string();
        let task = cx.spawn(async move |entity, cx| {
            if let Ok(run) = client
                .scripts
                .get_run(&project_id_owned, &run_id_owned)
                .await
            {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.apply_script_run_snapshot(
                                &project_id_owned,
                                &script_id_owned,
                                &run,
                            );
                            cx.notify();
                        });
                    }
                });
            }
            let mut terminal_seen = false;
            match client
                .scripts
                .stream_run(&project_id_owned, &run_id_owned)
                .await
            {
                Ok(mut stream) => {
                    while let Some(item) = stream.next().await {
                        match item {
                            Ok(event) => {
                                let done = matches!(
                                    &event,
                                    ScriptRunEvent::Status { status }
                                    | ScriptRunEvent::Exit { status, .. }
                                    if status.is_terminal()
                                );
                                let _ = cx.update(|cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.apply_script_run_event(
                                                &project_id_owned,
                                                &script_id_owned,
                                                &run_id_owned,
                                                &event,
                                            );
                                            cx.notify();
                                        });
                                    }
                                });
                                if done {
                                    terminal_seen = true;
                                    break;
                                }
                            }
                            Err(err) => {
                                log::warn!("Project script stream error: {err:#}");
                                break;
                            }
                        }
                    }
                }
                Err(err) => {
                    log::warn!("Failed to connect to project script stream: {err:#}");
                }
            }
            // Reconcile with the retained record: covers output that arrived
            // between the backfill fetch and the stream connecting, and
            // refreshes state after a dropped connection.
            if !terminal_seen
                && let Ok(run) = client
                    .scripts
                    .get_run(&project_id_owned, &run_id_owned)
                    .await
            {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.apply_script_run_snapshot(
                                &project_id_owned,
                                &script_id_owned,
                                &run,
                            );
                            cx.notify();
                        });
                    }
                });
            }
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, _| {
                        let key = (project_id_owned.clone(), run_id_owned.clone());
                        if this
                            .project_script_streams
                            .get(&key)
                            .is_some_and(|(live_seq, _)| *live_seq == seq)
                        {
                            this.project_script_streams.remove(&key);
                        }
                    });
                }
            });
        });
        self.project_script_streams
            .insert((project_id.to_string(), run_id.to_string()), (seq, task));
    }

    fn apply_script_run_snapshot(&mut self, project_id: &str, script_id: &str, run: &ScriptRun) {
        if let Some(state) = self.project_scripts_by_project.get_mut(project_id) {
            // A live stream owns running views; only snapshots can replace a
            // finished run or introduce one the watcher hasn't seen.
            let live = state.runs.get(script_id).is_some_and(|view| {
                view.run.as_ref().is_some_and(|current| {
                    current.run_id == run.run_id && current.status == ScriptRunStatus::Running
                })
            });
            if !live {
                let output = snapshot_output(run);
                state.runs.insert(
                    script_id.to_string(),
                    ProjectScriptRunView {
                        run: Some(run.clone()),
                        output,
                        starting: false,
                    },
                );
            }
        }
    }

    fn apply_script_run_event(
        &mut self,
        project_id: &str,
        script_id: &str,
        run_id: &str,
        event: &ScriptRunEvent,
    ) {
        let Some(state) = self.project_scripts_by_project.get_mut(project_id) else {
            return;
        };
        let Some(view) = state.runs.get_mut(script_id) else {
            return;
        };
        let Some(run) = view.run.as_mut() else {
            return;
        };
        if run.run_id != run_id {
            return;
        }
        match event {
            ScriptRunEvent::Status { status } => {
                run.status = *status;
            }
            ScriptRunEvent::Output { stream: _, text } => {
                push_run_output(&mut view.output, text);
            }
            ScriptRunEvent::Exit { status, exit_code } => {
                run.status = *status;
                run.exit_code = *exit_code;
            }
        }
    }
}
