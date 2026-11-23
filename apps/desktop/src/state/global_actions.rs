//! Methods behind the global shortcuts bound in `crate::keybindings`.
//!
//! They are registered as *global* action handlers (`App::on_action` in
//! `keybindings::init_handlers`), so they run regardless of keyboard focus.

use console_core::{
    DeskRow, ProjectInfoExt, SessionHeader, SessionHeaderExt, desk_last_active_ms, desk_rows,
};
use console_ui::workspace::ops as workspace_ops;
use console_ui::{IconName, MenuAlign, PaletteEntry, toggle_dropdown, toggle_popover};
use gpui::{Context, Focusable as _, WeakEntity, Window};

use super::ConsoleDesktopApp;

/// A shortcut as it reads on this platform.
fn kbd(mac: &'static str, other: &'static str) -> &'static str {
    if cfg!(target_os = "macos") {
        mac
    } else {
        other
    }
}

/// Static ⌘K entries shared by toggle and open paths. Lists commands
/// (New Chat / New Terminal) plus one row per saved server.
///
/// Switching between open workspaces lives behind ⌘T — see
/// [`workspace_palette_entries`]. This palette is for creating and routing, not
/// for jumping between work that is already open.
fn command_palette_entries(
    entity: WeakEntity<ConsoleDesktopApp>,
    environments: &[super::environments::Environment],
    sessions: &[SessionHeader],
) -> Vec<PaletteEntry> {
    let mut entries = vec![
        PaletteEntry::new("new-chat", "New Chat", {
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_floating_composer(window, cx));
                }
            }
        })
        .icon(IconName::ChatRoundLine)
        .shortcut(kbd("⌘N", "Ctrl+N")),
        PaletteEntry::new("new-terminal", "New Terminal", {
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_terminal_tab(window, cx));
                }
            }
        })
        .icon(IconName::Terminal)
        .shortcut(kbd("⇧⌘T", "Ctrl+Shift+T")),
        PaletteEntry::new("add-project", "Add Project", {
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_project_browse(window, cx));
                }
            }
        })
        .icon(IconName::FolderNew)
        .shortcut(kbd("⌘O", "Ctrl+O")),
        PaletteEntry::new("quick-open", "Go to File", {
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_quick_open(window, cx));
                }
            }
        })
        .icon(IconName::File)
        .shortcut(kbd("⌘P", "Ctrl+P")),
        PaletteEntry::new("toggle-left-sidebar", "Toggle Sidebar", {
            let entity = entity.clone();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.toggle_left_sidebar(cx));
                }
            }
        })
        .icon(IconName::SidebarMinimalistic)
        .shortcut(kbd("⌘B", "Ctrl+B")),
        PaletteEntry::new("open-files", "Open Files", {
            let entity = entity.clone();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_file_tree_tab(cx));
                }
            }
        })
        .icon(IconName::FolderOpen),
        PaletteEntry::new("open-changes", "Open Changes", {
            let entity = entity.clone();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_changes_for_active_pane(cx));
                }
            }
        })
        .icon(IconName::FileDiff),
        PaletteEntry::new("open-settings", "Open Settings", {
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_settings(window, cx));
                }
            }
        })
        .icon(IconName::Settings)
        .shortcut(kbd("⌘,", "Ctrl+,")),
    ];
    #[cfg(target_os = "macos")]
    entries.push(
        PaletteEntry::new("open-devices", "Open Devices", {
            let entity = entity.clone();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_device_tab(cx));
                }
            }
        })
        .icon(IconName::Smartphone),
    );
    // Browser tabs are deferred on Windows.
    #[cfg(not(target_os = "windows"))]
    entries.push(
        PaletteEntry::new("new-browser", "New Browser Tab", {
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_browser_tab(window, cx));
                }
            }
        })
        .icon(IconName::Globe)
        .shortcut(kbd("⇧⌘B", "Ctrl+Shift+B")),
    );

    for entry in &mut entries {
        entry.section = Some("Commands".into());
    }

    for environment in environments {
        let environment_id = environment.id.clone();
        let entity = entity.clone();
        let label = format!("Switch Server: {}", environment.name);
        entries.push(
            PaletteEntry::new(
                format!("environment-{}", environment_id),
                label,
                move |_window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.activate_environment(environment_id.clone(), cx)
                        });
                    }
                },
            )
            .icon(IconName::Server)
            .section("Servers"),
        );
    }

    // Expose every settings page under the dedicated "Settings" section.
    let settings_tabs = [
        (
            console_ui::settings::SettingsTab::Servers,
            IconName::Server,
            "Servers",
            &["servers", "environments", "backend"][..],
        ),
        (
            console_ui::settings::SettingsTab::Providers,
            IconName::CircleUser,
            "Providers",
            &["api keys", "credentials", "accounts", "login"][..],
        ),
        (
            console_ui::settings::SettingsTab::Usage,
            IconName::ChartColumn,
            "Usage",
            &["billing", "token costs", "quota", "limits"][..],
        ),
        (
            console_ui::settings::SettingsTab::Models,
            IconName::Brain,
            "Models",
            &["llm", "default model", "reasoning"][..],
        ),
        (
            console_ui::settings::SettingsTab::Integrations,
            IconName::Package,
            "Integrations",
            &["github", "gitlab", "atlassian", "jira"][..],
        ),
        (
            console_ui::settings::SettingsTab::Mcp,
            IconName::Cpu,
            "MCP Servers",
            &["tools", "context servers", "plugins"][..],
        ),
        (
            console_ui::settings::SettingsTab::Projects,
            IconName::Folder,
            "Projects",
            &["directories", "roots", "workspaces"][..],
        ),
        (
            console_ui::settings::SettingsTab::DeletedChats,
            IconName::TrashBinMinimalistic,
            "Deleted Chats",
            &["trash", "restore", "archived"][..],
        ),
        (
            console_ui::settings::SettingsTab::Browser,
            IconName::Globe,
            "Browser",
            &["browser use", "web", "chrome"][..],
        ),
        (
            console_ui::settings::SettingsTab::Keybindings,
            IconName::Keyboard,
            "Shortcuts",
            &["keybindings", "hotkeys"][..],
        ),
        (
            console_ui::settings::SettingsTab::Updates,
            IconName::Download,
            "Updates",
            &["version", "check for updates"][..],
        ),
    ];

    for (tab, icon, label, keywords) in settings_tabs {
        let entity = entity.clone();
        entries.push(
            PaletteEntry::new(
                format!("settings-{}", label.to_lowercase().replace(' ', "-")),
                format!("Go to {label}"),
                move |window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.open_settings_tab(tab, window, cx));
                    }
                },
            )
            .icon(icon)
            .keywords(keywords.iter().copied())
            .section("Settings"),
        );
    }

    // Every chat across every project, most recently modified first.
    let mut recent: Vec<&SessionHeader> = sessions.iter().collect();
    recent.sort_by_key(|session| std::cmp::Reverse(session.updated_at));
    for session in recent {
        let session_id = session.id.clone();
        let entity = entity.clone();
        let mut entry = PaletteEntry::new(
            format!("chat-{session_id}"),
            session.display_title().to_string(),
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.select_and_open_session(session_id.clone(), cx)
                    });
                }
            },
        )
        .section("Chats")
        .detail(console_ui::utils::format_time_ago(session.updated_at));
        entry = if session.provider.is_empty() {
            entry.icon(IconName::ChatRoundLine)
        } else {
            entry.provider_icon(session.provider.clone())
        };
        entries.push(entry);
    }

    entries
}

/// ⌘T entries: one row per open workspace, named by the chat it was made for.
/// The project (and the branch, for a worktree) sits on the right, where a tab
/// list would show "current" or a time. Selecting a row switches to that
/// workspace, restoring its panes and tabs as they were left.
fn workspace_palette_entries(
    entity: WeakEntity<ConsoleDesktopApp>,
    rows: Vec<DeskRow>,
) -> Vec<PaletteEntry> {
    rows.into_iter()
        .map(|row| {
            let session_id = row.session_id.clone();
            let entity = entity.clone();
            let detail = match &row.branch {
                Some(branch) => format!("{} · {branch}", row.project_name),
                None => row.project_name.clone(),
            };
            let mut keywords = vec![row.project_name.clone()];
            keywords.extend(row.branch.clone());
            let entry = PaletteEntry::new(
                format!("workspace-{}", row.session_id),
                row.title.clone(),
                move |_window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.select_and_open_session(session_id.clone(), cx);
                        });
                    }
                },
            )
            .section("Workspaces")
            .detail(detail)
            .keywords(keywords);
            if row.provider.is_empty() {
                entry.icon(IconName::ChatRoundLine)
            } else {
                entry.provider_icon(row.provider)
            }
        })
        .collect()
}

impl ConsoleDesktopApp {
    /// ⌘W — close the active tab, then point the pane at whatever tab became
    /// active (mirroring the tab bar's close button). A dirty file buffer
    /// prompts save / discard / cancel first.
    pub fn close_tab(&mut self, cx: &mut Context<Self>) {
        let Some(pane_id) = self.active_pane_id.clone() else {
            return;
        };
        let Some(tab_id) = self.active_tab_id() else {
            return;
        };
        self.request_close_tab(&pane_id, &tab_id, cx);
    }

    /// ⌘K — toggle the command palette. Static command entries are rebuilt here
    /// (with the current entity/client) so the palette always opens with fresh
    /// closures; async modes replace them via their own wrappers.
    pub fn toggle_command_palette(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let entity = cx.entity().downgrade();
        let environments = self.environments.clone();
        let sessions = self.sessions.clone();
        self.palette_search_task = None;
        self.command_palette.update(cx, |palette, cx| {
            palette.set_placeholder("Search commands, settings, servers and chats…", cx);
            palette.set_section_placeholders(
                vec![
                    ("Commands".into(), "Search commands…".into()),
                    ("Settings".into(), "Search settings…".into()),
                    ("Servers".into(), "Search servers…".into()),
                    ("Chats".into(), "Search chats…".into()),
                ],
                cx,
            );
            let search_entity = entity.clone();
            palette.set_query_handler(
                move |query, _window, cx| {
                    if let Some(app) = search_entity.upgrade() {
                        let query = query.to_string();
                        app.update(cx, |this, cx| this.search_palette_chats(query, cx));
                    }
                },
                cx,
            );
            palette.set_entries(
                command_palette_entries(entity, &environments, &sessions),
                cx,
            );
            palette.toggle(window, cx);
        });
        cx.notify();
    }

    /// Chats the sidebar has not loaded are found on the server, so the ⌘K
    /// palette can open any chat, not only the most recent ones.
    fn search_palette_chats(&mut self, query: String, cx: &mut Context<Self>) {
        let query = query.trim().to_string();
        if query.is_empty() {
            self.palette_search_task = None;
            return;
        }
        let client = self.client.clone();
        self.palette_search_task = Some(cx.spawn(async move |entity, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_millis(150))
                .await;
            let Ok(results) = client.sessions.search_by_title(&query).await else {
                return;
            };
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        if !this.command_palette.read(cx).is_open(cx) {
                            return;
                        }
                        // Opening a chat needs its header in the session list.
                        this.merge_sidebar_search_results(results, cx);
                        let entity = cx.entity().downgrade();
                        let environments = this.environments.clone();
                        let sessions = this.sessions.clone();
                        this.command_palette.update(cx, |palette, cx| {
                            palette.set_entries(
                                command_palette_entries(entity, &environments, &sessions),
                                cx,
                            );
                        });
                    });
                }
            });
        }));
    }

    /// ⌘P — open the quick file search palette scoped to the active pane's
    /// project root. `None` when no project is selected so the palette shows
    /// an empty state instead of falling back to the app's cwd.
    pub fn open_quick_open(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let root = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path);
        self.quick_open_palette
            .update(cx, |palette, cx| palette.open(root, window, cx));
        cx.notify();
    }

    /// ⌘⇧F — open the global content search panel scoped to the active
    /// pane's project root. No-op (does nothing visible) when no project is
    /// selected, since there is no root to search.
    pub fn toggle_global_search(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.global_search_panel.read(cx).is_open() {
            self.global_search_panel
                .update(cx, |panel, cx| panel.hide(cx));
            cx.notify();
            return;
        }
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let root = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path);
        if root.is_none() {
            return;
        }
        self.global_search_panel
            .update(cx, |panel, cx| panel.open(root, window, cx));
        cx.notify();
    }

    /// ⌘O — open the remote directory browser palette to add a project.
    pub fn open_project_browse(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.close_settings(window, cx);
        self.project_browse_palette
            .update(cx, |palette, cx| palette.open(window, cx));
        cx.notify();
    }

    /// Every open workspace as a switcher row, most recently used first. A
    /// workspace is open when it holds at least one tab: the one on screen plus
    /// the ones parked in `project_workspace_roots`.
    fn open_workspace_rows(&self) -> Vec<DeskRow> {
        let mut desks: Vec<(Option<String>, i64)> = Vec::new();
        if !workspace_ops::is_workspace_empty(&self.workspace_root) {
            desks.push((
                self.selected_project_id.clone(),
                desk_last_active_ms(&self.workspace_root),
            ));
        }
        for (key, root) in &self.project_workspace_roots {
            if *key != self.selected_project_id && !workspace_ops::is_workspace_empty(root) {
                desks.push((key.clone(), desk_last_active_ms(root)));
            }
        }
        desk_rows(
            &desks,
            &self.sessions,
            |session| self.workspace_key_for_header(session),
            |session| {
                self.projects
                    .iter()
                    .find(|project| project.matches_session(session))
                    .map(|project| project.name.clone())
                    .unwrap_or_else(|| "No Project".to_string())
            },
        )
    }

    /// ⌘T — toggle the workspace switcher.
    pub fn toggle_tab_palette(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let entity = cx.entity().downgrade();
        let rows = self.open_workspace_rows();
        self.tab_palette.update(cx, |palette, cx| {
            palette.set_placeholder("Switch workspace…", cx);
            palette.set_section_placeholders(
                vec![("Workspaces".into(), "Search workspaces…".into())],
                cx,
            );
            palette.set_entries(workspace_palette_entries(entity, rows), cx);
            palette.toggle(window, cx);
        });
        cx.notify();
    }

    /// ⌘L — move keyboard focus to the active pane's composer input.
    pub fn focus_composer(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        // Don't fight any palette for focus while it is open.
        if self.command_palette.read(cx).is_open(cx)
            || self.tab_palette.read(cx).is_open(cx)
            || self.quick_open_palette.read(cx).is_open(cx)
            || self.project_browse_palette.read(cx).is_open(cx)
            || self.floating_composer.open
            || self.global_search_panel.read(cx).is_open()
        {
            return;
        }
        let composer = self.active_composer_input();
        composer.update(cx, |input, cx| {
            window.focus(&input.focus_handle(cx), cx);
        });
    }

    /// ⌘/ — toggle the active pane's model picker. Open reuses the picker's
    /// own toggle observer (clear + autofocus); close returns focus to that
    /// pane's composer so the keyboard flow feels complete.
    pub fn toggle_model_picker(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        // Don't fight any palette for focus while it is open.
        if self.command_palette.read(cx).is_open(cx)
            || self.tab_palette.read(cx).is_open(cx)
            || self.quick_open_palette.read(cx).is_open(cx)
            || self.project_browse_palette.read(cx).is_open(cx)
        {
            return;
        }
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let handle = self.pane_model_menu(&pane_id);
        if handle.is_open() {
            handle.close(window, cx);
            let composer = self.composer_for_pane(&pane_id);
            composer.update(cx, |input, cx| {
                window.focus(&input.focus_handle(cx), cx);
            });
            return;
        }
        // As if the trigger were clicked: anchors to the chip's last
        // recorded bounds and no-ops until the trigger has drawn once.
        toggle_popover(&handle, MenuAlign::AboveLeft, window, cx);
    }

    /// ⌘. — toggle the active pane's thinking mode selector.
    pub fn toggle_thinking_picker(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        // Don't fight any palette for focus while it is open.
        if self.command_palette.read(cx).is_open(cx)
            || self.tab_palette.read(cx).is_open(cx)
            || self.quick_open_palette.read(cx).is_open(cx)
            || self.project_browse_palette.read(cx).is_open(cx)
        {
            return;
        }
        if self.floating_composer.open {
            if self.floating_composer.supported_thinking_levels.is_empty() {
                return;
            }
            let handle = self.floating_composer.thinking_menu.clone();
            if handle.is_open() {
                handle.close(window, cx);
                let input = self.floating_composer.input.clone();
                input.update(cx, |input, cx| {
                    window.focus(&input.focus_handle(cx), cx);
                });
                return;
            }
            toggle_dropdown(&handle, MenuAlign::AboveLeft, window, cx);
            return;
        }
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        if self.supported_thinking_levels_for_pane(&pane_id).is_empty() {
            return;
        }
        let handle = self.pane_thinking_menu(&pane_id);
        if handle.is_open() {
            handle.close(window, cx);
            let composer = self.composer_for_pane(&pane_id);
            composer.update(cx, |input, cx| {
                window.focus(&input.focus_handle(cx), cx);
            });
            return;
        }
        toggle_dropdown(&handle, MenuAlign::AboveLeft, window, cx);
    }

    /// `/` while a menu is open — move focus into the active pane's picker
    /// search field. Never opens the picker: when the search field itself
    /// already holds focus the keystroke instead inserts a literal `/`.
    pub fn focus_model_search(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        if !self.pane_model_menu(&pane_id).is_open() {
            return;
        }
        let search = self.pane_model_search(&pane_id);
        let focus = search.read(cx).focus();
        if focus.is_focused(window) {
            // The binding consumed the keystroke, so the field would never
            // see it as text: insert the `/` the user typed by hand.
            search.update(cx, |input, cx| {
                let at = input.cursor();
                input.replace_range(at..at, "/", cx);
            });
            return;
        }
        window.focus(&focus, cx);
    }
}
