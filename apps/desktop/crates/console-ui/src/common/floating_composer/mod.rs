//! Floating composer — a launcher card for describing a new task and spinning
//! up the session that runs it.
//!
//! Deliberately independent of the docked pane [`crate::common::ComposerView`]:
//! it has no run lifecycle (nothing is running until you press Enter), and puts
//! its selectors in a header rather than a footer. Every visual is borrowed from
//! the shared primitives — `MenuChip`, `dropdown_menu`, `popover`,
//! `ModelDropdownMenu` — so it tracks the docked composer without sharing any of
//! its state.
//!
//! **This is a plain struct, not a `Render` entity**, and that is load-bearing.
//! Its popup and chip row are derived from app state that changes every frame,
//! so they have to be computed while the app is mid-render. An `Entity` child
//! would be leased by gpui at the same moment the app is computing that data,
//! and gpui refuses a read of an already-leased entity — the card would have to
//! choose between a stale popup and a panic. Holding the state on the app and
//! building [`FloatingComposerView`] in the app's render, like every other
//! surface in the app, sidesteps the conflict entirely.

mod view;

pub use view::{Data as FloatingComposerData, FloatingComposerView};

use std::collections::{HashMap, HashSet};
use std::rc::Rc;

use console_core::{
    ApprovalMode, GitBranchInfo, ImageAttachment, Model, ProjectInfo, ProviderCatalogEntry,
    SelectedModel, ThinkingLevel,
};
use gpui::{App, AppContext, Entity, KeyBinding, Window};

use crate::common::{AutocompleteContentKey, AutocompleteView, PickerTab};
use crate::input::{ComposerInput, ComposerMention};
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
    /// An existing branch, worked on in the project folder itself: it is
    /// checked out first (a no-op when it already is), like the footer's
    /// branch picker. It never creates a worktree.
    FromBranch(String),
}

impl BranchChoice {
    /// Only "New worktree" provisions a worktree; every other choice runs in
    /// the project folder.
    pub fn creates_worktree(&self) -> bool {
        matches!(self, Self::NewWorktree)
    }

    /// The existing branch to check out in the project folder before the
    /// session starts, if one was picked.
    pub fn checkout_target(&self) -> Option<&str> {
        match self {
            Self::FromBranch(name) => Some(name),
            _ => None,
        }
    }

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
    /// How much the agent may do unattended. The card seeds this from the
    /// active pane and the user can widen or narrow it before launching, so a
    /// throwaway chat doesn't inherit "Full access" from whatever was focused.
    pub approval_mode: ApprovalMode,
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
    pub approval_mode: ApprovalMode,
}

impl FloatingSnapshot {
    /// The empty snapshot, for the card's default field values before the app
    /// has pushed a real one.
    pub fn empty() -> Self {
        Self {
            projects: Rc::new(Vec::new()),
            selected_project_id: None,
            branches: Rc::new(Vec::new()),
            providers: Rc::new(Vec::new()),
            models_by_provider: Rc::new(HashMap::new()),
            favorites: Rc::new(HashSet::new()),
            selected_model: None,
            thinking_level: None,
            supported_thinking_levels: Vec::new(),
            approval_mode: ApprovalMode::default(),
        }
    }
}

/// The launcher's state. Lives on the app entity, which builds
/// [`FloatingComposerView`] from it during the app's own render.
pub struct FloatingComposerState {
    pub input: Entity<ComposerInput>,
    /// Filter field owned by the model list. Held here so the query survives the
    /// per-frame rebuild of the dropdown.
    pub model_search: Entity<ComposerInput>,
    pub project_menu: ContextMenuHandle,
    pub branch_menu: ContextMenuHandle,
    pub model_menu: ContextMenuHandle,
    pub thinking_menu: ContextMenuHandle,
    pub approval_menu: ContextMenuHandle,
    pub picker_tab: PickerTab,
    pub open: bool,
    pub submitting: bool,
    pub error: Option<String>,
    pub projects: Rc<Vec<ProjectInfo>>,
    pub selected_project_id: Option<String>,
    pub branches: Rc<Vec<GitBranchInfo>>,
    pub branch: BranchChoice,
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub models_by_provider: Rc<HashMap<String, Vec<Model>>>,
    pub favorites: Rc<HashSet<String>>,
    pub selected_model: Option<SelectedModel>,
    pub thinking_level: Option<ThinkingLevel>,
    pub supported_thinking_levels: Vec<ThinkingLevel>,
    /// Approval mode for the session this launch creates. Seeded from the
    /// active pane on open and overridable in the card, so the mode is chosen
    /// alongside the branch rather than inherited silently.
    pub approval_mode: ApprovalMode,
    /// Staged images, mirrored from the app so the card can paint chips.
    pub attachments: Rc<Vec<ImageAttachment>>,
    /// The @-file / slash-command popup for the prompt field, if any.
    pub autocomplete: Option<AutocompleteView>,
    /// Content identity of `autocomplete`, so a per-frame rebuild that yields
    /// the same popup doesn't churn the card.
    pub autocomplete_key: Option<AutocompleteContentKey>,
}

impl FloatingComposerState {
    pub fn new(window: &mut Window, cx: &mut App) -> Self {
        let input = cx.new(|cx| ComposerInput::new(window, cx).placeholder("Describe a task…"));
        let model_search = cx.new(|cx| ComposerInput::new(window, cx).search_field());

        Self {
            input,
            model_search,
            project_menu: ContextMenuHandle::new(cx),
            branch_menu: ContextMenuHandle::new(cx),
            model_menu: ContextMenuHandle::new(cx),
            thinking_menu: ContextMenuHandle::new(cx),
            approval_menu: ContextMenuHandle::new(cx),
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
            approval_mode: ApprovalMode::default(),
            attachments: Rc::new(Vec::new()),
            autocomplete: None,
            autocomplete_key: None,
        }
    }

    /// Push an app-state snapshot in and show the card. Called fresh on every
    /// open so the defaults track whatever the user last had selected.
    pub fn show(&mut self, snapshot: FloatingSnapshot, window: &mut Window, cx: &mut App) {
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
            approval_mode,
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
        self.approval_mode = approval_mode;
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
    }

    /// Close the card. The caller notifies — this is app state, not an entity.
    pub fn hide(&mut self) {
        self.open = false;
        self.submitting = false;
        self.error = None;
    }

    /// Replace the staged-image list the card paints.
    ///
    /// Idempotent: the app re-derives attachments every frame, so writing an
    /// equal list must not count as a change. An empty list is compared by
    /// length rather than by `Rc` identity, because the store hands back a
    /// fresh `Rc` for "no attachments" each time.
    pub fn set_attachments(&mut self, attachments: Rc<Vec<ImageAttachment>>) -> bool {
        let unchanged = self.attachments.len() == attachments.len()
            && (attachments.is_empty() || Rc::ptr_eq(&self.attachments, &attachments));
        if unchanged {
            return false;
        }
        self.attachments = attachments;
        true
    }

    /// Replace the popup. Returns whether it actually changed, so the caller
    /// can skip a notify. `None` compares equal to `None`, which is the steady
    /// state before typing.
    pub fn set_autocomplete(&mut self, autocomplete: Option<AutocompleteView>) -> bool {
        let next_key = autocomplete.as_ref().map(AutocompleteView::content_key);
        if next_key == self.autocomplete_key {
            return false;
        }
        self.autocomplete_key = next_key;
        self.autocomplete = autocomplete;
        true
    }

    /// Re-seed the thinking controls. Called when the chosen model changes,
    /// since which levels exist depends on the model. A level the new model
    /// doesn't support falls back to its first one.
    pub fn set_thinking_context(
        &mut self,
        level: Option<ThinkingLevel>,
        supported: Vec<ThinkingLevel>,
    ) {
        self.thinking_level = match level {
            Some(current) if supported.contains(&current) => Some(current),
            _ => supported.first().copied(),
        };
        self.supported_thinking_levels = supported;
    }

    /// The project the card currently targets, used to scope @-file search.
    pub fn selected_project(&self) -> Option<&ProjectInfo> {
        let id = self.selected_project_id.as_deref()?;
        self.projects.iter().find(|project| project.id == id)
    }

    /// One frame of state for the view, cloned out so the view stays a plain
    /// struct with no back-reference to the card.
    ///
    /// `model_search_query` is passed in rather than read here so this doesn't
    /// need an `App`: the caller already has a context and reads that field
    /// once for the model dropdown.
    pub fn view_data(&self, model_search_query: String) -> FloatingComposerData {
        FloatingComposerData {
            projects: self.projects.clone(),
            selected_project_id: self.selected_project_id.clone(),
            branches: self.branches.clone(),
            branch: self.branch.clone(),
            providers: self.providers.clone(),
            models_by_provider: self.models_by_provider.clone(),
            favorites: self.favorites.clone(),
            selected_model: self.selected_model.clone(),
            picker_tab: self.picker_tab.clone(),
            model_search_query,
            thinking_level: self.thinking_level,
            supported_thinking_levels: self.supported_thinking_levels.clone(),
            approval_mode: self.approval_mode,
            attachments: self.attachments.clone(),
            autocomplete: self.autocomplete.clone(),
            submitting: self.submitting,
            error: self.error.clone(),
        }
    }

    /// Enter, or the Create button. Requires a project and a non-empty prompt.
    ///
    /// Enter is claimed by autocomplete while its popup is open — accepting a
    /// highlighted suggestion is the expected meaning there, so submitting on
    /// Enter would launch a session with a half-typed `@query`.
    pub fn submit(&mut self, text: String, cx: &mut App) -> Option<FloatingSubmit> {
        if !self.open || self.submitting || self.autocomplete.is_some() {
            return None;
        }
        if text.trim().is_empty() {
            return None;
        }
        let project_id = self.selected_project_id.clone()?;
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
            approval_mode: self.approval_mode,
        };
        self.submitting = true;
        self.error = None;
        self.autocomplete = None;
        self.autocomplete_key = None;
        Some(submit)
    }

    /// Show an error inside the card. The card stays open so the prompt the
    /// user typed isn't lost to a failed request.
    pub fn set_error(&mut self, error: Option<String>) {
        self.error = error;
        self.submitting = false;
    }

    pub fn choose_project(&mut self, id: String) {
        self.selected_project_id = Some(id);
        // An existing-branch base belongs to the previous project, so drop it
        // here rather than waiting for a fresh branch list.
        if matches!(self.branch, BranchChoice::FromBranch(_)) {
            self.branch = BranchChoice::default();
        }
    }

    /// Hand the card a freshly fetched branch list, for after a project switch.
    /// The project itself is already set by `choose_project`; this only
    /// delivers the refs, so the branch menu stops offering the previous
    /// project's branches.
    pub fn set_branches(&mut self, branches: Rc<Vec<GitBranchInfo>>) {
        self.branches = branches;
        // The chosen base may not exist in the new project.
        if let BranchChoice::FromBranch(name) = &self.branch
            && !self.branches.iter().any(|b| &b.name == name)
        {
            self.branch = BranchChoice::default();
        }
    }

    /// Mirror the app's live model catalog into the card.
    ///
    /// The card keeps its own copies because it is plain app state rather than
    /// a view over the app, and [`Self::show`] snapshots them only when it
    /// opens. Without this the picker keeps rendering that snapshot — the
    /// static catalog on first open, and whatever was cached at that moment
    /// afterwards — because `load_models_for_provider` and
    /// `toggle_model_favorite` write back to the app, never to the card.
    ///
    /// Idempotent: the app hands out the same `Rc` until it actually mutates,
    /// so an unchanged catalog costs three pointer comparisons. Returns whether
    /// anything changed, so the caller can skip a notify.
    pub fn set_model_catalog(
        &mut self,
        providers: Rc<Vec<ProviderCatalogEntry>>,
        models_by_provider: Rc<HashMap<String, Vec<Model>>>,
        favorites: Rc<HashSet<String>>,
    ) -> bool {
        let mut changed = false;
        if !Rc::ptr_eq(&self.providers, &providers) {
            self.providers = providers;
            changed = true;
        }
        if !Rc::ptr_eq(&self.models_by_provider, &models_by_provider) {
            self.models_by_provider = models_by_provider;
            changed = true;
        }
        if !Rc::ptr_eq(&self.favorites, &favorites) {
            self.favorites = favorites;
            changed = true;
        }
        changed
    }

    /// The model picker's tab changed. The card owns the active tab, so it has
    /// to record it — otherwise the list keeps rendering the previous tab and
    /// provider switching looks dead. The search box is cleared on switch to
    /// match the docked picker, where a filter from the old tab would otherwise
    /// hide the new provider's models.
    pub fn set_picker_tab(&mut self, tab: PickerTab, cx: &mut App) {
        self.picker_tab = tab;
        self.model_search.update(cx, |input, cx| input.clear(cx));
    }
}
