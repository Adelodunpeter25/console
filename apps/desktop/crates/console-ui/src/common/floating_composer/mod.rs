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
    GitBranchInfo, Model, ProjectInfo, ProviderCatalogEntry, SelectedModel, ThinkingLevel,
};
use gpui::{App, AppContext, Context, Entity, Window};

use crate::common::PickerTab;
use crate::input::{ComposerEvent, ComposerInput};
use crate::primitives::ContextMenuHandle;

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
    on_submit: Option<Rc<dyn Fn(FloatingSubmit, &mut App) + 'static>>,
    on_select_project: Option<Rc<dyn Fn(String, &mut App) + 'static>>,
    on_select_model: Option<Rc<dyn Fn(SelectedModel, &mut App) + 'static>>,
    on_select_thinking: Option<Rc<dyn Fn(ThinkingLevel, &mut App) + 'static>>,
    on_picker_tab: Option<Rc<dyn Fn(PickerTab, &mut App) + 'static>>,
    on_favorite: Option<Rc<dyn Fn(String, String, &mut App) + 'static>>,
}

impl FloatingComposer {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let input = cx.new(|cx| ComposerInput::new(window, cx).placeholder("Describe a task…"));
        let model_search = cx.new(|cx| ComposerInput::new(window, cx).search_field());

        // Enter is the card's submit key, so reuse the input's own Submit
        // event rather than binding a second handler to the same key.
        // The subscription lives as long as the entity, so dropping the handle
        // here is correct — gpui keeps it alive.
        let _subscription = cx.subscribe(&input, |this, _input, event: &ComposerEvent, cx| {
            if let ComposerEvent::Submit(text, _) = event {
                this.submit(text.clone(), cx);
            }
        });

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
            on_submit: None,
            on_select_project: None,
            on_select_model: None,
            on_select_thinking: None,
            on_picker_tab: None,
            on_favorite: None,
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
        self.open = true;

        self.input.update(cx, |input, cx| {
            input.clear(cx);
            input.set_context_files(Vec::new());
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

    /// Replace the project/branch lists when the project changes, so the branch
    /// menu never offers refs from the previously selected project.
    pub fn set_project_context(
        &mut self,
        project_id: Option<String>,
        branches: Rc<Vec<GitBranchInfo>>,
        cx: &mut Context<Self>,
    ) {
        self.selected_project_id = project_id;
        self.branches = branches;
        // An existing-branch choice can't survive a project switch.
        if matches!(self.branch, BranchChoice::FromBranch(_)) {
            self.branch = BranchChoice::default();
        }
        cx.notify();
    }

    /// Enter, or the Create button. Requires a project and a non-empty prompt.
    pub(super) fn submit(&mut self, text: String, cx: &mut Context<Self>) {
        if !self.open || self.submitting || text.trim().is_empty() {
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
        let submit = FloatingSubmit {
            prompt: text,
            project_id: Some(project_id),
            cwd,
            branch: self.branch.clone(),
            model: self.selected_model.clone(),
            thinking_level: self.thinking_level,
        };
        self.submitting = true;
        self.error = None;
        cx.notify();
        if let Some(on_submit) = self.on_submit.clone() {
            on_submit(submit, cx);
        }
    }

    pub(super) fn choose_project(&mut self, id: String, cx: &mut Context<Self>) {
        self.selected_project_id = Some(id.clone());
        cx.notify();
        if let Some(cb) = self.on_select_project.clone() {
            cb(id, cx);
        }
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
