//! Staged browser element annotations: chips above the composer, mirroring
//! image attachments (`attachments.rs`) but kept in a parallel map so the
//! structured fields survive drafts, history recall, and queue runs.

use std::rc::Rc;

use console_core::BrowserElementAnnotation;
use gpui::{Context, Focusable, Window};

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    pub(crate) fn annotations_for_pane(
        &self,
        pane_id: &str,
    ) -> Rc<Vec<BrowserElementAnnotation>> {
        self.browser_annotations
            .get(pane_id)
            .cloned()
            .unwrap_or_else(|| Rc::new(Vec::new()))
    }

    pub(crate) fn set_annotations_for_pane(
        &mut self,
        pane_id: &str,
        items: Vec<BrowserElementAnnotation>,
    ) {
        if items.is_empty() {
            self.browser_annotations.remove(pane_id);
        } else {
            self.browser_annotations
                .insert(pane_id.to_string(), Rc::new(items));
        }
    }

    pub(crate) fn append_annotations_for_pane(
        &mut self,
        pane_id: &str,
        mut items: Vec<BrowserElementAnnotation>,
    ) {
        if items.is_empty() {
            return;
        }
        match self.browser_annotations.get_mut(pane_id) {
            Some(existing) => Rc::make_mut(existing).append(&mut items),
            None => {
                self.browser_annotations
                    .insert(pane_id.to_string(), Rc::new(items));
            }
        }
    }

    pub(crate) fn remove_annotation(&mut self, pane_id: &str, index: usize, cx: &mut Context<Self>) {
        if let Some(staged) = self.browser_annotations.get_mut(pane_id) {
            if index < staged.len() {
                let removed = Rc::make_mut(staged).remove(index);
                let _ = removed;
                if staged.is_empty() {
                    self.browser_annotations.remove(pane_id);
                }
                self.schedule_drafts_save(cx);
                cx.notify();
            }
        }
    }

    #[allow(dead_code)]
    pub(crate) fn clear_annotations_for_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        if self.browser_annotations.remove(pane_id).is_some() {
            self.schedule_drafts_save(cx);
            cx.notify();
        }
    }

    /// Which pane currently shows `browser_id`, preferring the focused pane.
    /// Used to route an annotation from a workspace browser tab to the
    /// composer that stages it.
    fn pane_hosting_browser(&self, browser_id: &str) -> Option<String> {
        let leaves = self.workspace_root.leaves();
        let hosts = |leaf: &&console_core::LeafPaneNode| {
            leaf.tabs.iter().any(|tab| match tab {
                console_core::WorkspaceTabConfig::Browser {
                    browser_id: id,
                    ..
                } => id == browser_id,
                _ => false,
            })
        };
        if let Some(active) = &self.active_pane_id {
            if leaves
                .iter()
                .any(|leaf| &leaf.id == active && hosts(leaf))
            {
                return Some(active.clone());
            }
        }
        leaves
            .iter()
            .find(|leaf| hosts(leaf))
            .map(|leaf| leaf.id.clone())
    }

    /// Stage an annotation posted by a browser view's popover (Attach).
    pub(crate) fn stage_browser_annotation(
        &mut self,
        browser_id: &str,
        annotation: BrowserElementAnnotation,
        cx: &mut Context<Self>,
    ) {
        let Some(pane_id) = self.pane_hosting_browser(browser_id) else {
            return;
        };
        self.append_annotations_for_pane(&pane_id, vec![annotation]);
        self.schedule_drafts_save(cx);
        cx.notify();
    }

    pub(crate) fn restore_recalled_annotations(
        &mut self,
        pane_id: &str,
        message_id: Option<&str>,
        is_draft: bool,
        cx: &mut Context<Self>,
    ) {
        if is_draft {
            let parked = self
                .history_draft_annotations
                .remove(pane_id)
                .unwrap_or_else(|| Rc::new(Vec::new()));
            self.set_annotations_for_pane(pane_id, (*parked).clone());
            cx.notify();
            return;
        }
        if !self.history_draft_annotations.contains_key(pane_id) {
            let staged = self.annotations_for_pane(pane_id);
            self.history_draft_annotations
                .insert(pane_id.to_string(), staged);
        }
        let Some(message_id) = message_id else {
            return;
        };
        let Some(annotations) = self
            .transcript_for_pane(pane_id)
            .read(cx)
            .annotations_for_message(message_id)
        else {
            return;
        };
        self.set_annotations_for_pane(pane_id, annotations);
        cx.notify();
    }

    pub(crate) fn composer_annotations_for_pane(
        &self,
        pane_id: &str,
        cx: &mut Context<Self>,
    ) -> Rc<Vec<console_ui::ComposerAnnotation>> {
        let entity = cx.entity().downgrade();
        let chips = self
            .annotations_for_pane(pane_id)
            .iter()
            .enumerate()
            .map(|(index, ann)| {
                let remove_entity = entity.clone();
                let remove_pane = pane_id.to_string();
                let label = match (&ann.component_name, &ann.source_location) {
                    (Some(c), Some(s)) => format!("<{c} />  •  {s}"),
                    (Some(c), None) => format!("<{c} />"),
                    (None, Some(s)) => s.clone(),
                    (None, None) => ann.selector.clone(),
                };
                let tooltip = if ann.user_comment.trim().is_empty() {
                    label.clone()
                } else {
                    format!("{label}\n{}", ann.user_comment.trim())
                };
                console_ui::ComposerAnnotation::new(
                    ann.id.clone(),
                    label,
                    tooltip,
                    move |_window, cx| {
                        if let Some(app) = remove_entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.remove_annotation(&remove_pane, index, cx);
                            });
                        }
                    },
                )
            })
            .collect();
        Rc::new(chips)
    }

    #[allow(dead_code)]
    pub(crate) fn focus_annotation_input(
        &mut self,
        browser_id: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if let Some(view) = self.browser_views.get(browser_id).cloned() {
            window.focus(&view.read(cx).focus_handle(cx), cx);
        }
    }
}
