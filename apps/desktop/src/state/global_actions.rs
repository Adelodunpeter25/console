//! Methods behind the global shortcuts bound in `crate::keybindings`.
//!
//! They are registered as *global* action handlers (`App::on_action` in
//! `keybindings::init_handlers`), so they run regardless of keyboard focus.

use std::rc::Rc;

use console_core::CreateSessionDto;
use console_ui::PaletteEntry;
use gpui::{Context, Focusable as _, Window};

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Create a chat session for the active pane and open it as its tab.
    ///
    /// Shared by the sidebar's New Task row, the empty state's button, the
    /// command palette, and the ⌘N shortcut.
    pub fn create_new_chat(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let approval_mode = self.pane_approval_mode(&pane_id);
        let selected_model = self.pane_selected_model(&pane_id);
        let session_project_id = self.pane_project_id(&pane_id);
        let session_cwd = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path.clone())
            .unwrap_or_else(|| {
                std::env::current_dir()
                    .map(|p| p.to_string_lossy().to_string())
                    .unwrap_or_else(|_| ".".to_string())
            });
        cx.spawn(async move |entity, cx| {
            match client
                .sessions
                .create(CreateSessionDto {
                    cwd: session_cwd,
                    project_id: session_project_id,
                    model_id: selected_model
                        .as_ref()
                        .map(|model| model.model_id.clone()),
                    provider: selected_model
                        .as_ref()
                        .map(|model| model.provider.clone()),
                    title: Some("New Chat".into()),
                    approval_mode: Some(approval_mode.value().to_string()),
                })
                .await
            {
                Ok(new_session) => {
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.save_transcript_scroll_position(cx);
                                this.apply_session_header_for_pane(&pane_id, &new_session, cx);
                                this.clear_error_for_pane(&pane_id, cx);
                                if this.active_pane_id.as_deref() == Some(pane_id.as_str()) {
                                    this.selected_session_id = Some(new_session.id.clone());
                                }
                                Rc::make_mut(&mut this.sessions).insert(0, new_session.clone());
                                this.open_chat_tab_in_pane(
                                    &pane_id,
                                    new_session.id.clone(),
                                    "New Chat",
                                );
                                let new_chat_draft = this.get_draft_for_session(None).map(|s| s.to_string());
                                this.composer_for_pane(&pane_id).update(cx, |input, cx| {
                                    input.set_prompt_history(Vec::new(), cx);
                                    if let Some(draft_text) = &new_chat_draft {
                                        input.set_content(draft_text.clone(), cx);
                                    } else {
                                        input.clear(cx);
                                    }
                                });
                                if let Some(draft_text) = new_chat_draft {
                                    this.save_draft_for_session(Some(&new_session.id), &draft_text);
                                    this.clear_draft_for_session(None);
                                }
                                this.transcript_for_pane(&pane_id).update(cx, |t, cx| {
                                    t.set_messages(Vec::new(), cx);
                                });
                                cx.notify();
                            });
                        }
                    });
                }
                Err(error) => {
                    let message = format!("Unable to create a session: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| this.set_error(message, cx));
                        }
                    });
                }
            }
        })
        .detach();
    }

    /// ⌘W — close the active tab, then point the pane at whatever tab became
    /// active (mirroring the tab bar's close button).
    pub fn close_tab(&mut self, cx: &mut Context<Self>) {
        let Some(pane_id) = self.active_pane_id.clone() else {
            return;
        };
        let Some(tab_id) = self.active_tab_id() else {
            return;
        };
        let prev_active_session = self.active_session_for_pane(&pane_id).map(|s| s.to_string());

        // Commit the current composer text to sidebar draft state at close time.
        // This is the ONLY moment the sidebar updates for this session's draft.
        if let Some(ref sid) = prev_active_session {
            let text = self.composer_for_pane(&pane_id).read(cx).content().to_string();
            self.commit_draft_to_sidebar(sid, &text);
        }

        self.save_transcript_scroll_position(cx);
        self.close_workspace_tab(&pane_id, &tab_id);
        let transcript = self.transcript_for_pane(&pane_id);
        let composer = self.composer_for_pane(&pane_id);
        let new_active_session = self.active_session_for_pane(&pane_id).map(|s| s.to_string());

        if new_active_session == prev_active_session && new_active_session.is_some() {
            if self.right_sidebar_visible {
                self.refresh_inspector(cx);
            }
            cx.notify();
            return;
        }

        if let Some(sid) = new_active_session {
            self.selected_session_id = Some(sid.clone());
            composer.update(cx, |input, cx| {
                input.set_prompt_history(Vec::new(), cx);
            });
            transcript.update(cx, |t, cx| t.set_messages(Vec::new(), cx));
            self.load_session_messages_for_pane(pane_id.clone(), sid, cx);
        } else {
            self.selected_session_id = None;
            composer.update(cx, |input, cx| input.set_content("", cx));
            transcript.update(cx, |t, cx| t.set_messages(Vec::new(), cx));
        }
        if self.right_sidebar_visible {
            self.refresh_inspector(cx);
        }
        cx.notify();
    }

    /// ⌘K — toggle the command palette. Static command entries are rebuilt here
    /// (with the current entity/client) so the palette always opens with fresh
    /// closures; async modes replace them via their own wrappers.
    pub fn toggle_command_palette(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let entity = cx.entity().downgrade();
        self.command_palette.update(cx, |palette, cx| {
            palette.set_entries(
                vec![
                    PaletteEntry::new("new-chat", "New Chat", {
                        let entity = entity.clone();
                        move |_window, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| this.create_new_chat(cx));
                            }
                        }
                    }),
                    PaletteEntry::new("new-terminal", "New Terminal", {
                        let entity = entity.clone();
                        move |window, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| this.open_terminal_tab(window, cx));
                            }
                        }
                    }),
                ],
                cx,
            );
            palette.toggle(window, cx);
        });
        cx.notify();
    }

    /// ⌘P — open the quick file search palette scoped to the active pane's
    /// project root (falling back to the current directory).
    pub fn open_quick_open(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let root = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path.clone())
            .or_else(|| {
                std::env::current_dir()
                    .map(|p| p.to_string_lossy().to_string())
                    .ok()
            });
        self.quick_open_palette
            .update(cx, |palette, cx| palette.open(root, window, cx));
        cx.notify();
    }

    /// ⌘O — open the remote directory browser palette to add a project.
    pub fn open_project_browse(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.project_browse_palette
            .update(cx, |palette, cx| palette.open(window, cx));
        cx.notify();
    }

    /// Open (never toggle) the command palette — used by the sidebar search
    /// button. Builds the same static entries as ⌘K.
    pub fn open_command_palette(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.command_palette.read(cx).is_open(cx) {
            return;
        }
        let entity = cx.entity().downgrade();
        self.command_palette.update(cx, |palette, cx| {
            palette.set_entries(
                vec![
                    PaletteEntry::new("new-chat", "New Chat", {
                        let entity = entity.clone();
                        move |_window, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| this.create_new_chat(cx));
                            }
                        }
                    }),
                    PaletteEntry::new("new-terminal", "New Terminal", {
                        let entity = entity.clone();
                        move |window, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| this.open_terminal_tab(window, cx));
                            }
                        }
                    }),
                ],
                cx,
            );
            palette.show(window, cx);
        });
        cx.notify();
    }

    /// ⌘L — move keyboard focus to the active pane's composer input.
    pub fn focus_composer(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        // Don't fight any palette for focus while it is open.
        if self.command_palette.read(cx).is_open(cx)
            || self.quick_open_palette.read(cx).is_open(cx)
            || self.project_browse_palette.read(cx).is_open(cx)
        {
            return;
        }
        let composer = self.active_composer_input();
        composer.update(cx, |input, cx| {
            window.focus(&input.focus_handle(cx), cx);
        });
    }
}
