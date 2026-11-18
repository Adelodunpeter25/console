mod settings_takeover;
mod workspace_content;

use std::rc::Rc;

use console_core::ProjectInfoExt;
use console_ui::workspace::{
    ContentRenderer, WorkspaceDrag, WorkspaceDropAction, WorkspacePane, cancel_workspace_drags,
};
use console_ui::{ImageViewerModal, SidebarView, Theme, TitleBar};
use gpui::{
    App, Context, InteractiveElement, IntoElement, KeyDownEvent, MouseButton, MouseMoveEvent,
    MouseUpEvent, ParentElement, Render, Styled, Window, div, prelude::FluentBuilder,
};

use crate::state::ConsoleDesktopApp;

impl Render for ConsoleDesktopApp {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        self.maybe_persist_window_state(window, cx);
        // Keep script-shortcut dispatch aimed at the active project even when
        // the sidebar (and its panel) is hidden.
        self.sync_active_shortcuts_to_active_project();
        // The card's popup and chip row are derived from live input state, so
        // they are rebuilt here each frame rather than pushed by events. Skipped
        // while closed so a hidden card costs nothing.
        self.refresh_floating_composer(window, cx);

        // The settings take over the window: they replace the sidebar, the
        // workspace and the inspector, so a settings page is not something you
        // read next to a chat. Returned before the workspace is built, so
        // there is nothing underneath to show through — and before the
        // onboarding, so a step that opens the settings (providers) is not
        // hidden behind the panel that asked for it.
        if self.settings_open {
            return self.render_settings_takeover(window, cx);
        }

        // First-run setup replaces the entire window — title bar, sidebar and
        // inspector included — rather than floating over the workspace. Returned
        // before any of that is built, so there is nothing underneath to show
        // through.
        if self.onboarding_env_id.is_some() {
            return self.render_onboarding(cx);
        }

        let theme = Theme::current(cx);
        let entity = cx.entity().downgrade();

        let workspace_root = self.workspace_root.clone();
        let active_pane = self.active_pane_id.clone();
        let sessions = self.sessions.clone();
        // Shared "New Chat" action for the empty state's New Chat button. Opens
        // the floating composer, like ⌘N and the sidebar's New Task row, so the
        // project is picked in the card rather than inherited from the pane
        // (which is empty after the last tab closes).
        let on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = entity.clone();
            Rc::new(move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_floating_composer(window, cx));
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
                            this.request_close_tab(&pane_id, &tab_id, cx);
                        });
                    }
                },
            )
        };
        // File paths whose buffer has unsaved edits; their tab shows a dot in
        // place of the close X. Computed each render, and an observer on each
        // editor state repaints the app when dirtiness changes.
        let dirty_file_paths = self.dirty_file_paths(cx);
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
                })
                | Some(console_core::WorkspaceTabConfig::Browser {
                    title, project_id, ..
                })
                | Some(console_core::WorkspaceTabConfig::ChangesReview {
                    title, project_id, ..
                })
                | Some(console_core::WorkspaceTabConfig::FileTree {
                    title, project_id, ..
                })
                | Some(console_core::WorkspaceTabConfig::Device {
                    title, project_id, ..
                }) => {
                    let folder = project_id
                        .as_deref()
                        .map(console_core::project_id_of_workspace_key)
                        .and_then(|pid| {
                            self.projects
                                .iter()
                                .find(|p| p.id == pid)
                                .map(|p| p.name.clone())
                        })
                        .or_else(|| {
                            self.pane_real_project_id(active_pane_id).and_then(|pid| {
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
                Some(console_core::WorkspaceTabConfig::Settings { title, .. }) => {
                    Some(title.clone())
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
                    let clean_title = title
                        .lines()
                        .find(|l| !l.trim().is_empty())
                        .unwrap_or(title)
                        .trim();
                    match folder {
                        Some(f) if !f.is_empty() => Some(format!("{} — {}", clean_title, f)),
                        _ => Some(clean_title.to_string()),
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
                move |event: &KeyDownEvent, window, cx| {
                    if event.keystroke.key == "escape" {
                        // Close the image preview first — it sits above
                        // everything else, so it owns Escape while open.
                        if let Some(app) = entity.upgrade() {
                            let handled = app.update(cx, |this, cx| {
                                if this.zoomed_image.is_some() {
                                    this.zoomed_image = None;
                                    cx.notify();
                                    return true;
                                }
                                let transcript = this.active_transcript_view();
                                transcript.update(cx, |trans, cx| {
                                    if trans.is_prompts_open() {
                                        trans.close_prompts(cx);
                                        true
                                    } else if trans.has_active_cite_popup() {
                                        trans.clear_selection(window);
                                        true
                                    } else {
                                        false
                                    }
                                })
                            });
                            if handled {
                                return;
                            }
                        }
                        cancel_workspace_drags();
                    } else if (event.keystroke.key == "r" || event.keystroke.key == "R")
                        && !event.keystroke.modifiers.control
                        && !event.keystroke.modifiers.alt
                        && !event.keystroke.modifiers.platform
                    {
                        // Read the transcript handle and release the app first: the
                        // cite callback updates the app, and nesting that inside an
                        // app update panics.
                        if let Some(app) = entity.upgrade() {
                            let transcript = app.read(cx).active_transcript_view();
                            if transcript.update(cx, |trans, cx| trans.trigger_cite(window, cx)) {
                                cx.stop_propagation();
                            }
                        }
                    } else if (event.keystroke.key == "c" || event.keystroke.key == "C")
                        && (!event.keystroke.modifiers.control && !event.keystroke.modifiers.alt)
                    {
                        if let Some(app) = entity.upgrade() {
                            let transcript = app.read(cx).active_transcript_view();
                            if transcript.update(cx, |trans, cx| trans.trigger_copy(window, cx)) {
                                cx.stop_propagation();
                            }
                        }
                    } else if event.keystroke.modifiers.platform
                        && event.keystroke.modifiers.shift
                        && event.keystroke.key.eq_ignore_ascii_case("n")
                    {
                        let environment_id = entity
                            .upgrade()
                            .and_then(|app| app.read(cx).active_env_id.clone());
                        crate::window::open_workspace_window(
                            cx,
                            crate::window::WindowLaunchTarget::Fresh { environment_id },
                        );
                    } else if let Some(app) = entity.upgrade() {
                        // Project script shortcut. This handler sits at the
                        // window root, so it only sees keystrokes that matched
                        // no keymap binding and were consumed by no focused
                        // element — composer, terminal, browser, and menu
                        // shortcuts always win on their own. Palettes get an
                        // explicit guard since their inputs let most combos
                        // bubble.
                        if let Some(script_id) =
                            app.read(cx).match_script_shortcut(&event.keystroke)
                        {
                            if app.read(cx).any_palette_open(cx) {
                                return;
                            }
                            cx.stop_propagation();
                            app.update(cx, |this, cx| {
                                this.toggle_project_script(&script_id, cx);
                            });
                        }
                    }
                }
            })
            .on_mouse_move({
                let entity = entity.clone();
                move |event: &MouseMoveEvent, _, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let sidebar_changed = this.resize_sidebar(f32::from(event.position.x));
                            let split_changed = this.resize_split_drag(event.position);
                            if sidebar_changed || split_changed {
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
                            let split_changed = this.finish_split_resize();
                            if sidebar_changed || split_changed {
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
                    .with_ports(Some(ports_popover.into_any_element()))
            })
            // In-window menu bar on platforms without a native app menu
            // (Windows/Linux); macOS keeps its native menu bar and gets None.
            .children(crate::menubar::menu_bar(
                &self.menubar_file_menu,
                &self.menubar_edit_menu,
                cx,
            ))
            // Sidebar + workspace sit in a row below the title bar.
            .child(
                div()
                    .flex_1()
                    .min_h_0()
                    .flex()
                    .overflow_hidden()
                    .child(
                        SidebarView::new(
                            self.sidebar_visible,
                            self.sidebar_width,
                            sessions,
                            self.projects.clone(),
                            self.selected_session_id
                                .clone()
                                // A workspace with just one chat keeps it
                                // highlighted while a terminal, file, or
                                // browser tab is in front.
                                .or_else(|| self.focused_chat().map(|(_, sid)| sid.to_string())),
                            self.collapsed_projects.clone(),
                            self.running_sessions_snapshot(),
                            self.waiting_sessions_snapshot(),
                            self.draft_summaries(),
                            self.pinned_collapsed,
                            self.settled_collapsed,
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
                                let entity = entity.clone();
                                move |window: &mut Window, cx: &mut App| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.open_floating_composer(window, cx)
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |key: String, window: &mut Window, cx: &mut App| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.open_floating_composer_for_project(
                                                &key, window, cx,
                                            )
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |window: &mut Window, cx: &mut App| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.start_sidebar_search(window, cx);
                                        });
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
                                            this.toggle_sidebar_pinned(cx);
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |_w, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.settled_collapsed = !this.settled_collapsed;
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
                                move |id: String, _window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.toggle_pin_session(id, cx);
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |id: String, _window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.settle_session(id, cx);
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |id: String, _window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.unsettle_session(id, cx);
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |id: String, _window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.toggle_session_read(id, cx);
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |id: String, _w, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.delete_session(id, cx);
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
                                                console_ui::settings::SettingsTab::Servers,
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
                        )
                        .search(
                            self.sidebar_search_active,
                            self.sidebar_search_query.clone(),
                            Some(self.sidebar_search_input.clone()),
                            {
                                let entity = entity.clone();
                                move |window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.start_sidebar_search(window, cx);
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.reset_or_close_sidebar_search(window, cx);
                                        });
                                    }
                                }
                            },
                            {
                                let entity = entity.clone();
                                move |window, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.clear_sidebar_search(window, cx);
                                        });
                                    }
                                }
                            },
                        ),
                    )
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
                                self.ensure_new_tab_menus(cx);
                                // Handed over once: a later render of the same
                                // split must not animate it again.
                                let new_split_pane = self.new_split_pane.take();
                                let new_tab_menus = self.new_tab_menus.clone();
                                let tab_follows: std::collections::HashMap<
                                    String,
                                    console_ui::workspace::TabStripFollow,
                                > = workspace_root
                                    .leaves()
                                    .into_iter()
                                    .filter_map(|leaf| {
                                        self.workspace_pane_states.get(&leaf.id).map(|state| {
                                            (leaf.id.clone(), state.tab_strip_follow.clone())
                                        })
                                    })
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
                                .with_dirty_paths(dirty_file_paths.clone())
                                .with_new_pane(new_split_pane)
                                .with_new_tab_menu({
                                    let entity = entity.clone();
                                    let menus = new_tab_menus.clone();
                                    move |pane_id| {
                                        let handle = menus.get(pane_id)?.clone();
                                        Some((
                                            handle,
                                            crate::state::new_tab_menu_items(
                                                entity.clone(),
                                                pane_id.to_string(),
                                            ),
                                        ))
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
            // Palette overlays (each renders nothing while closed).
            .child(self.command_palette.clone())
            .child(self.tab_palette.clone())
            .child(self.quick_open_palette.clone())
            .child(self.project_browse_palette.clone())
            .when_some(self.floating_composer_view(window, cx), |el, card| {
                el.child(card)
            })
            .child(self.global_search_panel.clone())
            .when_some(self.pending_html_open.clone(), |el, (pane_id, path)| {
                let file_name = std::path::Path::new(&path)
                    .file_name()
                    .and_then(|n| n.to_str())
                    .unwrap_or(&path)
                    .to_string();
                let app_dismiss = cx.entity().downgrade();
                let app_browser = cx.entity().downgrade();
                let app_editor = cx.entity().downgrade();
                let app_ext = cx.entity().downgrade();
                let pane_b = pane_id.clone();
                let path_b = path.clone();
                let pane_e = pane_id.clone();
                let path_e = path.clone();
                let path_x = path.clone();
                let is_dark = theme.is_dark;
                el.child(
                    div()
                        .id("html-open-chooser-backdrop")
                        .absolute()
                        .inset_0()
                        .bg(theme.backdrop)
                        .flex()
                        .items_center()
                        .justify_center()
                        .on_mouse_down(MouseButton::Left, move |_, _window, cx| {
                            if let Some(app) = app_dismiss.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.pending_html_open = None;
                                    cx.notify();
                                });
                            }
                        })
                        .child(
                            div()
                                .id("html-open-chooser-card")
                                .w(gpui::px(320.0))
                                .rounded(gpui::px(12.0))
                                .bg(theme.menu)
                                .border_1()
                                .border_color(theme.border_strong)
                                .p(gpui::px(14.0))
                                .flex()
                                .flex_col()
                                .gap(gpui::px(6.0))
                                .on_mouse_down(MouseButton::Left, |_, _, cx| {
                                    cx.stop_propagation();
                                })
                                .child(
                                    div()
                                        .px(gpui::px(8.0))
                                        .pb(gpui::px(6.0))
                                        .text_size(gpui::px(13.0))
                                        .font_weight(gpui::FontWeight::SEMIBOLD)
                                        .text_color(theme.text)
                                        .truncate()
                                        .child(file_name),
                                )
                                .child(
                                    div()
                                        .id("html-choice-browser")
                                        .px(gpui::px(10.0))
                                        .py(gpui::px(8.0))
                                        .rounded(gpui::px(8.0))
                                        .hover(|s| s.bg(theme.overlay_strong))
                                        .cursor_pointer()
                                        .text_size(gpui::px(13.0))
                                        .text_color(theme.text)
                                        .child("Open in browser")
                                        .on_mouse_down(MouseButton::Left, move |_, _window, cx| {
                                            if let Some(app) = app_browser.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    this.pending_html_open = None;
                                                    this.open_html_in_browser_tab(
                                                        &pane_b,
                                                        path_b.clone(),
                                                        cx,
                                                    );
                                                });
                                            }
                                        }),
                                )
                                .child(
                                    div()
                                        .id("html-choice-editor")
                                        .px(gpui::px(10.0))
                                        .py(gpui::px(8.0))
                                        .rounded(gpui::px(8.0))
                                        .hover(|s| s.bg(theme.overlay_strong))
                                        .cursor_pointer()
                                        .text_size(gpui::px(13.0))
                                        .text_color(theme.text)
                                        .child("Open in code editor")
                                        .on_mouse_down(MouseButton::Left, move |_, _window, cx| {
                                            if let Some(app) = app_editor.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    this.pending_html_open = None;
                                                    this.open_file_tab_in_pane_force_editor(
                                                        &pane_e,
                                                        path_e.clone(),
                                                        cx,
                                                    );
                                                });
                                            }
                                        }),
                                )
                                .child(
                                    div()
                                        .id("html-choice-external")
                                        .px(gpui::px(10.0))
                                        .py(gpui::px(8.0))
                                        .rounded(gpui::px(8.0))
                                        .hover(|s| s.bg(theme.overlay_strong))
                                        .cursor_pointer()
                                        .text_size(gpui::px(13.0))
                                        .text_color(theme.text_secondary)
                                        .child("Open externally")
                                        .on_mouse_down(MouseButton::Left, move |_, _window, cx| {
                                            if let Some(app) = app_ext.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    this.pending_html_open = None;
                                                    cx.notify();
                                                });
                                            }
                                            let p = path_x.clone();
                                            let (tx, rx) = tokio::sync::oneshot::channel();
                                            tokio::spawn(async move {
                                                let _ = tx.send(
                                                    console_ui::chat::resolve_html_url(&p, is_dark)
                                                        .await,
                                                );
                                            });
                                            cx.spawn(async move |cx| {
                                                if let Some(url) = rx.await.ok().flatten() {
                                                    let _ = cx.update(|cx| cx.open_url(&url));
                                                }
                                            })
                                            .detach();
                                        }),
                                ),
                        ),
                )
            })
            // Boxed so both render paths share a single return type — the
            // first-run setup branch above returns an AnyElement.
            .into_any_element()
    }
}
