//! Host side of editor saving: the API write and its outcome.
//!
//! The desktop client never writes workspace files on its own disk. An
//! editor's save handler only enqueues a [`FileSaveRequest`] and returns
//! `SaveStatus::Deferred`; this task performs `POST /api/fs/save` and then
//! reports back through `EditorState::mark_saved`, which keeps edits made
//! while the save was in flight marked unsaved.

use gpui::prelude::*;

use super::types::{ConsoleDesktopApp, FileSaveRequest};

impl ConsoleDesktopApp {
    /// Drain editor saves and write them through the API, one at a time per
    /// path so two quick cmd-s presses cannot land out of order.
    pub(crate) fn init_file_saves(
        &mut self,
        mut requests: tokio::sync::mpsc::UnboundedReceiver<FileSaveRequest>,
        cx: &mut Context<Self>,
    ) {
        cx.spawn(async move |this, cx| {
            while let Some(request) = requests.recv().await {
                let Some(app) = this.upgrade() else {
                    return;
                };
                let queued = cx.update(|cx| {
                    app.update(cx, |this, _cx| this.begin_file_save(&request.path))
                });
                // A save is already running for this path: it is marked pending
                // and re-read from the editor when it finishes, so this request
                // is deliberately dropped (its text is stale by definition).
                if !queued {
                    continue;
                }
                let Some(app) = this.upgrade() else {
                    return;
                };
                let (client, base_etag) = cx.update(|cx| {
                    app.update(cx, |this, _cx| {
                        (
                            this.client.clone(),
                            this.viewer_file_etags.get(&request.path).cloned(),
                        )
                    })
                });
                let outcome = client
                    .fs
                    .save_file(&request.path, &request.text, base_etag.as_deref())
                    .await;
                let Some(app) = this.upgrade() else {
                    return;
                };
                cx.update(|cx| {
                    app.update(cx, |this, cx| {
                        this.finish_file_save(&request, outcome, cx);
                    })
                });
            }
        })
        .detach();
    }

    /// Claim the save slot for `path`. Returns `false` when a save is already
    /// running, in which case the path is marked pending instead.
    fn begin_file_save(&mut self, path: &str) -> bool {
        if !self.file_saves_in_flight.insert(path.to_string()) {
            self.file_saves_pending.insert(path.to_string());
            return false;
        }
        true
    }

    fn finish_file_save(
        &mut self,
        request: &FileSaveRequest,
        outcome: Result<console_core::FsSaveOutcome, console_core::FsSaveError>,
        cx: &mut Context<Self>,
    ) {
        self.file_saves_in_flight.remove(&request.path);
        match outcome {
            Ok(saved) => {
                if let Some((_, _, state, _)) = self.viewer_editor_views.get(&request.path) {
                    state.update(cx, |state, cx| state.mark_saved(request.revision, cx));
                }
                self.viewer_file_etags
                    .insert(request.path.clone(), saved.etag.clone());
            }
            Err(err) => {
                // Nothing was written and the buffer stays dirty: reporting it
                // is the only thing the host can do, since the package's own
                // `SaveFailed` event belongs to its on-disk writer.
                self.report_file_save_failure(&request.path, &err, cx);
            }
        }

        if self.file_saves_pending.remove(&request.path) {
            self.resave_path(&request.path, cx);
        }
        cx.notify();
    }

    /// Re-run a save for `path` after edits arrived while one was in flight.
    fn resave_path(&mut self, path: &str, cx: &mut Context<Self>) {
        let Some((_, _, state, _)) = self.viewer_editor_views.get(path) else {
            return;
        };
        if !state.read(cx).is_dirty() {
            return;
        }
        let _ = state.update(cx, |state, cx| state.save(cx));
    }

    /// Surface a failed API save through the app's existing notification
    /// stack. The buffer stays dirty; nothing is retried automatically.
    fn report_file_save_failure(
        &mut self,
        path: &str,
        err: &console_core::FsSaveError,
        cx: &mut Context<Self>,
    ) {
        use gpui_component::notification::Notification;
        let name = std::path::Path::new(path)
            .file_name()
            .map(|name| name.to_string_lossy().into_owned())
            .unwrap_or_else(|| path.to_string());
        let note = Notification::error(err.to_string()).title(format!("Could not save {name}"));
        let Some(handle) = cx.active_window() else {
            log::error!("Failed to save {}: {err}", path);
            return;
        };
        let _ = handle.update(cx, |_, window, cx| {
            use gpui_component::WindowExt;
            window.push_notification(note, cx);
        });
    }
}
