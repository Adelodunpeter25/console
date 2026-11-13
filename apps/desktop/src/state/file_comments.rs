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
use console_ui::{ComposerComment, FileCommentCard, FileCommentCardState, FileCommentOverlay};
use gpui::{App, AppContext as _, Context, IntoElement as _, Window};
use std::rc::Rc;

use super::app::ConsoleDesktopApp;
use super::types::FileCommentRequest;
use crate::types::workspace::{FileComment, FileCommentState};
use console_proto::{CreateFileCommentRequest, FileCommentSide};

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
                            app.update(cx, |this, cx| this.open_file_comment(request, window, cx));
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
                |tab| matches!(tab, console_core::WorkspaceTabConfig::File { path: tab_path, .. } | console_core::WorkspaceTabConfig::Diff { path: tab_path, .. } if tab_path == path),
            )
        };
        if let Some(active) = &self.active_pane_id
            && leaves
                .iter()
                .any(|leaf| &leaf.id == active && shows_path(leaf))
        {
            return active.clone();
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
                        console_core::WorkspaceTabConfig::File { path, .. }
                        | console_core::WorkspaceTabConfig::Diff { path, .. } => Some(path.clone()),
                        _ => None,
                    })
            })
    }

    /// Project-relative path shown in prompts, as the agent expects.
    pub(crate) fn comment_display_path(&self, pane_id: &str, path: &str) -> String {
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

    /// The comment card's input plus the subscription that turns Enter into a
    /// commit. The subscription is owned by the comment it belongs to.
    pub(super) fn new_comment_input(
        &mut self,
        pane_id: &str,
        id: u64,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> (gpui::Entity<console_ui::ComposerInput>, gpui::Subscription) {
        let input = cx.new(|cx| {
            console_ui::ComposerInput::new(window, cx).placeholder("Add a comment for the AI...")
        });
        let submit_pane = pane_id.to_string();
        let subscription =
            cx.subscribe(
                &input,
                move |this, _input, event: &ComposerEvent, cx| match event {
                    // `submit()` clears the field right after emitting, and the event
                    // is delivered after that, so the text must come from the event.
                    ComposerEvent::Submit(text, _) => {
                        this.commit_file_comment_text(&submit_pane, id, text.clone(), cx)
                    }
                    ComposerEvent::Edited | ComposerEvent::Focus => cx.notify(),
                    _ => {}
                },
            );
        (input, subscription)
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
                && comment.old_side == request.old_side
                && comment.start_line == request.start_line
                && comment.end_line == request.end_line
        });
        if staged_exists {
            return;
        }

        let id = pane.next_file_comment_id;
        let (input, subscription) = self.new_comment_input(&pane_id, id, window, cx);

        let Some(pane) = self.workspace_pane_states.get_mut(&pane_id) else {
            return;
        };
        if let Some((index, _)) = stale_draft {
            pane.file_comments.remove(index);
        }
        pane.next_file_comment_id = id + 1;
        pane.file_comments.push(FileComment {
            id,
            server_id: None,
            old_side: request.old_side,
            staged: false,
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
        if let Some((_, old_path)) = stale_draft
            && old_path != request.path
        {
            self.sync_comment_selections(&pane_id, &old_path, cx);
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
        let text = self
            .workspace_pane_states
            .get(pane_id)
            .and_then(|pane| pane.file_comments.iter().find(|c| c.id == comment_id))
            .map(|comment| comment.input.read(cx).content().to_string())
            .unwrap_or_default();
        self.commit_file_comment_text(pane_id, comment_id, text, cx);
    }

    pub(super) fn commit_file_comment_text(
        &mut self,
        pane_id: &str,
        comment_id: u64,
        text: String,
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
        let text = text.trim().to_owned();
        if text.is_empty() {
            return;
        }
        let path = comment.path.clone();
        let was_draft = comment.state == FileCommentState::Draft;
        comment.text = text.clone();
        comment.state = FileCommentState::Added;
        comment.focus_pending = false;
        let server_id = comment.server_id.clone();
        let (old_side, start_line, end_line) =
            (comment.old_side, comment.start_line, comment.end_line);
        comment.input.update(cx, |input, cx| input.clear(cx));
        self.sync_comment_selections(pane_id, &path, cx);
        if was_draft {
            let snippet = self.comment_snippet(&path, old_side, start_line, end_line);
            self.save_new_comment(pane_id, comment_id, path, text, snippet, cx);
        } else if let Some(server_id) = server_id {
            let client = self.client.clone();
            cx.background_spawn(async move {
                let _ = client.file_comments.update_text(&server_id, &text).await;
            })
            .detach();
        }
        cx.notify();
    }

    /// The commented lines as written now, kept so a reader can tell the
    /// comment was written against different text.
    fn comment_snippet(&self, path: &str, old_side: bool, start: u32, end: u32) -> String {
        let take = (end.saturating_sub(start) + 1).min(3) as usize;
        let snippet = if old_side || !self.open_file_contents.contains_key(path) {
            let Some((diff, _)) = self.open_diff_contents.get(path) else {
                return String::new();
            };
            diff.lines
                .iter()
                .filter(|line| {
                    let number = if old_side { line.old_no } else { line.new_no };
                    number.is_some_and(|n| n as u32 >= start && n as u32 <= end)
                })
                .take(take)
                .map(|line| line.text.clone())
                .collect::<Vec<_>>()
                .join("\n")
        } else {
            self.open_file_contents[path]
                .lines()
                .skip(start.saturating_sub(1) as usize)
                .take(take)
                .collect::<Vec<_>>()
                .join("\n")
        };
        snippet.chars().take(240).collect()
    }

    fn save_new_comment(
        &mut self,
        pane_id: &str,
        comment_id: u64,
        path: String,
        text: String,
        snippet: String,
        cx: &mut Context<Self>,
    ) {
        let Some(project_id) = self.pane_real_project_id(pane_id) else {
            return;
        };
        let Some(pane) = self.workspace_pane_states.get(pane_id) else {
            return;
        };
        let Some(comment) = pane.file_comments.iter().find(|c| c.id == comment_id) else {
            return;
        };
        let request = CreateFileCommentRequest {
            path: self.comment_display_path(pane_id, &path),
            side: if comment.old_side {
                FileCommentSide::Old
            } else {
                FileCommentSide::New
            } as i32,
            start_line: comment.start_line,
            end_line: comment.end_line,
            text: text.clone(),
            snippet,
        };
        let client = self.client.clone();
        let pane_id = pane_id.to_string();
        cx.spawn(async move |entity, cx| {
            let Ok(saved) = client.file_comments.create(&project_id, &request).await else {
                return;
            };
            // The card may have been deleted or edited while the save was in flight.
            let followup = cx.update(|cx| {
                let app = entity.upgrade()?;
                app.update(cx, |this, _| {
                    let comment = this
                        .workspace_pane_states
                        .get_mut(&pane_id)?
                        .file_comments
                        .iter_mut()
                        .find(|c| c.id == comment_id)?;
                    comment.server_id = Some(saved.id.clone());
                    (comment.text != text).then(|| (comment.text.clone(), saved.id.clone()))
                })
            });
            let orphaned = cx.update(|cx| {
                entity.upgrade().is_some_and(|app| {
                    app.read(cx)
                        .workspace_pane_states
                        .get(&pane_id)
                        .is_none_or(|pane| !pane.file_comments.iter().any(|c| c.id == comment_id))
                })
            });
            if orphaned {
                let _ = client.file_comments.delete(&saved.id).await;
            } else if let Some((text, id)) = followup {
                let _ = client.file_comments.update_text(&id, &text).await;
            }
        })
        .detach();
    }

    /// Fetch the project's saved comments the first time a pane shows a file
    /// or diff, and again when its project changes.
    pub(crate) fn ensure_file_comments_loaded(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let Some(project_id) = self.pane_real_project_id(pane_id) else {
            return;
        };
        let Some(root) = self.selected_project_for_pane(pane_id).map(|p| p.path) else {
            return;
        };
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        if pane.comments_project.as_deref() == Some(project_id.as_str()) {
            return;
        }
        pane.comments_project = Some(project_id.clone());
        let client = self.client.clone();
        let pane_id = pane_id.to_string();
        cx.spawn(async move |entity, cx| {
            let Ok(items) = client.file_comments.list(&project_id).await else {
                return;
            };
            cx.update(|cx| {
                let Some(handle) = cx.active_window() else {
                    return;
                };
                let _ = handle.update(cx, |_, window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.adopt_loaded_comments(&pane_id, &project_id, &root, items, window, cx)
                        });
                    }
                });
            });
        })
        .detach();
    }

    fn adopt_loaded_comments(
        &mut self,
        pane_id: &str,
        project_id: &str,
        root: &str,
        items: Vec<console_proto::FileComment>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        if pane.comments_project.as_deref() != Some(project_id) {
            return;
        }
        let staged: Vec<String> = pane
            .file_comments
            .iter()
            .filter(|c| c.staged)
            .filter_map(|c| c.server_id.clone())
            .collect();
        let mut touched: Vec<String> = Vec::new();
        pane.file_comments.retain(|comment| {
            let keep = comment.state != FileCommentState::Added || comment.server_id.is_none();
            if !keep {
                touched.push(comment.path.clone());
            }
            keep
        });
        for item in items {
            let Some(pane) = self.workspace_pane_states.get(pane_id) else {
                return;
            };
            if pane
                .file_comments
                .iter()
                .any(|c| c.server_id.as_deref() == Some(item.id.as_str()))
            {
                continue;
            }
            let id = pane.next_file_comment_id;
            let (input, subscription) = self.new_comment_input(pane_id, id, window, cx);
            let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
                return;
            };
            pane.next_file_comment_id = id + 1;
            let path = std::path::Path::new(root)
                .join(&item.path)
                .to_string_lossy()
                .into_owned();
            touched.push(path.clone());
            pane.file_comments.push(FileComment {
                id,
                staged: staged.contains(&item.id),
                server_id: Some(item.id),
                old_side: item.side == FileCommentSide::Old as i32,
                path,
                start_line: item.start_line,
                end_line: item.end_line,
                text: item.text,
                input,
                state: FileCommentState::Added,
                focus_pending: false,
                _subscription: subscription,
            });
        }
        touched.sort();
        touched.dedup();
        for path in touched {
            self.sync_comment_selections(pane_id, &path, cx);
        }
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
        let Some(path) = self.workspace_pane_states.get(pane_id).and_then(|pane| {
            pane.file_comments
                .iter()
                .find(|comment| comment.id == comment_id)
                .map(|comment| comment.path.clone())
        }) else {
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

    /// Delete a comment for good (server row and line highlight).
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
        let removed = pane.file_comments.remove(index);
        if let Some(server_id) = removed.server_id {
            let client = self.client.clone();
            cx.background_spawn(async move {
                let _ = client.file_comments.delete(&server_id).await;
            })
            .detach();
        }
        self.sync_comment_selections(pane_id, &removed.path, cx);
        cx.notify();
    }

    /// Mirror the pane's comment ranges into the editor's persistent
    /// highlight. The editor also records the range on mouse-up; this keeps
    /// cancel/remove and reopened tabs in sync.
    pub(super) fn sync_comment_selections(
        &mut self,
        pane_id: &str,
        path: &str,
        cx: &mut Context<Self>,
    ) {
        let ranges = self
            .workspace_pane_states
            .get(pane_id)
            .map(|pane| {
                pane.file_comments
                    .iter()
                    .filter(|comment| comment.path == path && !comment.old_side)
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
            if !comment.staged || comment.state == FileCommentState::Draft {
                continue;
            }
            let text = comment.text.trim();
            if text.is_empty() {
                continue;
            }
            let path = self.comment_display_path(pane_id, &comment.path);
            let mut lines = Self::comment_lines(comment.start_line, comment.end_line);
            if comment.old_side {
                lines.push_str(" (before the change)");
            }
            blocks.push(
                console_shared::composer::file_comments::format_file_comment(&path, &lines, text),
            );
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

    /// Unstage every comment once it has been sent. They stay saved.
    fn clear_staged_comments(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        for comment in &mut pane.file_comments {
            comment.staged = false;
        }
        cx.notify();
    }

    /// Stage a saved comment for the next message, or take it back out.
    pub(crate) fn toggle_comment_staged(
        &mut self,
        pane_id: &str,
        comment_id: u64,
        cx: &mut Context<Self>,
    ) {
        let Some(comment) = self
            .workspace_pane_states
            .get_mut(pane_id)
            .and_then(|pane| pane.file_comments.iter_mut().find(|c| c.id == comment_id))
        else {
            return;
        };
        if comment.state == FileCommentState::Draft {
            return;
        }
        comment.staged = !comment.staged;
        cx.notify();
    }

    /// Chips above the pane composer's input: the comments staged for the
    /// next message. Dismissing a chip unstages it; the comment stays saved.
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
            .filter(|comment| comment.staged && comment.state != FileCommentState::Draft)
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
                                this.toggle_comment_staged(&remove_pane, remove_id, cx);
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
        self.ensure_file_comments_loaded(pane_id, cx);
        let entity = cx.entity().downgrade();
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return Vec::new();
        };
        let mut overlays = Vec::new();
        for comment in pane
            .file_comments
            .iter_mut()
            .filter(|comment| comment.path == path && !comment.old_side)
        {
            if comment.focus_pending {
                comment.focus_pending = false;
                let focus = comment.input.read(cx).focus();
                window.on_next_frame(move |window, cx| window.focus(&focus, cx));
            }

            let id = comment.id;
            let start_line = comment.start_line;
            let end_line = comment.end_line;
            let staged = comment.staged;
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
                        app.update(cx, |this, cx| this.commit_file_comment(&pane_id, id, cx));
                    }
                }
            };
            let on_cancel = {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_window: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.cancel_file_comment(&pane_id, id, cx));
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
                        app.update(cx, |this, cx| this.remove_file_comment(&pane_id, id, cx));
                    }
                }
            };

            let on_send = {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_window: &mut Window, cx: &mut App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.toggle_comment_staged(&pane_id, id, cx));
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
                on_send,
                staged,
            );
            overlays.push(FileCommentOverlay::new(start_line, end_line, card));
        }
        overlays
    }

    /// Mirror the pane's comments for a diff into the diff view: tinted rows
    /// plus an inline card under the last row of each range.
    pub(crate) fn sync_diff_comments(
        &mut self,
        pane_id: &str,
        path: &str,
        view: &gpui::Entity<editor_ui::DiffView>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.ensure_file_comments_loaded(pane_id, cx);
        let entity = cx.entity().downgrade();
        let Some(pane) = self.workspace_pane_states.get_mut(pane_id) else {
            return;
        };
        let mut selections = Vec::new();
        let mut overlays = Vec::new();
        for comment in pane.file_comments.iter_mut().filter(|c| c.path == path) {
            if comment.focus_pending {
                comment.focus_pending = false;
                let focus = comment.input.read(cx).focus();
                window.on_next_frame(move |window, cx| window.focus(&focus, cx));
            }
            let side = if comment.old_side {
                editor_ui::DiffSide::Old
            } else {
                editor_ui::DiffSide::New
            };
            selections.push(editor_ui::DiffCommentTarget {
                side,
                start_line: comment.start_line,
                end_line: comment.end_line,
            });

            let editing = comment.state != FileCommentState::Added;
            let text_rows = comment.text.lines().count().max(1) as u32
                + (comment.text.len() / 80) as u32;
            let rows = if editing { 9 } else { 4 + text_rows };

            let id = comment.id;
            let state = comment.state;
            let staged = comment.staged;
            let text = comment.text.clone();
            let input = comment.input.clone();
            let card_path = comment.path.clone();
            let (start_line, end_line) = (comment.start_line, comment.end_line);
            let pane_id = pane_id.to_string();
            let entity = entity.clone();
            overlays.push(editor_ui::DiffRowOverlay::new(
                side,
                end_line,
                rows,
                move |_window, _cx| {
                    let act = |f: fn(&mut ConsoleDesktopApp, &str, u64, &mut Context<ConsoleDesktopApp>)| {
                        let entity = entity.clone();
                        let pane_id = pane_id.clone();
                        move |_window: &mut Window, cx: &mut App| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| f(this, &pane_id, id, cx));
                            }
                        }
                    };
                    FileCommentCard::new(
                        id,
                        card_path.clone(),
                        start_line,
                        end_line,
                        match state {
                            FileCommentState::Draft => FileCommentCardState::Draft,
                            FileCommentState::Added => FileCommentCardState::Added,
                            FileCommentState::Editing => FileCommentCardState::Editing,
                        },
                        text.clone(),
                        input.clone(),
                        act(ConsoleDesktopApp::commit_file_comment),
                        act(ConsoleDesktopApp::cancel_file_comment),
                        act(ConsoleDesktopApp::edit_file_comment),
                        act(ConsoleDesktopApp::remove_file_comment),
                        act(ConsoleDesktopApp::toggle_comment_staged),
                        staged,
                    )
                    .into_any_element()
                },
            ));
        }
        view.update(cx, |view, cx| {
            view.set_comment_selections(selections, cx);
            view.set_row_overlays(overlays, cx);
        });
    }
}
