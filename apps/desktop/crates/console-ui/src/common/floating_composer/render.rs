//! Frame assembly for [`super::FloatingComposer`].
//!
//! Keeps `mod.rs` about state — what is selected, which callbacks are
//! registered — while this file owns the per-frame work: cloning the snapshot
//! out, rebuilding the model dropdown, and re-bridging every view callback back
//! through a weak entity reference so a click updates the owner and notifies in
//! one place.

use gpui::{
    App, Context, ExternalPaths, FocusHandle, Focusable, IntoElement, Render, WeakEntity, Window,
    div,
};

use super::{FloatingComposer, FloatingComposerView, view};
use crate::common::ModelDropdownMenu;

impl FloatingComposer {
    /// Everything [`view`] needs to paint a frame, cloned out of `self` so the
    /// view stays a plain struct with no back-reference.
    fn view_data(&self, cx: &App) -> view::Data {
        view::Data {
            projects: self.projects.clone(),
            selected_project_id: self.selected_project_id.clone(),
            branches: self.branches.clone(),
            branch: self.branch.clone(),
            providers: self.providers.clone(),
            models_by_provider: self.models_by_provider.clone(),
            favorites: self.favorites.clone(),
            selected_model: self.selected_model.clone(),
            picker_tab: self.picker_tab.clone(),
            model_search_query: self.model_search.read(cx).content().to_string(),
            thinking_level: self.thinking_level,
            supported_thinking_levels: self.supported_thinking_levels.clone(),
            attachments: self.attachments.clone(),
            autocomplete: self.autocomplete.clone(),
            submitting: self.submitting,
            error: self.error.clone(),
        }
    }

    /// The model list is rebuilt every frame like the docked composer's, wired
    /// to this entity's own search field and callbacks.
    ///
    /// The tab change updates the card *and* the app: the card owns
    /// `picker_tab`, so without the local write the list would keep painting
    /// the previous tab; the app owns the model catalog, so without the
    /// callback a newly-added provider would show only its static fallback.
    fn model_menu_view(&self, this: WeakEntity<Self>, data: &view::Data) -> ModelDropdownMenu {
        let on_select = self.on_select_model.clone();
        let on_tab = self.on_picker_tab.clone();
        let on_favorite = self.on_favorite.clone();
        ModelDropdownMenu::new(
            data.providers.clone(),
            data.selected_model.clone(),
            data.picker_tab.clone(),
            data.favorites.clone(),
            data.models_by_provider.clone(),
            self.model_search.clone(),
            data.model_search_query.clone(),
            move |provider, model_id, _window, cx| {
                if let Some(cb) = on_select.clone() {
                    cb(console_core::SelectedModel { provider, model_id }, cx);
                }
            },
            move |tab, _window, cx| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| this.set_picker_tab(tab.clone(), cx));
                }
                if let Some(cb) = on_tab.clone() {
                    cb(tab, cx);
                }
            },
            move |provider, model_id, _window, cx| {
                if let Some(cb) = on_favorite.clone() {
                    cb(provider, model_id, cx);
                }
            },
        )
    }
}

impl Focusable for FloatingComposer {
    fn focus_handle(&self, cx: &App) -> FocusHandle {
        self.input.read(cx).focus()
    }
}

impl Render for FloatingComposer {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if !self.open {
            return div().into_any_element();
        }

        // One weak handle, cloned per callback: each view callback upgrades it,
        // so a click updates the owner and notifies exactly once. The two
        // branches differ only in element type, so both are boxed to
        // `AnyElement` rather than trying to unify them at the signature.
        let this = cx.entity().downgrade();
        let data = self.view_data(cx);
        let model_menu = self.model_menu_view(this.clone(), &data);

        FloatingComposerView::new(
            self.input.clone(),
            self.project_menu.clone(),
            self.branch_menu.clone(),
            self.model_menu.clone(),
            self.thinking_menu.clone(),
            model_menu,
        )
        .data(data)
        .on_dismiss({
            let this = this.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| this.on_scrim_click(cx));
                }
            }
        })
        .on_choose_project({
            let this = this.clone();
            let project_menu = self.project_menu.clone();
            move |id: String, window: &mut Window, cx: &mut App| {
                project_menu.close(window, cx);
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| this.choose_project(id, cx));
                }
            }
        })
        .on_choose_branch({
            let this = this.clone();
            let branch_menu = self.branch_menu.clone();
            move |choice: super::BranchChoice, window: &mut Window, cx: &mut App| {
                branch_menu.close(window, cx);
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| this.choose_branch(choice, cx));
                }
            }
        })
        .on_choose_model({
            let this = this.clone();
            // Close the popover so the chip underneath is actually visible —
            // the card re-renders the new model behind an open 400px list.
            let model_menu = self.model_menu.clone();
            move |provider: String, model_id: String, window: &mut Window, cx: &mut App| {
                model_menu.close(window, cx);
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        this.choose_model(console_core::SelectedModel { provider, model_id }, cx)
                    });
                }
            }
        })
        .on_choose_thinking({
            let this = this.clone();
            let thinking_menu = self.thinking_menu.clone();
            move |level: console_core::ThinkingLevel, window: &mut Window, cx: &mut App| {
                thinking_menu.close(window, cx);
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| this.choose_thinking(level, cx));
                }
            }
        })
        .on_submit({
            let this = this.clone();
            move |prompt: String, _window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| this.submit(prompt, cx));
                }
            }
        })
        .on_pick_image({
            let this = this.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_pick_image.clone() {
                            cb(cx);
                        }
                    });
                }
            }
        })
        .on_drop_files({
            let this = this.clone();
            move |paths: &ExternalPaths, window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_drop_files.clone() {
                            cb(paths, window, cx);
                        }
                    });
                }
            }
        })
        .on_remove_attachment({
            let this = this.clone();
            move |index: usize, _window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_remove_attachment.clone() {
                            cb(index, cx);
                        }
                    });
                }
            }
        })
        .on_preview_attachment({
            let this = this.clone();
            move |index: usize, _window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_preview_attachment.clone() {
                            cb(index, cx);
                        }
                    });
                }
            }
        })
        .on_autocomplete_next({
            let this = this.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_autocomplete_next.clone() {
                            cb(cx);
                        }
                    });
                }
            }
        })
        .on_autocomplete_previous({
            let this = this.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_autocomplete_previous.clone() {
                            cb(cx);
                        }
                    });
                }
            }
        })
        .on_autocomplete_confirm({
            let this = this.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_autocomplete_confirm.clone() {
                            cb(cx);
                        }
                    });
                }
            }
        })
        .on_autocomplete_dismiss({
            let this = this.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| {
                        if let Some(cb) = this.on_autocomplete_dismiss.clone() {
                            cb(cx);
                        }
                    });
                }
            }
        })
        .on_cancel({
            let this = this.clone();
            move |_window: &mut Window, cx: &mut App| {
                if let Some(this) = this.upgrade() {
                    this.update(cx, |this, cx| this.hide(cx));
                }
            }
        })
        .render(window, cx)
    }
}
