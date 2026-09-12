mod workspace_content;

use std::rc::Rc;

use console_ui::workspace::{
    ContentRenderer, WorkspaceDrag, WorkspaceDropAction, WorkspacePane, cancel_workspace_drags,
};
use console_ui::{
    ImageViewerModal, RightSidebar, RightSidebarBottomSplit, SidebarView, TerminalTabInfo, Theme,
    TitleBar,
};
use gpui::{
    App, Context, InteractiveElement, IntoElement, KeyDownEvent, MouseButton, MouseMoveEvent,
    MouseUpEvent, ParentElement, Render, Styled, Window, div, prelude::FluentBuilder,
};

use crate::state::ConsoleDesktopApp;

impl Render for ConsoleDesktopApp {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        self.maybe_persist_window_state(window, cx);
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
                            this.activate_workspace_tab(&pane_id, &tab_id, cx);
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
                            this.close_tab_and_sync_pane(&pane_id, &tab_id, cx);
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
                                console_core::SplitDirection::Horizontal,
                                true,
                                window,
                                cx,
                            );
                        }
                        WorkspaceDropAction::SplitRight => {
                            this.move_workspace_tab_to_split(
                                target_pane_id,
                                drag,
                                console_core::SplitDirection::Horizontal,
                                false,
                                window,
                                cx,
                            );
                        }
                        // Stacked rows are terminal-only: any other tab dropped
                        // on the top/bottom zones no-ops instead of retabbing.
                        WorkspaceDropAction::SplitTop
                            if matches!(
                                drag.tab,
                                console_core::WorkspaceTabConfig::Terminal { .. }
                            ) =>
                        {
                            this.move_workspace_tab_to_split(
                                target_pane_id,
                                drag,
                                console_core::SplitDirection::Vertical,
                                true,
                                window,
                                cx,
                            );
                        }
                        WorkspaceDropAction::SplitBottom
                            if matches!(
                                drag.tab,
                                console_core::WorkspaceTabConfig::Terminal { .. }
                            ) =>
                        {
                            this.move_workspace_tab_to_split(
                                target_pane_id,
                                drag,
                                console_core::SplitDirection::Vertical,
                                false,
                                window,
                                cx,
                            );
                        }
                        WorkspaceDropAction::SplitTop | WorkspaceDropAction::SplitBottom => {}
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
        // Title reflects the active tab in the focused pane: File/Diff shows
        // "filename — folder", otherwise the selected chat's "title — folder".
        let titlebar_text: Option<String> = {
            let active_pane_id = active_pane.as_deref().unwrap_or("pane-main");
            let active_tab = workspace_root
                .leaves()
                .into_iter()
                .find(|l| l.id == active_pane_id)
                .and_then(|leaf| {
                    leaf.active_tab_id
                        .as_deref()
                        .and_then(|id| leaf.tabs.iter().find(|t| t.id() == id))
                });
            match active_tab {
                Some(console_core::WorkspaceTabConfig::File {
                    title, project_id, ..
                })
                | Some(console_core::WorkspaceTabConfig::Diff {
                    title, project_id, ..
                })
                | Some(console_core::WorkspaceTabConfig::Terminal {
                    title, project_id, ..
                }) => {
                    let folder = project_id
                        .as_deref()
                        .and_then(|pid| {
                            self.projects
                                .iter()
                                .find(|p| p.id == pid)
                                .map(|p| p.name.clone())
                        })
                        .or_else(|| {
                            self.pane_project_id(active_pane_id).and_then(|pid| {
                                self.projects
                                    .iter()
                                    .find(|p| p.id == pid)
                                    .map(|p| p.name.clone())
                            })
                        })
                        .or_else(|| {
                            self.selected_session_id
                                .as_deref()
                                .and_then(|sid| self.sessions.iter().find(|s| s.id == sid))
                                .and_then(|session| {
                                    self.projects
                                        .iter()
                                        .find(|p| p.matches_session(session))
                                        .map(|p| p.name.clone())
                                        .or_else(|| {
                                            let cwd = &session.cwd;
                                            if cwd.is_empty() {
                                                None
                                            } else {
                                                Some(console_ui::utils::format_folder_display_name(
                                                    cwd,
                                                ))
                                            }
                                        })
                                })
                        });
                    match folder {
                        Some(f) if !f.is_empty() => Some(format!("{} — {}", title, f)),
                        _ => Some(title.clone()),
                    }
                }
                Some(console_core::WorkspaceTabConfig::Chat {
                    session_id, title, ..
                }) => {
                    let session = self.sessions.iter().find(|s| &s.id == session_id);
                    let folder = session.and_then(|s| {
                        self.projects
                            .iter()
                            .find(|p| p.matches_session(s))
                            .map(|p| p.name.clone())
                            .or_else(|| {
                                if s.cwd.is_empty() {
                                    None
                                } else {
                                    Some(console_ui::utils::format_folder_display_name(&s.cwd))
                                }
                            })
                    });
                    match folder {
                        Some(f) if !f.is_empty() => Some(format!("{} — {}", title, f)),
                        _ => Some(title.clone()),
                    }
                }
                None => None,
            }
        };
        let on_toggle_sidebar: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.toggle_left_sidebar(cx);
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
        let on_select_inspector_tab: Rc<
            dyn Fn(console_ui::InspectorTab, &mut Window, &mut App) + 'static,
        > = {
            let entity = entity.clone();
            Rc::new(move |tab, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.set_inspector_tab(tab, cx);
                    });
                }
            })
        };
        let on_open_auxiliary_tab: Rc<
            dyn Fn(console_ui::AuxiliaryTab, &mut Window, &mut App) + 'static,
        > = {
            let entity = entity.clone();
            Rc::new(move |tab, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_auxiliary_tab(tab, cx));
                }
            })
        };
        let on_close_auxiliary_tab: Rc<
            dyn Fn(console_ui::AuxiliaryTab, &mut Window, &mut App) + 'static,
        > = {
            let entity = entity.clone();
            Rc::new(move |tab, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.close_auxiliary_tab(tab, cx));
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
                    app.update(cx, |this, cx| match this.inspector_active_tab {
                        console_ui::InspectorTab::Primary(console_ui::PrimaryTab::AllFiles) => {
                            this.open_file_tab(path, cx);
                        }
                        console_ui::InspectorTab::Primary(console_ui::PrimaryTab::Changes) => {
                            this.open_diff_tab(path, cx);
                        }
                        console_ui::InspectorTab::Auxiliary(_) => {}
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
        let on_begin_right_sidebar_bottom_resize: Rc<dyn Fn(f32, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |start_y, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.begin_right_sidebar_bottom_resize(start_y);
                        cx.notify();
                    });
                }
            })
        };
        let on_select_right_sidebar_bottom_tab: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |tab_idx, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.select_right_sidebar_terminal_tab(tab_idx, cx);
                    });
                }
            })
        };
        let on_close_right_sidebar_bottom_tab: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |tab_idx, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.close_right_sidebar_terminal(tab_idx, cx);
                    });
                }
            })
        };
        let on_new_right_sidebar_terminal: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.add_right_sidebar_terminal(window, cx);
                    });
                }
            })
        };
        let on_toggle_right_sidebar_bottom_collapsed: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.toggle_right_sidebar_bottom_collapsed(cx);
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
            .on_key_down({
                let entity = entity.clone();
                move |event: &KeyDownEvent, _, cx| {
                    if event.keystroke.key == "escape" {
                        // Close the image preview first — it sits above
                        // everything else, so it owns Escape while open.
                        if let Some(app) = entity.upgrade() {
                            let had_preview = app.update(cx, |this, cx| {
                                if this.zoomed_image.is_some() {
                                    this.zoomed_image = None;
                                    cx.notify();
                                    true
                                } else {
                                    false
                                }
                            });
                            if had_preview {
                                return;
                            }
                        }
                        cancel_workspace_drags();
                    } else if event.keystroke.modifiers.platform
                        && event.keystroke.modifiers.shift
                        && event.keystroke.key.eq_ignore_ascii_case("n")
                    {
                        let environment_id = entity
                            .upgrade()
                            .map(|app| app.read(cx).active_env_id.clone())
                            .flatten();
                        crate::window::open_workspace_window(
                            cx,
                            crate::window::WindowLaunchTarget::Fresh { environment_id },
                        );
                    }
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
                            let right_sidebar_bottom_changed =
                                this.resize_right_sidebar_bottom(f32::from(event.position.y));
                            let split_changed = this.resize_split_drag(event.position);
                            if sidebar_changed
                                || right_sidebar_changed
                                || right_sidebar_bottom_changed
                                || split_changed
                            {
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
                            let right_sidebar_bottom_changed =
                                this.finish_right_sidebar_bottom_resize();
                            let split_changed = this.finish_split_resize();
                            if sidebar_changed
                                || right_sidebar_changed
                                || right_sidebar_bottom_changed
                                || split_changed
                            {
                                cx.notify();
                            }
                        });
                    }
                }
            })
            .child({
                let ports = self.forwarded_ports_for_active_workspace();
                let entity_ports = entity.clone();
                let entity_unforward = entity.clone();
                let ports_popover = console_ui::PortsPopover::new(
                    ports,
                    self.ports_menu_handle.clone(),
                    move |url, window, cx| {
                        if let Some(app) = entity_ports.upgrade() {
                            app.update(cx, |this, cx| this.open_port_in_browser(url, window, cx));
                        }
                    },
                    move |port, _window, cx| {
                        if let Some(app) = entity_unforward.upgrade() {
                            app.update(cx, |this, cx| this.unforward_port(port, cx));
                        }
                    },
                );

                TitleBar::new(titlebar_text, self.sidebar_width, on_toggle_sidebar)
                    .with_right_sidebar_toggle(self.right_sidebar_visible, on_toggle_right_sidebar)
                    .with_ports(Some(ports_popover.into_any_element()))
            })
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
                        self.sidebar_sort_mode,
                        self.collapsed_projects.clone(),
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
                                        this.select_and_open_session(id, cx);
                                    });
                                }
                            }
                        },
                        {
                            move |id: String, _w, cx: &mut App| {
                                crate::window::open_workspace_window(
                                    cx,
                                    crate::window::WindowLaunchTarget::Session(id),
                                );
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
                            move |key: String, _w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.toggle_sidebar_project(key, cx);
                                    });
                                }
                            }
                        },
                        {
                            let entity = entity.clone();
                            move |_w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.toggle_sidebar_sort_mode(cx);
                                    });
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
                                        this.close_matching_workspace_tabs(
                                            |t| t.id() == format!("chat:{}", id),
                                            cx,
                                        );
                                        for root in this.project_workspace_roots.values_mut() {
                                            console_ui::workspace::ops::close_matching_tabs(
                                                root,
                                                |t| t.id() == format!("chat:{}", id),
                                            );
                                        }
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
                                        this.maybe_refresh_inspector(cx);
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
                                        this.discard_draft(&key, cx);
                                        // If the discarded draft is currently shown in the
                                        // active composer, clear the input so stale text
                                        // doesn't remain.
                                        let active_sid = this
                                            .active_pane_id
                                            .as_deref()
                                            .and_then(|pane_id| {
                                                this.active_session_for_pane(pane_id)
                                            })
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
                                let tab_follows: std::collections::HashMap<
                                    String,
                                    console_ui::workspace::TabStripFollow,
                                > = self
                                    .workspace_pane_states
                                    .iter()
                                    .map(|(k, v)| (k.clone(), v.tab_strip_follow.clone()))
                                    .collect();
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
                                .with_tab_follow(move |pane_id| tab_follows.get(pane_id).cloned())
                                .with_new_tab({
                                    let entity = entity.clone();
                                    move |_pane_id, _window, cx| {
                                        if let Some(app) = entity.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.create_new_chat(cx);
                                            });
                                        }
                                    }
                                })
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
                    // Conductor-style Right Sidebar Inspector (All files & Changes & Subagents + Terminal bottom split)
                    .when(self.right_sidebar_visible, |el| {
                        let subagents = self
                            .selected_session_id
                            .as_deref()
                            .and_then(|id| self.session_subagents.get(id).cloned())
                            .unwrap_or_else(|| Rc::new(Vec::new()));

                        self.ensure_right_sidebar_terminal(window, cx);

                        let (_, active_cwd) = self.active_inspector_target();
                        let active_term_state = active_cwd
                            .as_ref()
                            .and_then(|cwd| self.right_sidebar_terminals_by_cwd.get(cwd));

                        let tabs: Vec<TerminalTabInfo> = active_term_state
                            .map(|state| {
                                state
                                    .terminals
                                    .iter()
                                    .enumerate()
                                    .map(|(idx, _)| TerminalTabInfo {
                                        id: idx,
                                        title: format!("Terminal {}", idx + 1),
                                    })
                                    .collect()
                            })
                            .unwrap_or_default();

                        let active_idx = active_term_state.map(|s| s.active_idx).unwrap_or(0);
                        let terminal_element = active_term_state
                            .and_then(|state| state.terminals.get(active_idx))
                            .map(|(_, term)| term.clone().into_any_element());

                        let bottom_split = RightSidebarBottomSplit::new(
                            self.right_sidebar_bottom_height,
                            self.right_sidebar_bottom_collapsed,
                            tabs,
                            active_idx,
                            terminal_element,
                            on_select_right_sidebar_bottom_tab,
                            on_begin_right_sidebar_bottom_resize,
                        )
                        .with_close_tab(on_close_right_sidebar_bottom_tab)
                        .with_new_terminal(on_new_right_sidebar_terminal)
                        .with_toggle_collapsed(on_toggle_right_sidebar_bottom_collapsed);

                        let browser_visible = self.right_sidebar_visible
                            && self.inspector_active_tab
                                == console_ui::InspectorTab::Auxiliary(
                                    console_ui::AuxiliaryTab::Browser,
                                );
                        let browser_element = if browser_visible {
                            let browser = self.browser_view_for_inspector(window, cx);
                            let is_overlay_open = self.command_palette.read(cx).is_open(cx)
                                || self.quick_open_palette.read(cx).is_open(cx)
                                || self.project_browse_palette.read(cx).is_open(cx);
                            browser.update(cx, |view, cx| {
                                view.sync_native_state(true, is_overlay_open, cx);
                            });
                            Some(browser.into_any_element())
                        } else {
                            if let Some(ref browser) = self.browser_view {
                                browser.update(cx, |view, cx| {
                                    view.sync_native_state(false, false, cx);
                                });
                            }
                            None
                        };
                        let device_visible = self.right_sidebar_visible
                            && self.inspector_active_tab
                                == console_ui::InspectorTab::Auxiliary(
                                    console_ui::AuxiliaryTab::Devices,
                                );
                        let device_element = if device_visible {
                            let device = self.device_view_for_inspector(window, cx);
                            let is_overlay_open = self.command_palette.read(cx).is_open(cx)
                                || self.quick_open_palette.read(cx).is_open(cx)
                                || self.project_browse_palette.read(cx).is_open(cx);
                            device.update(cx, |view, cx| {
                                view.sync_native_state(true, is_overlay_open, cx);
                            });
                            Some(device.into_any_element())
                        } else {
                            if let Some(ref device) = self.device_view {
                                device.update(cx, |view, cx| {
                                    view.sync_native_state(false, false, cx);
                                });
                            }
                            None
                        };

                        el.child(
                            RightSidebar::new(
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
                                self.inspector_open_auxiliary_tabs.clone(),
                                self.inspector_add_tab_menu.clone(),
                                on_select_inspector_tab,
                                on_open_auxiliary_tab,
                                on_close_auxiliary_tab,
                                on_toggle_inspector_folder,
                                on_select_inspector_file,
                                on_toggle_subagent,
                                on_copy_summary,
                                on_refresh_inspector,
                                on_begin_right_sidebar_resize,
                            )
                            .with_bottom_split(Some(bottom_split))
                            .with_browser_view(browser_element)
                            .with_device_view(device_element)
                            .subagent_markdown_views(self.subagent_markdown_views.clone()),
                        )
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
