//! Staged browser element annotations: chips above the composer, mirroring
//! image attachments (`attachments.rs`) but kept in a parallel map so the
//! structured fields survive drafts, history recall, and queue runs.

use std::rc::Rc;

use console_core::BrowserElementAnnotation;
use gpui::Context;

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    pub(crate) fn annotations_for_pane(&self, pane_id: &str) -> Rc<[BrowserElementAnnotation]> {
        self.browser_annotations
            .get(pane_id)
            .cloned()
            .unwrap_or_else(|| Rc::from([]))
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
                .insert(pane_id.to_string(), Rc::from(items));
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
            Some(existing) => super::types::rc_slice_edit(existing, |v| v.append(&mut items)),
            None => {
                self.browser_annotations
                    .insert(pane_id.to_string(), Rc::from(items));
            }
        }
    }

    pub(crate) fn remove_annotation(
        &mut self,
        pane_id: &str,
        index: usize,
        cx: &mut Context<Self>,
    ) {
        if let Some(staged) = self.browser_annotations.get_mut(pane_id)
            && index < staged.len()
        {
            let removed = {
                let mut vec = staged.to_vec();
                let r = vec.remove(index);
                *staged = Rc::from(vec);
                r
            };
            let _ = removed;
            if staged.is_empty() {
                self.browser_annotations.remove(pane_id);
            }
            self.save_pane_draft(pane_id, cx);
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
                console_core::WorkspaceTabConfig::Browser { browser_id: id, .. } => {
                    id == browser_id
                }
                _ => false,
            })
        };
        if let Some(active) = &self.active_pane_id
            && leaves.iter().any(|leaf| &leaf.id == active && hosts(leaf))
        {
            return Some(active.clone());
        }
        leaves
            .iter()
            .find(|leaf| hosts(leaf))
            .map(|leaf| leaf.id.clone())
    }

    /// Stage an annotation posted by a browser view's popover (Attach).
    /// Routes directly to the active chat composer so the chip is immediately
    /// staged. Returns true when the chip landed on a chat composer — the
    /// browser then leaves keyboard focus to the app instead of refocusing
    /// the page.
    pub(crate) fn stage_browser_annotation(
        &mut self,
        browser_id: &str,
        annotation: BrowserElementAnnotation,
        cx: &mut Context<Self>,
    ) -> bool {
        // The chat this annotation belongs to: the selected chat, or — while a
        // browser tab is in front — the workspace's only chat, so annotation
        // chips land in that composer just like file comments do.
        let target = self
            .focused_chat()
            .map(|(p, s)| (p.to_string(), s.to_string()));
        let Some((pane_id, session_id)) = target else {
            // No chat to route to: the legacy fallback — the browser keeps its
            // own keyboard focus, nothing to focus.
            let fallback = self
                .pane_hosting_browser(browser_id)
                .unwrap_or_else(|| "pane-main".to_string());
            self.append_annotations_for_pane(&fallback, vec![annotation]);
            self.save_pane_draft(&fallback, cx);
            cx.notify();
            return false;
        };
        self.append_annotations_for_pane(&pane_id, vec![annotation]);
        self.save_pane_draft(&pane_id, cx);
        cx.notify();
        // Focus stays in the browser webview after Attach, so the prompt the
        // user types next would vanish into the page. Move it to the chat
        // composer the chip was staged on — but only when that chat is the
        // pane's visible tab; otherwise the composer is not on screen.
        let chat_tab_id = format!("chat:{session_id}");
        let chat_visible = self
            .workspace_root
            .find_leaf(&pane_id)
            .and_then(|leaf| leaf.active_tab_id.as_deref())
            .is_some_and(|active| active == chat_tab_id);
        if !chat_visible {
            return false;
        }
        let target_pane_id = pane_id.clone();
        let entity = cx.entity().downgrade();
        cx.defer(move |cx| {
            let Some(window) = cx.active_window() else {
                return;
            };
            let _ = window.update(cx, |_, window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        // Both the GPUI focus chain and the input's own
                        // cursor/blink state need to know.
                        let composer = this.composer_for_pane(&target_pane_id);
                        let focus = composer.read(cx).focus();
                        window.focus(&focus, cx);
                        composer.update(cx, |input, _cx| input.focus());
                    });
                }
            });
        });
        true
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
                .unwrap_or_else(|| Rc::from([]));
            self.set_annotations_for_pane(pane_id, parked.to_vec());
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
    ) -> Rc<[console_ui::ComposerAnnotation]> {
        let entity = cx.entity().downgrade();
        let chips: Vec<_> = self
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
                    (None, None) => console_shared::format::format_selector_label(&ann.selector),
                };
                let full_target = match (&ann.component_name, &ann.source_location) {
                    (Some(c), Some(s)) => format!("<{c} />  •  {s}"),
                    (Some(c), None) => format!("<{c} />"),
                    (None, Some(s)) => s.clone(),
                    (None, None) => ann.selector.clone(),
                };
                let tooltip = if ann.user_comment.trim().is_empty() {
                    full_target
                } else {
                    format!("{full_target}\n{}", ann.user_comment.trim())
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
        Rc::from(chips)
    }
}
