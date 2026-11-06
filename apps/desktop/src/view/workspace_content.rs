use console_core::types::ChangesScope;
use console_core::{
    ApprovalMode, ApproveToolPermissionDto, SelectedModel, SessionHeaderExt, UpdateSessionDto,
};
use console_ui::workspace::EmptyChatState;
use console_ui::{
    ApprovalModeDropdown, ComposerView, ModelDropdownMenu, PermissionInteractionCard, PickerTab,
    QuestionInteractionCard, ShellEntry, ShellsPill, SubagentsPill, TasksPill, Theme,
    WorkspaceFooter, centered_stripe, composer_pills, error_banner, notice_banner,
    queued_prompts_stack,
};
use gpui::{
    App, Context, IntoElement, ParentElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::state::{ConsoleDesktopApp, ImageFileState};

impl ConsoleDesktopApp {
    pub(crate) fn render_workspace_content(
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

        // Settings tab: render the embedded settings view for the active pane.
        if let Some(console_core::WorkspaceTabConfig::Settings {
            active_tab: subtab, ..
        }) = active_tab
        {
            let tab = subtab
                .as_deref()
                .and_then(console_ui::settings::SettingsTab::from_id)
                .unwrap_or(console_ui::settings::SettingsTab::Providers);
            let view = self.get_or_create_settings_view(tab, window, cx);
            return div().size_full().child(view).into_any_element();
        }

        // Terminal tab: render the live terminal surface for its id.
        // No cwd fallback: rehydrated tabs without a selected project must
        // not respawn with the process cwd or "." (it may not exist on the
        // current server). Surface a placeholder instead.
        if let Some(console_core::WorkspaceTabConfig::Terminal { terminal_id, .. }) = active_tab {
            if !self.terminals.contains_key(terminal_id) {
                let Some(cwd) = self
                    .selected_project_for_pane(pane_id)
                    .map(|project| project.path.clone())
                else {
                    let theme = Theme::current(cx);
                    return div()
                        .size_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .text_color(theme.text_ghost)
                        .text_size(px(12.0))
                        .child("Select a project to open a terminal")
                        .into_any_element();
                };
                self.get_or_create_terminal_view(terminal_id, cwd, window, cx);
            }
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

        // Browser tab: render the native browser surface for its id.
        // Rehydrated tabs (persisted across launches) have no live view
        // yet: recreate it and re-navigate to the stored url.
        if let Some(console_core::WorkspaceTabConfig::Browser {
            browser_id, url, ..
        }) = active_tab
        {
            let view = self.get_or_create_browser_view(
                browser_id,
                if url.is_empty() {
                    None
                } else {
                    Some(url.clone())
                },
                window,
                cx,
            );
            let occluded = self.any_palette_open(cx) || view.read(cx).overlay_open(cx);
            view.update(cx, |v, cx| {
                v.sync_native_state(true, occluded, cx);
            });
            return div().size_full().child(view).into_any_element();
        }

        // File tab: render full-page MarkdownViewer, FileViewer, ImagePreview, or BlockedFilePanel
        if let Some(console_core::WorkspaceTabConfig::File { path, .. }) = active_tab {
            let theme = Theme::current(cx);
            let kind = console_core::file_kind_for_path(path);

            // A text/markdown file that could not be read shows why, with a
            // retry, instead of the loading placeholder it would otherwise sit
            // on forever.
            if matches!(
                kind,
                console_core::FileKind::Markdown | console_core::FileKind::Text
            ) && !self.open_file_contents.contains_key(path)
            {
                if let Some(message) = self.open_file_errors.get(path).cloned() {
                    let retry_path = path.clone();
                    let entity = cx.entity().downgrade();
                    return console_ui::BlockedFilePanel::new("Couldn't open file", message)
                        .icon(console_ui::IconName::TriangleAlert)
                        .on_retry(move |_w, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.open_file_errors.remove(&retry_path);
                                    this.fetch_file_tab_content(retry_path.clone(), cx);
                                    cx.notify();
                                });
                            }
                        })
                        .into_any_element();
                }
            }

            match kind {
                console_core::FileKind::RasterImage => {
                    if !self.open_image_contents.contains_key(path) {
                        self.fetch_file_tab_content(path.clone(), cx);
                    }
                    let state = self
                        .open_image_contents
                        .get(path)
                        .cloned()
                        .unwrap_or(ImageFileState::Loading);
                    match state {
                        ImageFileState::Loading => {
                            return div()
                                .size_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .gap(px(8.0))
                                .text_color(theme.text_tertiary)
                                .text_size(px(13.0))
                                .child(console_ui::app_icon(
                                    console_ui::IconName::LoaderCircle,
                                    16.0,
                                    theme.text_tertiary,
                                ))
                                .child("Loading image...")
                                .into_any_element();
                        }
                        ImageFileState::Loaded {
                            image,
                            w,
                            h,
                            size_bytes,
                            mime,
                        } => {
                            let meta = console_ui::ImageMeta {
                                width: w,
                                height: h,
                                size_bytes: Some(size_bytes),
                                mime_type: Some(mime),
                            };
                            let zoom_img = image.clone();
                            let entity = cx.entity().downgrade();
                            return console_ui::ImagePreview::new(path.clone(), image)
                                .meta(meta)
                                .on_zoom(move |_w, cx| {
                                    let img = zoom_img.clone();
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.zoomed_image = Some(img);
                                            cx.notify();
                                        });
                                    }
                                })
                                .into_any_element();
                        }
                        ImageFileState::Failed { message } => {
                            let retry_path = path.clone();
                            let entity = cx.entity().downgrade();
                            return console_ui::BlockedFilePanel::new(
                                "Failed to load image",
                                message,
                            )
                            .icon(console_ui::IconName::TriangleAlert)
                            .on_retry(move |_w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.fetch_file_tab_content(retry_path.clone(), cx);
                                    });
                                }
                            })
                            .into_any_element();
                        }
                        ImageFileState::Blocked { title, message } => {
                            return console_ui::BlockedFilePanel::new(title, message)
                                .icon(console_ui::IconName::File)
                                .into_any_element();
                        }
                    }
                }
                console_core::FileKind::Blocked => {
                    if !self.open_image_contents.contains_key(path) {
                        self.fetch_file_tab_content(path.clone(), cx);
                    }
                    let state = self
                        .open_image_contents
                        .get(path)
                        .cloned()
                        .unwrap_or(ImageFileState::Loading);
                    match state {
                        ImageFileState::Loading => {
                            return div()
                                .size_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .gap(px(8.0))
                                .text_color(theme.text_tertiary)
                                .text_size(px(13.0))
                                .child(console_ui::app_icon(
                                    console_ui::IconName::LoaderCircle,
                                    16.0,
                                    theme.text_tertiary,
                                ))
                                .child("Checking file...")
                                .into_any_element();
                        }
                        ImageFileState::Blocked { title, message } => {
                            return console_ui::BlockedFilePanel::new(title, message)
                                .icon(console_ui::IconName::File)
                                .into_any_element();
                        }
                        ImageFileState::Failed { message } => {
                            let retry_path = path.clone();
                            let entity = cx.entity().downgrade();
                            return console_ui::BlockedFilePanel::new(
                                "Preview unavailable",
                                message,
                            )
                            .icon(console_ui::IconName::TriangleAlert)
                            .on_retry(move |_w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.fetch_file_tab_content(retry_path.clone(), cx);
                                    });
                                }
                            })
                            .into_any_element();
                        }
                        _ => {
                            return console_ui::BlockedFilePanel::new(
                                "Binary file",
                                format!(
                                    "\"{}\" isn't a text file, so there's nothing to preview here.",
                                    path
                                ),
                            )
                            .icon(console_ui::IconName::File)
                            .into_any_element();
                        }
                    }
                }
                console_core::FileKind::Svg => {
                    let mode = self
                        .svg_preview_mode
                        .get(path)
                        .copied()
                        .unwrap_or(console_ui::SvgViewMode::Preview);
                    match mode {
                        console_ui::SvgViewMode::Preview => {
                            if !self.open_image_contents.contains_key(path) {
                                self.fetch_file_tab_content(path.clone(), cx);
                            }
                            let state = self
                                .open_image_contents
                                .get(path)
                                .cloned()
                                .unwrap_or(ImageFileState::Loading);
                            match state {
                                ImageFileState::Loading => {
                                    return div()
                                        .size_full()
                                        .flex()
                                        .items_center()
                                        .justify_center()
                                        .gap(px(8.0))
                                        .text_color(theme.text_tertiary)
                                        .text_size(px(13.0))
                                        .child(console_ui::app_icon(
                                            console_ui::IconName::LoaderCircle,
                                            16.0,
                                            theme.text_tertiary,
                                        ))
                                        .child("Loading SVG preview...")
                                        .into_any_element();
                                }
                                ImageFileState::Loaded {
                                    image,
                                    w,
                                    h,
                                    size_bytes,
                                    mime,
                                } => {
                                    let meta = console_ui::ImageMeta {
                                        width: w,
                                        height: h,
                                        size_bytes: Some(size_bytes),
                                        mime_type: Some(mime),
                                    };
                                    let zoom_img = image.clone();
                                    let toggle_path = path.clone();
                                    let entity = cx.entity().downgrade();
                                    return console_ui::ImagePreview::new(path.clone(), image)
                                        .meta(meta)
                                        .is_svg(true)
                                        .svg_mode(console_ui::SvgViewMode::Preview)
                                        .on_toggle_svg_mode(move |new_mode, _w, cx| {
                                            if let Some(app) = entity.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    this.svg_preview_mode
                                                        .insert(toggle_path.clone(), new_mode);
                                                    cx.notify();
                                                });
                                            }
                                        })
                                        .on_zoom({
                                            let entity = cx.entity().downgrade();
                                            move |_w, cx| {
                                                let img = zoom_img.clone();
                                                if let Some(app) = entity.upgrade() {
                                                    app.update(cx, |this, cx| {
                                                        this.zoomed_image = Some(img);
                                                        cx.notify();
                                                    });
                                                }
                                            }
                                        })
                                        .into_any_element();
                                }
                                ImageFileState::Failed { message } => {
                                    let retry_path = path.clone();
                                    let entity = cx.entity().downgrade();
                                    return console_ui::BlockedFilePanel::new(
                                        "Failed to render SVG",
                                        message,
                                    )
                                    .icon(console_ui::IconName::TriangleAlert)
                                    .on_retry(move |_w, cx| {
                                        if let Some(app) = entity.upgrade() {
                                            app.update(cx, |this, cx| {
                                                this.fetch_file_tab_content(retry_path.clone(), cx);
                                            });
                                        }
                                    })
                                    .into_any_element();
                                }
                                ImageFileState::Blocked { title, message } => {
                                    return console_ui::BlockedFilePanel::new(title, message)
                                        .icon(console_ui::IconName::File)
                                        .into_any_element();
                                }
                            }
                        }
                        console_ui::SvgViewMode::Source => {
                            if !self.open_file_contents.contains_key(path) {
                                self.fetch_file_tab_content(path.clone(), cx);
                            }
                            let content = self
                                .open_file_contents
                                .get(path)
                                .cloned()
                                .or_else(|| std::fs::read_to_string(path).ok())
                                .unwrap_or_else(|| "Loading file content...".to_string());

                            let editor_view =
                                self.get_or_build_editor_view(path, &content, &theme, cx);
                            let file_viewer = console_ui::FileViewer::new(editor_view);

                            let toggle_path = path.clone();
                            let entity = cx.entity().downgrade();
                            let header = console_ui::svg_source_header(
                                theme,
                                Some(Rc::new(move |new_mode, _w, cx| {
                                    if let Some(app) = entity.upgrade() {
                                        app.update(cx, |this, cx| {
                                            this.svg_preview_mode
                                                .insert(toggle_path.clone(), new_mode);
                                            cx.notify();
                                        });
                                    }
                                })),
                            );

                            return div()
                                .size_full()
                                .flex()
                                .flex_col()
                                .child(header)
                                .child(div().flex_1().min_h_0().size_full().child(file_viewer))
                                .into_any_element();
                        }
                    }
                }
                console_core::FileKind::Markdown => {
                    if !self.open_file_contents.contains_key(path) {
                        self.fetch_file_tab_content(path.clone(), cx);
                    }

                    let content = self
                        .open_file_contents
                        .get(path)
                        .cloned()
                        .or_else(|| std::fs::read_to_string(path).ok())
                        .unwrap_or_else(|| "Loading file content...".to_string());

                    let view = self.get_or_build_markdown_view(path, &content);
                    let selection = self.viewer_markdown_selection(path);
                    let block_count = view.borrow().block_count();
                    let list_state =
                        self.viewer_list_state(&format!("md:{}", path), block_count, 60.0);
                    let scrollbar_state = self.viewer_scrollbar_state(&format!("md:{}", path));
                    // Clicked links route like the transcript's: file paths open
                    // as workspace tabs, web URLs open in the embedded browser.
                    // Without a handler links fall through to the OS URL opener,
                    // which rejects bare file paths (macOS error -50).
                    let pane = pane_id.to_owned();
                    let entity: gpui::WeakEntity<ConsoleDesktopApp> = cx.entity().downgrade();
                    let link_handler: console_ui::markdown::render::LinkHandler =
                        std::rc::Rc::new(move |url: &str, window, cx| {
                            if let Some(app) = entity.upgrade() {
                                if console_shared::markdown::file_links::is_http_url(url) {
                                    app.update(cx, |this, cx| {
                                        this.open_chat_url_in_browser(url.to_owned(), window, cx);
                                    });
                                } else if console_shared::markdown::file_links::is_file_link(url) {
                                    app.update(cx, |this, cx| {
                                        this.open_file_link(url.to_owned(), &pane.clone(), cx);
                                    });
                                }
                            }
                        });
                    return console_ui::MarkdownViewer::new(path.clone(), view, list_state)
                        .selection(selection)
                        .scrollbar_state(scrollbar_state)
                        .link_handler(link_handler)
                        .into_any_element();
                }
                console_core::FileKind::Text => {
                    if !self.open_file_contents.contains_key(path) {
                        self.fetch_file_tab_content(path.clone(), cx);
                    }

                    let content = self
                        .open_file_contents
                        .get(path)
                        .cloned()
                        .or_else(|| std::fs::read_to_string(path).ok())
                        .unwrap_or_else(|| "Loading file content...".to_string());

                    let editor_view = self.get_or_build_editor_view(path, &content, &theme, cx);
                    let comment_cards = self.file_comment_cards(pane_id, path, window, cx);
                    let viewer = console_ui::FileViewer::new(editor_view).comments(comment_cards);
                    let composer = self.pane_composer_view(pane_id, true, true, window, cx);
                    let collapse = self.composer_collapse_on_scroll(pane_id, cx);
                    return div()
                        .relative()
                        .size_full()
                        .child(viewer)
                        .child(collapse)
                        .child(composer)
                        .into_any_element();
                }
            }
        }

        // Diff tab: render full-page DiffViewer
        if let Some(console_core::WorkspaceTabConfig::Diff { path, .. }) = active_tab {
            let (diff_result, raw_diff) = self
                .open_diff_contents
                .get(path)
                .cloned()
                .unwrap_or_else(|| (console_core::DiffResult::default(), String::new()));
            let theme = Theme::current(cx);
            let diff_view = self.get_or_build_diff_view(path, &diff_result, &raw_diff, &theme, cx);
            let composer = self.pane_composer_view(pane_id, true, true, window, cx);
            let collapse = self.composer_collapse_on_scroll(pane_id, cx);
            return div()
                .relative()
                .size_full()
                .child(console_ui::DiffViewer::new(diff_view))
                .child(collapse)
                .child(composer)
                .into_any_element();
        }

        // Changes review tab: every changed file in the tab's scope, stacked
        // with per-file mark-as-reviewed state.
        if let Some(console_core::WorkspaceTabConfig::ChangesReview {
            session_id, scope, ..
        }) = active_tab
        {
            let tab_id = active_tab.map(|tab| tab.id()).unwrap_or_default();
            let changes = self
                .changes_review_data
                .get(&tab_id)
                .cloned()
                .unwrap_or_else(|| Rc::new(Vec::new()));
            let collapsed = self
                .changes_review_collapsed
                .get(&tab_id)
                .cloned()
                .unwrap_or_default();

            let entity = cx.entity().downgrade();
            let tab_id_for_collapse = tab_id.clone();
            let on_toggle_collapsed: Rc<dyn Fn(String, &mut Window, &mut gpui::App)> = {
                let entity = entity.clone();
                let tab_id = tab_id_for_collapse;
                Rc::new(
                    move |path: String, _window: &mut Window, cx: &mut gpui::App| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.toggle_changes_review_collapsed(&tab_id, &path, cx);
                            });
                        }
                    },
                )
            };
            let tab_id_for_reviewed = tab_id.clone();
            let session_id_for_reviewed = session_id.clone();
            let on_toggle_reviewed: Rc<dyn Fn(String, u32, &mut Window, &mut gpui::App)> = {
                let entity = entity.clone();
                let tab_id = tab_id_for_reviewed;
                let session_id = session_id_for_reviewed;
                Rc::new(
                    move |path: String,
                          turn_index: u32,
                          _window: &mut Window,
                          cx: &mut gpui::App| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.toggle_change_reviewed(
                                    &tab_id,
                                    session_id.clone(),
                                    path,
                                    turn_index,
                                    cx,
                                );
                            });
                        }
                    },
                )
            };

            return console_ui::ReviewTab::new(
                *scope,
                changes,
                collapsed,
                on_toggle_collapsed,
                on_toggle_reviewed,
            )
            .into_any_element();
        }

        let pane_id = pane_id.to_owned();
        self.ensure_workspace_pane_state(&pane_id, window, cx);
        let pane_transcript = self.transcript_for_pane(&pane_id);
        let pane_project_id = self.pane_real_project_id(&pane_id);
        let pane_branches = self.pane_branches(&pane_id);
        let pane_branch_loaded = self.pane_branch_loaded(&pane_id);
        let pane_is_git_repository = self.pane_is_git_repository(&pane_id);
        let pane_branch_pending = self.pane_branch_pending(&pane_id);
        let pane_project_menu = self.pane_project_menu(&pane_id);
        let pane_branch_menu = self.pane_branch_menu(&pane_id);
        let pane_usage_menu = self.pane_usage_menu(&pane_id);
        let pane_session_id = active_tab.and_then(|tab| match tab {
            console_core::WorkspaceTabConfig::Chat { session_id, .. } => Some(session_id.clone()),
            _ => None,
        });

        let theme = Theme::current(cx);
        let entity = cx.entity().downgrade();
        let client = self.client.clone();
        let queued_prompts = self.queued_prompts_for_pane(&pane_id);
        let error_message = self
            .error_for_pane(&pane_id)
            .map(|error| error.message.clone());
        let error_pane_id = pane_id.clone();
        let error_selection = self.error_selection.clone();
        let agent_notice = self.agent_notice_for_pane(&pane_id);
        let todo_items = self.todo_items_for_pane(&pane_id);
        let todo_total = todo_items.len();
        let todo_completed = todo_items
            .iter()
            .filter(|item| {
                matches!(
                    item.status.to_ascii_lowercase().as_str(),
                    "done" | "completed" | "complete"
                )
            })
            .count();
        let has_active_todos = todo_total > 0 && todo_completed < todo_total;
        let subagents = pane_session_id
            .as_deref()
            .and_then(|sid| self.session_subagents.get(sid))
            .cloned()
            .unwrap_or_default();
        let subagent_count = subagents.len();
        let subagents_running = subagents.iter().any(|sub| sub.status == "running");
        let running_shells = pane_session_id
            .as_deref()
            .map(|sid| self.running_shells_for_session(sid))
            .unwrap_or_default();
        let selected_sid = pane_session_id.clone();
        let changes_summary = pane_transcript.read(cx).changes_summary();
        let on_open_changes: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>> =
            pane_session_id.as_ref().map(|session_id| {
                let entity = entity.clone();
                let session_id = session_id.clone();
                let handler: Rc<dyn Fn(&mut Window, &mut App) + 'static> =
                    Rc::new(move |_window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.open_changes_review_tab(
                                    session_id.clone(),
                                    ChangesScope::AllTurns,
                                    cx,
                                );
                            });
                        }
                    });
                handler
            });

        let tasks_menu = self.pane_tasks_menu(&pane_id);
        if !has_active_todos && tasks_menu.is_open() {
            // The pill disappears with the last unfinished task; drop its
            // popover too so it cannot reopen stale on the next run.
            tasks_menu.close(window, cx);
        }
        let subagents_menu = self.pane_subagents_menu(&pane_id);
        if subagent_count == 0 && subagents_menu.is_open() {
            subagents_menu.close(window, cx);
        }
        let shells_menu = self.pane_shells_menu(&pane_id);
        if running_shells.is_empty() && shells_menu.is_open() {
            shells_menu.close(window, cx);
        }
        let tasks = has_active_todos.then(|| TasksPill {
            completed: todo_completed,
            total: todo_total,
            menu: tasks_menu,
            items: Rc::new(todo_items.clone()),
        });
        let subagents_pill = (subagent_count > 0).then(|| SubagentsPill {
            running: subagents_running,
            menu: subagents_menu,
            items: subagents.clone(),
        });
        let shells_pill = (!running_shells.is_empty()).then(|| {
            let pane_session_id = pane_session_id.clone();
            let entity = entity.clone();
            let on_stop: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> =
                Rc::new(move |work_id, _window, cx| {
                    if let (Some(app), Some(sid)) = (entity.upgrade(), pane_session_id.clone()) {
                        app.update(cx, |this, cx| this.stop_shell(sid, work_id, cx));
                    }
                });
            ShellsPill {
                menu: shells_menu,
                shells: Rc::new(
                    running_shells
                        .iter()
                        .map(|work| ShellEntry {
                            id: work.id.clone(),
                            command: work.shell_command(),
                        })
                        .collect(),
                ),
                on_stop,
            }
        });
        let pills_key = format!("composer-pills-{pane_id}");
        let composer_pills_row = composer_pills(
            &pills_key,
            tasks,
            subagents_pill,
            shells_pill,
            changes_summary.zip(on_open_changes),
            theme,
        );

        // Built before the composer's `.when` closure so the closure only
        // captures the finished element, not `&mut self`.
        let pane_composer_element = self.pane_composer_view(&pane_id, false, false, window, cx);

        div()
            .flex_1()
            .min_h_0()
            .w_full()
            .bg(theme.chat_canvas)
            .flex()
            .flex_col()
            .overflow_hidden()
        .child(
            div()
                .relative()
                .flex_1()
                .w_full()
                .overflow_hidden()
                // Clicking an image in a message opens the app's
                // image preview modal with its data URI. The handler is wired
                // once during app initialization, not mutated from this
                // render path.
                .child(pane_transcript)
                // Status pills float over the transcript's bottom edge rather
                // than occupying a band of their own; the transcript keeps a
                // matching bottom inset so the last row can scroll clear.
                .when_some(composer_pills_row, |el, pills| el.child(pills)),
        )
        .when_some(error_message, |el, error| {
            let entity = entity.clone();
            el.child(error_banner(
                error,
                theme,
                error_selection.clone(),
                Some(Rc::new(move |cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.clear_error_for_pane(&error_pane_id, cx));
                    }
                })),
                cx,
            ))
        })
        .when_some(agent_notice, |el, notice| {
            el.child(notice_banner(notice, theme))
        })
        .when(!queued_prompts.is_empty(), |el| {
            let theme = Theme::current(cx);
            let entity_edit = entity.clone();
            let entity_delete = entity.clone();
            let entity_steer = entity.clone();
            let pane_for_edit = pane_id.clone();
            let pane_for_delete = pane_id.clone();
            let pane_for_steer = pane_id.clone();
            el.child(queued_prompts_stack(
                queued_prompts,
                Some(Rc::new(move |prompt_id, _window, cx| {
                    if let Some(app) = entity_edit.upgrade() {
                        let pid = pane_for_edit.clone();
                        app.update(cx, |this, cx| {
                            this.edit_queued_prompt_for_pane(pid, prompt_id, cx);
                        });
                    }
                })),
                Some(Rc::new(move |prompt_id, _window, cx| {
                    if let Some(app) = entity_delete.upgrade() {
                        let pid = pane_for_delete.clone();
                        app.update(cx, |this, cx| {
                            this.delete_queued_prompt_for_pane(pid, prompt_id, cx);
                        });
                    }
                })),
                Some(Rc::new(move |prompt_id, _window, cx| {
                    if let Some(app) = entity_steer.upgrade() {
                        let pid = pane_for_steer.clone();
                        app.update(cx, |this, cx| {
                            this.steer_queued_prompt_for_pane(pid, prompt_id, cx);
                        });
                    }
                })),
                theme,
            ))
        })
        .when_some(self.pending_question_for_pane(&pane_id), |el, question| {
            let entity_for_answer = entity.clone();
            let question_input = self.question_input_for_pane(&pane_id);
            let selected = self.question_selected_for_pane(&pane_id);
            let is_multi = question.is_multi_select.unwrap_or(false);
            let skippable = question.skippable != Some(false);
            let answer_sid = selected_sid.clone();
            let skip_sid = selected_sid.clone();
            let entity_for_skip = entity.clone();
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
            .selected(selected);
            let question_card = if skippable {
                question_card.on_skip(move |_window, cx| {
                    let Some(sid) = skip_sid.clone() else {
                        return;
                    };
                    if let Some(app) = entity_for_skip.upgrade() {
                        app.update(cx, |this, cx| {
                            this.skip_pending_question_for_session(sid, cx);
                        });
                    }
                })
            } else {
                question_card
            };
            let question_card = question_card
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
                                    if let Err(error) = client
                                        .runs
                                        .approve_permission(
                                            &sid,
                                            ApproveToolPermissionDto {
                                                request_id: req_id,
                                                allow,
                                            },
                                        )
                                        .await
                                    {
                                        let message = format!(
                                            "Unable to update tool permission: {error}"
                                        );
                                        cx.update(|cx| {
                                            if let Some(app) = entity_for_error.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    this.set_error_for_session(&sid, message, cx)
                                                });
                                            }
                                        });
                                    }
                                })
                                .detach();
                            });
                        }
                    },
                ),
                8.0,
            ))
        })
        .when(selected_sid.is_some(), |el| {
            el.child({
                let project_pane_id = pane_id.clone();
                let clear_project_pane_id = pane_id.clone();
                let branch_pane_id = pane_id.clone();
                let new_worktree_pane_id = pane_id.clone();
                // Get current provider from active session
                let current_provider = self
                    .active_session_for_pane(&pane_id)
                    .and_then(|sid| {
                        self.sessions.iter().find(|s| s.id == sid).map(|s| s.provider.clone())
                    })
                    .unwrap_or_else(|| "claude".to_string());

                // Get usage report for current provider
                let usage_report = self
                    .usage_reports
                    .as_ref()
                    .and_then(|reports| reports.get(&current_provider).cloned().flatten());

                // Estimated context occupancy for the active session
                let context_snapshot = self
                    .active_session_for_pane(&pane_id)
                    .and_then(|sid| self.context_usage.get(&sid).cloned());

                let workspace_footer = WorkspaceFooter::new(
                    self.projects.clone(),
                    pane_project_id.clone(),
                    pane_branches.clone(),
                    pane_branch_loaded,
                    pane_is_git_repository,
                    pane_branch_pending,
                    pane_project_menu.clone(),
                    pane_branch_menu.clone(),
                    pane_usage_menu.clone(),
                    usage_report,
                    self.usage_loading,
                    context_snapshot,
                    {
                        let entity = entity.clone();
                        move |id: String, _w, cx| {
                            if let Some(app) = entity.upgrade() {
                                let pane_id = project_pane_id.clone();
                                app.update(cx, |this, cx| {
                                    this.select_project_for_pane(pane_id, id, cx)
                                });
                            }
                        }
                    },
                    {
                        let entity = entity.clone();
                        move |window, cx| {
                            if let Some(app) = entity.upgrade() {
                                // Open the ⌘O directory browser palette — it
                                // works against remote backends, where the
                                // native dialog can't see the host filesystem.
                                app.update(cx, |this, cx| this.open_project_browse(window, cx));
                            }
                        }
                    },
                    {
                        let entity = entity.clone();
                        move |_w, cx| {
                            if let Some(app) = entity.upgrade() {
                                let pane_id = clear_project_pane_id.clone();
                                app.update(cx, |this, cx| {
                                    this.clear_project_for_pane(pane_id, cx)
                                });
                            }
                        }
                    },
                    {
                        let entity = entity.clone();
                        move |repo: console_core::GithubRepo, _w, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.open_repo_task(repo, cx)
                                });
                            }
                        }
                    },
                    {
                        let entity = entity.clone();
                        move |_w, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.refresh_github_repos(cx)
                                });
                            }
                        }
                    },
                    {
                        let entity = entity.clone();
                        move |window, cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    // Not open_settings: it toggles, and
                                    // closing settings the user already had
                                    // open would be a surprise from a menu
                                    // row.
                                    this.open_settings_tab(
                                        console_ui::settings::SettingsTab::Providers,
                                        window,
                                        cx,
                                    );
                                });
                            }
                        }
                    },
                    {
                        let entity = entity.clone();
                        move |name: String, _w, cx| {
                            if let Some(app) = entity.upgrade() {
                                let pane_id = branch_pane_id.clone();
                                app.update(cx, |this, cx| {
                                    this.checkout_branch_for_pane(pane_id, name, cx)
                                });
                            }
                        }
                    },
                    {
                        let entity = entity.clone();
                        move |_w, cx| {
                            if let Some(app) = entity.upgrade() {
                                let pane_id = new_worktree_pane_id.clone();
                                app.update(cx, |this, cx| {
                                    this.new_worktree_for_pane(pane_id, cx)
                                });
                            }
                        }
                    },
                )
                .project_locked(self.session_has_messages(&pane_id))
                .github_section(
                    self.github_repos.clone(),
                    self.github_repos_loading,
                    self.github_repos_error.clone(),
                )
                .repo_cloning(self.repo_task_cloning.clone());

                div()
                    .relative()
                    .w_full()
                    .bg(theme.chat_canvas)
                    .flex()
                    .flex_col()
                    .items_center()
                    .child(pane_composer_element)
                    .child(workspace_footer)
            })
        })
        .into_any_element()
    }

    /// The pane's chat composer, used both as the docked footer in chat tabs
    /// and as a floating card over file/diff tabs.
    /// Wheel scrolling over a file/diff collapses the floating composer back
    /// to its resting bar by dropping its focus.
    fn composer_collapse_on_scroll(
        &self,
        pane_id: &str,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        let focus = self.composer_for_pane(pane_id).read(cx).focus();
        div()
            .absolute()
            .top_0()
            .left_0()
            .right_0()
            .bottom(gpui::px(console_ui::FLOATING_COMPOSER_INSET))
            .child(console_ui::scroll_watch(move |window, _cx| {
                if focus.is_focused(window) {
                    window.blur();
                    window.refresh();
                }
            }))
            .into_any_element()
    }

    fn pane_composer_view(
        &mut self,
        pane_id: &str,
        floating: bool,
        reveal_chat: bool,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        let theme = Theme::current(cx);
        let pane_composer = self.composer_for_pane(pane_id);
        let pane_selected_model = self.effective_model_for_pane(pane_id);
        let pane_picker_tab = self.pane_picker_tab(pane_id);
        let pane_approval_mode = self.pane_approval_mode(pane_id);
        let pane_thinking_level = self.pane_thinking_level(pane_id);
        let pane_supported_thinking_levels = self.supported_thinking_levels_for_pane(pane_id);
        let pane_model_menu = self.pane_model_menu(pane_id);
        let pane_approval_menu = self.pane_approval_menu(pane_id);
        let pane_model_search = self.pane_model_search(pane_id);
        let pane_model_search_query = pane_model_search.read(cx).content().to_owned();
        let pane_session_id = self.active_session_for_pane(pane_id);
        let autocomplete = self.composer_autocomplete_for_pane(
            pane_id,
            pane_session_id.as_deref(),
            window,
            cx,
        );
        let comments = self.composer_comments_for_pane(pane_id, cx);
        let annotations = self.composer_annotations_for_pane(pane_id, cx);
        let attachments = self.attachments_for_pane(pane_id);

        let entity = cx.entity().downgrade();
        let client = self.client.clone();
        let providers = self.providers.clone();
        let favorites = self.favorites.clone();
        let models_by_provider = self.models_by_provider.clone();

        let model_dropdown = ModelDropdownMenu::new(
            providers,
            pane_selected_model.clone(),
            pane_picker_tab,
            favorites,
            models_by_provider,
            pane_model_search.clone(),
            pane_model_search_query,
            {
                let entity = entity.clone();
                let model_handle = pane_model_menu.clone();
                let pane_id = pane_id.to_string();
                move |prov: String, m_id: String, window, cx| {
                    model_handle.close(window, cx);
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let model = SelectedModel {
                                provider: prov.clone(),
                                model_id: m_id.clone(),
                            };
                            this.set_pane_model(&pane_id, Some(model));
                            this.update_session_settings_for_pane(
                                pane_id.clone(),
                                UpdateSessionDto {
                                    title: None,
                                    cwd: None,
                                    project_id: None,
                                    model_id: Some(m_id.clone()),
                                    provider: Some(prov.clone()),
                                    approval_mode: None,
                                    thinking_level: None,
                                    pinned: None,
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
                let search = pane_model_search.clone();
                let pane_id = pane_id.to_string();
                move |tab: PickerTab, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let PickerTab::Provider(ref name) = tab {
                                this.load_models_for_provider(name, cx);
                            }
                            this.set_pane_picker_tab(&pane_id, tab);
                            search.update(cx, |input, cx| input.clear(cx));
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
                            this.toggle_model_favorite(prov, mid, cx);
                        });
                    }
                }
            },
        );

        let approval_dropdown = ApprovalModeDropdown::new(pane_approval_mode, {
            let entity = entity.clone();
            let approval_handle = pane_approval_menu.clone();
            let pane_id = pane_id.to_string();
            move |mode: ApprovalMode, window, cx| {
                approval_handle.close(window, cx);
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.set_pane_approval_mode(&pane_id, mode);
                        this.update_session_settings_for_pane(
                            pane_id.clone(),
                            UpdateSessionDto {
                                title: None,
                                cwd: None,
                                project_id: None,
                                model_id: None,
                                provider: None,
                                approval_mode: Some(mode.value().to_string()),
                                thinking_level: None,
                                pinned: None,
                            },
                            cx,
                        );
                        cx.notify();
                    });
                }
            }
        });

        let composer = ComposerView::new(
            pane_composer.clone(),
            {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let prompt = this.composer_for_pane(&pane_id).read(cx).content().to_string();
                            let prompt = this.prompt_with_staged_comments(&pane_id, &prompt, cx);
                            let attachments = (*this.attachments_for_pane(&pane_id)).clone();
                            let annotations = (*this.annotations_for_pane(&pane_id)).clone();
                            let context_files = console_shared::composer::mentions::submitted_mention_paths(
                                this.composer_for_pane(&pane_id).read(cx).mentions(),
                            );
                            if reveal_chat {
                                this.reveal_chat_for_pane(&pane_id, cx);
                            }
                            if this.is_active_session_running_for_pane(&pane_id) {
                                this.queue_prompt_for_pane(
                                    pane_id.clone(),
                                    prompt,
                                    attachments,
                                    annotations,
                                    context_files,
                                    cx,
                                );
                                return;
                            }
                            this.active_pane_id = Some(pane_id.clone());
                            this.selected_session_id = this.active_session_for_pane(&pane_id);
                            this.submit_prompt_with_context(
                                prompt,
                                attachments,
                                annotations,
                                context_files,
                                cx,
                            );
                        });
                    }
                }
            },
            {
                let client = client.clone();
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_w, cx| {
                    let client = client.clone();
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.active_pane_id = Some(pane_id.clone());
                            this.selected_session_id = this.active_session_for_pane(&pane_id);
                            if let Some(sid) = &this.selected_session_id {
                                let sid_clone = sid.clone();
                                this.set_session_running(&sid_clone, None);
                                let entity_for_error = entity.clone();
                                cx.spawn(async move |_entity, cx| {
                                    if let Err(error) = client.runs.abort(&sid_clone).await {
                                        let message = format!(
                                            "Unable to stop the agent run: {error}"
                                        );
                                        cx.update(|cx| {
                                            if let Some(app) = entity_for_error.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    this.set_error_for_session(&sid_clone, message, cx)
                                                });
                                            }
                                        });
                                    }
                                })
                                .detach();
                            }
                            cx.notify();
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.pick_image(&pane_id, cx);
                        });
                    }
                }
            },
        )
        .running(self.is_active_session_running_for_pane(pane_id))
        .has_queued_prompt(self.has_queued_prompt_for_pane(pane_id))
        .on_queue({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let prompt = this.composer_for_pane(&pane_id).read(cx).content().to_string();
                        let prompt = this.prompt_with_staged_comments(&pane_id, &prompt, cx);
                        let attachments = (*this.attachments_for_pane(&pane_id)).clone();
                        let annotations = (*this.annotations_for_pane(&pane_id)).clone();
                        let context_files = console_shared::composer::mentions::submitted_mention_paths(
                            this.composer_for_pane(&pane_id).read(cx).mentions(),
                        );
                        if reveal_chat {
                            this.reveal_chat_for_pane(&pane_id, cx);
                        }
                        this.queue_prompt_for_pane(
                            pane_id.clone(),
                            prompt,
                            attachments,
                            annotations,
                            context_files,
                            cx,
                        );
                    });
                }
            }
        })
        .selected_model(pane_selected_model)
        .approval_mode(pane_approval_mode)
        .thinking_level(pane_thinking_level)
        .supported_thinking_levels(pane_supported_thinking_levels)
        .on_cycle_thinking({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.cycle_thinking_level_for_pane(&pane_id, cx);
                    });
                }
            }
        })
        .model_dropdown(model_dropdown, pane_model_menu)
        .approval_dropdown(approval_dropdown, pane_approval_menu)
        .autocomplete(autocomplete)
        .on_autocomplete_next({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
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
            let pane_id = pane_id.to_string();
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
            let pane_id = pane_id.to_string();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.accept_highlighted_autocomplete_for_pane(&pane_id, cx);
                    });
                }
            }
        })
        .on_autocomplete_dismiss({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.dismiss_autocomplete_for_pane(&pane_id, cx);
                    });
                }
            }
        })
        .attachments(attachments)
        .comments(comments)
        .annotations(annotations)
        .floating(floating)
        .on_remove_attachment({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |index, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.remove_attachment(&pane_id, index, cx);
                    });
                }
            }
        })
        .on_remove_annotation({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |index, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.remove_annotation(&pane_id, index, cx);
                    });
                }
            }
        })
        .on_preview_attachment({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |index, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.preview_attachment(&pane_id, index, cx);
                    });
                }
            }
        })
        .on_drop_files({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |paths, window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.stage_dropped_files(&pane_id, paths, window, cx);
                    });
                }
            }
        });

        if floating {
            div()
                .absolute()
                .bottom_0()
                .left_0()
                .right_0()
                .flex()
                .justify_center()
                .child(
                    div()
                        .w_full()
                        .max_w(px(768.0))
                        .child(composer),
                )
                .into_any_element()
        } else {
            div()
                .relative()
                .w_full()
                .max_w(px(768.0))
                .bg(theme.chat_canvas)
                .child(composer)
                .into_any_element()
        }
    }

    /// Bring the pane's active chat to the front after a file-tab send so the
    /// streamed answer is visible. A brand-new session is opened by the run
    /// path itself.
    fn reveal_chat_for_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let Some(session_id) = self.active_session_for_pane(pane_id) else {
            return;
        };
        let title = self
            .sessions
            .iter()
            .find(|session| session.id == session_id)
            .map(|session| session.display_title().to_string())
            .unwrap_or_else(|| "Chat".to_string());
        self.open_chat_tab_in_pane(pane_id, session_id, title);
        cx.notify();
    }
}

