mod workspace_content;

use std::rc::Rc;

use console_ui::workspace::{
    ContentRenderer, WorkspaceDrag, WorkspaceDropAction, WorkspacePane, cancel_workspace_drags,
};
use console_ui::{ImageViewerModal, RightSidebar, SidebarView, Theme, TitleBar};
use gpui::{
    App, Context, InteractiveElement, IntoElement, KeyDownEvent, MouseButton, MouseMoveEvent,
    MouseUpEvent, ParentElement, Render, Styled, Window, div, prelude::FluentBuilder,
};

use crate::state::ConsoleDesktopApp;

impl Render for ConsoleDesktopApp {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        self.persist_window_state(window, cx);
        let theme = Theme::current(cx);
        let entity = cx.entity().downgrade();
        let client = self.client.clone();

        let workspace_root = self.workspace_root.clone();
        let active_pane = self.active_pane_id.clone();
        let sessions = self.sessions.clone();
        // Shared "New Chat" action: used by the sidebar's New Task row, the
        // empty state's New Chat button, and the palette. The logic lives on
        // the entity (`create_new_chat`) so the ⌘N shortcut shares it.
        let on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.create_new_chat(cx));
                }
            })
        };
        let render_content: ContentRenderer = Rc::new({
            let entity = entity.clone();
            let on_new_chat = on_new_chat.clone();
            move |pane_id, active_tab, window, cx| {
                let entity = entity.clone();
                let on_new_chat = on_new_chat.clone();
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.render_workspace_content(pane_id, active_tab, on_new_chat, window, cx)
                    })
                } else {
                    div().into_any_element()
                }
            }
        });
        let on_select_tab: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(
                move |pane_id: String, tab_id: String, _w: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let prev_sid = this
                                .active_session_for_pane(&pane_id)
                                .map(|s| s.to_string());
                            this.select_workspace_tab(&pane_id, &tab_id);
                            if let Some(sid) = tab_id.strip_prefix("chat:") {
                                this.selected_session_id = Some(sid.to_string());
                                let already_loaded = this
                                    .workspace_pane_states
                                    .get(&pane_id)
                                    .and_then(|state| state.loaded_session_id.as_deref())
                                    == Some(sid);
                                if already_loaded || prev_sid.as_deref() == Some(sid) {
                                    if this.right_sidebar_visible {
                                        this.refresh_inspector(cx);
                                    }
                                    cx.notify();
                                    return;
                                }
                                let draft = this.get_draft_for_session(Some(sid)).map(|s| s.to_string());
                                this.composer_for_pane(&pane_id).update(cx, |input, cx| {
                                    input.set_prompt_history(Vec::new(), cx);
                                    if let Some(draft_text) = draft {
                                        input.set_content(draft_text, cx);
                                    } else {
                                        input.clear(cx);
                                    }
                                });
                                this.transcript_for_pane(&pane_id).update(cx, |t, cx| {
                                    t.set_messages(Vec::new(), cx);
                                });
                                this.load_session_messages_for_pane(
                                    pane_id.clone(),
                                    sid.to_string(),
                                    cx,
                                );
                            } else {
                                this.selected_session_id = None;
                            }
                            if this.right_sidebar_visible {
                                this.refresh_inspector(cx);
                            }
                            cx.notify();
                        });
                    }
                },
            )
        };
        let on_close_tab: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(
                move |pane_id: String, tab_id: String, _w: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let prev_active_session = this
                                .active_session_for_pane(&pane_id)
                                .map(|s| s.to_string());
                            this.save_transcript_scroll_position(cx);
                            this.close_workspace_tab(&pane_id, &tab_id);
                            this.active_pane_id = Some(pane_id.clone());
                            let transcript = this.transcript_for_pane(&pane_id);
                            let composer = this.composer_for_pane(&pane_id);
                            let new_active_session = this
                                .active_session_for_pane(&pane_id)
                                .map(|s| s.to_string());

                            if new_active_session == prev_active_session && new_active_session.is_some() {
                                if this.right_sidebar_visible {
                                    this.refresh_inspector(cx);
                                }
                                cx.notify();
                                return;
                            }

                            if let Some(sid) = new_active_session {
                                this.selected_session_id = Some(sid.clone());
                                let already_loaded = this
                                    .workspace_pane_states
                                    .get(&pane_id)
                                    .and_then(|state| state.loaded_session_id.as_deref())
                                    == Some(&sid);
                                if already_loaded {
                                    if this.right_sidebar_visible {
                                        this.refresh_inspector(cx);
                                    }
                                    cx.notify();
                                    return;
                                }
                                let draft = this.get_draft_for_session(Some(&sid)).map(|s| s.to_string());
                                composer.update(cx, |input, cx| {
                                    input.set_prompt_history(Vec::new(), cx);
                                    if let Some(draft_text) = draft {
                                        input.set_content(draft_text, cx);
                                    } else {
                                        input.clear(cx);
                                    }
                                });
                                transcript.update(cx, |t, cx| t.set_messages(Vec::new(), cx));
                                this.load_session_messages_for_pane(
                                    pane_id.clone(),
                                    sid,
                                    cx,
                                );
                            } else {
                                this.selected_session_id = None;
                                let draft = this.get_draft_for_session(None).map(|s| s.to_string());
                                composer.update(cx, |input, cx| {
                                    if let Some(draft_text) = draft {
                                        input.set_content(draft_text, cx);
                                    } else {
                                        input.clear(cx);
                                    }
                                });
                                transcript.update(cx, |t, cx| t.set_messages(Vec::new(), cx));
                            }
                            if this.right_sidebar_visible {
                                this.refresh_inspector(cx);
                            }
                            cx.notify();
                        });
                    }
                },
            )
        };
        let on_drop_tab: Rc<
            dyn Fn(String, WorkspaceDrag, WorkspaceDropAction, &mut Window, &mut App) + 'static,
        > = {
            let entity = entity.clone();
            Rc::new(move |target_pane_id, drag, action, window, cx| {
                if drag.is_cancelled() {
                    return;
                }
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| match action {
                        WorkspaceDropAction::AddTab => {
                            this.move_workspace_tab_to_pane(target_pane_id, drag, window, cx);
                        }
                        WorkspaceDropAction::SplitLeft => {
                            this.move_workspace_tab_to_split(
                                target_pane_id,
                                drag,
                                true,
                                window,
                                cx,
                            );
                        }
                        WorkspaceDropAction::SplitRight => {
                            this.move_workspace_tab_to_split(
                                target_pane_id,
                                drag,
                                false,
                                window,
                                cx,
                            );
                        }
                    });
                }
            })
        };
        let on_close_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |pane_id, _window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.close_workspace_pane(&pane_id, cx));
                }
            })
        };
        let on_focus_pane: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |pane_id, _window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.focus_workspace_pane(&pane_id, cx));
                }
            })
        };
        // The unified title bar shows the selected session's identity:
        // "chat title — folder name", mirroring the Electron app.
        let titlebar_text = self
            .selected_session_id
            .as_deref()
            .and_then(|sid| self.sessions.iter().find(|s| s.id == sid))
            .map(|session| {
                let folder = self
                    .projects
                    .iter()
                    .find(|project| project.matches_session(session))
                    .map(|project| project.name.clone())
                    .unwrap_or_else(|| console_ui::utils::format_folder_display_name(&session.cwd));
                if folder.is_empty() {
                    session.display_title().to_string()
                } else {
                    format!("{} — {}", session.title, folder)
                }
            });
        let on_toggle_sidebar: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.set_sidebar_visible(!this.sidebar_visible);
                        cx.notify();
                    });
                }
            })
        };
        let on_toggle_right_sidebar: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.toggle_right_sidebar(cx);
                    });
                }
            })
        };
        let on_select_inspector_tab: Rc<dyn Fn(console_ui::InspectorTab, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |tab, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.set_inspector_tab(tab, cx);
                    });
                }
            })
        };
        let on_toggle_inspector_folder: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |path, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.toggle_inspector_folder(path, cx);
                    });
                }
            })
        };
        let on_select_inspector_file: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |path, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match this.inspector_active_tab {
                            console_ui::InspectorTab::AllFiles => {
                                this.open_file_tab(path, cx);
                            }
                            console_ui::InspectorTab::Changes => {
                                this.open_diff_tab(path, cx);
                            }
                            console_ui::InspectorTab::Subagents => {}
                        }
                    });
                }
            })
        };
        let on_toggle_subagent: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |subagent_id, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.toggle_subagent_expanded(subagent_id, cx);
                    });
                }
            })
        };
        let on_copy_summary: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> = {
            Rc::new(move |summary, _w, cx| {
                cx.write_to_clipboard(gpui::ClipboardItem::new_string(summary));
            })
        };
        let on_refresh_inspector: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.refresh_inspector(cx);
                    });
                }
            })
        };
        let on_begin_right_sidebar_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |start_x, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.begin_right_sidebar_resize(start_x);
                        cx.notify();
                    });
                }
            })
        };

        div()
            .id("app-root")
            .relative()
            .size_full()
            .bg(theme.canvas)
            .flex()
            .flex_col()
            .overflow_hidden()
            .on_action(cx.listener(Self::copy_selection_action))
            // Invalidate a workspace drag on Escape so a drop queued just after
            // cancellation cannot mutate the pane tree.
            .on_key_down(|event: &KeyDownEvent, _, _| {
                if event.keystroke.key == "escape" {
                    cancel_workspace_drags();
                }
            })
            .on_mouse_move({
                let entity = entity.clone();
                move |event: &MouseMoveEvent, _, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let sidebar_changed = this.resize_sidebar(f32::from(event.position.x));
                            let right_sidebar_changed =
                                this.resize_right_sidebar(f32::from(event.position.x));
                            let split_changed = this.resize_split_drag(event.position);
                            if sidebar_changed || right_sidebar_changed || split_changed {
                                cx.notify();
                            }
                        });
                    }
                }
            })
            .on_mouse_up(MouseButton::Left, {
                let entity = entity.clone();
                move |_: &MouseUpEvent, _, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let sidebar_changed = this.finish_sidebar_resize();
                            let right_sidebar_changed = this.finish_right_sidebar_resize();
                            let split_changed = this.finish_split_resize();
                            if sidebar_changed || right_sidebar_changed || split_changed {
                                cx.notify();
                            }
                        });
                    }
                }
            })
            .child(
                TitleBar::new(
                    titlebar_text,
                    self.sidebar_width,
                    on_toggle_sidebar,
                )
                .with_right_sidebar_toggle(self.right_sidebar_visible, on_toggle_right_sidebar),
            )
            // Sidebar + workspace sit in a row below the title bar.
            .child(
                div()
                    .flex_1()
                    .min_h_0()
                    .flex()
                    .overflow_hidden()
                    .child(SidebarView::new(
                        self.sidebar_visible,
                        self.sidebar_width,
                        sessions,
                        self.projects.clone(),
                        self.selected_session_id.clone(),
                        self.collapsed_groups.clone(),
                        self.running_sessions_snapshot(),
                        self.waiting_sessions_snapshot(),
                        self.draft_summaries(),
                        self.drafts_collapsed,
                        self.sidebar_list_state.clone(),
                        self.environment_rows(),
                        self.server_menu.clone(),
                        {
                            let entity = entity.clone();
                            move |id: String, _w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        let active_pane_id = this
                                            .active_pane_id
                                            .clone()
                                            .unwrap_or_else(|| "pane-main".to_string());
                                        let prev_sid = this
                                            .active_session_for_pane(&active_pane_id)
                                            .map(|s| s.to_string());

                                        this.save_transcript_scroll_position(cx);
                                        this.selected_session_id = Some(id.clone());
                                        // Use the session's real title for the tab
                                        // (falling back like the sidebar row does),
                                        // not a hardcoded "Chat".
                                        let title = this
                                            .sessions
                                            .iter()
                                            .find(|s| s.id == id)
                                            .map(|s| s.display_title().to_string())
                                            .unwrap_or_else(|| "Chat".to_string());
                                        this.open_chat_tab(id.clone(), title);

                                        if prev_sid.as_deref() == Some(&id) {
                                            cx.notify();
                                            return;
                                        }

                                        let draft = this.get_draft_for_session(Some(&id)).map(|s| s.to_string());
                                        this.active_composer_input().update(cx, |input, cx| {
                                            input.set_prompt_history(Vec::new(), cx);
                                            if let Some(draft_text) = draft {
                                                input.set_content(draft_text, cx);
                                            } else {
                                                input.clear(cx);
                                            }
                                        });
                                        this.active_transcript_view().update(cx, |t, cx| {
                                            t.set_messages(Vec::new(), cx);
                                        });
                                        this.load_session_messages(id, cx);
                                        cx.notify();
                                    });
                                }
                            }
                        },
                        {
                            let on_new = on_new_chat.clone();
                            move |window: &mut Window, cx: &mut App| (on_new)(window, cx)
                        },
                        {
                            // Sidebar search button opens the command palette.
                            let entity = entity.clone();
                            move |window: &mut Window, cx: &mut App| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_command_palette(window, cx);
                                    });
                                }
                            }
                        },
                        {
                            // Sidebar "Add Project" opens the directory browser
                            // palette (works against remote backends, where a
                            // native dialog can't see the host filesystem).
                            let entity = entity.clone();
                            move |window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_project_browse(window, cx);
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |group: console_ui::utils::SessionDateGroup, _w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| this.toggle_sidebar_group(group, cx));
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |_w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.drafts_collapsed = !this.drafts_collapsed;
                                        cx.notify();
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |id: String, window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.begin_session_rename(id, window, cx);
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |_window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| this.commit_session_rename(cx));
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.cancel_session_rename(window, cx)
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            let client = client.clone();
                            move |id: String, _w, cx| {
                                let client = client.clone();
                                let del_id = id.clone();
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.save_transcript_scroll_position(cx);
                                        Rc::make_mut(&mut this.sessions).retain(|s| s.id != id);
                                        this.close_matching_workspace_tabs(|t| {
                                            t.id() == format!("chat:{}", id)
                                        });
                                        // Drop all run-derived state for the deleted
                                        // session, now keyed by session id.
                                        this.running_sessions.remove(&id);
                                        this.pending_permissions.remove(&id);
                                        this.pending_questions.remove(&id);
                                        this.question_selected.remove(&id);
                                        this.todo_items.remove(&id);
                                        this.agent_notices.remove(&id);
                                        if this.selected_session_id.as_deref() == Some(&id) {
                                            this.selected_session_id =
                                                this.active_pane_id.as_deref().and_then(
                                                    |pane_id| this.active_session_for_pane(pane_id),
                                                );
                                            let pane_id = this
                                                .active_pane_id
                                                .clone()
                                                .unwrap_or_else(|| "pane-main".to_string());
                                            let composer = this.composer_for_pane(&pane_id);
                                            let transcript = this.transcript_for_pane(&pane_id);
                                            composer.update(cx, |input, cx| {
                                                input.set_prompt_history(Vec::new(), cx);
                                            });
                                            transcript.update(cx, |t, cx| {
                                                t.set_messages(Vec::new(), cx);
                                            });
                                            if let Some(next_id) = this.selected_session_id.clone()
                                            {
                                                this.load_session_messages_for_pane(
                                                    pane_id, next_id, cx,
                                                );
                                            }
                                        }
                                        if this.right_sidebar_visible {
                                            this.refresh_inspector(cx);
                                        }
                                        cx.notify();

                                        cx.spawn(async move |entity, cx| {
                                            if let Err(error) =
                                                client.sessions.delete(&del_id).await
                                            {
                                                let message = format!(
                                                    "Unable to delete the session: {error}"
                                                );
                                                cx.update(|cx| {
                                                    if let Some(app) = entity.upgrade() {
                                                        app.update(cx, |this, cx| {
                                                            this.set_error(message, cx)
                                                        });
                                                    }
                                                });
                                            }
                                        })
                                        .detach();
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |key: String, _w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        let is_new_chat = key == "new_chat";
                                        this.discard_draft(&key);
                                        // If the discarded draft is currently shown in the
                                        // active composer, clear the input so stale text
                                        // doesn't remain.
                                        let active_sid = this
                                            .active_pane_id
                                            .as_deref()
                                            .and_then(|pane_id| this.active_session_for_pane(pane_id))
                                            .map(|s| s.to_string());
                                        let should_clear = if is_new_chat {
                                            active_sid.is_none()
                                        } else {
                                            active_sid.as_deref() == Some(key.as_str())
                                        };
                                        if should_clear {
                                            let composer = this.active_composer_input();
                                            composer.update(cx, |input, cx| input.clear(cx));
                                        }
                                        cx.notify();
                                    });
                                }
                            }
                        },
                        self.session_rename_id.clone(),
                        Some(self.session_rename_input.clone()),
                        {
                            let entity = entity.clone();
                            move |start_x, _w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.begin_sidebar_resize(start_x);
                                        cx.notify();
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_settings(window, cx);
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |window, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_settings_tab(
                                            console_ui::settings::SettingsTab::Connection,
                                            window,
                                            cx,
                                        );
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |env_id: String, _w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.activate_environment(env_id, cx);
                                    });
                                }
                            }
                        },
                    ))
                    // The workspace pane is a sibling of the sidebar, inside
                    // the same row, so it sits to the right of it.
                    .child(
                        div()
                            .flex_1()
                            .h_full()
                            .flex()
                            .flex_col()
                            .overflow_hidden()
                            .child({
                                let entity_for_split = entity.clone();
                                WorkspacePane::new(
                                    workspace_root,
                                    active_pane,
                                    render_content,
                                    on_select_tab,
                                    on_close_tab,
                                    on_drop_tab,
                                    on_close_pane,
                                    on_focus_pane,
                                )
                                .with_resize_split(
                                    move |split_id, direction, start_pos, window, cx| {
                                        if let Some(app) = entity_for_split.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.begin_split_resize(
                                                    split_id, direction, start_pos, window,
                                                );
                                                cx.notify();
                                            });
                                        }
                                    },
                                )
                            }),
                    )
                    // Conductor-style Right Sidebar Inspector (All files & Changes & Subagents)
                    .when(self.right_sidebar_visible, |el| {
                        let subagents = self
                            .selected_session_id
                            .as_deref()
                            .and_then(|id| self.session_subagents.get(id).cloned())
                            .unwrap_or_else(|| Rc::new(Vec::new()));

                        el.child(RightSidebar::new(
                            self.right_sidebar_width,
                            self.inspector_active_tab,
                            self.inspector_search_query.clone(),
                            self.inspector_tree.clone(),
                            self.inspector_working_changes.clone(),
                            self.inspector_session_changes.clone(),
                            subagents,
                            (*self.inspector_expanded_folders).clone(),
                            self.expanded_subagents.clone(),
                            self.inspector_selected_path.clone(),
                            on_select_inspector_tab,
                            on_toggle_inspector_folder,
                            on_select_inspector_file,
                            on_toggle_subagent,
                            on_copy_summary,
                            on_refresh_inspector,
                            on_begin_right_sidebar_resize,
                        ))
                    }),
            )
            .when_some(self.zoomed_image.clone(), |el, image| {
                el.child(ImageViewerModal::new(image, "Image preview", {
                    let entity = entity.clone();
                    move |_w, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.zoomed_image = None;
                                cx.notify();
                            });
                        }
                    }
                }))
            })
            // Palette overlays (each renders nothing while closed).
            .child(self.command_palette.clone())
            .child(self.quick_open_palette.clone())
            .child(self.project_browse_palette.clone())
    }
}
