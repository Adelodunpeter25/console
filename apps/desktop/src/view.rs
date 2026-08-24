use std::rc::Rc;

use console_core::{
    ApprovalMode, ApproveToolPermissionDto, CreateSessionDto, ModelFavorite, SelectedModel,
    UpdateSessionDto,
};
use console_ui::workspace::{
    ContentRenderer, EmptyChatState, WorkspaceDrag, WorkspaceDropAction, WorkspacePane,
    cancel_workspace_drags,
};
use console_ui::{
    ApprovalModeDropdown, CommandPalette, ComposerView, ImageViewerModal, ModelDropdownMenu,
    PaletteEntry, PermissionInteractionCard, PickerTab, QuestionInteractionCard, SidebarView,
    Theme, TitleBar, WorkspaceFooter, centered_stripe, error_banner, notice_banner, todo_card,
};
use gpui::{
    App, Context, InteractiveElement, IntoElement, KeyDownEvent, MouseButton, MouseMoveEvent,
    MouseUpEvent, ParentElement, Render, Styled, Window, div, prelude::FluentBuilder, px,
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
        // Shared "New Chat" action: used by the sidebar's New Task row and
        // the empty state's New Chat button.
        let on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            let client = client.clone();
            Rc::new(move |_w, cx| {
                let client = client.clone();
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let pane_id = this
                            .active_pane_id
                            .clone()
                            .unwrap_or_else(|| "pane-main".to_string());
                        let approval_mode = this.pane_approval_mode(&pane_id);
                        let selected_model = this.pane_selected_model(&pane_id);
                        let session_project_id = this.pane_project_id(&pane_id);
                        let session_cwd = this
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
                                                this.apply_session_header_for_pane(
                                                    &pane_id,
                                                    &new_session,
                                                    cx,
                                                );
                                                this.clear_error(cx);
                                                if this.active_pane_id.as_deref()
                                                    == Some(pane_id.as_str())
                                                {
                                                    this.selected_session_id =
                                                        Some(new_session.id.clone());
                                                }
                                                Rc::make_mut(&mut this.sessions).insert(0, new_session.clone());
                                                this.open_chat_tab_in_pane(
                                                    &pane_id,
                                                    new_session.id.clone(),
                                                    "New Chat",
                                                );
                                                this.composer_for_pane(&pane_id).update(
                                                    cx,
                                                    |input, cx| {
                                                        input.set_prompt_history(Vec::new(), cx);
                                                    },
                                                );
                                                this.transcript_for_pane(&pane_id).update(
                                                    cx,
                                                    |t, cx| {
                                                        t.set_messages(Vec::new(), cx);
                                                    },
                                                );
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
                    });
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
        // Command palette entries. Rebuilt each frame so closures capture the
        // current entity/client; `set_entries` never notifies, so this does
        // not loop the render.
        self.command_palette.update(cx, |palette, cx| {
            palette.set_entries(
                vec![
                    PaletteEntry::new("new-chat", "New Chat", {
                        let on_new_chat = on_new_chat.clone();
                        move |window, cx| (on_new_chat)(window, cx)
                    }),
                    PaletteEntry::new("new-terminal", "New Terminal", {
                        let entity = entity.clone();
                        move |window, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.open_terminal_tab(window, cx);
                                });
                            }
                        }
                    }),
                ],
                cx,
            );
        });
        let on_select_tab: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(
                move |pane_id: String, tab_id: String, _w: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.save_transcript_scroll_position(cx);
                            this.select_workspace_tab(&pane_id, &tab_id);
                            if let Some(sid) = tab_id.strip_prefix("chat:") {
                                this.selected_session_id = Some(sid.to_string());
                                this.composer_for_pane(&pane_id).update(cx, |input, cx| {
                                    input.set_prompt_history(Vec::new(), cx);
                                });
                                this.transcript_for_pane(&pane_id).update(cx, |t, cx| {
                                    t.set_messages(Vec::new(), cx);
                                });
                                this.load_session_messages_for_pane(
                                    pane_id.clone(),
                                    sid.to_string(),
                                    cx,
                                );
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
                            this.save_transcript_scroll_position(cx);
                            this.close_workspace_tab(&pane_id, &tab_id);
                            this.active_pane_id = Some(pane_id.clone());
                            let transcript = this.transcript_for_pane(&pane_id);
                            let composer = this.composer_for_pane(&pane_id);
                            if let Some(active) = this.active_tab_id() {
                                if let Some(sid) = active.strip_prefix("chat:") {
                                    this.selected_session_id = Some(sid.to_string());
                                    composer.update(cx, |input, cx| {
                                        input.set_prompt_history(Vec::new(), cx);
                                    });
                                    transcript.update(cx, |t, cx| t.set_messages(Vec::new(), cx));
                                    this.load_session_messages_for_pane(
                                        pane_id.clone(),
                                        sid.to_string(),
                                        cx,
                                    );
                                }
                            } else {
                                this.selected_session_id = None;
                                composer.update(cx, |input, cx| input.set_content("", cx));
                                transcript.update(cx, |t, cx| t.set_messages(Vec::new(), cx));
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
                    .find(|project| {
                        session.project_id.as_deref() == Some(project.id.as_str())
                            || (!session.cwd.is_empty() && session.cwd == project.path)
                    })
                    .map(|project| project.name.clone())
                    .unwrap_or_else(|| {
                        std::path::Path::new(&session.cwd)
                            .file_name()
                            .map(|n| n.to_string_lossy().to_string())
                            .unwrap_or_default()
                    });
                if folder.is_empty() {
                    session.title.clone()
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
                            if this.resize_sidebar(f32::from(event.position.x)) {
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
                            if this.finish_sidebar_resize() {
                                cx.notify();
                            }
                        });
                    }
                }
            })
            .child(TitleBar::new(
                titlebar_text,
                self.sidebar_width,
                on_toggle_sidebar,
            ))
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
                        self.sidebar_list_state.clone(),
                        {
                            let entity = entity.clone();
                            move |id: String, _w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.save_transcript_scroll_position(cx);
                                        this.selected_session_id = Some(id.clone());
                                        // Use the session's real title for the tab
                                        // (falling back like the sidebar row does),
                                        // not a hardcoded "Chat".
                                        let title = this
                                            .sessions
                                            .iter()
                                            .find(|s| s.id == id)
                                            .map(|s| {
                                                if s.title.trim().is_empty() {
                                                    "New Chat".to_string()
                                                } else {
                                                    s.title.clone()
                                                }
                                            })
                                            .unwrap_or_else(|| "Chat".to_string());
                                        this.open_chat_tab(id.clone(), title);
                                        this.active_composer_input().update(cx, |input, cx| {
                                            input.set_prompt_history(Vec::new(), cx);
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
                                        this.command_palette.update(cx, |palette, cx| {
                                            palette.show(window, cx);
                                        });
                                        cx.notify();
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |_w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| this.add_project(cx));
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
                            .child(WorkspacePane::new(
                                workspace_root,
                                active_pane,
                                render_content,
                                on_select_tab,
                                on_close_tab,
                                on_drop_tab,
                                on_close_pane,
                                on_focus_pane,
                            )),
                    ),
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
            // Command palette overlay (renders nothing while closed).
            .child(self.command_palette.clone())
    }
}

impl ConsoleDesktopApp {
    /// Renders the content area of the active workspace pane: the
    /// transcript, interaction banners/cards, and the composer + footer.
    pub fn render_workspace_content(
        &mut self,
        pane_id: &str,
        active_tab: Option<&console_core::WorkspaceTabConfig>,
        on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        // No chat tab open in this pane — show the centered empty state
        // (no transcript, no composer, no footer) instead of chat chrome.
        if active_tab.is_none() {
            return EmptyChatState::new(on_new_chat).into_any_element();
        }

        // Terminal tab: render the live terminal surface for its id.
        if let Some(console_core::WorkspaceTabConfig::Terminal { terminal_id, .. }) = active_tab {
            let theme = Theme::current(cx);
            return match self.terminals.get(terminal_id) {
                Some(view) => div().size_full().child(view.clone()).into_any_element(),
                None => div()
                    .size_full()
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_color(theme.text_ghost)
                    .text_size(px(12.0))
                    .child("Terminal session ended")
                    .into_any_element(),
            };
        }

        let pane_id = pane_id.to_owned();
        self.ensure_workspace_pane_state(&pane_id, window, cx);
        let pane_transcript = self.transcript_for_pane(&pane_id);
        let pane_composer = self.composer_for_pane(&pane_id);
        let pane_selected_model = self.pane_selected_model(&pane_id);
        let pane_picker_tab = self.pane_picker_tab(&pane_id);
        let pane_approval_mode = self.pane_approval_mode(&pane_id);
        let pane_project_id = self.pane_project_id(&pane_id);
        let pane_branches = self.pane_branches(&pane_id);
        let pane_branch_loaded = self.pane_branch_loaded(&pane_id);
        let pane_is_git_repository = self.pane_is_git_repository(&pane_id);
        let pane_branch_pending = self.pane_branch_pending(&pane_id);
        let pane_model_menu = self.pane_model_menu(&pane_id);
        let pane_approval_menu = self.pane_approval_menu(&pane_id);
        let pane_project_menu = self.pane_project_menu(&pane_id);
        let pane_branch_menu = self.pane_branch_menu(&pane_id);
        let pane_model_search = self.pane_model_search(&pane_id);
        // Snapshot the query this frame; the dropdown filters against it. Edits
        // notify the app (see the model_search subscription), so this read is
        // always fresh when the dropdown re-renders.
        let pane_model_search_query = pane_model_search.read(cx).content().to_owned();
        let pane_session_id = active_tab.and_then(|tab| match tab {
            console_core::WorkspaceTabConfig::Chat { session_id, .. } => Some(session_id.clone()),
            _ => None,
        });
        let autocomplete =
            self.composer_autocomplete_for_pane(&pane_id, pane_session_id.as_deref(), window, cx);

        let theme = Theme::current(cx);
        let entity = cx.entity().downgrade();
        let client = self.client.clone();
        let error_message = self.error_message.clone();
        let error_selection = self.error_selection.clone();
        let agent_notice = self.agent_notice_for_pane(&pane_id);
        let todo_items = self.todo_items_for_pane(&pane_id);
        let selected_sid = pane_session_id.clone();

        div()
            .flex_1()
            .min_h_0()
            .w_full()
            .flex()
            .flex_col()
            .overflow_hidden()
        .child(
            div()
                .flex_1()
                .w_full()
                .overflow_hidden()
                // Clicking an image in a message opens the app's
                // image preview modal with its data URI. The handler is wired
                // once during app initialization, not mutated from this
                // render path.
                .child(pane_transcript),
        )
        .when_some(error_message, |el, error| {
            let entity = entity.clone();
            el.child(error_banner(
                error,
                theme,
                error_selection.clone(),
                Some(Rc::new(move |cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.clear_error(cx));
                    }
                })),
                cx,
            ))
        })
        .when_some(agent_notice, |el, notice| {
            el.child(notice_banner(notice, theme))
        })
        .when(!todo_items.is_empty(), |el| {
            el.child(todo_card(todo_items.clone(), theme))
        })
        .when_some(self.pending_question_for_pane(&pane_id), |el, question| {
            let entity_for_answer = entity.clone();
            let question_input = self.question_input_for_pane(&pane_id);
            let selected = self.question_selected_for_pane(&pane_id);
            let is_multi = question.is_multi_select.unwrap_or(false);
            let answer_sid = selected_sid.clone();
            let question_card = QuestionInteractionCard::new(
                question,
                true,
                false,
                move |answer, _window, cx| {
                    let Some(sid) = answer_sid.clone() else {
                        return;
                    };
                    if let Some(app) = entity_for_answer.upgrade() {
                        app.update(cx, |this, cx| {
                            this.answer_pending_question_for_session(sid, answer, cx);
                        });
                    }
                },
            )
            .custom_input(question_input.clone())
            .selected(selected)
            .on_select({
                // Toggle against the session that owns this card, not whichever
                // pane holds focus at click time.
                let entity = entity.clone();
                let answer_sid = selected_sid.clone();
                move |option, _window, cx| {
                    let Some(sid) = answer_sid.clone() else {
                        return;
                    };
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if is_multi {
                                let mut sel = this.question_selected_for_session(&sid);
                                if !sel.insert(option.clone()) {
                                    sel.remove(&option);
                                }
                                this.set_question_selected_for_session(&sid, sel);
                            } else {
                                let mut sel = std::collections::HashSet::new();
                                sel.insert(option);
                                this.set_question_selected_for_session(&sid, sel);
                            }
                            cx.notify();
                        });
                    }
                }
            });
            el.child(centered_stripe(question_card, 8.0))
        })
        .when_some(self.pending_permission_for_pane(&pane_id), |el, perm| {
            let client = client.clone();
            let entity = entity.clone();
            // Approve against the session that rendered this card, never the
            // globally selected one: focus can sit in another split while the
            // user clicks, and clearing by the wrong id would strand the run.
            let active_sid = selected_sid.clone().unwrap_or_default();

            el.child(centered_stripe(
                PermissionInteractionCard::new(
                            perm.clone(),
                            false,
                            move |allow, _window, cx| {
                                let client = client.clone();
                                let sid = active_sid.clone();
                                let req_id = perm.request_id.clone();
                                let entity = entity.clone();

                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.set_pending_permission_for_session(&sid, None);
                                        cx.notify();

                                        let entity_for_error = entity.clone();
                                        cx.spawn(async move |_entity, cx| {
                                            if let Err(error) = client.runs.approve_permission(
                                                &sid,
                                                ApproveToolPermissionDto {
                                                    request_id: req_id,
                                                    allow,
                                                },
                                            ).await {
                                                let message = format!("Unable to update tool permission: {error}");
                                                cx.update(|cx| {
                                                    if let Some(app) = entity_for_error.upgrade() {
                                                        app.update(cx, |this, cx| this.set_error(message, cx));
                                                    }
                                                });
                                            }
                                        }).detach();
                                    });
                                }
                            },
                        ),
                8.0,
            )
            )
        })
        .when(selected_sid.is_some(), |el| el.child({
            let providers = self.providers.clone();
            let selected_model = pane_selected_model.clone();
            let active_tab = pane_picker_tab.clone();
            let favorites = self.favorites.clone();
            let model_handle = pane_model_menu.clone();
            let approval_handle = pane_approval_menu.clone();
            let model_handle_for_select = model_handle.clone();
            let approval_handle_for_select = approval_handle.clone();
            let project_pane_id = pane_id.clone();
            let clear_project_pane_id = pane_id.clone();
            let branch_pane_id = pane_id.clone();
            let model_pane_id = pane_id.clone();
            let picker_pane_id = pane_id.clone();
            let approval_pane_id = pane_id.clone();
            let composer_pane_id = pane_id.clone();
            let submit_pane_id = composer_pane_id.clone();
            let abort_pane_id = pane_id.clone();
            let add_project_pane_id = pane_id.clone();
            let composer_autocomplete = autocomplete;
            let workspace_footer = WorkspaceFooter::new(
                self.projects.clone(),
                pane_project_id.clone(),
                pane_branches.clone(),
                pane_branch_loaded,
                pane_is_git_repository,
                pane_branch_pending,
                pane_project_menu.clone(),
                pane_branch_menu.clone(),
                {
                    let entity = entity.clone();
                    move |id: String, _w, cx| {
                        if let Some(app) = entity.upgrade() {
                            let pane_id = project_pane_id.clone();
                            app.update(cx, |this, cx| this.select_project_for_pane(pane_id, id, cx));
                        }
                    }
                },
                {
                    let entity = entity.clone();
                    move |_w, cx| {
                        if let Some(app) = entity.upgrade() {
                            let pane_id = add_project_pane_id.clone();
                            app.update(cx, |this, cx| this.add_project_for_pane(pane_id, cx));
                        }
                    }
                },
                {
                    let entity = entity.clone();
                    move |_w, cx| {
                        if let Some(app) = entity.upgrade() {
                            let pane_id = clear_project_pane_id.clone();
                            app.update(cx, |this, cx| this.clear_project_for_pane(pane_id, cx));
                        }
                    }
                },
                {
                    let entity = entity.clone();
                    move |name: String, _w, cx| {
                        if let Some(app) = entity.upgrade() {
                            let pane_id = branch_pane_id.clone();
                            app.update(cx, |this, cx| this.checkout_branch_for_pane(pane_id, name, cx));
                        }
                    }
                },
            )
            .project_locked(self.session_has_messages(&pane_id));

            let model_dropdown = ModelDropdownMenu::new(
                providers,
                selected_model,
                active_tab,
                favorites,
                self.models_by_provider.clone(),
                pane_model_search.clone(),
                pane_model_search_query,
                {
                    let entity = entity.clone();
                    move |prov: String, m_id: String, window, cx| {
                        model_handle_for_select.close(window, cx);
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.set_pane_model(
                                    &model_pane_id,
                                    Some(SelectedModel {
                                        provider: prov.clone(),
                                        model_id: m_id.clone(),
                                    }),
                                );

                                this.update_session_settings_for_pane(
                                    model_pane_id.clone(),
                                    UpdateSessionDto {
                                        title: None,
                                        cwd: None,
                                        model_id: Some(m_id.clone()),
                                        provider: Some(prov.clone()),
                                        approval_mode: None,
                                    },
                                    cx,
                                );
                                cx.notify();
                            });
                        }
                    }
                },
                {
                    let entity = entity.clone();
                    let search_for_tab = pane_model_search.clone();
                    move |tab: PickerTab, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                // Fetch the newly-selected provider's live models
                                // if not already cached. The popover-open handler
                                // eagerly loads all providers, but this covers the
                                // case where a provider was added to the catalog
                                // after the last open.
                                if let PickerTab::Provider(ref name) = tab {
                                    this.load_models_for_provider(name, cx);
                                }
                                this.set_pane_picker_tab(&picker_pane_id, tab);
                                // Clear the filter when switching provider/favorites
                                // tabs, matching the Electron picker's selectTab.
                                search_for_tab.update(cx, |input, cx| input.clear(cx));
                                cx.notify();
                            });
                        }
                    }
                },
                {
                    let entity = entity.clone();
                    move |prov: String, mid: String, _window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                let key = format!("{}:{}", prov, mid);
                                let is_favorite = if this.favorites.contains(&key) {
                                    Rc::make_mut(&mut this.favorites).remove(&key);
                                    false
                                } else {
                                    Rc::make_mut(&mut this.favorites).insert(key);
                                    true
                                };
                                let client = this.client.clone();
                                let entity_for_error = entity.clone();
                                cx.spawn(async move |_entity, cx| {
                                    if let Err(error) = client
                                        .model_favorites
                                        .set(
                                            ModelFavorite {
                                                provider: prov,
                                                model_id: mid,
                                            },
                                            is_favorite,
                                        )
                                        .await
                                    {
                                        let message = format!("Unable to update model favorite: {error}");
                                        cx.update(|cx| {
                                            if let Some(app) = entity_for_error.upgrade() {
                                                app.update(cx, |this, cx| this.set_error(message, cx));
                                            }
                                        });
                                    }
                                }).detach();
                                cx.notify();
                            });
                        }
                    }
                },
            );

            let approval_dropdown = ApprovalModeDropdown::new(
                pane_approval_mode,
                {
                    let entity = entity.clone();
                    move |mode: ApprovalMode, window, cx| {
                        approval_handle_for_select.close(window, cx);
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.set_pane_approval_mode(&approval_pane_id, mode);
                                this.update_session_settings_for_pane(
                                    approval_pane_id.clone(),
                                    UpdateSessionDto {
                                        title: None,
                                        cwd: None,
                                        model_id: None,
                                        provider: None,
                                        approval_mode: Some(mode.value().to_string()),
                                    },
                                    cx,
                                );
                                cx.notify();
                            });
                        }
                    }
                },
            );

            div()
                .relative()
                .w_full()
                .bg(theme.chat_canvas)
                .flex()
                .flex_col()
                .items_center()
                .child(
                    div()
                        .relative()
                        .w_full()
                        .max_w(px(768.0))
                        .bg(theme.chat_canvas)
                        .child(
                            ComposerView::new(
                                pane_composer.clone(),
                                {
                                    let entity = entity.clone();
                                    move |_w, cx| {
                                        if let Some(app) = entity.upgrade() {
                                            app.update(cx, |this, cx| {
                                                let prompt = this
                                                    .composer_for_pane(&submit_pane_id)
                                                    .read(cx)
                                                    .content()
                                                    .to_string();
                                                let attachments =
                                                    (*this.attachments_for_pane(&submit_pane_id))
                                                        .clone();
                                                this.active_pane_id = Some(submit_pane_id.clone());
                                                this.selected_session_id = this.active_session_for_pane(&submit_pane_id);
                                                this.submit_prompt(prompt, attachments, cx);
                                            });
                                        }
                                    }
                                },
                                {
                                    let client = client.clone();
                                    let entity = entity.clone();
                                    move |_w, cx| {
                                        let client = client.clone();
                                        if let Some(app) = entity.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.active_pane_id = Some(abort_pane_id.clone());
                                                this.selected_session_id = this.active_session_for_pane(&abort_pane_id);
                                                if let Some(sid) = &this.selected_session_id {
                                                    // Clear the running state for the chat that
                                                    // owns the run, keyed by session so the Stop
                                                    // button only affects this chat.
                                                    let sid_clone = sid.clone();
                                                    this.set_session_running(&sid_clone, None);
                                                    let entity_for_error = entity.clone();
                                                    cx.spawn(async move |_entity, cx| {
                                                        if let Err(error) = client.runs.abort(&sid_clone).await {
                                                            let message = format!("Unable to stop the agent run: {error}");
                                                            cx.update(|cx| {
                                                                if let Some(app) = entity_for_error.upgrade() {
                                                                    app.update(cx, |this, cx| this.set_error(message, cx));
                                                                }
                                                            });
                                                        }
                                                    }).detach();
                                                }
                                                cx.notify();
                                            });
                                        }
                                    }
                                },
                                {
                                    let entity = entity.clone();
                                    move |_w, cx| {
                                        if let Some(app) = entity.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.pick_image(cx);
                                            });
                                        }
                                    }
                                },
                            )
                            .running(self.is_active_session_running_for_pane(&pane_id))
                            .selected_model(pane_selected_model)
                            .approval_mode(pane_approval_mode)
                            .model_dropdown(model_dropdown, model_handle)
                            .approval_dropdown(approval_dropdown, approval_handle)
                            .autocomplete(composer_autocomplete)
                            .on_autocomplete_next({
                                let entity = entity.clone();
                                let pane_id = composer_pane_id.clone();
                                move |_window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.move_autocomplete_for_pane(&pane_id, true, cx);
                                        });
                                    }
                                }
                            })
                            .on_autocomplete_previous({
                                let entity = entity.clone();
                                let pane_id = composer_pane_id.clone();
                                move |_window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.move_autocomplete_for_pane(&pane_id, false, cx);
                                        });
                                    }
                                }
                            })
                            .on_autocomplete_confirm({
                                let entity = entity.clone();
                                let pane_id = composer_pane_id.clone();
                                move |_window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.accept_highlighted_autocomplete_for_pane(
                                                &pane_id, cx,
                                            );
                                        });
                                    }
                                }
                            })
                            .on_autocomplete_dismiss({
                                let entity = entity.clone();
                                let pane_id = composer_pane_id.clone();
                                move |_window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.dismiss_autocomplete_for_pane(&pane_id, cx);
                                        });
                                    }
                                }
                            })
                            .attachments(self.attachments_for_pane(&composer_pane_id))
                            .on_remove_attachment({
                                let entity = entity.clone();
                                move |index, _w, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.remove_attachment(index, cx);
                                        });
                                    }
                                }
                            })
                            .on_preview_attachment({
                                let entity = entity.clone();
                                move |index, _w, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.preview_attachment(index, cx);
                                        });
                                    }
                                }
                            })
                            .on_drop_files({
                                let entity = entity.clone();
                                move |paths, window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.stage_dropped_files(
                                                paths, window, cx,
                                            );
                                        });
                                    }
                                }
                            }),
                        ),
                )
                .child(workspace_footer)
        }))
            .into_any_element()
    }
}
