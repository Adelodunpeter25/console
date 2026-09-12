//! Methods behind the global shortcuts bound in `crate::keybindings`.
//!
//! They are registered as *global* action handlers (`App::on_action` in
//! `keybindings::init_handlers`), so they run regardless of keyboard focus.

use std::rc::Rc;

use console_core::{CreateSessionDto, WorkspaceNode, WorkspaceTabConfig};
use console_ui::{IconName, MenuAlign, PaletteEntry, toggle_popover};
use gpui::{Context, Focusable as _, WeakEntity, Window};

use super::ConsoleDesktopApp;

/// Static ⌘K entries shared by toggle and open paths. Lists commands
/// (New Chat / New Terminal) plus one row per saved server.
///
/// Chat and terminal *tab* switching lives behind ⌘⇧P — see
/// [`tab_palette_entries`]. This palette is for creating and routing, not
/// for jumping between tabs that are already open.
fn command_palette_entries(
    entity: WeakEntity<ConsoleDesktopApp>,
    environments: &[super::environments::Environment],
) -> Vec<PaletteEntry> {
    let mut entries = vec![
        PaletteEntry::new("new-chat", "New Chat", {
            let entity = entity.clone();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.create_new_chat(cx));
                }
            }
        })
        .icon(IconName::ChatRoundLine),
        PaletteEntry::new("new-terminal", "New Terminal", {
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_terminal_tab(window, cx));
                }
            }
        })
        .icon(IconName::Terminal),
    ];

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
            .icon(IconName::Server),
        );
    }

    entries
}

/// ⌘⇧P entries: open tabs in two visual groups — terminals first (always
/// fewer, top of the list), then chat tabs sorted by recently-updated. Each
/// entry routes to that tab via [`ConsoleDesktopApp::activate_workspace_tab`].
fn tab_palette_entries(
    entity: WeakEntity<ConsoleDesktopApp>,
    workspace_root: &WorkspaceNode,
) -> Vec<PaletteEntry> {
    // Collect every open tab in layout order, tagged with its pane id and the
    // pane's index for the visible label suffix.
    let leaves: Vec<&console_core::LeafPaneNode> = workspace_root.leaves();
    let pane_count = leaves.len();
    let mut chat_entries: Vec<(i64, PaletteEntry)> = Vec::new();
    let mut terminal_entries: Vec<PaletteEntry> = Vec::new();

    for (pane_index, leaf) in leaves.iter().enumerate() {
        for tab in &leaf.tabs {
            match tab {
                WorkspaceTabConfig::Terminal { .. } => {
                    terminal_entries.push(tab_palette_entry(
                        entity.clone(),
                        &leaf.id,
                        pane_index,
                        pane_count,
                        tab,
                    ));
                }
                WorkspaceTabConfig::Chat { .. } => {
                    let recency = tab.last_active_at_ms().unwrap_or(i64::MIN);
                    chat_entries.push((
                        recency,
                        tab_palette_entry(
                            entity.clone(),
                            &leaf.id,
                            pane_index,
                            pane_count,
                            tab,
                        ),
                    ));
                }
                // File and Diff tabs are out of scope for the ⌘⇧P palette;
                // they're reachable via the tab bar and ⌘P file search.
                WorkspaceTabConfig::File { .. } | WorkspaceTabConfig::Diff { .. } => {}
            }
        }
    }

    // Chat sorted by recency: most-recently updated first. Tabs without a
    // timestamp share the bottom of the list in insertion order.
    chat_entries.sort_by(|(a, _), (b, _)| b.cmp(a));
    let mut entries = terminal_entries;
    entries.extend(chat_entries.into_iter().map(|(_, entry)| entry));
    entries
}

fn tab_palette_entry(
    entity: WeakEntity<ConsoleDesktopApp>,
    pane_id: &str,
    pane_index: usize,
    pane_count: usize,
    tab: &WorkspaceTabConfig,
) -> PaletteEntry {
    let tab_id = tab.id();
    let title = tab.title().to_string();
    let pane_suffix = if pane_count > 1 {
        format!(" · P{}", pane_index + 1)
    } else {
        String::new()
    };
    let label = format!("{}{}", title, pane_suffix);
    let icon = match tab {
        WorkspaceTabConfig::Chat { .. } => IconName::ChatRoundLine,
        _ => IconName::Terminal,
    };
    let pane_id_for_handler = pane_id.to_string();
    let tab_id_for_handler = tab_id.clone();
    PaletteEntry::new(format!("tab-{}", tab_id), label, move |_window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.activate_workspace_tab(&pane_id_for_handler, &tab_id_for_handler, cx);
            });
        }
    })
    .icon(icon)
}

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
        let session_project_id = self
            .pane_project_id(&pane_id)
            .or_else(|| self.selected_project_id.clone());
        let session_cwd = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path.clone())
            .or_else(|| {
                session_project_id.as_ref().and_then(|pid| {
                    self.projects
                        .iter()
                        .find(|p| &p.id == pid)
                        .map(|p| p.path.clone())
                })
            });
        cx.spawn(async move |entity, cx| {
            match client
                .sessions
                .create(CreateSessionDto {
                    cwd: session_cwd,
                    project_id: session_project_id,
                    model_id: selected_model.as_ref().map(|model| model.model_id.clone()),
                    provider: selected_model.as_ref().map(|model| model.provider.clone()),
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
                                let new_chat_draft = this.get_draft_with_mentions(None);
                                this.composer_for_pane(&pane_id).update(cx, |input, cx| {
                                    input.set_prompt_history(Vec::new(), cx);
                                    if let Some((draft_text, mentions)) = &new_chat_draft {
                                        input.set_content_with_mentions(
                                            draft_text.clone(),
                                            mentions.clone(),
                                            cx,
                                        );
                                    } else {
                                        input.clear(cx);
                                    }
                                });
                                if let Some((draft_text, mentions)) = new_chat_draft {
                                    this.save_draft_for_session(
                                        Some(&new_session.id),
                                        &draft_text,
                                        &mentions,
                                        cx,
                                    );
                                    this.clear_draft_for_session(None, cx);
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
        self.close_tab_and_sync_pane(&pane_id, &tab_id, cx);
    }

    /// ⌘K — toggle the command palette. Static command entries are rebuilt here
    /// (with the current entity/client) so the palette always opens with fresh
    /// closures; async modes replace them via their own wrappers.
    pub fn toggle_command_palette(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let entity = cx.entity().downgrade();
        let environments = self.environments.clone();
        self.command_palette.update(cx, |palette, cx| {
            palette.set_entries(
                command_palette_entries(entity, &environments),
                cx,
            );
            palette.toggle(window, cx);
        });
        cx.notify();
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
            .map(|project| project.path.clone());
        self.quick_open_palette
            .update(cx, |palette, cx| palette.open(root, window, cx));
        cx.notify();
    }

    /// ⌘O — open the remote directory browser palette to add a project.
    pub fn open_project_browse(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.close_settings(cx);
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
        let environments = self.environments.clone();
        self.command_palette.update(cx, |palette, cx| {
            palette.set_entries(
                command_palette_entries(entity, &environments),
                cx,
            );
            palette.show(window, cx);
        });
        cx.notify();
    }

    /// ⌘⇧P — toggle the open-tab palette. Terminal tabs appear first (always
    /// the smaller group), then chat tabs sorted by recency.
    pub fn toggle_tab_palette(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let entity = cx.entity().downgrade();
        let workspace_root = self.workspace_root.clone();
        self.tab_palette.update(cx, |palette, cx| {
            palette.set_entries(tab_palette_entries(entity, &workspace_root), cx);
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
