//! App-side wiring for the floating composer launcher.
//!
//! ⌘N opens the card with a snapshot of the current model / thinking defaults;
//! submitting it creates the session the user described and opens it in the
//! active pane, exactly like the old direct `create_new_chat` did — the only
//! difference is that project, branch and model are chosen in the card rather
//! than inherited from whichever pane happened to be focused.
//!
//! The card's state lives on `ConsoleDesktopApp` (see
//! `console_ui::common::FloatingComposerState`) and its element is built in the
//! app's render, so all of this mutates plain fields — there is no second
//! entity to lease.

use std::rc::Rc;

use console_core::{CreateSessionDto, CreateWorktreeSpec, SelectedModel, ThinkingLevel};
use console_ui::common::{
    BranchChoice, FloatingComposerView, FloatingSnapshot, FloatingSubmit, ModelDropdownMenu,
    PickerTab,
};
use gpui::{App, Context, ExternalPaths, Window};

use super::ConsoleDesktopApp;
use super::autocomplete::FLOATING_AUTOCOMPLETE_KEY;

/// Reserved key for the card's staged images. The attachment store is keyed by
/// owner rather than strictly by pane, so the card claims a key of its own and
/// never collides with a real pane id.
pub(crate) const FLOATING_KEY: &str = "__floating_composer__";

impl ConsoleDesktopApp {
    /// ⌘N — open the launcher instead of creating a session immediately.
    /// Re-invoking while it's open just re-focuses the prompt.
    pub fn open_floating_composer(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.floating_composer.open {
            let handle = self.floating_composer.input.read(cx).focus();
            window.focus(&handle, cx);
            return;
        }

        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let project_id = self.pane_real_project_id(&pane_id).or_else(|| {
            self.selected_project_id
                .as_deref()
                .map(|key| console_core::project_id_of_workspace_key(key).to_owned())
        });

        // The card is the only consumer of this state while it's open, so
        // refresh the branch list for whatever project it's about to show.
        if let Some(project_id) = project_id.clone() {
            self.refresh_branches_for_project(&project_id, cx);
        }

        let snapshot = FloatingSnapshot {
            projects: self.projects.clone(),
            selected_project_id: project_id,
            branches: self.branches.clone(),
            providers: self.providers.clone(),
            models_by_provider: self.models_by_provider.clone(),
            favorites: self.favorites.clone(),
            selected_model: self.pane_selected_model(&pane_id),
            thinking_level: self.pane_thinking_level(&pane_id),
            supported_thinking_levels: self.supported_thinking_levels_for_pane(&pane_id),
            // Seeded so the chip reflects where the active pane already sits,
            // but overridable — the launched session takes the card's value,
            // not this.
            approval_mode: self.pane_approval_mode(&pane_id),
        };
        self.floating_composer.show(snapshot, window, cx);

        // Mirror the docked picker's popover-open handler: fetch the live
        // model list rather than opening on the static catalog. The docked
        // picker only ever shows a provider tab (bootstrap resolves Favorites
        // away at startup); this card keeps Favorites as its default, which
        // names no provider of its own, so fall back to the selected model's.
        let provider = match &self.floating_composer.picker_tab {
            PickerTab::Provider(name) => Some(name.clone()),
            PickerTab::Favorites => self
                .floating_composer
                .selected_model
                .as_ref()
                .map(|model| model.provider.clone()),
        };
        if let Some(provider) = provider {
            self.load_models_for_provider(&provider, cx);
        }

        cx.notify();
    }

    /// Close the card. Wired to the card's Escape binding.
    pub fn close_floating_composer(&mut self, cx: &mut Context<Self>) {
        self.floating_composer.hide();
        cx.notify();
    }

    /// Rebuild the card's derived state each frame: the @-file / slash-command
    /// popup follows the prompt's caret, and the chip row follows the shared
    /// attachment store (a paste stages asynchronously, so it can't be pushed at
    /// the moment it happens).
    ///
    /// Safe to call from the app's render because the card is app state, not a
    /// second leased entity.
    pub fn refresh_floating_composer(&mut self, window: &Window, cx: &mut Context<Self>) {
        if !self.floating_composer.open {
            return;
        }
        let input = self.floating_composer.input.clone();
        // File search is scoped to the card's selected project, which changes
        // before the app's own `selected_project_id` catches up.
        let project_root = self
            .floating_composer
            .selected_project()
            .map(|project| project.path.clone())
            .unwrap_or_default();
        let autocomplete = self.composer_autocomplete_for_owner(
            FLOATING_AUTOCOMPLETE_KEY,
            input,
            project_root,
            // No session yet — the launch creates it. Slash commands fall back
            // to the defaults the server lists without a session.
            None,
            window,
            cx,
        );
        let attachments = self.attachments_for_pane(FLOATING_KEY);
        // The picker reads the card's own copies, so the app's catalog has to
        // be mirrored in each frame — a fetch that lands while the card is
        // open would otherwise never be seen.
        let providers = self.providers.clone();
        let models_by_provider = self.models_by_provider.clone();
        let favorites = self.favorites.clone();
        let changed = self.floating_composer.set_autocomplete(autocomplete)
            | self.floating_composer.set_attachments(attachments)
            | self
                .floating_composer
                .set_model_catalog(providers, models_by_provider, favorites);
        if changed {
            cx.notify();
        }
    }

    /// Pick a project in the card: refetch its branches and clear any
    /// existing-branch choice that no longer applies.
    pub fn floating_project_changed(&mut self, project_id: String, cx: &mut Context<Self>) {
        self.floating_composer.choose_project(project_id.clone());
        self.refresh_branches_for_project(&project_id, cx);
        cx.notify();
    }

    /// A different model was picked: which thinking levels exist depends on the
    /// model, so re-seed the card's thinking controls.
    pub fn floating_model_changed(&mut self, model: SelectedModel, cx: &mut Context<Self>) {
        let supported = self.supported_thinking_levels_for_model(Some(&model));
        let current = self.thinking_level;
        self.floating_composer.selected_model = Some(model);
        self.floating_composer
            .set_thinking_context(current, supported);
        cx.notify();
    }

    /// Switching provider tabs may need that provider's live model list, which
    /// the static catalog doesn't carry. The card has already recorded the new
    /// tab, so this is only the fetch.
    pub fn floating_picker_tab_changed(&mut self, tab: PickerTab, cx: &mut Context<Self>) {
        if let PickerTab::Provider(name) = &tab {
            self.load_models_for_provider(name, cx);
        }
        cx.notify();
    }

    /// Submit the card: create the session it described and run the prompt.
    pub fn submit_floating_composer(&mut self, text: String, cx: &mut Context<Self>) -> bool {
        let Some(submit) = self.floating_composer.submit(text, cx) else {
            return false;
        };
        self.create_session_from_floating(submit, cx);
        true
    }

    /// Stage a pasted clipboard image / file path into the card.
    pub fn floating_paste_attachments(
        &mut self,
        entries: Vec<gpui::ClipboardEntry>,
        cx: &mut Context<Self>,
    ) {
        self.stage_clipboard_attachments(FLOATING_KEY, entries, cx);
        cx.notify();
    }

    /// Drop image files onto the card.
    pub fn floating_drop_files(
        &mut self,
        paths: &ExternalPaths,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.stage_dropped_files(FLOATING_KEY, paths, window, cx);
        cx.notify();
    }

    /// Open the native image picker for the card.
    pub fn floating_pick_image(&mut self, cx: &mut Context<Self>) {
        self.pick_image(FLOATING_KEY, cx);
        cx.notify();
    }

    pub fn floating_remove_attachment(&mut self, index: usize, cx: &mut Context<Self>) {
        self.remove_attachment(FLOATING_KEY, index, cx);
        cx.notify();
    }

    pub fn floating_preview_attachment(&mut self, index: usize, cx: &mut Context<Self>) {
        self.preview_attachment(FLOATING_KEY, index, cx);
        cx.notify();
    }

    pub fn floating_autocomplete_next(&mut self, cx: &mut Context<Self>) {
        self.move_autocomplete_for_owner(FLOATING_AUTOCOMPLETE_KEY, true, cx);
    }

    pub fn floating_autocomplete_previous(&mut self, cx: &mut Context<Self>) {
        self.move_autocomplete_for_owner(FLOATING_AUTOCOMPLETE_KEY, false, cx);
    }

    pub fn floating_autocomplete_confirm(&mut self, cx: &mut Context<Self>) {
        let input = self.floating_composer.input.clone();
        self.accept_highlighted_autocomplete_for_owner(FLOATING_AUTOCOMPLETE_KEY, &input, cx);
    }

    pub fn floating_autocomplete_dismiss(&mut self, cx: &mut Context<Self>) {
        self.dismiss_autocomplete_for_owner(FLOATING_AUTOCOMPLETE_KEY, cx);
    }

    /// Build the card's element for this frame. Called from the app's render,
    /// alongside every other surface.
    pub fn floating_composer_view(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Option<gpui::AnyElement> {
        if !self.floating_composer.open {
            return None;
        }
        let attachments = self.attachments_for_pane(FLOATING_KEY);
        self.floating_composer.set_attachments(attachments);
        let query = self
            .floating_composer
            .model_search
            .read(cx)
            .content()
            .to_string();
        let data = self.floating_composer.view_data(query);
        let entity = cx.entity().downgrade();
        let card = &self.floating_composer;
        let model_menu = ModelDropdownMenu::new(
            data.providers.clone(),
            data.selected_model.clone(),
            data.picker_tab.clone(),
            data.favorites.clone(),
            data.models_by_provider.clone(),
            card.model_search.clone(),
            data.model_search_query.clone(),
            {
                let entity = entity.clone();
                move |provider, model_id, window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.floating_composer.model_menu.close(window, cx);
                            this.floating_model_changed(SelectedModel { provider, model_id }, cx);
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                move |tab, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.floating_composer.set_picker_tab(tab.clone(), cx);
                            this.floating_picker_tab_changed(tab, cx);
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                move |provider, model_id, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.toggle_model_favorite(provider, model_id, cx)
                        });
                    }
                }
            },
        );

        let card = &self.floating_composer;
        let view = FloatingComposerView::new(
            card.input.clone(),
            card.project_menu.clone(),
            card.branch_menu.clone(),
            card.model_menu.clone(),
            card.thinking_menu.clone(),
            card.approval_menu.clone(),
            model_menu,
        )
        .data(data)
        .on_dismiss({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.close_floating_composer(cx));
                }
            }
        })
        .on_cancel({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.close_floating_composer(cx));
                }
            }
        })
        .on_choose_project({
            let entity = entity.clone();
            move |id: String, window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.project_menu.close(window, cx);
                        this.floating_project_changed(id, cx);
                    });
                }
            }
        })
        .on_choose_branch({
            let entity = entity.clone();
            move |choice: BranchChoice, window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.branch_menu.close(window, cx);
                        this.floating_composer.branch = choice;
                        cx.notify();
                    });
                }
            }
        })
        .on_choose_model({
            let entity = entity.clone();
            move |provider: String, model_id: String, window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.model_menu.close(window, cx);
                        this.floating_model_changed(SelectedModel { provider, model_id }, cx);
                    });
                }
            }
        })
        .on_choose_thinking({
            let entity = entity.clone();
            move |level: ThinkingLevel, window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.thinking_menu.close(window, cx);
                        this.floating_composer.thinking_level = Some(level);
                        this.thinking_level = Some(level);
                        cx.notify();
                    });
                }
            }
        })
        .on_choose_approval({
            let entity = entity.clone();
            move |mode: console_core::ApprovalMode, window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.approval_menu.close(window, cx);
                        this.floating_composer.approval_mode = mode;
                        cx.notify();
                    });
                }
            }
        })
        .on_pick_image({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_pick_image(cx));
                }
            }
        })
        .on_drop_files({
            let entity = entity.clone();
            move |paths: &ExternalPaths, window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_drop_files(paths, window, cx));
                }
            }
        })
        .on_remove_attachment({
            let entity = entity.clone();
            move |index: usize, _window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_remove_attachment(index, cx));
                }
            }
        })
        .on_preview_attachment({
            let entity = entity.clone();
            move |index: usize, _window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_preview_attachment(index, cx));
                }
            }
        })
        .on_autocomplete_next({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_autocomplete_next(cx));
                }
            }
        })
        .on_autocomplete_previous({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_autocomplete_previous(cx));
                }
            }
        })
        .on_autocomplete_confirm({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_autocomplete_confirm(cx));
                }
            }
        })
        .on_autocomplete_dismiss({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.floating_autocomplete_dismiss(cx));
                }
            }
        })
        .on_submit({
            let entity = entity.clone();
            move |prompt: String, _window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.submit_floating_composer(prompt, cx);
                    });
                }
            }
        });

        Some(view.render(window, cx))
    }

    /// Refetch the branch list for `project_id`, storing it on the app and
    /// pushing it into the open card.
    fn refresh_branches_for_project(&mut self, project_id: &str, cx: &mut Context<Self>) {
        let Some(project) = self.projects.iter().find(|p| p.id == project_id) else {
            return;
        };
        let path = project.path.clone();
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let Ok(result) = client.git.list_branches(Some(&path)).await else {
                return;
            };
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.branches = Rc::new(result.branches);
                        this.branch_loaded = true;
                        this.branch_is_git_repository = result.is_git_repository;
                        this.floating_composer.set_branches(this.branches.clone());
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Create the session the launcher described, then run its prompt. Mirrors
    /// `create_new_chat`'s post-create bookkeeping.
    fn create_session_from_floating(&mut self, submit: FloatingSubmit, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let client = self.client.clone();
        // The card's choice, not the pane's: the mode is picked in the card
        // precisely so a launch doesn't inherit whatever the focused pane
        // happened to be set to.
        let approval_mode = submit.approval_mode;
        let prompt = submit.prompt.clone();
        let title = title_from_prompt(&prompt);
        // Attachments staged in the card move onto the new session; the card's
        // own copy is dropped so reopening starts clean.
        let staged_attachments = (*submit.attachments).clone();
        self.set_attachments_for_pane(FLOATING_KEY, Vec::new());
        self.autocomplete_states.remove(FLOATING_AUTOCOMPLETE_KEY);
        let context_files: Vec<String> = submit
            .mentions
            .iter()
            .map(|mention| mention.path.clone())
            .collect();
        let mentions = submit.mentions.clone();
        let _ = &mentions;
        // The card's choices, not the pane's: `submit_prompt_with_context`
        // reads model/thinking off the pane, so they are pushed in below
        // before the run starts or the agent would use whatever the focused
        // pane happened to have.
        let card_model = submit.model.clone();
        let card_thinking = submit.thinking_level;

        // Only "New worktree" provisions anything; a plain project session
        // sends no worktree key at all. Picking an existing branch works in
        // the project folder on that branch (checked out first below).
        let worktree = submit
            .branch
            .creates_worktree()
            .then(CreateWorktreeSpec::default);
        // Skip the checkout when the picked branch is already the one checked
        // out in the project folder (the common "stay on main" case).
        let checkout = submit
            .branch
            .checkout_target()
            .filter(|name| !self.branches.iter().any(|b| b.current && b.name == *name))
            .map(str::to_owned)
            .zip(submit.cwd.clone());

        cx.spawn(async move |entity, cx| {
            if let Some((branch, cwd)) = checkout.as_ref() {
                if let Err(error) = client.git.checkout_branch(Some(cwd), branch).await {
                    // Keep the card open with the prompt intact; nothing was
                    // created, and the project folder stays on its branch.
                    let message = format!("Unable to switch to {branch}: {error}");
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.floating_composer.set_error(Some(message));
                                cx.notify();
                            });
                        }
                    });
                    return;
                }
            }
            let result = client
                .sessions
                .create(CreateSessionDto {
                    cwd: submit.cwd,
                    project_id: submit.project_id,
                    model_id: submit.model.as_ref().map(|m| m.model_id.clone()),
                    provider: submit.model.as_ref().map(|m| m.provider.clone()),
                    title: Some(title.clone()),
                    approval_mode: Some(approval_mode.value().to_string()),
                    thinking_level: submit.thinking_level,
                    worktree,
                })
                .await;

            cx.update(|cx| {
                let Some(app) = entity.upgrade() else { return };
                app.update(cx, |this, cx| match result {
                    Ok(new_session) => {
                        this.floating_composer.hide();
                        this.save_transcript_scroll_position(cx);
                        this.apply_session_header_for_pane(&pane_id, &new_session, cx);
                        this.clear_error_for_pane(&pane_id, cx);
                        if this.active_pane_id.as_deref() == Some(pane_id.as_str()) {
                            this.selected_session_id = Some(new_session.id.clone());
                        }
                        Rc::make_mut(&mut this.sessions).insert(0, new_session.clone());
                        this.open_chat_tab_in_pane(&pane_id, new_session.id.clone(), &title);
                        // The project folder's branch just changed, so the
                        // footer's branch list is stale until it reloads.
                        if let Some((_, cwd)) = checkout.clone() {
                            this.reload_branches_for_pane(pane_id.clone(), cwd, cx);
                        }
                        this.sync_workspace_webviews(cx);
                        this.transcript_for_pane(&pane_id).update(cx, |t, cx| {
                            t.set_messages(Vec::new(), cx);
                        });
                        // Adopt the card's selections for this pane so the run
                        // below — and the composer it leaves behind — agree
                        // with what the user chose in the card.
                        if let Some(model) = card_model.clone() {
                            this.set_pane_model(&pane_id, Some(model));
                        }
                        this.set_pane_thinking_level(&pane_id, card_thinking);
                        // Same for approval mode: the composer the launch
                        // leaves behind must show the mode the session is
                        // actually running in, not the pane's previous one.
                        this.set_pane_approval_mode(&pane_id, approval_mode);
                        // The run targets the active pane, so make sure the
                        // pane the session just opened in is the active one.
                        this.active_pane_id = Some(pane_id.clone());
                        this.selected_session_id = this.active_session_for_pane(&pane_id);
                        // Auto-send: the prompt that described the task *is*
                        // the task, so it goes straight to the agent instead of
                        // waiting for a second Enter. This also clears the
                        // composer, stages the attachments, and pushes the
                        // optimistic user message into the transcript.
                        this.submit_prompt_with_context(
                            prompt.clone(),
                            staged_attachments.clone(),
                            context_files.clone(),
                            cx,
                        );
                        cx.notify();
                    }
                    Err(error) => {
                        // Keep the card open with the prompt intact so a failed
                        // launch costs one retry, not a retype.
                        this.floating_composer
                            .set_error(Some(format!("Unable to create a session: {error}")));
                        cx.notify();
                    }
                });
                let _ = mentions;
            });
        })
        .detach();
    }
}

/// First non-empty line of the prompt, trimmed to a title-sized string.
fn title_from_prompt(prompt: &str) -> String {
    let line = prompt.lines().find(|l| !l.trim().is_empty()).unwrap_or("");
    let trimmed = line.trim();
    if trimmed.is_empty() {
        return "New Chat".to_owned();
    }
    let mut out = String::new();
    for ch in trimmed.chars() {
        if out.chars().count() >= 60 {
            out.push('…');
            break;
        }
        out.push(ch);
    }
    out
}
