//! App-side wiring for the floating composer launcher.
//!
//! ⌘N opens the card with a snapshot of the current model / thinking defaults;
//! submitting it creates the session the user described and opens it in the
//! active pane, exactly like the old direct `create_new_chat` did — the only
//! difference is that project, branch and model are chosen in the card rather
//! than inherited from whichever pane happened to be focused.

use std::rc::Rc;

use console_core::{CreateSessionDto, CreateWorktreeSpec, SelectedModel, ThinkingLevel};
use console_ui::common::{BranchChoice, FloatingSnapshot, FloatingSubmit, PickerTab};
use gpui::{ClipboardEntry, Context, ExternalPaths, Focusable, Window};

use super::ConsoleDesktopApp;
use super::autocomplete::FLOATING_AUTOCOMPLETE_KEY;

/// Reserved key for the card's staged images. The attachment store is keyed by
/// owner rather than strictly by pane, so the card claims a key of its own and
/// never collides with a real pane id.
const FLOATING_KEY: &str = "__floating_composer__";

impl ConsoleDesktopApp {
    /// ⌘N — open the launcher instead of creating a session immediately.
    /// Re-invoking while it's open just re-focuses the prompt.
    pub fn open_floating_composer(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.floating_composer.read(cx).is_open() {
            let handle = self.floating_composer.read(cx).focus_handle(cx);
            window.focus(&handle, cx);
            return;
        }

        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let project_id = self
            .pane_project_id(&pane_id)
            .or_else(|| self.selected_project_id.clone());

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
        };

        let app = cx.entity().downgrade();
        self.floating_composer.update(cx, |composer, cx| {
            // Callbacks take `&mut App` and reach the app through a weak
            // handle, so a stale card can never resurrect a dead session.
            composer.set_on_submit(
                {
                    let app = app.clone();
                    move |submit: FloatingSubmit, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.create_session_from_floating(submit, cx)
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_select_project(
                {
                    let app = app.clone();
                    move |project_id: String, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.floating_project_changed(project_id, cx)
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_select_model(
                {
                    let app = app.clone();
                    move |model: SelectedModel, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| this.floating_model_changed(model, cx));
                        }
                    }
                },
                cx,
            );
            composer.set_on_select_thinking(
                {
                    let app = app.clone();
                    move |level: ThinkingLevel, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, _cx| this.thinking_level = Some(level));
                        }
                    }
                },
                cx,
            );
            composer.set_on_picker_tab(
                {
                    let app = app.clone();
                    move |tab: PickerTab, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| this.floating_picker_tab_changed(tab, cx));
                        }
                    }
                },
                cx,
            );
            composer.set_on_favorite(
                {
                    let app = app.clone();
                    move |provider: String, model_id: String, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.toggle_model_favorite(provider, model_id, cx)
                            });
                        }
                    }
                },
                cx,
            );
            // Attachments reuse the pane-keyed stores under a reserved key, so
            // the same stage/remove/preview code serves both surfaces.
            composer.set_on_pick_image(
                {
                    let app = app.clone();
                    move |cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| this.pick_image(FLOATING_KEY, cx));
                        }
                    }
                },
                cx,
            );
            composer.set_on_paste_attachments(
                {
                    let app = app.clone();
                    move |entries: Vec<ClipboardEntry>, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.stage_clipboard_attachments(FLOATING_KEY, entries, cx)
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_drop_files(
                {
                    let app = app.clone();
                    move |paths: &ExternalPaths, window: &mut Window, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.stage_dropped_files(FLOATING_KEY, paths, window, cx)
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_remove_attachment(
                {
                    let app = app.clone();
                    move |index: usize, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.remove_attachment(FLOATING_KEY, index, cx)
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_preview_attachment(
                {
                    let app = app.clone();
                    move |index: usize, cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.preview_attachment(FLOATING_KEY, index, cx)
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_autocomplete_next(
                {
                    let app = app.clone();
                    move |cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.move_autocomplete_for_owner(
                                    FLOATING_AUTOCOMPLETE_KEY,
                                    true,
                                    cx,
                                )
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_autocomplete_previous(
                {
                    let app = app.clone();
                    move |cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.move_autocomplete_for_owner(
                                    FLOATING_AUTOCOMPLETE_KEY,
                                    false,
                                    cx,
                                )
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_autocomplete_confirm(
                {
                    let app = app.clone();
                    let composer = self.floating_composer.clone();
                    move |cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                let input = composer.read(cx).input().clone();
                                this.accept_highlighted_autocomplete_for_owner(
                                    FLOATING_AUTOCOMPLETE_KEY,
                                    &input,
                                    cx,
                                )
                            });
                        }
                    }
                },
                cx,
            );
            composer.set_on_autocomplete_dismiss(
                {
                    let app = app.clone();
                    move |cx: &mut gpui::App| {
                        if let Some(app) = app.upgrade() {
                            app.update(cx, |this, cx| {
                                this.dismiss_autocomplete_for_owner(FLOATING_AUTOCOMPLETE_KEY, cx)
                            });
                        }
                    }
                },
                cx,
            );
            composer.show(snapshot, window, cx);
        });
    }

    /// Rebuild the card's derived state each frame: the @-file / slash-command
    /// popup follows the prompt's caret, and the chip row follows the shared
    /// attachment store (a paste stages asynchronously, so it can't be pushed
    /// at the moment it happens).
    ///
    /// Called from the app's render. Does nothing while the card is closed.
    ///
    /// The card is written through `cx.defer` rather than inline: this runs
    /// inside a render pass, and mutating an entity mid-render is not a place
    /// to be borrowing. The defer lands before the next paint, so the popup
    /// still appears on the frame after the keystroke.
    ///
    /// Both setters are idempotent — this runs on every frame, and an
    /// unconditional write would notify the card, which would schedule another
    /// frame, which would come back here. That loop never idles and eventually
    /// exhausts memory.
    pub fn refresh_floating_composer(&mut self, window: &Window, cx: &mut Context<Self>) {
        if !self.floating_composer.read(cx).is_open() {
            return;
        }

        let input = self.floating_composer.read(cx).input().clone();
        // File search is scoped to the card's selected project, which changes
        // before the app's own `selected_project_id` catches up.
        let project_root = self
            .floating_composer
            .read(cx)
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
        let composer = self.floating_composer.clone();
        cx.defer(move |cx| {
            composer.update(cx, |composer, cx| {
                composer.set_autocomplete(autocomplete, cx);
                composer.set_attachments(attachments, cx);
            });
        });
    }

    /// A different project was picked in the card. The card has already
    /// updated its own selection, so this only needs to refetch that project's
    /// branches and push them back in.
    fn floating_project_changed(&mut self, project_id: String, cx: &mut Context<Self>) {
        self.refresh_branches_for_project(&project_id, cx);
    }

    /// A different model was picked: which thinking levels exist depends on the
    /// model, so re-seed the card's thinking controls.
    ///
    /// Deferred because this runs from inside the composer's own update (a chip
    /// click → its callback → here), and re-entering the composer to mutate it
    /// mid-update is not safe.
    fn floating_model_changed(&mut self, model: SelectedModel, cx: &mut Context<Self>) {
        let supported = self.supported_thinking_levels_for_model(Some(&model));
        let current = self.thinking_level;
        let composer = self.floating_composer.clone();
        cx.defer(move |cx| {
            composer.update(cx, |composer, cx| {
                composer.set_thinking_context(current, supported, cx)
            });
        });
    }

    /// Switching provider tabs may need that provider's live model list, which
    /// the static catalog doesn't carry. The card has already recorded the new
    /// tab, so this is only the fetch.
    fn floating_picker_tab_changed(&mut self, tab: PickerTab, cx: &mut Context<Self>) {
        if let PickerTab::Provider(name) = &tab {
            self.load_models_for_provider(name, cx);
        }
        cx.notify();
    }

    /// Refetch the branch list for `project_id`, storing it on the app and
    /// pushing it into the open card.
    ///
    /// Mirrors the pane-scoped loaders, minus the per-pane bookkeeping: the
    /// card is the only reader while it is open. The card is only touched once
    /// the fetch resolves, which also keeps it out of the composer's update.
    fn refresh_branches_for_project(&mut self, project_id: &str, cx: &mut Context<Self>) {
        let Some(project) = self.projects.iter().find(|p| p.id == project_id) else {
            return;
        };
        let path = project.path.clone();
        let client = self.client.clone();
        let app = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            let Ok(result) = client.git.list_branches(Some(&path)).await else {
                return;
            };
            cx.update(|cx| {
                if let Some(app) = app.upgrade() {
                    app.update(cx, |this, cx| {
                        this.branches = Rc::new(result.branches);
                        this.branch_loaded = true;
                        this.branch_is_git_repository = result.is_git_repository;
                        this.floating_composer.update(cx, |composer, cx| {
                            composer.set_branches(this.branches.clone(), cx)
                        });
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Create the session the launcher described, then open it in the active
    /// pane. Mirrors `create_new_chat`'s post-create bookkeeping.
    pub fn create_session_from_floating(&mut self, submit: FloatingSubmit, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let client = self.client.clone();
        let approval_mode = self.pane_approval_mode(&pane_id);
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
        // The card's choices, not the pane's: `submit_prompt_with_context`
        // reads model/thinking off the pane, so they are pushed in below
        // before the run starts or the agent would use whatever the focused
        // pane happened to have.
        let card_model = submit.model.clone();
        let card_thinking = submit.thinking_level;

        // Only a worktree request provisions anything; a plain project session
        // sends no worktree key at all.
        let worktree = match &submit.branch {
            BranchChoice::Project => None,
            // Server auto-derives a slugged branch name.
            BranchChoice::NewWorktree => Some(CreateWorktreeSpec::default()),
            // The picked branch names the new worktree's branch.
            BranchChoice::FromBranch(base) => Some(CreateWorktreeSpec {
                branch: Some(base.clone()),
            }),
        };

        cx.spawn(async move |entity, cx| {
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
                        this.floating_composer
                            .update(cx, |composer, cx| composer.hide(cx));
                        this.save_transcript_scroll_position(cx);
                        this.apply_session_header_for_pane(&pane_id, &new_session, cx);
                        this.clear_error_for_pane(&pane_id, cx);
                        if this.active_pane_id.as_deref() == Some(pane_id.as_str()) {
                            this.selected_session_id = Some(new_session.id.clone());
                        }
                        Rc::make_mut(&mut this.sessions).insert(0, new_session.clone());
                        this.open_chat_tab_in_pane(&pane_id, new_session.id.clone(), &title);
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
                        let message = format!("Unable to create a session: {error}");
                        this.floating_composer
                            .update(cx, |composer, cx| composer.set_error(Some(message), cx));
                    }
                });
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
