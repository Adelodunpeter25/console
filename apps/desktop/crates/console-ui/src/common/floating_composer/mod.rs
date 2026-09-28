//! Floating composer — a launcher card for describing a new task and spinning
//! up the session that runs it.
//!
//! Deliberately independent of the docked pane [`crate::common::ComposerView`]:
//! it owns its own [`ComposerInput`], has no run lifecycle (nothing is running
//! until you press Enter), and puts its selectors in a header rather than a
//! footer. Every visual is borrowed from the shared primitives — `MenuChip`,
//! `dropdown_menu`, `popover`, `ModelDropdownMenu` — so it tracks the docked
//! composer without sharing any of its state and without costing a change to
//! it.
//!
//! This file owns *state*: what's selected and which callbacks are registered.
//! [`render`] turns that into a frame, and [`view`] paints it.

mod render;
mod view;

pub use view::FloatingComposerView;

use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use console_core::{
    GitBranchInfo, ImageAttachment, Model, ProjectInfo, ProviderCatalogEntry, SelectedModel,
    ThinkingLevel,
};
use gpui::{App, AppContext, ClipboardEntry, Context, Entity, ExternalPaths, KeyBinding, Window};

use crate::common::{AutocompleteContentKey, AutocompleteView, PickerTab};
use crate::input::{ComposerAttachmentPaste, ComposerEvent, ComposerInput, ComposerMention};
use crate::primitives::ContextMenuHandle;

/// The card's own key context. Only Escape is declared here: while a suggestion
/// popup is open, the prompt field's autocomplete context wins first and
/// dismisses the popup instead of closing the card. Suggestion navigation needs
/// no bindings of its own — `autocomplete::init` already binds down/up/enter/tab
/// for any `ComposerInput`.
pub(crate) const CONTEXT: &str = "FloatingComposer";

gpui::actions!(floating_composer, [Cancel]);

/// Register the card's key bindings. Called once at app init, like every other
/// surface's bindings.
pub fn init(cx: &mut App) {
    let context: Option<&str> = Some(CONTEXT);
    cx.bind_keys([KeyBinding::new("escape", Cancel, context)]);
}

/// Where the new session's checkout comes from.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub enum BranchChoice {
    /// Run directly in the project directory — no worktree.
    #[default]
    Project,
    /// A fresh worktree + branch, auto-named by the server.
    NewWorktree,
    /// A fresh worktree cut from an existing branch.
    FromBranch(String),
}

impl BranchChoice {
    /// Label for the footer's branch chip.
    pub fn label(&self) -> String {
        match self {
            Self::Project => "Project branch".to_owned(),
            Self::NewWorktree => "New worktree".to_owned(),
            Self::FromBranch(name) => name.clone(),
        }
    }
}

/// Everything the app needs to create the session the user described.
#[derive(Clone, Default)]
pub struct FloatingSubmit {
    pub prompt: String,
    /// The prompt's inline @-file mentions, carried onto the new session so the
    /// chips the user assembled aren't flattened into plain text.
    pub mentions: Vec<ComposerMention>,
    /// Staged images, carried onto the new session so the launch doesn't
    /// silently drop a pasted screenshot.
    pub attachments: Rc<Vec<ImageAttachment>>,
    pub project_id: Option<String>,
    pub cwd: Option<String>,
    pub branch: BranchChoice,
    pub model: Option<SelectedModel>,
    pub thinking_level: Option<ThinkingLevel>,
}

/// The app-state snapshot pushed into the card on open.
pub struct FloatingSnapshot {
    pub projects: Rc<Vec<ProjectInfo>>,
    pub selected_project_id: Option<String>,
    pub branches: Rc<Vec<GitBranchInfo>>,
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub models_by_provider: Rc<HashMap<String, Vec<Model>>>,
    pub favorites: Rc<HashSet<String>>,
    pub selected_model: Option<SelectedModel>,
    pub thinking_level: Option<ThinkingLevel>,
    pub supported_thinking_levels: Vec<ThinkingLevel>,
}

/// The launcher card. Owns its input and its dropdown handles; the app pushes a
/// snapshot of app state in on open and receives a [`FloatingSubmit`] on Enter.
pub struct FloatingComposer {
    input: Entity<ComposerInput>,
    /// Filter field owned by the model list. Lives here so the query survives
    /// the per-frame rebuild of the dropdown.
    model_search: Entity<ComposerInput>,
    project_menu: ContextMenuHandle,
    branch_menu: ContextMenuHandle,
    model_menu: ContextMenuHandle,
    thinking_menu: ContextMenuHandle,
    picker_tab: PickerTab,
    open: bool,
    submitting: bool,
    error: Option<String>,
    projects: Rc<Vec<ProjectInfo>>,
    selected_project_id: Option<String>,
    branches: Rc<Vec<GitBranchInfo>>,
    branch: BranchChoice,
    providers: Rc<Vec<ProviderCatalogEntry>>,
    models_by_provider: Rc<HashMap<String, Vec<Model>>>,
    favorites: Rc<HashSet<String>>,
    selected_model: Option<SelectedModel>,
    thinking_level: Option<ThinkingLevel>,
    supported_thinking_levels: Vec<ThinkingLevel>,
    /// Staged images, mirrored from the app so the card can paint chips.
    attachments: Rc<Vec<ImageAttachment>>,
    /// The @-file / slash-command popup for the prompt field, if any.
    autocomplete: Option<AutocompleteView>,
    /// Content identity of `autocomplete`, so a per-frame rebuild that yields
    /// the same popup doesn't notify and keep the app rendering forever.
    autocomplete_key: Option<AutocompleteContentKey>,
    on_submit: Option<Rc<dyn Fn(FloatingSubmit, &mut App) + 'static>>,
    on_select_project: Option<Rc<dyn Fn(String, &mut App) + 'static>>,
    on_select_model: Option<Rc<dyn Fn(SelectedModel, &mut App) + 'static>>,
    on_select_thinking: Option<Rc<dyn Fn(ThinkingLevel, &mut App) + 'static>>,
    on_picker_tab: Option<Rc<dyn Fn(PickerTab, &mut App) + 'static>>,
    on_favorite: Option<Rc<dyn Fn(String, String, &mut App) + 'static>>,
    on_pick_image: Option<Rc<dyn Fn(&mut App) + 'static>>,
    on_paste_attachments: Option<Rc<dyn Fn(Vec<ClipboardEntry>, &mut App) + 'static>>,
    on_drop_files: Option<Rc<dyn Fn(&ExternalPaths, &mut Window, &mut App) + 'static>>,
    on_remove_attachment: Option<Rc<dyn Fn(usize, &mut App) + 'static>>,
    on_preview_attachment: Option<Rc<dyn Fn(usize, &mut App) + 'static>>,
    on_autocomplete_next: Option<Rc<dyn Fn(&mut App) + 'static>>,
    on_autocomplete_previous: Option<Rc<dyn Fn(&mut App) + 'static>>,
    on_autocomplete_confirm: Option<Rc<dyn Fn(&mut App) + 'static>>,
    on_autocomplete_dismiss: Option<Rc<dyn Fn(&mut App) + 'static>>,
}

impl FloatingComposer {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let input = cx.new(|cx| ComposerInput::new(window, cx).placeholder("Describe a task…"));
        let model_search = cx.new(|cx| ComposerInput::new(window, cx).search_field());

        // Enter is the card's submit key, so reuse the input's own Submit
        // event rather than binding a second handler to the same key.
        // The subscriptions live as long as the entity, so dropping the handles
        // here is correct — gpui keeps them alive.
        let _subscription = cx.subscribe(&input, |this, _input, event: &ComposerEvent, cx| {
            match event {
                ComposerEvent::Submit(text, _) => this.submit(text.clone(), cx),
                // Typing and focus changes repaint the card, which owns the
                // autocomplete popup and the enabled/disabled state of Create.
                ComposerEvent::Edited | ComposerEvent::Focus => cx.notify(),
                // Backspace on an empty field removes the last staged image, the
                // same affordance the docked composer offers.
                ComposerEvent::BackspaceOnEmpty => {
                    let count = this.attachments.len();
                    if count > 0 {
                        if let Some(cb) = this.on_remove_attachment.clone() {
                            cb(count - 1, cx);
                        }
                    }
                }
                _ => {}
            }
        });
        let _paste_subscription = cx.subscribe(
            &input,
            |this, _input, event: &ComposerAttachmentPaste, cx| {
                if let Some(cb) = this.on_paste_attachments.clone() {
                    cb(event.0.clone(), cx);
                }
            },
        );

        Self {
            input,
            model_search,
            project_menu: ContextMenuHandle::new(cx),
            branch_menu: ContextMenuHandle::new(cx),
            model_menu: ContextMenuHandle::new(cx),
            thinking_menu: ContextMenuHandle::new(cx),
            picker_tab: PickerTab::Favorites,
            open: false,
            submitting: false,
            error: None,
            projects: Rc::new(Vec::new()),
            selected_project_id: None,
            branches: Rc::new(Vec::new()),
            branch: BranchChoice::default(),
            providers: Rc::new(Vec::new()),
            models_by_provider: Rc::new(HashMap::new()),
            favorites: Rc::new(HashSet::new()),
            selected_model: None,
            thinking_level: None,
            supported_thinking_levels: Vec::new(),
            attachments: Rc::new(Vec::new()),
            autocomplete: None,
            autocomplete_key: None,
            on_submit: None,
            on_select_project: None,
            on_select_model: None,
            on_select_thinking: None,
            on_picker_tab: None,
            on_favorite: None,
            on_pick_image: None,
            on_paste_attachments: None,
            on_drop_files: None,
            on_remove_attachment: None,
            on_preview_attachment: None,
            on_autocomplete_next: None,
            on_autocomplete_previous: None,
            on_autocomplete_confirm: None,
            on_autocomplete_dismiss: None,
        }
    }

    /// Push an app-state snapshot in and show the card. Called fresh on every
    /// open so the defaults track whatever the user last had selected.
    pub fn show(
        &mut self,
        snapshot: FloatingSnapshot,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let FloatingSnapshot {
            projects,
            selected_project_id,
            branches,
            providers,
            models_by_provider,
            favorites,
            selected_model,
            thinking_level,
            supported_thinking_levels,
        } = snapshot;

        self.projects = projects;
        self.selected_project_id = selected_project_id;
        self.branches = branches;
        self.providers = providers;
        self.models_by_provider = models_by_provider;
        self.favorites = favorites;
        self.selected_model = selected_model;
        self.thinking_level = thinking_level;
        self.supported_thinking_levels = supported_thinking_levels;
        // An existing-branch base is a per-launch decision, not a sticky
        // preference — reset it rather than inheriting the last one.
        self.branch = BranchChoice::default();
        self.submitting = false;
        self.error = None;
        self.autocomplete = None;
        self.autocomplete_key = None;
        self.attachments = Rc::new(Vec::new());
        self.open = true;

        self.input.update(cx, |input, cx| {
            input.clear(cx);
            input.set_context_files(Vec::new());
            input.set_prompt_history(Vec::new(), cx);
        });
        window.focus(&self.input.read(cx).focus(), cx);
        cx.notify();
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.open = false;
        self.submitting = false;
        self.error = None;
        cx.notify();
    }

    pub fn is_open(&self) -> bool {
        self.open
    }

    pub fn set_on_submit(
        &mut self,
        callback: impl Fn(FloatingSubmit, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_submit = Some(Rc::new(callback));
    }

    pub fn set_on_select_project(
        &mut self,
        callback: impl Fn(String, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_select_project = Some(Rc::new(callback));
    }

    pub fn set_on_select_model(
        &mut self,
        callback: impl Fn(SelectedModel, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_select_model = Some(Rc::new(callback));
    }

    pub fn set_on_select_thinking(
        &mut self,
        callback: impl Fn(ThinkingLevel, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_select_thinking = Some(Rc::new(callback));
    }

    /// Switching provider tabs may need that provider's live model list, which
    /// only the app can fetch.
    pub fn set_on_picker_tab(
        &mut self,
        callback: impl Fn(PickerTab, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_picker_tab = Some(Rc::new(callback));
    }

    pub fn set_on_favorite(
        &mut self,
        callback: impl Fn(String, String, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_favorite = Some(Rc::new(callback));
    }

    /// Mark the card busy while the create request is in flight.
    pub fn set_submitting(&mut self, submitting: bool, cx: &mut Context<Self>) {
        self.submitting = submitting;
        cx.notify();
    }

    /// Show an error inside the card. The card stays open so the prompt the
    /// user typed isn't lost to a failed request.
    pub fn set_error(&mut self, error: Option<String>, cx: &mut Context<Self>) {
        self.error = error;
        self.submitting = false;
        cx.notify();
    }

    /// The model picker's tab changed. The card owns the active tab, so it has
    /// to record it — otherwise the list keeps rendering the previous tab and
    /// provider switching looks dead. The search box is cleared on switch to
    /// match the docked picker, where a filter from the old tab would otherwise
    /// hide the new provider's models.
    pub fn set_picker_tab(&mut self, tab: PickerTab, cx: &mut Context<Self>) {
        self.picker_tab = tab;
        self.model_search.update(cx, |input, cx| input.clear(cx));
        cx.notify();
    }

    pub fn set_on_pick_image(
        &mut self,
        callback: impl Fn(&mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_pick_image = Some(Rc::new(callback));
    }

    /// Clipboard images / file paths pasted into the prompt.
    pub fn set_on_paste_attachments(
        &mut self,
        callback: impl Fn(Vec<ClipboardEntry>, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_paste_attachments = Some(Rc::new(callback));
    }

    pub fn set_on_drop_files(
        &mut self,
        callback: impl Fn(&ExternalPaths, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_drop_files = Some(Rc::new(callback));
    }

    pub fn set_on_remove_attachment(
        &mut self,
        callback: impl Fn(usize, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_remove_attachment = Some(Rc::new(callback));
    }

    pub fn set_on_preview_attachment(
        &mut self,
        callback: impl Fn(usize, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_preview_attachment = Some(Rc::new(callback));
    }

    pub fn set_on_autocomplete_next(
        &mut self,
        callback: impl Fn(&mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_autocomplete_next = Some(Rc::new(callback));
    }

    pub fn set_on_autocomplete_previous(
        &mut self,
        callback: impl Fn(&mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_autocomplete_previous = Some(Rc::new(callback));
    }

    pub fn set_on_autocomplete_confirm(
        &mut self,
        callback: impl Fn(&mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_autocomplete_confirm = Some(Rc::new(callback));
    }

    pub fn set_on_autocomplete_dismiss(
        &mut self,
        callback: impl Fn(&mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_autocomplete_dismiss = Some(Rc::new(callback));
    }

    /// Replace the staged-image list the card paints.
    ///
    /// Idempotent: the app re-derives attachments every frame, so writing an
    /// equal list must not notify or the card would keep itself dirty forever.
    /// An empty list is compared by length rather than by `Rc` identity, because
    /// the store hands back a fresh `Rc` for "no attachments" each time.
    pub fn set_attachments(
        &mut self,
        attachments: Rc<Vec<ImageAttachment>>,
        cx: &mut Context<Self>,
    ) {
        let unchanged = self.attachments.len() == attachments.len()
            && (attachments.is_empty() || Rc::ptr_eq(&self.attachments, &attachments));
        if unchanged {
            return;
        }
        self.attachments = attachments;
        cx.notify();
    }

    /// Push the current @-file / slash-command popup, rebuilt each frame by the
    /// app from the prompt's caret position.
    ///
    /// Idempotent for the same reason as [`Self::set_attachments`]: the popup is
    /// rebuilt every frame, so only a change in its *content* may notify. `None`
    /// compares equal to `None`, which is the steady state before typing.
    pub fn set_autocomplete(
        &mut self,
        autocomplete: Option<AutocompleteView>,
        cx: &mut Context<Self>,
    ) {
        let next_key = autocomplete.as_ref().map(AutocompleteView::content_key);
        if next_key == self.autocomplete_key {
            return;
        }
        self.autocomplete_key = next_key;
        self.autocomplete = autocomplete;
        cx.notify();
    }

    /// Re-seed the thinking controls. Called when the chosen model changes,
    /// since which levels exist depends on the model. A level the new model
    /// doesn't support falls back to its first one.
    pub fn set_thinking_context(
        &mut self,
        level: Option<ThinkingLevel>,
        supported: Vec<ThinkingLevel>,
        cx: &mut Context<Self>,
    ) {
        self.thinking_level = match level {
            Some(current) if supported.contains(&current) => Some(current),
            _ => supported.first().copied(),
        };
        self.supported_thinking_levels = supported;
        cx.notify();
    }

    /// The prompt field, so the app can drive the card's autocomplete against
    /// it the same way it does for a pane's composer.
    pub fn input(&self) -> &Entity<ComposerInput> {
        &self.input
    }

    /// The project the card currently targets, used to scope @-file search.
    pub fn selected_project(&self) -> Option<&ProjectInfo> {
        let id = self.selected_project_id.as_deref()?;
        self.projects.iter().find(|project| project.id == id)
    }

    /// Hand the card a freshly fetched branch list, for after a project switch.
    /// The project itself is already set by `choose_project`; this only
    /// delivers the refs, so the branch menu stops offering the previous
    /// project's branches.
    pub fn set_branches(&mut self, branches: Rc<Vec<GitBranchInfo>>, cx: &mut Context<Self>) {
        self.branches = branches;
        // The chosen base may not exist in the new project.
        if let BranchChoice::FromBranch(name) = &self.branch {
            if !self.branches.iter().any(|b| &b.name == name) {
                self.branch = BranchChoice::default();
            }
        }
        cx.notify();
    }

    /// Enter, or the Create button. Requires a project and a non-empty prompt.
    ///
    /// Enter is claimed by autocomplete while its popup is open — accepting a
    /// highlighted suggestion is the expected meaning there, so submitting on
    /// Enter would launch a session with a half-typed `@query`.
    pub(super) fn submit(&mut self, text: String, cx: &mut Context<Self>) {
        if !self.open || self.submitting || self.autocomplete.is_some() {
            return;
        }
        if text.trim().is_empty() {
            return;
        }
        let Some(project_id) = self.selected_project_id.clone() else {
            self.error = Some("Pick a project first".into());
            cx.notify();
            return;
        };
        let cwd = self
            .projects
            .iter()
            .find(|p| p.id == project_id)
            .map(|p| p.path.clone());
        // Mentions and staged images are read here, before the field is
        // cleared, so the created session inherits what the user attached.
        let mentions = self.input.read(cx).mentions().to_vec();
        let submit = FloatingSubmit {
            prompt: text,
            mentions,
            attachments: self.attachments.clone(),
            project_id: Some(project_id),
            cwd,
            branch: self.branch.clone(),
            model: self.selected_model.clone(),
            thinking_level: self.thinking_level,
        };
        self.submitting = true;
        self.error = None;
        self.autocomplete = None;
        cx.notify();
        if let Some(on_submit) = self.on_submit.clone() {
            on_submit(submit, cx);
        }
    }

    pub(super) fn choose_project(&mut self, id: String, cx: &mut Context<Self>) {
        self.selected_project_id = Some(id.clone());
        // An existing-branch base belongs to the previous project, so drop it
        // here rather than waiting for the app to push a new branch list.
        if matches!(self.branch, BranchChoice::FromBranch(_)) {
            self.branch = BranchChoice::default();
        }
        cx.notify();
        if let Some(cb) = self.on_select_project.clone() {
            cb(id, cx);
        }
    }

    pub(super) fn choose_branch(&mut self, choice: BranchChoice, cx: &mut Context<Self>) {
        self.branch = choice;
        cx.notify();
    }

    pub(super) fn choose_model(&mut self, model: SelectedModel, cx: &mut Context<Self>) {
        self.selected_model = Some(model.clone());
        cx.notify();
        if let Some(cb) = self.on_select_model.clone() {
            cb(model, cx);
        }
    }

    pub(super) fn choose_thinking(&mut self, level: ThinkingLevel, cx: &mut Context<Self>) {
        self.thinking_level = Some(level);
        cx.notify();
        if let Some(cb) = self.on_select_thinking.clone() {
            cb(level, cx);
        }
    }

    /// A backdrop click closes the card. The choice is local to the card, so
    /// there is no app round-trip — same as the command palette.
    pub(super) fn on_scrim_click(&mut self, cx: &mut Context<Self>) {
        self.hide(cx);
    }
}
