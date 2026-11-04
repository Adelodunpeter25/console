//! File line comments: pick a range in the editor's comment gutter, type a
//! note, and send it to the active chat as a `<path>:<lines>` prompt.
//!
//! `editor-ui` owns the gutter interaction and reports a `CommentTarget`
//! through a plain callback — it has no app context — so the handler pushes
//! the request into a channel and a spawned task opens the card. The card is
//! window-level (built in the app's render) so the editor's scroll viewport
//! cannot clip it.

use console_core::SessionHeaderExt;
use console_ui::FileCommentComposer;
use gpui::{AnyElement, App, Context, IntoElement, Window};

use super::app::ConsoleDesktopApp;
use super::types::{FileCommentDraft, FileCommentRequest};

impl ConsoleDesktopApp {
    /// Wire the shared comment prompt field and start draining gutter picks.
    pub(crate) fn init_file_comments(
        &mut self,
        mut requests: tokio::sync::mpsc::UnboundedReceiver<FileCommentRequest>,
        cx: &mut Context<Self>,
    ) {
        use console_ui::input::ComposerEvent;
        let input = self.file_comment_input.clone();
        self._subscriptions.push(cx.subscribe(
            &input,
            |this, _input, event: &ComposerEvent, cx| match event {
                ComposerEvent::Submit(text, _) => this.submit_file_comment(text.clone(), cx),
                // The Send button's enabled state is derived from the live
                // content, and the card lives in the app's render, so every
                // edit has to repaint the app too.
                ComposerEvent::Edited | ComposerEvent::Focus => cx.notify(),
                _ => {}
            },
        ));

        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            while let Some(request) = requests.recv().await {
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.open_file_comment(request, cx));
                    }
                });
            }
        })
        .detach();
    }

    fn open_file_comment(&mut self, request: FileCommentRequest, cx: &mut Context<Self>) {
        self.file_comment_input
            .update(cx, |input, cx| input.clear(cx));
        self.file_comment = Some(FileCommentDraft {
            path: request.path,
            start_line: request.start_line,
            end_line: request.end_line,
            focus_pending: true,
        });
        cx.notify();
    }

    pub(crate) fn cancel_file_comment(&mut self, cx: &mut Context<Self>) {
        if self.file_comment.take().is_some() {
            cx.notify();
        }
    }

    /// Turn the card's text into a chat prompt and submit it the same way
    /// Enter would. The path is shown project-relative (`src/main.rs:23`),
    /// which is what the agent and the transcript expect.
    fn submit_file_comment(&mut self, text: String, cx: &mut Context<Self>) {
        let Some(draft) = self.file_comment.take() else {
            return;
        };
        let text = text.trim().to_owned();
        if text.is_empty() {
            cx.notify();
            return;
        }

        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".to_string());
        let root = self
            .selected_project_for_pane(&pane_id)
            .map(|project| project.path);
        let display_path = root
            .as_deref()
            .and_then(|root| std::path::Path::new(&draft.path).strip_prefix(root).ok())
            .map(|path| path.to_string_lossy().into_owned())
            .unwrap_or_else(|| draft.path.clone());
        let lines = if draft.start_line == draft.end_line {
            draft.start_line.to_string()
        } else {
            format!("{}–{}", draft.start_line, draft.end_line)
        };
        let prompt = format!("{display_path}:{lines}\n\n{text}");

        // Surface the chat the comment is about before the run starts; a new
        // session is opened by the submit path itself.
        if let Some(session_id) = self.active_session_for_pane(&pane_id) {
            let title = self
                .sessions
                .iter()
                .find(|session| session.id == session_id)
                .map(|session| session.display_title().to_string())
                .unwrap_or_else(|| "Chat".to_string());
            self.open_chat_tab_in_pane(&pane_id, session_id, title);
        }

        let composer = self.active_composer_input();
        composer.update(cx, |input, cx| {
            input.set_content(prompt, cx);
            input.submit(cx);
        });
        cx.notify();
    }

    /// Build the open card as a window-level overlay. `None` while closed.
    pub(crate) fn file_comment_view(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Option<AnyElement> {
        let draft = self.file_comment.as_ref()?;
        let (path, start_line, end_line, focus_pending) = (
            draft.path.clone(),
            draft.start_line,
            draft.end_line,
            draft.focus_pending,
        );
        if focus_pending {
            let handle = self.file_comment_input.read(cx).focus();
            // Deferred: the overlay joins the dispatch tree at the end of
            // this render, so focusing now lands on the wrong node.
            window.on_next_frame(move |window, cx| window.focus(&handle, cx));
            if let Some(draft) = self.file_comment.as_mut() {
                draft.focus_pending = false;
            }
        }

        let entity = cx.entity().downgrade();
        let on_submit = {
            let entity = entity.clone();
            move |text: String, _window: &mut Window, cx: &mut App| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.submit_file_comment(text.clone(), cx));
                }
            }
        };
        let on_cancel = move |_window: &mut Window, cx: &mut App| {
            if let Some(app) = entity.upgrade() {
                app.update(cx, |this, cx| this.cancel_file_comment(cx));
            }
        };

        Some(
            FileCommentComposer::new(
                path,
                start_line,
                end_line,
                self.file_comment_input.clone(),
                on_submit,
                on_cancel,
            )
            .into_any_element(),
        )
    }
}
