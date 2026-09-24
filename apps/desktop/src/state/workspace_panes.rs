use console_core::WorkspaceTabConfig;
use console_ui::workspace::{WorkspaceDrag, ops as workspace_ops};
use gpui::{Context, Focusable as _, Window};

use crate::state::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Close a tab and synchronize the pane's selected session, composer, and
    /// transcript. All close entry points use this so the tab-bar button and
    /// keyboard shortcut cannot drift apart.
    pub fn close_tab_and_sync_pane(&mut self, pane_id: &str, tab_id: &str, cx: &mut Context<Self>) {
        let previous_session = self.active_session_for_pane(pane_id);

        if let Some(session_id) = previous_session.as_deref() {
            let input = self.composer_for_pane(pane_id).read(cx);
            let text = input.content().to_string();
            let mentions = input.mentions().to_vec();
            let context_files = input.context_files().to_vec();
            let attachments = self.attachments_for_pane(pane_id);
            self.commit_draft_to_sidebar_with_context(session_id, &text, &mentions, &context_files, &attachments, cx);
        }

        self.save_transcript_scroll_position(cx);
        self.close_workspace_tab(pane_id, tab_id, cx);
        self.active_pane_id = Some(pane_id.to_string());

        let next_session = self.active_session_for_pane(pane_id);
        let transcript = self.transcript_for_pane(pane_id);
        let composer = self.composer_for_pane(pane_id);

        if next_session == previous_session {
            self.maybe_refresh_inspector(cx);
            cx.notify();
            return;
        }

        match next_session {
            Some(session_id) => {
                self.selected_session_id = Some(session_id.clone());
                let draft = self.get_draft_with_mentions(Some(&session_id));
                let draft_context_files = self.get_draft_context_files(Some(&session_id));
                composer.update(cx, |input, cx| {
                    input.set_prompt_history(Vec::new(), cx);
                    if let Some((draft, mentions)) = draft {
                        input.set_content_with_mentions(draft, mentions, cx);
                    } else {
                        input.clear(cx);
                    }
                    input.set_context_files(draft_context_files);
                    cx.notify();
                });
                // Keep the previous transcript visible until the new load
                // succeeds. A failed request must not strand the pane empty.
                self.load_session_messages_for_pane(pane_id.to_string(), session_id, cx);
            }
            None => {
                self.selected_session_id = None;
                let draft = self.get_draft_with_mentions(None);
                let draft_context_files = self.get_draft_context_files(None);
                composer.update(cx, |input, cx| {
                    if let Some((draft, mentions)) = draft {
                        input.set_content_with_mentions(draft, mentions, cx);
                    } else {
                        input.clear(cx);
                    }
                    input.set_context_files(draft_context_files);
                    cx.notify();
                });
                transcript.update(cx, |t, cx| t.set_messages(Vec::new(), cx));
            }
        }

        self.maybe_refresh_inspector(cx);
        self.sync_workspace_webviews(cx);
        cx.notify();
    }

    /// Close a tab in a pane. Returns the newly active tab id, if any.
    pub fn close_workspace_tab(
        &mut self,
        pane_id: &str,
        tab_id: &str,
        cx: &mut Context<Self>,
    ) -> Option<String> {
        let closed_tab = workspace_ops::find_tab(&self.workspace_root, tab_id);
        let res = workspace_ops::close_tab(&mut self.workspace_root, pane_id, tab_id);
        // Evict from stashed workspace caches too, or a closed tab outlives
        // its close in memory while disk already dropped it.
        for root in self.project_workspace_roots.values_mut() {
            workspace_ops::close_matching_tabs(root, |tab| tab.id() == tab_id);
        }
        if let Some(tab) = closed_tab {
            self.dispose_closed_tab(&tab, cx);
        }
        self.trim_file_caches();
        self.persist_workspaces();
        res
    }

    /// Close every tab matching `predicate` across all panes.
    pub fn close_matching_workspace_tabs(
        &mut self,
        predicate: impl Fn(&WorkspaceTabConfig) -> bool,
        cx: &mut Context<Self>,
    ) {
        let closed: Vec<WorkspaceTabConfig> = self
            .workspace_root
            .leaves()
            .iter()
            .flat_map(|leaf| leaf.tabs.iter())
            .filter(|tab| predicate(tab))
            .cloned()
            .collect();
        workspace_ops::close_matching_tabs(&mut self.workspace_root, predicate);
        for tab in &closed {
            self.dispose_closed_tab(tab, cx);
        }
        self.trim_file_caches();
        self.persist_workspaces();
    }

    /// Release resources owned by a tab leaving the current tree: terminal
    /// views (dropping the entity cancels its tasks and releases the PTY —
    /// otherwise closed terminals leak shell processes until restart) and
    /// cached file/viewer state for closed file tabs. Terminals in cached
    /// background workspaces are intentionally kept alive.
    fn dispose_closed_tab(&mut self, tab: &WorkspaceTabConfig, cx: &mut Context<Self>) {
        match tab {
            WorkspaceTabConfig::Terminal { terminal_id, .. } => {
                let still_referenced = self
                    .workspace_root
                    .leaves()
                    .iter()
                    .flat_map(|leaf| leaf.tabs.iter())
                    .any(|open_tab| {
                        matches!(
                            open_tab,
                            WorkspaceTabConfig::Terminal {
                                terminal_id: open_id,
                                ..
                            } if open_id == terminal_id
                        )
                    });
                if !still_referenced {
                    if let Some(view) = self.terminals.remove(terminal_id) {
                        view.update(cx, |terminal, _| terminal.kill());
                    }
                }
            }
            WorkspaceTabConfig::File { path, .. } | WorkspaceTabConfig::Diff { path, .. } => {
                self.evict_file_caches_for_path(path);
            }
            WorkspaceTabConfig::Browser { browser_id, .. } => {
                let still_referenced = self
                    .workspace_root
                    .leaves()
                    .iter()
                    .flat_map(|leaf| leaf.tabs.iter())
                    .any(|open_tab| {
                        matches!(
                            open_tab,
                            WorkspaceTabConfig::Browser {
                                browser_id: open_id,
                                ..
                            } if open_id == browser_id
                        )
                    });
                if !still_referenced {
                    if let Some(view) = self.browser_views.remove(browser_id) {
                        view.update(cx, |browser, cx| browser.close(cx));
                    }
                }
            }
            WorkspaceTabConfig::Chat { .. } => {}
        }
    }

    /// Activate a tab in a pane.
    pub fn select_workspace_tab(&mut self, pane_id: &str, tab_id: &str) {
        workspace_ops::select_tab(&mut self.workspace_root, pane_id, tab_id);
        // Refresh the recency timestamp on the freshly-selected tab so the
        // ⌘⇧P palette can sort open tabs by recent activity. Skipped when the
        // tab is unknown (e.g. id collision or stale persisted state).
        if let Some(leaf) = self.workspace_root.leaf_mut(pane_id) {
            if let Some(tab) = leaf.tabs.iter_mut().find(|tab| tab.id() == tab_id) {
                tab.set_last_active_at_ms(Some(chrono::Utc::now().timestamp_millis()));
            }
        }
        self.active_pane_id = Some(pane_id.to_string());
        self.persist_workspaces();
    }

    /// Activate a tab and sync the pane's content to it: selected session,
    /// project picker, transcript/composer, inspector. Shared by mouse clicks
    /// and the Option+1–9 shortcuts so both switch content, not just the tab
    /// highlight.
    pub fn activate_workspace_tab(&mut self, pane_id: &str, tab_id: &str, cx: &mut Context<Self>) {
        let prev_sid = self.active_session_for_pane(pane_id).map(|s| s.to_string());
        self.select_workspace_tab(pane_id, tab_id);
        self.sync_workspace_webviews(cx);
        if let Some(sid) = tab_id.strip_prefix("chat:") {
            super::macos_notifications::clear_for_session(sid);
            self.selected_session_id = Some(sid.to_string());
            if let Some(session) = self.sessions.iter().find(|s| s.id == sid) {
                if let Some(pid) = &session.project_id {
                    if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
                        state.selected_project_id = Some(pid.clone());
                    }
                }
            }
            let transcript_has_messages =
                self.transcript_for_pane(pane_id).read(cx).message_count() > 0;
            let already_loaded = self
                .workspace_pane_states
                .get(pane_id)
                .and_then(|state| state.loaded_session_id.as_deref())
                == Some(sid);
            if transcript_has_messages && (already_loaded || prev_sid.as_deref() == Some(sid)) {
                self.maybe_refresh_inspector(cx);
                cx.notify();
                return;
            }
            let draft = self.get_draft_with_mentions(Some(sid));
            let draft_ctx_files = self.get_draft_context_files(Some(sid));
            // Load attachments from the draft for this session
            let draft_attachments = self
                .drafts
                .get(sid)
                .map(|draft| draft.attachments.clone())
                .unwrap_or_default();
            self.set_attachments_for_pane(pane_id, draft_attachments);
            self.composer_for_pane(pane_id).update(cx, |input, cx| {
                input.set_prompt_history(Vec::new(), cx);
                if let Some((draft_text, mentions)) = draft {
                    input.set_content_with_mentions(draft_text, mentions, cx);
                } else {
                    input.clear(cx);
                }
                input.set_context_files(draft_ctx_files);
                cx.notify();
            });
            // Switching tabs: clear synchronously so the old session's
            // messages never linger while the new session loads. (The
            // close-tab path intentionally keeps the old transcript
            // visible until its load succeeds; here the tab highlight
            // already moved, so stale content reads as lag.)
            if prev_sid.as_deref() != Some(sid) {
                self.transcript_for_pane(pane_id).update(cx, |t, cx| {
                    t.set_messages(Vec::new(), cx);
                });
            }
            self.load_session_messages_for_pane(pane_id.to_string(), sid.to_string(), cx);
        } else if let Some(terminal_id) = tab_id.strip_prefix("term:") {
            // Focus terminal content so keyboard input works immediately (deferred)
            self.selected_session_id = None;
            let terminal_id = terminal_id.to_string();
            let entity = cx.entity().downgrade();
            cx.defer(move |cx| {
                if let Some(window) = cx.active_window() {
                    let _ = window.update(cx, |_, window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                if let Some(terminal) = this.terminals.get(&terminal_id) {
                                    window.focus(&terminal.read(cx).focus_handle(cx), cx);
                                }
                            });
                        }
                    });
                }
            });
        } else if let Some(browser_id) = tab_id.strip_prefix("browser:") {
            // Focus browser content so keyboard input (⌘L for address bar, etc.) works immediately (deferred)
            self.selected_session_id = None;
            let browser_id = browser_id.to_string();
            let entity = cx.entity().downgrade();
            cx.defer(move |cx| {
                if let Some(window) = cx.active_window() {
                    let _ = window.update(cx, |_, window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                if let Some(browser) = this.browser_views.get(&browser_id) {
                                    window.focus(&browser.read(cx).focus_handle(cx), cx);
                                }
                            });
                        }
                    });
                }
            });
        } else {
            self.selected_session_id = None;
        }
        self.maybe_refresh_inspector(cx);
        cx.notify();
    }

    /// Activate the nth tab (0-based) of the active workspace pane, regardless
    /// of tab type (chat, terminal, file, diff). Backs the Option+1–9
    /// shortcuts; a no-op when no pane is active or it has fewer tabs.
    pub fn select_workspace_tab_by_index(&mut self, index: usize, cx: &mut Context<Self>) {
        let Some(pane_id) = self.active_pane_id.clone() else {
            return;
        };
        let Some(leaf) = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == pane_id)
        else {
            return;
        };
        let Some(tab) = leaf.tabs.get(index) else {
            return;
        };
        let tab_id = tab.id();
        self.activate_workspace_tab(&pane_id, &tab_id, cx);
    }

    pub fn focus_workspace_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        if self
            .workspace_root
            .leaves()
            .into_iter()
            .all(|leaf| leaf.id != pane_id)
        {
            return;
        }
        self.active_pane_id = Some(pane_id.to_string());
        self.selected_session_id = self.active_session_for_pane(pane_id);
        if let Some(session_id) = &self.selected_session_id {
            if let Some(session) = self.sessions.iter().find(|s| &s.id == session_id) {
                if let Some(pid) = &session.project_id {
                    if let Some(state) = self.workspace_pane_states.get_mut(pane_id) {
                        state.selected_project_id = Some(pid.clone());
                    }
                }
            }
        }
        self.maybe_refresh_inspector(cx);
        cx.notify();
    }

    /// Close a split pane and collapse its parent into the remaining sibling.
    pub fn close_workspace_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let removed_tabs: Vec<WorkspaceTabConfig> = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == pane_id)
            .map(|leaf| leaf.tabs.clone())
            .unwrap_or_default();
        if !workspace_ops::close_pane(&mut self.workspace_root, pane_id) {
            return;
        }
        for tab in &removed_tabs {
            self.dispose_closed_tab(tab, cx);
        }
        self.trim_file_caches();
        self.workspace_pane_states.remove(pane_id);
        self.todo_items.remove(pane_id);
        if self.active_pane_id.as_deref() == Some(pane_id) {
            let next_pane_id = self.workspace_root.first_leaf().map(|leaf| leaf.id.clone());
            self.active_pane_id = next_pane_id.clone();
            self.selected_session_id = next_pane_id
                .as_deref()
                .and_then(|id| self.active_session_for_pane(id));
            if let Some(next_pane_id) = next_pane_id {
                let transcript = self.transcript_for_pane(&next_pane_id);
                let composer = self.composer_for_pane(&next_pane_id);
                composer.update(cx, |input, cx| input.set_content("", cx));
                if let Some(session_id) = self.selected_session_id.clone() {
                    self.load_session_messages_for_pane(next_pane_id, session_id, cx);
                } else {
                    transcript.update(cx, |transcript, cx| {
                        transcript.set_messages(Vec::new(), cx);
                    });
                }
            }
        }
        self.persist_workspaces();
        cx.notify();
    }

    /// If a sidebar drag carries a session whose chat tab is already open in
    /// any pane, focus that pane/tab instead of creating a duplicate.
    /// Returns true when the drop was handled this way.
    /// Tab-bar drags are moves of the tab itself, so they are always allowed through.
    fn focus_existing_chat_tab_from_drag(
        &mut self,
        drag: &WorkspaceDrag,
        cx: &mut Context<Self>,
    ) -> bool {
        if drag.source_pane_id.is_some() {
            return false;
        }
        let session_id = match &drag.tab {
            WorkspaceTabConfig::Chat { session_id, .. } => session_id.clone(),
            _ => return false,
        };
        let tab_id = format!("chat:{session_id}");
        let Some(existing_pane_id) = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.tabs.iter().any(|t| t.id() == tab_id))
            .map(|leaf| leaf.id.clone())
        else {
            return false;
        };

        workspace_ops::select_tab(&mut self.workspace_root, &existing_pane_id, &tab_id);
        self.active_pane_id = Some(existing_pane_id);
        self.selected_session_id = Some(session_id);
        self.persist_workspaces();
        cx.notify();
        true
    }

    /// A sidebar drag names a session rather than moving a tab. When that
    /// session belongs to another folder, switch workspaces (like clicking it)
    /// instead of dropping a foreign tab into this workspace's split tree.
    /// Returns true when the drop was diverted this way.
    fn divert_foreign_sidebar_drag(
        &mut self,
        drag: &WorkspaceDrag,
        cx: &mut Context<Self>,
    ) -> bool {
        if drag.source_pane_id.is_some() {
            return false;
        }
        let WorkspaceTabConfig::Chat { session_id, .. } = &drag.tab else {
            return false;
        };
        let target_project_id = self.target_project_for_session(session_id).map(|p| p.id);
        if target_project_id != self.selected_project_id {
            self.select_and_open_session(session_id.clone(), cx);
            return true;
        }
        false
    }

    /// Move a dragged tab into the existing target pane as a new tab.
    pub fn move_workspace_tab_to_pane(
        &mut self,
        target_pane_id: String,
        drag: WorkspaceDrag,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.divert_foreign_sidebar_drag(&drag, cx) {
            return;
        }
        if self.focus_existing_chat_tab_from_drag(&drag, cx) {
            return;
        }
        self.save_transcript_scroll_position(cx);
        let source_pane_id = drag.source_pane_id.clone();
        let tab = drag.tab.clone();
        let Some(target_pane_id) = workspace_ops::move_tab_to_pane(
            &mut self.workspace_root,
            source_pane_id.clone().as_deref(),
            &target_pane_id,
            drag.tab,
        ) else {
            return;
        };

        if source_pane_id.as_deref() != Some(target_pane_id.as_str()) {
            if let Some(source_pane_id) = source_pane_id {
                if let Some(source_session_id) = self.active_session_for_pane(&source_pane_id) {
                    let source_transcript = self.transcript_for_pane(&source_pane_id);
                    source_transcript.update(cx, |transcript, cx| {
                        transcript.set_messages(Vec::new(), cx);
                    });
                    self.load_session_messages_for_pane(source_pane_id, source_session_id, cx);
                } else if self
                    .workspace_root
                    .leaves()
                    .iter()
                    .any(|leaf| leaf.id == source_pane_id)
                {
                    self.transcript_for_pane(&source_pane_id)
                        .update(cx, |transcript, cx| {
                            transcript.set_messages(Vec::new(), cx);
                        });
                }
            }
        }

        self.active_pane_id = Some(target_pane_id.clone());
        self.ensure_workspace_pane_state(&target_pane_id, window, cx);
        if let WorkspaceTabConfig::Chat { session_id, .. } = tab {
            self.selected_session_id = Some(session_id.clone());
            self.composer_for_pane(&target_pane_id)
                .update(cx, |input, cx| {
                    input.set_prompt_history(Vec::new(), cx);
                });
            self.transcript_for_pane(&target_pane_id)
                .update(cx, |transcript, cx| {
                    transcript.set_messages(Vec::new(), cx);
                });
            self.load_session_messages_for_pane(target_pane_id, session_id, cx);
        }
        self.persist_workspaces();
        cx.notify();
    }

    /// Move a dragged tab into a new split adjacent to the target pane.
    /// `insert_first` puts the dragged tab's pane before the target pane
    /// (left for horizontal, above for vertical splits).
    pub fn move_workspace_tab_to_split(
        &mut self,
        target_pane_id: String,
        drag: WorkspaceDrag,
        direction: console_core::SplitDirection,
        insert_first: bool,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.divert_foreign_sidebar_drag(&drag, cx) {
            return;
        }
        if self.focus_existing_chat_tab_from_drag(&drag, cx) {
            return;
        }
        self.save_transcript_scroll_position(cx);
        let source_pane_id = drag.source_pane_id.clone();
        let tab = drag.tab.clone();
        let Some(new_pane_id) = workspace_ops::move_tab_to_split(
            &mut self.workspace_root,
            source_pane_id.as_deref(),
            &target_pane_id,
            drag.tab,
            direction,
            insert_first,
        ) else {
            return;
        };

        if let Some(source_pane_id) = source_pane_id {
            if let Some(source_session_id) = self.active_session_for_pane(&source_pane_id) {
                let source_transcript = self.transcript_for_pane(&source_pane_id);
                source_transcript.update(cx, |transcript, cx| {
                    transcript.set_messages(Vec::new(), cx);
                });
                self.load_session_messages_for_pane(source_pane_id, source_session_id, cx);
            } else if self
                .workspace_root
                .leaves()
                .iter()
                .any(|leaf| leaf.id == source_pane_id)
            {
                self.transcript_for_pane(&source_pane_id)
                    .update(cx, |transcript, cx| {
                        transcript.set_messages(Vec::new(), cx);
                    });
            }
        }

        self.ensure_workspace_pane_state(&new_pane_id, window, cx);
        self.active_pane_id = Some(new_pane_id.clone());
        if let WorkspaceTabConfig::Chat { session_id, .. } = tab {
            self.selected_session_id = Some(session_id.clone());
            let draft = self.get_draft_with_mentions(Some(&session_id));
            let draft_ctx_files2 = self.get_draft_context_files(Some(&session_id));
            self.composer_for_pane(&new_pane_id)
                .update(cx, |input, cx| {
                    input.set_prompt_history(Vec::new(), cx);
                    if let Some((draft_text, mentions)) = draft {
                        input.set_content_with_mentions(draft_text, mentions, cx);
                    } else {
                        input.clear(cx);
                    }
                    input.set_context_files(draft_ctx_files2);
                    cx.notify();
                });
            self.transcript_for_pane(&new_pane_id)
                .update(cx, |transcript, cx| {
                    transcript.set_messages(Vec::new(), cx);
                });
            self.load_session_messages_for_pane(new_pane_id, session_id, cx);
        }
        self.persist_workspaces();
        cx.notify();
    }
}
