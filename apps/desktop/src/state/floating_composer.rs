//! App-side wiring for the floating composer launcher.
//!
//! ⌘N opens the card with a snapshot of the current model / thinking defaults;
//! submitting it creates the session the user described and opens it in the
//! active pane. Project, branch and model are chosen in the card rather than
//! inherited from whichever pane happened to be focused. Every New Chat entry
//! point — ⌘N, the sidebar, the empty state, the palette and the tab bar's `+` —
//! opens it.
//!
//! The card's state lives on `ConsoleDesktopApp` (see
//! `console_ui::common::FloatingComposerState`) and its element is built in the
//! app's render, so all of this mutates plain fields — there is no second
//! entity to lease.

use std::rc::Rc;

use crate::persistence::store::{PersistedBranchChoice, PersistedFloatingChoices};

use console_core::{
    AgentMessage, ApprovalMode, CreateSessionDto, CreateWorktreeSpec, ImageAttachment,
    LaunchPromptDto, LaunchWorktreeDto, SelectedModel, SessionHeader, ThinkingLevel,
};
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
        } else {
            self.branches = Rc::new(Vec::new());
        }

        let snapshot = FloatingSnapshot {
            projects: self.projects.clone(),
            selected_project_id: project_id,
            github_repos: self.github_repos.clone(),
            github_repos_loading: self.github_repos_loading,
            github_repos_error: self.github_repos_error.clone(),
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
        self.restore_floating_stash(cx);

        // Mirror the docked picker's popover-open handler: fetch the live
        // model list rather than opening on the static catalog. Favorites
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

    /// A sidebar project's `+`: open the launcher with that project chosen.
    /// `key` is a project id, or the sidebar's no-project key. The choice is
    /// applied after the stashed prompt is restored so it wins over a stashed
    /// project, while the stashed text stays.
    pub fn open_floating_composer_for_project(
        &mut self,
        key: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.open_floating_composer(window, cx);
        if key == console_ui::layout::NO_PROJECT_KEY {
            self.floating_no_project_chosen(cx);
        } else if self.projects.iter().any(|p| p.id == key) {
            self.floating_project_changed(key.to_owned(), cx);
        }
    }

    /// Close the card. Wired to the card's Escape binding.
    pub fn close_floating_composer(&mut self, cx: &mut Context<Self>) {
        self.stash_floating_prompt(cx);
        self.floating_composer.hide();
        cx.notify();
    }

    /// Keep an unsent prompt (text, mentions, images) under the card's own
    /// draft key so reopening restores it. The key is never listed in the
    /// sidebar and is never synced.
    fn stash_floating_prompt(&mut self, cx: &mut Context<Self>) {
        let input = self.floating_composer.input.read(cx);
        let text = input.content().to_string();
        let mentions = input.mentions().to_vec();
        let context_files = input.context_files().to_vec();
        let attachments = (*self.attachments_for_pane(FLOATING_KEY)).clone();
        self.save_draft_for_key(
            FLOATING_KEY,
            &text,
            &mentions,
            &context_files,
            &attachments,
            &[],
            cx,
        );
        let card = &self.floating_composer;
        let choices = PersistedFloatingChoices {
            project_id: card.selected_project_id.clone(),
            branch: match &card.branch {
                BranchChoice::Project => PersistedBranchChoice::Project,
                BranchChoice::NewWorktree => PersistedBranchChoice::NewWorktree,
                BranchChoice::FromBranch(name) => PersistedBranchChoice::FromBranch(name.clone()),
            },
            worktree_base: card.worktree_base.clone(),
            model: card
                .selected_model
                .as_ref()
                .map(|m| (m.provider.clone(), m.model_id.clone())),
            thinking_level: card.thinking_level,
            approval_mode: card.approval_mode.value().to_string(),
        };
        if let Some(draft) = self.drafts.get_mut(FLOATING_KEY)
            && draft.floating.as_ref() != Some(&choices)
        {
            draft.floating = Some(choices);
            self.schedule_drafts_save(cx);
        }
    }

    /// Re-apply the pickers stashed with the prompt. Each one is checked
    /// against live state and left at its default when it no longer exists.
    fn restore_floating_choices(
        &mut self,
        choices: PersistedFloatingChoices,
        cx: &mut Context<Self>,
    ) {
        let project_exists = choices
            .project_id
            .as_deref()
            .is_some_and(|id| self.projects.iter().any(|p| p.id == id));
        if project_exists && let Some(id) = choices.project_id.clone() {
            self.floating_project_changed(id, cx);
            // The branch list is still the previous project's here; `set_branches`
            // drops a base that isn't in the list once the fetch lands.
            self.floating_composer.branch = match choices.branch {
                PersistedBranchChoice::Project => BranchChoice::Project,
                PersistedBranchChoice::NewWorktree => BranchChoice::NewWorktree,
                PersistedBranchChoice::FromBranch(name) => BranchChoice::FromBranch(name),
            };
            self.floating_composer.worktree_base = choices.worktree_base;
        } else if choices.project_id.is_none() {
            self.floating_no_project_chosen(cx);
        }

        if let Some((provider, model_id)) = choices.model {
            let known = self.providers.iter().any(|p| p.name == provider)
                && self
                    .models_by_provider
                    .get(&provider)
                    .is_none_or(|models| models.iter().any(|m| m.id == model_id));
            if known {
                let model = SelectedModel { provider, model_id };
                let supported = self.supported_thinking_levels_for_model(Some(&model));
                self.floating_composer.selected_model = Some(model);
                self.floating_composer
                    .set_thinking_context(choices.thinking_level, supported);
            }
        }
        self.floating_composer.approval_mode = ApprovalMode::from_value(&choices.approval_mode);
        cx.notify();
    }

    fn restore_floating_stash(&mut self, cx: &mut Context<Self>) {
        let Some((text, mentions)) = self.get_draft_with_mentions_for_key(FLOATING_KEY) else {
            return;
        };
        let context_files = self.get_draft_context_files_for_key(FLOATING_KEY);
        let attachments = self
            .drafts
            .get(FLOATING_KEY)
            .map(|draft| draft.attachments.clone())
            .unwrap_or_default();
        let choices = self
            .drafts
            .get(FLOATING_KEY)
            .and_then(|draft| draft.floating.clone());
        self.set_attachments_for_pane(FLOATING_KEY, attachments);
        self.floating_composer.input.update(cx, |input, cx| {
            input.set_content_with_mentions(text, mentions, cx);
            input.set_context_files(context_files);
        });
        if let Some(choices) = choices {
            self.restore_floating_choices(choices, cx);
        }
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
        let chat_scope_project_id = self
            .floating_composer
            .selected_project()
            .map(|project| project.id.clone());
        let autocomplete = self
            .composer_autocomplete_for_owner(
                FLOATING_AUTOCOMPLETE_KEY,
                input,
                project_root,
                // No session yet — the launch creates it. Slash commands fall back
                // to the defaults the server lists without a session; `$` chat
                // refs fall back to the card's selected project.
                None,
                chat_scope_project_id.as_deref(),
                window,
                cx,
            )
            .map(console_ui::AutocompleteView::below);
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
        // Dictation state: mirrored like the attachments, so a take started
        // from the card (or a failure text) shows live. Only a take targeted
        // at the card lights its recording state; the elapsed ticker bumps
        // stt_recording_secs, which lands here on the next frame.
        let mic_recording =
            self.stt_recorder.is_some() && self.stt_recording_pane.as_deref() == Some(FLOATING_KEY);
        let mic = (
            self.stt_enabled(),
            mic_recording,
            if mic_recording {
                self.stt_recording_secs
            } else {
                0
            },
            self.stt_transcribing,
            self.stt_mic_error.clone(),
        );
        let card = &mut self.floating_composer;
        let mic_changed = card.mic_enabled != mic.0
            || card.mic_recording != mic.1
            || card.mic_elapsed_secs != mic.2
            || card.mic_transcribing != mic.3
            || card.mic_error != mic.4;
        if mic_changed {
            (
                card.mic_enabled,
                card.mic_recording,
                card.mic_elapsed_secs,
                card.mic_transcribing,
                card.mic_error,
            ) = mic;
        }
        if changed || mic_changed {
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

    /// Explicitly select "No project": drop project scope and branch state.
    pub fn floating_no_project_chosen(&mut self, cx: &mut Context<Self>) {
        self.floating_composer.choose_no_project();
        self.branches = Rc::new(Vec::new());
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
    ///
    /// `context_files` are the prompt's mention paths, passed in because an
    /// Enter submit has already cleared the field by the time this runs.
    pub fn submit_floating_composer(
        &mut self,
        text: String,
        context_files: Vec<String>,
        cx: &mut Context<Self>,
    ) -> bool {
        // A repo clone in flight selects its project when it lands; launching
        // before that would silently use the previous selection.
        if self.repo_task_cloning.is_some() {
            return false;
        }
        let Some(submit) = self.floating_composer.submit(text, cx) else {
            return false;
        };
        self.create_session_from_floating(submit, context_files, cx);
        true
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
        let mut data = self.floating_composer.view_data(query);
        // The clone runs in app state, not the card's, so feed the name in
        // each frame — the card's "Cloning <name>…" line then tracks it live.
        data.repo_cloning = self.repo_task_cloning.clone();
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
            card.model_menu.clone(),
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
            card.worktree_base_menu.clone(),
            card.model_menu.clone(),
            card.thinking_menu.clone(),
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
        .on_no_project({
            let entity = entity.clone();
            move |window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.project_menu.close(window, cx);
                        this.floating_no_project_chosen(cx);
                    });
                }
            }
        })
        .on_new_project({
            let entity = entity.clone();
            move |window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.project_menu.close(window, cx);
                        this.close_floating_composer(cx);
                        this.open_project_browse(window, cx);
                    });
                }
            }
        })
        .on_choose_github_repo({
            let entity = entity.clone();
            move |repo: console_core::GithubRepo, window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.floating_composer.project_menu.close(window, cx);
                        this.floating_clone_repo(repo, cx);
                    });
                }
            }
        })
        .on_refresh_github_repos({
            let entity = entity.clone();
            move |_window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.refresh_github_repos(cx));
                }
            }
        })
        .on_connect_github({
            let entity = entity.clone();
            move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // Not open_settings: it toggles, and closing the
                        // settings the user already had open would be a
                        // surprise coming from a menu row.
                        this.open_settings_tab(
                            console_ui::settings::SettingsTab::Providers,
                            window,
                            cx,
                        );
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
        .on_choose_worktree_base({
            let entity = entity.clone();
            move |base: Option<String>, _window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // The menu stays open — picking a cut point is a
                        // two-step choice, unlike the mode rows.
                        this.floating_composer.worktree_base = base;
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
        .on_toggle_mic({
            let entity = entity.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // The card dictates into its own draft: the reserved
                        // key routes the transcript back to the card's input.
                        this.toggle_stt_recording(FLOATING_KEY.to_string(), cx);
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
                        let context_files =
                            console_shared::composer::mentions::submitted_mention_paths(
                                this.floating_composer.input.read(cx).mentions(),
                            );
                        this.submit_floating_composer(prompt, context_files, cx);
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

    /// Create the session the launcher described, then run its prompt.
    fn create_session_from_floating(
        &mut self,
        submit: FloatingSubmit,
        context_files: Vec<String>,
        cx: &mut Context<Self>,
    ) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        // A launch can switch the chat into a brand-new desk whose pane has no
        // `WorkspacePaneState` yet. Capturing the window lets the result create
        // that state before writing to the transcript — otherwise the optimistic
        // prompt lands in the fallback (main) transcript, which that pane never
        // renders.
        let window_handle = cx.active_window();
        let client = self.client.clone();
        // The card's choice, not the pane's: the mode is picked in the card
        // precisely so a launch doesn't inherit whatever the focused pane
        // happened to be set to.
        let approval_mode = submit.approval_mode;
        let prompt = submit.prompt.clone();
        // Attachments staged in the card move onto the new session; the card's
        // own copy is dropped so reopening starts clean.
        let staged_attachments = (*submit.attachments).clone();
        self.set_attachments_for_pane(FLOATING_KEY, Vec::new());
        self.autocomplete_states.remove(FLOATING_AUTOCOMPLETE_KEY);
        // Only pastes whose chip is still in the prompt are uploaded.
        let live_pastes: Vec<_> = submit
            .pending_pastes
            .iter()
            .filter(|paste| context_files.contains(&paste.path))
            .cloned()
            .collect();
        let mentions = submit.mentions.clone();
        let _ = &mentions;
        // The card's choices, not the pane's: `submit_prompt_with_context`
        // reads model/thinking off the pane, so they are pushed in below
        // before the run starts or the agent would use whatever the focused
        // pane happened to have.
        let card_model = submit.model.clone();
        let card_thinking = submit.thinking_level;

        // "New worktree" with no live pastes takes the instant launch: the
        // server creates the session now, provisions the worktree and runs
        // the prompt in the background, so the card closes this frame. Live
        // pastes must reach the per-session paste endpoint before the prompt
        // runs, which the instant launch can't wait for — those fall back to
        // the synchronous path below.
        if submit.branch.creates_worktree() && live_pastes.is_empty() {
            self.launch_worktree_instant(
                pane_id,
                submit,
                prompt,
                context_files,
                staged_attachments,
                card_model,
                card_thinking,
                approval_mode,
                window_handle,
                cx,
            );
            return;
        }

        // Only "New worktree" provisions anything; a plain project session
        // sends no worktree key at all. Picking an existing branch works in
        // the project folder on that branch (checked out first below). The
        // optional base is the branch the new worktree branch is cut from;
        // None lets the server resolve the repo default.
        let worktree = submit
            .branch
            .creates_worktree()
            .then(|| CreateWorktreeSpec {
                branch: None,
                base_branch: submit.worktree_base.clone(),
            });
        // Skip the checkout when the picked branch is already the one checked
        // out in the project folder (the common "stay on main" case).
        let checkout = submit
            .branch
            .checkout_target()
            .filter(|name| !self.branches.iter().any(|b| b.current && b.name == *name))
            .map(str::to_owned)
            .zip(submit.cwd.clone());

        cx.spawn(async move |entity, cx| {
            if let Some((branch, cwd)) = checkout.as_ref()
                && let Err(error) = client.git.checkout_branch(Some(cwd), branch).await
            {
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
            let result = client
                .sessions
                .create(CreateSessionDto {
                    cwd: submit.cwd,
                    project_id: submit.project_id,
                    model_id: submit.model.as_ref().map(|m| m.model_id.clone()),
                    provider: submit.model.as_ref().map(|m| m.provider.clone()),
                    title: None,
                    approval_mode: Some(approval_mode.value().to_string()),
                    thinking_level: submit.thinking_level,
                    worktree,
                })
                .await;

            // The paste endpoint is per-session, so the card's pastes upload
            // only now that the session exists; a failed one goes inline.
            let mut prompt = prompt;
            let mut context_files = context_files;
            let mut paste_error = None;
            if let Ok(session) = &result
                && !live_pastes.is_empty()
            {
                let mut resolved = Vec::with_capacity(live_pastes.len());
                for paste in live_pastes {
                    let path = match client.runs.create_paste(&session.id, &paste.text).await {
                        Ok(uploaded) => Some(uploaded.path),
                        Err(error) => {
                            paste_error = Some(error.to_string());
                            None
                        }
                    };
                    resolved.push((paste, path));
                }
                (prompt, context_files) =
                    console_ui::common::resolve_pending_pastes(&prompt, &context_files, &resolved);
            }

            let finish =
                move |this: &mut Self, window: Option<&mut Window>, cx: &mut Context<Self>| {
                    match result {
                        Ok(new_session) => {
                            this.floating_composer.hide();
                            if this.drafts.remove(FLOATING_KEY).is_some() {
                                this.schedule_drafts_save(cx);
                            }
                            this.open_floating_session(
                                &pane_id,
                                &new_session,
                                card_model.clone(),
                                card_thinking,
                                approval_mode,
                                checkout.clone(),
                                window,
                                cx,
                            );
                            // Auto-send: the prompt that described the task *is*
                            // the task, so it goes straight to the agent instead of
                            // waiting for a second Enter. This also clears the
                            // composer, stages the attachments, and pushes the
                            // optimistic user message into the transcript.
                            this.submit_prompt_with_context(
                                prompt.clone(),
                                staged_attachments.clone(),
                                Vec::new(),
                                context_files.clone(),
                                cx,
                            );
                            if let Some(error) = paste_error.as_ref() {
                                this.set_error(format!("Paste upload failed: {error}"), cx);
                            }
                            cx.notify();
                        }
                        Err(error) => {
                            // Keep the card open with the prompt intact so a failed
                            // launch costs one retry, not a retype.
                            this.floating_composer
                                .set_error(Some(format!("Unable to create a session: {error}")));
                            cx.notify();
                        }
                    }
                    let _ = mentions;
                };
            cx.update(|cx| {
                if let Some(handle) = window_handle {
                    let _ = handle.update(cx, move |_, window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, move |this, cx| finish(this, Some(window), cx));
                        }
                    });
                } else if let Some(app) = entity.upgrade() {
                    app.update(cx, move |this, cx| finish(this, None, cx));
                }
            });
        })
        .detach();
    }

    /// Open the chat a launch just created in `pane_id`: header, tab,
    /// workspace, transcript, and the card's model/thinking/approval adopted
    /// for the pane. Shared by the synchronous and instant-launch paths;
    /// neither hides the card nor sends the prompt here.
    fn open_floating_session(
        &mut self,
        pane_id: &str,
        new_session: &SessionHeader,
        card_model: Option<SelectedModel>,
        card_thinking: Option<ThinkingLevel>,
        approval_mode: ApprovalMode,
        checkout: Option<(String, String)>,
        window: Option<&mut Window>,
        cx: &mut Context<Self>,
    ) {
        self.apply_session_header_for_pane(pane_id, new_session, cx);
        self.clear_error_for_pane(pane_id, cx);
        if self.active_pane_id.as_deref() == Some(pane_id) {
            self.selected_session_id = Some(new_session.id.clone());
        }
        Rc::make_mut(&mut self.sessions).insert(0, new_session.clone());
        self.switch_workspace_for_session(&new_session.id, pane_id, cx);
        let target_pane = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| pane_id.to_string());
        // A fresh desk's pane has no state until the first render creates it.
        // Without this, transcript writes below fall back to the main
        // transcript that this pane never renders.
        if let Some(window) = window {
            self.ensure_workspace_pane_state(&target_pane, window, cx);
        }
        // The new chat replaces this pane's transcript, so the
        // chat it was showing keeps its position.
        self.save_pane_scroll_position(&target_pane, cx);
        self.open_chat_tab_in_pane(
            &target_pane,
            new_session.id.clone(),
            "Generating title…",
            cx,
        );
        // The footer's branches describe this chat's own folder. A fresh
        // desk's pane has no branch state yet, and a checkout just moved the
        // project folder, so load against the cwd the chat will actually use.
        let branch_cwd = checkout
            .map(|(_, cwd)| cwd)
            .or_else(|| Some(new_session.cwd.clone()).filter(|cwd| !cwd.is_empty()))
            .or_else(|| {
                self.selected_project_for_pane(&target_pane)
                    .map(|project| project.path.clone())
            });
        if let Some(cwd) = branch_cwd {
            self.reload_branches_for_pane(target_pane.clone(), cwd, cx);
        }
        self.sync_workspace_webviews(cx);
        self.transcript_for_pane(&target_pane).update(cx, |t, cx| {
            t.set_turns(Vec::new(), cx);
            t.clear_session_changes(cx);
        });
        // Adopt the card's selections for this pane so the run
        // below — and the composer it leaves behind — agree
        // with what the user chose in the card.
        if let Some(model) = card_model {
            self.set_pane_model(&target_pane, Some(model));
        }
        self.set_pane_thinking_level(&target_pane, card_thinking);
        // Same for approval mode: the composer the launch
        // leaves behind must show the mode the session is
        // actually running in, not the pane's previous one.
        self.set_pane_approval_mode(&target_pane, approval_mode);
        // The run targets the active pane, so make sure the
        // pane the session just opened in is the active one.
        self.active_pane_id = Some(target_pane.clone());
        self.selected_session_id = self.active_session_for_pane(&target_pane);
    }

    /// Instant worktree launch: `POST /api/sessions/with-worktree` creates
    /// the session immediately; the server provisions the worktree and runs
    /// the prompt in the background. The card closes this frame and the pane
    /// opens on the new chat; the run attaches once the launch reports ready.
    fn launch_worktree_instant(
        &mut self,
        pane_id: String,
        submit: FloatingSubmit,
        prompt: String,
        context_files: Vec<String>,
        staged_attachments: Vec<ImageAttachment>,
        card_model: Option<SelectedModel>,
        card_thinking: Option<ThinkingLevel>,
        approval_mode: ApprovalMode,
        window_handle: Option<gpui::AnyWindowHandle>,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let optimistic_user_msg = AgentMessage::User {
            id: None,
            content: prompt.clone(),
            attachments: (!staged_attachments.is_empty()).then(|| staged_attachments.clone()),
            context_files: (!context_files.is_empty()).then(|| context_files.clone()),
            annotations: None,
            created_at: Some(chrono::Utc::now().timestamp()),
            approval_mode: Some(approval_mode.value().to_string()),
            source: None,
        };
        let launch = LaunchWorktreeDto {
            cwd: submit.cwd.clone(),
            project_id: submit.project_id.clone(),
            title: None,
            model_id: submit.model.as_ref().map(|m| m.model_id.clone()),
            provider: submit.model.as_ref().map(|m| m.provider.clone()),
            approval_mode: Some(approval_mode.value().to_string()),
            thinking_level: submit.thinking_level,
            worktree: CreateWorktreeSpec {
                branch: None,
                base_branch: submit.worktree_base.clone(),
            },
            prompt: LaunchPromptDto {
                prompt,
                context_files,
                attachments: staged_attachments,
                model_id: submit.model.as_ref().map(|m| m.model_id.clone()),
                provider: submit.model.as_ref().map(|m| m.provider.clone()),
                approval_mode: Some(approval_mode.value().to_string()),
                thinking_level: submit.thinking_level,
            },
        };
        cx.spawn(async move |entity, cx| {
            let result = client.sessions.launch_with_worktree(launch).await;
            let finish =
                move |this: &mut Self, window: Option<&mut Window>, cx: &mut Context<Self>| {
                    match result {
                        Ok(new_session) => {
                            this.floating_composer.hide();
                            if this.drafts.remove(FLOATING_KEY).is_some() {
                                this.schedule_drafts_save(cx);
                            }
                            this.open_floating_session(
                                &pane_id,
                                &new_session,
                                card_model,
                                card_thinking,
                                approval_mode,
                                None,
                                window,
                                cx,
                            );
                            let target_pane = this
                                .active_pane_id
                                .clone()
                                .unwrap_or_else(|| pane_id.clone());
                            this.transcript_for_pane(&target_pane).update(cx, |t, cx| {
                                t.push_message(optimistic_user_msg, cx);
                            });
                            // The server starts the run once the worktree exists;
                            // watch the launch state and attach when it lands.
                            this.watch_worktree_launch(&target_pane, new_session.id.clone(), cx);
                            cx.notify();
                        }
                        Err(error) => {
                            // Nothing was created; the card stays open with the
                            // prompt intact so a failed launch costs one retry.
                            this.floating_composer
                                .set_error(Some(format!("Unable to create a session: {error}")));
                            cx.notify();
                        }
                    }
                };
            cx.update(|cx| {
                if let Some(handle) = window_handle {
                    let _ = handle.update(cx, move |_, window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, move |this, cx| finish(this, Some(window), cx));
                        }
                    });
                } else if let Some(app) = entity.upgrade() {
                    app.update(cx, move |this, cx| finish(this, None, cx));
                }
            });
        })
        .detach();
    }

    /// Retry a failed worktree launch from the pane's banner. The server
    /// re-runs the stored payload; this re-arms the launch watcher.
    pub(crate) fn retry_worktree_launch(&mut self, session_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        self.clear_error_for_session(&session_id, cx);
        let pane_id = self
            .workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| {
                leaf.tabs
                    .iter()
                    .any(|t| t.id() == format!("chat:{session_id}"))
            })
            .map(|leaf| leaf.id.clone());
        cx.spawn(async move |entity, cx| {
            let result = client.sessions.retry_worktree_launch(&session_id).await;
            cx.update(|cx| {
                let Some(app) = entity.upgrade() else { return };
                app.update(cx, |this, cx| match (&result, pane_id) {
                    (Ok(()), Some(pane_id)) => {
                        this.watch_worktree_launch(&pane_id, session_id, cx);
                    }
                    (Err(error), _) => {
                        this.set_persistent_error_for_session(
                            &session_id,
                            format!("Worktree launch failed: {error}"),
                            cx,
                        );
                    }
                    (Ok(()), None) => {}
                });
            });
        })
        .detach();
    }

    /// Discard a failed worktree launch: delete the dead session (which
    /// closes its tab, drops its state, and removes it server-side).
    pub(crate) fn discard_worktree_launch(&mut self, session_id: String, cx: &mut Context<Self>) {
        self.clear_error_for_session(&session_id, cx);
        self.delete_session(session_id, cx);
    }

    /// Poll a pending worktree launch until it leaves provisioning. Ready →
    /// refresh the header (the worktree re-pointed the cwd) and attach to the
    /// auto-started run — the same reattach used when opening a session that
    /// already has a run. Failed → persistent error in the pane; its Retry
    /// and Discard render from the banner.
    fn watch_worktree_launch(&mut self, pane_id: &str, session_id: String, cx: &mut Context<Self>) {
        let client = self.client.clone();
        let pane_id = pane_id.to_string();
        cx.spawn(async move |entity, cx| {
            // The launch rarely outlives a few seconds (one `git worktree
            // add`); five minutes is a generous ceiling before giving up on
            // the attach — the session itself stays usable via refresh.
            for _ in 0..300 {
                tokio::time::sleep(std::time::Duration::from_millis(1000)).await;
                let Ok(detail) = client.sessions.get(&session_id, None).await else {
                    continue;
                };
                let state = detail.header.launch_state.clone();
                let launch_error = detail.header.launch_error.clone();
                let finished = cx.update(|cx| {
                    let Some(app) = entity.upgrade() else {
                        return true;
                    };

                    app.update(cx, |this, cx| match state.as_deref() {
                        Some("failed") => {
                            this.set_persistent_error_for_session(
                                &session_id,
                                format!(
                                    "Worktree launch failed: {}",
                                    launch_error.unwrap_or_default()
                                ),
                                cx,
                            );
                            true
                        }
                        Some(_) => false,
                        None => {
                            this.apply_session_header_for_pane(&pane_id, &detail.header, cx);
                            if this.active_session_for_pane(&pane_id).as_deref()
                                == Some(session_id.as_str())
                                && !detail.turns.is_empty()
                            {
                                this.transcript_for_pane(&pane_id).update(cx, |t, cx| {
                                    t.set_turns(detail.turns, cx);
                                });
                            }
                            this.attach_session_run_for_pane(
                                pane_id.clone(),
                                session_id.clone(),
                                None,
                                cx,
                            );
                            true
                        }
                    })
                });
                if finished {
                    return;
                }
            }
        })
        .detach();
    }
}
