use std::rc::Rc;
use console_core::{
    ApprovalMode, ApproveToolPermissionDto, ModelFavorite, SelectedModel, UpdateSessionDto,
};
use console_ui::workspace::EmptyChatState;
use console_ui::{
    ApprovalModeDropdown, ComposerView, ModelDropdownMenu, PermissionInteractionCard, PickerTab,
    QuestionInteractionCard, Theme, WorkspaceFooter, centered_stripe, error_banner, notice_banner,
    todo_card,
};
use gpui::{App, Context, IntoElement, ParentElement, Styled, Window, div, prelude::FluentBuilder, px};

use crate::state::ConsoleDesktopApp;

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

        // File tab: render full-page MarkdownViewer if markdown, or FileViewer
        if let Some(console_core::WorkspaceTabConfig::File { path, .. }) = active_tab {
            let content = self
                .open_file_contents
                .get(path)
                .cloned()
                .unwrap_or_else(|| "Loading file content...".to_string());

            let is_markdown = {
                let lower = path.to_lowercase();
                lower.ends_with(".md") || lower.ends_with(".markdown") || lower.ends_with(".mdx")
            };

            if is_markdown {
                let view = self.get_or_build_markdown_view(path, &content);
                let selection = self.viewer_markdown_selection(path);
                return console_ui::MarkdownViewer::new(path.clone(), view)
                    .selection(selection)
                    .into_any_element();
            }

            let lines = self.get_or_build_file_lines(path, &content);
            let line_count = lines.len();
            let list_state = self.viewer_list_state(
                &format!("file:{}", path),
                line_count,
                console_ui::CODE_LINE_HEIGHT,
            );
            let selection_state = self.viewer_selection_state(&format!("file:{}", path));
            let scrollbar_state = self.viewer_scrollbar_state(&format!("file:{}", path));
            return console_ui::FileViewer::new(path.clone(), content, list_state)
                .rc_lines(lines)
                .selection_state(selection_state)
                .scrollbar_state(scrollbar_state)
                .into_any_element();
        }

        // Diff tab: render full-page DiffViewer
        if let Some(console_core::WorkspaceTabConfig::Diff { path, .. }) = active_tab {
            let (diff_result, raw_diff) = self
                .open_diff_contents
                .get(path)
                .cloned()
                .unwrap_or_else(|| (console_core::DiffResult::default(), String::new()));
            let theme = Theme::current(cx);
            let lines = self.get_or_build_diff_lines(path, &diff_result, &theme);
            let line_count = lines.len();
            let list_state = self.viewer_list_state(
                &format!("diff:{}", path),
                line_count,
                console_ui::CODE_LINE_HEIGHT,
            );
            let selection_state = self.viewer_selection_state(&format!("diff:{}", path));
            let scrollbar_state = self.viewer_scrollbar_state(&format!("diff:{}", path));
            return console_ui::DiffViewer::new(path.clone(), diff_result, raw_diff, list_state)
                .rc_lines(lines)
                .selection_state(selection_state)
                .scrollbar_state(scrollbar_state)
                .into_any_element();
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
        let error_message = self.error_for_pane(&pane_id).map(|error| error.message.clone());
        let error_pane_id = pane_id.clone();
        let error_selection = self.error_selection.clone();
        let agent_notice = self.agent_notice_for_pane(&pane_id);
        let todo_items = self.todo_items_for_pane(&pane_id);
        let selected_sid = pane_session_id.clone();

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
                        app.update(cx, |this, cx| this.clear_error_for_pane(&error_pane_id, cx));
                    }
                })),
                cx,
            ))
        })
        .when_some(agent_notice, |el, notice| {
            el.child(notice_banner(notice, theme))
        })
        .when(
            !todo_items.is_empty()
                && todo_items.iter().any(|i| {
                    !matches!(
                        i.status.to_ascii_lowercase().as_str(),
                        "done" | "completed" | "complete"
                    )
                }),
            |el| {
                let entity = entity.clone();
                let toggle_pane_id = pane_id.clone();
                let collapsed = self.is_todos_collapsed_for_pane(&pane_id);
                el.child(todo_card(
                    todo_items.clone(),
                    collapsed,
                    Some(Rc::new(move |_window, cx| {
                        if let Some(app) = entity.upgrade() {
                            let pane_id = toggle_pane_id.clone();
                            app.update(cx, |this, cx| {
                                this.toggle_todos_collapsed_for_pane(&pane_id);
                                cx.notify();
                            });
                        }
                    })),
                    theme,
                ))
            },
        )
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
                        move |name: String, _w, cx| {
                            if let Some(app) = entity.upgrade() {
                                let pane_id = branch_pane_id.clone();
                                app.update(cx, |this, cx| {
                                    this.checkout_branch_for_pane(pane_id, name, cx)
                                });
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
                                            let message = format!(
                                                "Unable to update model favorite: {error}"
                                            );
                                            cx.update(|cx| {
                                                if let Some(app) = entity_for_error.upgrade() {
                                                    app.update(cx, |this, cx| {
                                                        this.set_error(message, cx)
                                                    });
                                                }
                                            });
                                        }
                                    })
                                    .detach();
                                    cx.notify();
                                });
                            }
                        }
                    },
                );

                let approval_dropdown = ApprovalModeDropdown::new(pane_approval_mode, {
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
                });

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
                                                    let attachments = (*this
                                                        .attachments_for_pane(&submit_pane_id))
                                                    .clone();
                                                    this.active_pane_id =
                                                        Some(submit_pane_id.clone());
                                                    this.selected_session_id = this
                                                        .active_session_for_pane(&submit_pane_id);
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
                                                    this.active_pane_id =
                                                        Some(abort_pane_id.clone());
                                                    this.selected_session_id = this
                                                        .active_session_for_pane(&abort_pane_id);
                                                    if let Some(sid) = &this.selected_session_id {
                                                        // Clear the running state for the chat that
                                                        // owns the run, keyed by session so the Stop
                                                        // button only affects this chat.
                                                        let sid_clone = sid.clone();
                                                        this.set_session_running(&sid_clone, None);
                                                        let entity_for_error = entity.clone();
                                                        cx.spawn(async move |_entity, cx| {
                                                            if let Err(error) =
                                                                client.runs.abort(&sid_clone).await
                                                            {
                                                                let message = format!(
                                                                    "Unable to stop the agent run: {error}"
                                                                );
                                                                cx.update(|cx| {
                                                                    if let Some(app) =
                                                                        entity_for_error.upgrade()
                                                                    {
                                                                        app.update(cx, |this, cx| {
                                                                            this.set_error_for_session(&sid_clone, message, cx)
                                                                        });
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
                                                this.move_autocomplete_for_pane(
                                                    &pane_id, false, cx,
                                                );
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
                                                this.stage_dropped_files(paths, window, cx);
                                            });
                                        }
                                    }
                                }),
                            ),
                    )
                    .child(workspace_footer)
            })
        })
        .into_any_element()
    }
}
