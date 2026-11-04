//! File line comments: pick a range in the editor's comment gutter, write a
//! note, and keep it staged in the pane's composer.
//!
//! `editor-ui` owns the gutter interaction and reports a `CommentTarget`
//! through a plain callback — it has no app context — so the handler pushes
//! the request into a channel and a spawned task opens the inline card. The
//! card is rendered by the file viewer as an overlay anchored under the
//! selected rows; the same comment is mirrored as a chip above the floating
//! composer's input.

use console_ui::input::ComposerEvent;
use console_ui::{
    ComposerComment, FileCommentCard, FileCommentCardState, FileCommentOverlay,
};
use gpui::{App, AppContext as _, Context, Window};
use std::rc::Rc;

use super::app::ConsoleDesktopApp;
use super::types::FileCommentRequest;
use crate::types::workspace::{FileComment, FileCommentState};

impl ConsoleDesktopApp {
    /// Start draining gutter picks. Each request opens a draft card in the
    /// pane that hosts the file.
    pub(crate) fn init_file_comments(
        &mut self,
        mut requests: tokio::sync::mpsc::UnboundedReceiver<FileCommentRequest>,
        cx: &mut Context<Self>,
    ) {
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            while let Some(request) = requests.recv().await {
                let _ = cx.update(|cx| {
                    let Some(handle) = cx.active_window() else {
                        return;
                    };
                    let _ = handle.update(cx, |_, window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.open_file_comment(request, window, cx)
                            });
                        }
                    });
                });
            }
        })
        .detach();
    }

    /// Resolve which pane currently shows `path`, preferring the focused pane.
    fn pane_hosting_file(&self, path: &str) -> String {
        let leaves = self.workspace_root.leaves();
        let shows_path = |leaf: &&console_core::LeafPaneNode| {
            leaf.tabs.iter().any(
                |tab| matches!(tab, console_core::WorkspaceTabConfig::File { path: tab_path, .. } if tab_path == path),
            )
        };
        if let Some(active) = &self.active_pane_id {
            if leaves
                .iter()
                .any(|leaf| &leaf.id == active && shows_path(&leaf))
            {
                return active.clone();
            }
        }
        leaves
            .iter()
            .find(|leaf| shows_path(leaf))
            .map(|leaf| leaf.id.clone())
            .or_else(|| self.active_pane_id.clone())
            .unwrap_or_else(|| "pane-main".to_string())
    }

    /// The file currently active in `pane_id`, if the pane is showing one.
    fn pane_active_file_path(&self, pane_id: &str) -> Option<String> {
        self.workspace_root
            .leaves()
            .into_iter()
            .find(|leaf| leaf.id == pane_id)
            .and_then(|leaf| {
                let active_id = leaf.active_tab_id.as_deref()?;
                leaf.tabs
                    .iter()
                    .find(|tab| tab.id() == active_id)
                    .and_then(|tab| match tab {
                        console_core::WorkspaceTabConfig::File { path, .. } => Some(path.clone()),
                        _ => None,
                    })
            })
    }

    /// Project-relative path shown in prompts, as the agent expects.
    fn comment_display_path(&self, pane_id: &str, path: &str) -> String {
        self.selected_project_for_pane(pane_id)
            .map(|project| project.path.clone())
            .and_then(|root| {
                std::path::Path::new(path)
                    .strip_prefix(root)
                    .ok()
                    .map(|path| path.to_string_lossy().into_owned())
            })
            .unwrap_or_else(|| path.to_string())
    }

    fn comment_lines(start_line: u32, end_line: u32) -> String {
        if start_line == end_line {
            start_line.to_string()
        } else {
            format!("{start_line}–{end_line}")
        }
    }

    fn open_file_comment(
        &mut self,
        request: FileCommentRequest,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let pane_id = self.pane_hosting_file(&request.path);
        let Some(pane) = self.workspace_pane_states.get(&pane_id) else {
            return;
        };

        // Selecting a new range replaces any uncommitted draft, but never a
        // comment the user already staged.
        let stale_draft = pane
            .file_comments
            .iter()
            .position(|comment| comment.state == FileCommentState::Draft)
            .map(|index| (index, pane.file_comments[index].path.clone()));
        let staged_exists = pane.file_comments.iter().any(|comment| {
            comment.state != FileCommentState::Draft
                && comment.path == request.path
                && comment.start_line == request.start_line
                && comment.end_line == request.end_line
        });
        if staged_exists {
            return;
        }

        let id = pane.next_file_comment_id;
        let input = cx.new(|cx| {
            console_ui::ComposerInput::new(window, cx)
                .placeholder("Add a comment for the AI...")
        });

        let submit_pane = pane_id.clone();
        let submit_id = id;
        let subscription = cx.subscribe(
            &input,
            move |this, _input, event: &ComposerEvent, cx| match event {
                ComposerEvent::Submit(_, _) => {
                    this.commit_file_comment(&submit_pane, submit_id, cx)
                }
                ComposerEvent::Edited | ComposerEvent::Focus => cx.notify(),
                _ => {}
            },
        );

        let Some(pane) = self.workspace_pane_states.get_mut(&pane_id) else {
            return;
        };
        if let Some((index, _)) = stale_draft {
            pane.file_comments.remove(index);
        }
        pane.next_file_comment_id = id + 1;
        pane.file_comments.push(FileComment {
            id,
            path: request.path.clone(),
            start_line: request.start_line,
            end_line: request.end_line,
            text: String::new(),
            input,
            state: FileCommentState::Draft,
            focus_pending: true,
            _subscription: subscription,
        });

        self.sync_comment_selections(&pane_id, &request.path, cx);
        if let Some((_, old_path)) = stale_draft {
            if old_path != request.path {
                self.sync_comment_selections(&pane_id, &old_path, cx);
            }
        }
        cx.notify();
    }

    /// Add/Save the card's current input as the comment body.
    pub(crate) fn commit_file_comment(
        &mut self,
        pane_id: &str,
        comment_id: u64,
        cx: &mut Context<Self>,
    ) {
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        let Some(comment) = pane
            .file_comments
            .iter_mut()
            .find(|comment| comment.id == comment_id)
        else {
            return;
        };
        let text = comment.input.read(cx).content().trim().to_owned();
        if text.is_empty() {
            return;
        }
        let path = comment.path.clone();
        comment.text = text;
        comment.state = FileCommentState::Added;
        comment.focus_pending = false;
        comment.input.update(cx, |input, cx| input.clear(cx));
        self.sync_comment_selections(pane_id, &path, cx);
        cx.notify();
    }

    /// Escape from a draft drops it; Escape from editing returns the card to
    /// its committed text.
    pub(crate) fn cancel_file_comment(
        &mut self,
        pane_id: &str,
        comment_id: u64,
        cx: &mut Context<Self>,
    ) {
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        let Some(index) = pane
            .file_comments
            .iter()
            .position(|comment| comment.id == comment_id)
        else {
            return;
        };
        let path = pane.file_comments[index].path.clone();
        match pane.file_comments[index].state {
            FileCommentState::Draft => {
                pane.file_comments.remove(index);
            }
            FileCommentState::Editing => {
                pane.file_comments[index].state = FileCommentState::Added;
                pane.file_comments[index].focus_pending = false;
            }
            FileCommentState::Added => {}
        }
        self.sync_comment_selections(pane_id, &path, cx);
        cx.notify();
    }

    /// Reopen a staged comment in the inline card.
    pub(crate) fn edit_file_comment(
        &mut self,
        pane_id: &str,
        comment_id: u64,
        cx: &mut Context<Self>,
    ) {
        let Some(path) = self
            .workspace_pane_states
            .get(pane_id)
            .and_then(|pane| {
                pane.file_comments
                    .iter()
                    .find(|comment| comment.id == comment_id)
                    .map(|comment| comment.path.clone())
            })
        else {
            return;
        };
        // A chip in the chat composer has no inline card to focus; bring the
        // file forward first so Edit lands somewhere visible.
        if self.pane_active_file_path(pane_id).as_deref() != Some(path.as_str()) {
            self.open_file_tab_in_pane(pane_id, path, cx);
        }
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        let Some(comment) = pane
            .file_comments
            .iter_mut()
            .find(|comment| comment.id == comment_id)
        else {
            return;
        };
        if comment.state == FileCommentState::Draft {
            return;
        }
        comment.state = FileCommentState::Editing;
        comment.focus_pending = true;
        let text = comment.text.clone();
        comment
            .input
            .update(cx, |input, cx| input.set_content(text, cx));
        cx.notify();
    }

    /// Remove a comment from the message (and clear its line highlight).
    pub(crate) fn remove_file_comment(
        &mut self,
        pane_id: &str,
        comment_id: u64,
        cx: &mut Context<Self>,
    ) {
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        let Some(index) = pane
            .file_comments
            .iter()
            .position(|comment| comment.id == comment_id)
        else {
            return;
        };
        let path = pane.file_comments.remove(index).path;
        self.sync_comment_selections(pane_id, &path, cx);
        cx.notify();
    }

    /// Mirror the pane's comment ranges into the editor's persistent
    /// highlight. The editor also records the range on mouse-up; this keeps
    /// cancel/remove and reopened tabs in sync.
    fn sync_comment_selections(&mut self, pane_id: &str, path: &str, cx: &mut Context<Self>) {
        let ranges = self
            .workspace_pane_states
            .get(pane_id)
            .map(|pane| {
                pane.file_comments
                    .iter()
                    .filter(|comment| comment.path == path)
                    .map(|comment| editor_ui::CommentTarget {
                        start_line: comment.start_line,
                        end_line: comment.end_line,
                    })
                    .collect::<Vec<_>>()
            })
            .unwrap_or_default();
        if let Some((_, _, state, _)) = self.viewer_editor_views.get(path) {
            state.update(cx, |editor, cx| {
                editor.set_comment_selections(ranges);
                cx.notify();
            });
        }
    }

    /// The committed comments as prompt blocks. A comment-only send is valid
    /// and carries only these blocks.
    pub(crate) fn staged_comment_prompt(&self, pane_id: &str) -> String {
        let Some(pane) = self.workspace_pane_states.get(pane_id) else {
            return String::new();
        };
        let mut blocks = Vec::new();
        for comment in &pane.file_comments {
            if comment.state == FileCommentState::Draft {
                continue;
            }
            let text = comment.text.trim();
            if text.is_empty() {
                continue;
            }
            let path = self.comment_display_path(pane_id, &comment.path);
            let lines = Self::comment_lines(comment.start_line, comment.end_line);
            blocks.push(console_shared::composer::file_comments::format_file_comment(
                &path, &lines, text,
            ));
        }
        blocks.join("\n\n")
    }

    /// Merge the typed prompt with any staged comments and consume them, the
    /// way a send consumes mentions. Comments sit after the user's text so the
    /// instruction reads first. Nothing is consumed when the result is empty.
    pub(crate) fn prompt_with_staged_comments(
        &mut self,
        pane_id: &str,
        prompt: &str,
        cx: &mut Context<Self>,
    ) -> String {
        let comments = self.staged_comment_prompt(pane_id);
        let prompt = prompt.trim();
        let merged = match (prompt.is_empty(), comments.is_empty()) {
            (true, _) => comments.clone(),
            (_, true) => prompt.to_string(),
            _ => format!("{prompt}\n\n{comments}"),
        };
        if !comments.is_empty() && !merged.trim().is_empty() {
            self.clear_staged_comments(pane_id, cx);
        }
        merged
    }

    /// Drop every committed comment after it has been sent. Drafts stay.
    fn clear_staged_comments(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        let mut paths = Vec::new();
        pane.file_comments.retain(|comment| {
            if comment.state == FileCommentState::Draft {
                return true;
            }
            if !paths.contains(&comment.path) {
                paths.push(comment.path.clone());
            }
            false
        });
        for path in paths {
            self.sync_comment_selections(pane_id, &path, cx);
        }
        cx.notify();
    }

    /// Chips above the pane composer's input. Only committed comments appear;
    /// a draft still has its inline card and is not part of a message yet.
    pub(crate) fn composer_comments_for_pane(
        &self,
        pane_id: &str,
        cx: &mut Context<Self>,
    ) -> Rc<Vec<ComposerComment>> {
        let Some(pane) = self.workspace_pane_states.get(pane_id) else {
            return Rc::new(Vec::new());
        };
        let entity = cx.entity().downgrade();
        let comments = pane
            .file_comments
            .iter()
            .filter(|comment| comment.state != FileCommentState::Draft)
            .map(|comment| {
                let edit_entity = entity.clone();
                let edit_pane = pane_id.to_string();
                let edit_id = comment.id;
                let remove_entity = entity.clone();
                let remove_pane = pane_id.to_string();
                let remove_id = comment.id;
                ComposerComment::new(
                    comment.id,
                    comment.path.clone(),
                    comment.label(),
                    move |_window, cx| {
                        if let Some(app) = edit_entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.edit_file_comment(&edit_pane, edit_id, cx);
                            });
                        }
                    },
                    move |_window, cx| {
                        if let Some(app) = remove_entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.remove_file_comment(&remove_pane, remove_id, cx);
                            });
                        }
                    },
                )
            })
            .collect();
        Rc::new(comments)
    }

    /// Cards for the active file, anchored under their ranges by `FileViewer`.
    pub(crate) fn file_comment_cards(
        &mut self,
        pane_id: &str,
        path: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Vec<FileCommentOverlay> {
        let entity = cx.entity().downgrade();
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return Vec::new();
        };
        let mut overlays = Vec::new();
        for comment in pane
            .file_comments
            .iter_mut()
            .filter(|comment| comment.path == path)
        {
            if comment.focus_pending {
                comment.focus_pending = false;
                let focus = comment.input.read(cx).focus();
                window.on_next_frame(move |window, cx| window.focus(&focus, cx));
            }

            let id = comment.id;
            let start_line = comment.start_line;
            let end_line = comment.end_line;
            let card_state = match comment.state {
                FileCommentState::Draft => FileCommentCardState::Draft,
                FileCommentState::Added => FileCommentCardState::Added,
                FileCommentState::Editing => FileCommentCardState::Editing,
            };

            let on_commit = {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_window: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.commit_file_comment(&pane_id, id, cx)
                        });
                    }
                }
            };
            let on_cancel = {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_window: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.cancel_file_comment(&pane_id, id, cx)
                        });
                    }
                }
            };
            let on_edit = {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_window: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.edit_file_comment(&pane_id, id, cx));
                    }
                }
            };
            let on_remove = {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_window: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.remove_file_comment(&pane_id, id, cx)
                        });
                    }
                }
            };

            let card = FileCommentCard::new(
                id,
                comment.path.clone(),
                start_line,
                end_line,
                card_state,
                comment.text.clone(),
                comment.input.clone(),
                on_commit,
                on_cancel,
                on_edit,
                on_remove,
            );
            overlays.push(FileCommentOverlay::new(start_line, end_line, card));
        }
        overlays
    }
}
