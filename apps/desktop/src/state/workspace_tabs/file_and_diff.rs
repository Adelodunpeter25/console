use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use gpui::Context;

use crate::state::FileMediaDetails;
use crate::state::app::{ConsoleDesktopApp, ImageFileState};

fn is_html_path(path: &str) -> bool {
    std::path::Path::new(path)
        .extension()
        .and_then(|e| e.to_str())
        .is_some_and(|ext| ext.eq_ignore_ascii_case("html") || ext.eq_ignore_ascii_case("htm"))
}

impl ConsoleDesktopApp {
    /// Resolve a clicked transcript file link against the session cwd with a
    /// pane project-path fallback, then open it as a workspace tab.
    pub fn open_file_link(&mut self, link: String, pane_id: &str, cx: &mut Context<Self>) {
        use console_shared::markdown::file_links as links;
        let line = links::parse_line_suffix(&link);
        let cwd = self.transcript_for_pane(pane_id).read(cx).session_cwd();
        let project_path = self
            .selected_project_for_pane(pane_id)
            .map(|project| project.path);
        let resolved = links::resolve_file_link(&link, cwd.as_deref(), project_path.as_deref());

        // A bare name (`file_tree.rs`) only resolves when the file sits in the
        // cwd itself. Open the direct guess when it exists; otherwise search
        // the project for the name instead of opening a path that is not there.
        // The split is created only when a tab is about to open in it: a bare
        // name still needs a server-side search first, and an early split
        // would sit on the chat empty state for that whole round trip.
        let direct_exists = std::path::Path::new(&resolved).is_file();
        if !links::is_bare_file_name(&link) || direct_exists {
            let target_pane = self.target_subtab_pane_id();
            if is_html_path(&resolved) {
                self.open_html_in_browser_tab(&target_pane, resolved, cx);
            } else if let Some(line) = line {
                self.open_file_tab_at_line(&target_pane, resolved, line, None, cx);
            } else {
                self.open_file_tab_in_pane(&target_pane, resolved, cx);
            }
            return;
        }
        let name = links::strip_line_suffix(link.trim()).to_owned();
        let root = project_path.or_else(|| cwd.clone());
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let found = client
                .assist
                .search_files(None, &name, root.as_deref(), false)
                .await
                .map(|response| response.items)
                .unwrap_or_default();
            // The search is fuzzy; keep only files actually named `name`.
            let candidates: Vec<String> = found
                .into_iter()
                .filter(|item| !item.is_dir)
                .map(|item| item.absolute_path)
                .filter(|path| {
                    std::path::Path::new(path)
                        .file_name()
                        .and_then(|n| n.to_str())
                        .is_some_and(|n| n.eq_ignore_ascii_case(&name))
                })
                .collect();
            let chosen = links::closest_match(cwd.as_deref(), &candidates)
                .map(str::to_owned)
                .unwrap_or(resolved);
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let target_pane = this.target_subtab_pane_id();
                        if is_html_path(&chosen) {
                            this.open_html_in_browser_tab(&target_pane, chosen, cx);
                        } else if let Some(line) = line {
                            this.open_file_tab_at_line(&target_pane, chosen, line, None, cx);
                        } else {
                            this.open_file_tab_in_pane(&target_pane, chosen, cx);
                        }
                    });
                }
            });
        })
        .detach();
    }

    pub fn open_file_tab(&mut self, path: String, cx: &mut Context<Self>) {
        let pane_id = self.target_subtab_pane_id();
        self.open_file_tab_in_pane(&pane_id, path, cx);
    }

    /// Open a file tab and select/scroll to a 1-based line number.
    pub fn open_file_tab_at_line(
        &mut self,
        pane_id: &str,
        path: String,
        line: u32,
        window: Option<&mut gpui::Window>,
        cx: &mut Context<Self>,
    ) {
        self.open_file_tab_in_pane(pane_id, path.clone(), cx);
        if let Some((_, _, _, view)) = self.viewer_editor_views.get(&path) {
            view.update(cx, |view, cx| {
                view.select_line(line, cx);
            });
            if let Some(window) = window {
                let focus_handle = view.read(cx).focus_handle();
                window.focus(&focus_handle, cx);
            }
        } else {
            self.pending_file_line.insert(path, line);
        }
    }

    pub fn open_file_tab_in_pane(&mut self, pane_id: &str, path: String, cx: &mut Context<Self>) {
        self.open_file_tab_in_pane_force_editor(pane_id, path, cx);
    }

    pub fn open_html_in_browser_tab(
        &mut self,
        pane_id: &str,
        path: String,
        cx: &mut Context<Self>,
    ) {
        let is_dark = console_ui::theme::Theme::current(cx).is_dark;
        let pane_id = pane_id.to_string();
        let (tx, rx) = tokio::sync::oneshot::channel();
        tokio::spawn(async move {
            let _ = tx.send(console_ui::chat::resolve_html_url(&path, is_dark).await);
        });
        cx.spawn(async move |entity, cx| {
            let Some(url) = rx.await.ok().flatten() else {
                return;
            };
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let (browser_id, tab) =
                            this.new_browser_tab(Some(url), None, None, &pane_id);
                        workspace_ops::open_tab(&mut this.workspace_root, &pane_id, tab);
                        this.active_pane_id = Some(pane_id);
                        this.sync_workspace_webviews(cx);
                        this.persist_workspaces();
                        let _ = browser_id;
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    pub fn open_file_tab_in_pane_force_editor(
        &mut self,
        pane_id: &str,
        path: String,
        cx: &mut Context<Self>,
    ) {
        let title = std::path::Path::new(&path)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(&path)
            .to_string();

        let preview_target = self.preview_tab.as_ref().and_then(|(tab_id, opened_at)| {
            if opened_at.elapsed() < std::time::Duration::from_secs(600) {
                Some(tab_id.clone())
            } else {
                None
            }
        });

        let tab = WorkspaceTabConfig::File {
            path: path.clone(),
            title,
            project_id: self.pane_workspace_key(pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };

        let new_tab_id = workspace_ops::replace_or_open_tab(
            &mut self.workspace_root,
            pane_id,
            preview_target.as_deref(),
            tab,
        );

        let created_at = self
            .preview_tab
            .as_ref()
            .filter(|(tid, _)| preview_target.as_deref() == Some(tid.as_str()))
            .map(|(_, instant)| *instant)
            .unwrap_or_else(std::time::Instant::now);

        self.preview_tab = Some((new_tab_id, created_at));
        self.active_pane_id = Some(pane_id.to_string());
        self.file_tree_selected_path = Some(path.clone());
        self.trim_file_caches(cx);
        self.sync_workspace_webviews(cx);
        self.persist_workspaces();

        // Reopening is an explicit retry: forget an earlier failure.
        self.open_file_errors.remove(&path);

        // Fetch file content or image preview
        self.fetch_file_tab_content(path, cx);

        cx.notify();
    }

    /// Fetch tab content depending on file kind (text/markdown vs raster image/SVG vs blocked).
    pub fn fetch_file_tab_content(&mut self, path: String, cx: &mut Context<Self>) {
        if self.open_file_errors.contains_key(&path) {
            return;
        }
        let kind = console_core::file_kind_for_path(&path);
        match kind {
            console_core::FileKind::Markdown | console_core::FileKind::Text => {
                // Content comes only from the API: this client never reads
                // workspace files off its own disk.
                let client = self.client.clone();
                let file_path = path;
                cx.spawn(async move |entity, cx| {
                    match client.fs.read_file_with_etag(&file_path).await {
                        Ok(file) => {
                            cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_file_contents
                                            .insert(file_path.clone(), file.content.content);
                                        if let Some(etag) = file.etag {
                                            // A dirty buffer deliberately keeps the
                                            // OLD etag: the load path must not
                                            // launder a stale buffer into looking
                                            // current, or the conflict check on the
                                            // next save would pass and clobber the
                                            // change that arrived meanwhile.
                                            let dirty = this
                                                .viewer_editor_views
                                                .get(&file_path)
                                                .is_some_and(|(_, _, state, _)| {
                                                    state.read(cx).is_dirty()
                                                });
                                            if !dirty {
                                                this.viewer_file_etags.insert(file_path, etag);
                                            }
                                        }
                                        cx.notify();
                                    });
                                }
                            });
                        }
                        Err(err) => {
                            log::warn!("Failed to read file for tab {}: {}", file_path, err);
                            let message = err.to_string();
                            cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_file_errors.insert(file_path, message);
                                        cx.notify();
                                    });
                                }
                            });
                        }
                    }
                })
                .detach();
            }
            console_core::FileKind::RasterImage | console_core::FileKind::Svg => {
                self.open_image_contents
                    .insert(path.clone(), ImageFileState::Loading);
                cx.notify();

                let client = self.client.clone();
                let file_path = path;
                let is_svg = kind == console_core::FileKind::Svg;

                cx.spawn(async move |entity, cx| {
                    let res = client.fs.read_file_bytes(&file_path).await;
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                match res {
                                    Ok((bytes, content_type)) => {
                                        if is_svg {
                                            if let Some((png_bytes, w, h)) =
                                                console_ui::rasterize_svg(&bytes, 2048)
                                            {
                                                let img = std::sync::Arc::new(
                                                    gpui::Image::from_bytes(
                                                        gpui::ImageFormat::Png,
                                                        png_bytes,
                                                    ),
                                                );
                                                this.open_image_contents.insert(
                                                    file_path.clone(),
                                                    ImageFileState::Loaded {
                                                        image: img,
                                                        w: Some(w),
                                                        h: Some(h),
                                                        size_bytes: bytes.len() as u64,
                                                        mime: "image/svg+xml".to_string(),
                                                    },
                                                );
                                            } else {
                                                this.open_image_contents.insert(
                                                    file_path.clone(),
                                                    ImageFileState::Blocked {
                                                        title: "SVG Preview Unavailable".to_string(),
                                                        message: "The SVG could not be rendered as a preview. You can view the raw XML in Source mode.".to_string(),
                                                    },
                                                );
                                                this.svg_preview_mode.insert(
                                                    file_path.clone(),
                                                    console_ui::SvgViewMode::Source,
                                                );
                                            }
                                            // Also populate text content for SVG Source mode
                                            if let Ok(text) = String::from_utf8(bytes) {
                                                this.open_file_contents
                                                    .insert(file_path.clone(), text);
                                            }
                                        } else {
                                            // Raster image
                                            let format =
                                                gpui::ImageFormat::from_mime_type(&content_type);
                                            if let Some(fmt) = format {
                                                let dims = console_ui::image_dimensions(&bytes);
                                                let (w, h) = dims.unzip();
                                                let img = std::sync::Arc::new(
                                                    gpui::Image::from_bytes(fmt, bytes.clone()),
                                                );
                                                this.open_image_contents.insert(
                                                    file_path.clone(),
                                                    ImageFileState::Loaded {
                                                        image: img,
                                                        w,
                                                        h,
                                                        size_bytes: bytes.len() as u64,
                                                        mime: content_type,
                                                    },
                                                );
                                            } else {
                                                this.open_image_contents.insert(
                                                    file_path.clone(),
                                                    ImageFileState::Failed {
                                                        message: format!(
                                                            "Unsupported image format: {}",
                                                            content_type
                                                        ),
                                                    },
                                                );
                                            }
                                        }
                                    }
                                    Err(err) => {
                                        let file_name = std::path::Path::new(&file_path)
                                            .file_name()
                                            .and_then(|n| n.to_str())
                                            .unwrap_or(&file_path);

                                        let (title, message, is_blocked) = if let Some(raw_err) =
                                            err.downcast_ref::<console_core::RawFileError>()
                                        {
                                            let title = match raw_err.code.as_deref() {
                                                Some("LOCKFILE_BLOCKED") => {
                                                    "Lockfiles can't be previewed"
                                                }
                                                Some("FILE_TOO_LARGE") => "File too large",
                                                Some("BINARY_FILE") => "Binary file",
                                                _ => "Preview unavailable",
                                            };
                                            let message =
                                                raw_err.error.clone().unwrap_or_else(|| {
                                                    format!("\"{}\" cannot be previewed.", file_name)
                                                });
                                            (title.to_string(), message, true)
                                        } else if let Ok(raw_err) =
                                            serde_json::from_str::<console_core::RawFileError>(
                                                &err.to_string(),
                                            )
                                        {
                                            let title = match raw_err.code.as_deref() {
                                                Some("LOCKFILE_BLOCKED") => {
                                                    "Lockfiles can't be previewed"
                                                }
                                                Some("FILE_TOO_LARGE") => "File too large",
                                                Some("BINARY_FILE") => "Binary file",
                                                _ => "Preview unavailable",
                                            };
                                            let message = raw_err.error.unwrap_or_else(|| {
                                                format!("\"{}\" cannot be previewed.", file_name)
                                            });
                                            (title.to_string(), message, true)
                                        } else {
                                            (
                                                "Failed to load file".to_string(),
                                                err.to_string(),
                                                false,
                                            )
                                        };

                                        let state = if is_blocked {
                                            ImageFileState::Blocked { title, message }
                                        } else {
                                            ImageFileState::Failed { message }
                                        };
                                        this.open_image_contents.insert(file_path, state);
                                    }
                                }
                                cx.notify();
                            });
                        }
                    });
                })
                .detach();
            }
            console_core::FileKind::Video
            | console_core::FileKind::Audio
            | console_core::FileKind::Pdf
            | console_core::FileKind::Details => {
                self.open_media_contents.insert(
                    path.clone(),
                    FileMediaDetails {
                        kind: media_kind_for(kind),
                        mime_label: String::new(),
                        size_bytes: None,
                    },
                );
                cx.notify();

                let client = self.client.clone();
                let file_path = path;
                let file_kind = kind;
                cx.spawn(async move |entity, cx| {
                    // Media kinds probe the media endpoint for size+mime;
                    // details kinds have no endpoint, so size comes from the
                    // parent directory listing instead.
                    let result = match file_kind {
                        console_core::FileKind::Video
                        | console_core::FileKind::Audio
                        | console_core::FileKind::Pdf => {
                            client.fs.media_file_meta(&file_path).await.map(|meta| {
                                meta.map(|m| FileMediaDetails {
                                    kind: media_kind_for(file_kind),
                                    mime_label: m.content_type,
                                    size_bytes: m.size_bytes,
                                })
                            })
                        }
                        _ => client.fs.file_size(&file_path).await.map(|size| {
                            Some(FileMediaDetails {
                                kind: None,
                                mime_label: FileMediaDetails::details_label(&file_path),
                                size_bytes: size,
                            })
                        }),
                    };
                    cx.update(|cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                match result {
                                    Ok(Some(details)) => {
                                        // Details files have no player kind;
                                        // their card renders from mime_label
                                        // + size, and `kind` only picks the icon.
                                        this.open_media_contents.insert(file_path.clone(), details);
                                    }
                                    Ok(None) => {
                                        this.open_media_contents.insert(
                                            file_path.clone(),
                                            FileMediaDetails {
                                                kind: media_kind_for(file_kind),
                                                mime_label: "File not found".to_string(),
                                                size_bytes: None,
                                            },
                                        );
                                    }
                                    Err(err) => {
                                        this.open_file_errors
                                            .insert(file_path.clone(), err.to_string());
                                    }
                                }
                                cx.notify();
                            });
                        }
                    });
                })
                .detach();
            }
            console_core::FileKind::Blocked => {
                let file_name = std::path::Path::new(&path)
                    .file_name()
                    .and_then(|n| n.to_str())
                    .unwrap_or(&path);

                let (title, message) = if console_core::is_lock_file(file_name) {
                    (
                        "Lockfiles can't be previewed".to_string(),
                        format!(
                            "\"{file_name}\" is a generated lockfile — open it on your machine instead."
                        ),
                    )
                } else {
                    (
                        "Binary file".to_string(),
                        format!(
                            "\"{file_name}\" isn't a text file, so there's nothing to preview here."
                        ),
                    )
                };
                self.open_image_contents
                    .insert(path, ImageFileState::Blocked { title, message });
                cx.notify();
            }
        }
    }

    /// Open a diff tab, optionally with a known diff body (a turn's recorded
    /// change) so it shows exactly that diff instead of looking one up.
    pub fn open_diff_tab_with_text(
        &mut self,
        pane_id: &str,
        path: String,
        turn_index: Option<u32>,
        diff_override: Option<String>,
        cx: &mut Context<Self>,
    ) {
        // The diff opens in `pane_id`, but its checkout and session come from
        // the chat's pane: a split beside the chat has neither.
        let lookup_pane = self
            .workspace_chat()
            .map(|(pane, _)| pane.to_owned())
            .unwrap_or_else(|| pane_id.to_owned());
        let title = format!(
            "Diff: {}",
            std::path::Path::new(&path)
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or(&path)
        );

        let preview_target = self.preview_tab.as_ref().and_then(|(tab_id, opened_at)| {
            if opened_at.elapsed() < std::time::Duration::from_secs(600) {
                Some(tab_id.clone())
            } else {
                None
            }
        });

        let tab = WorkspaceTabConfig::Diff {
            path: path.clone(),
            title,
            project_id: self.pane_workspace_key(pane_id),
            last_active_at_ms: Some(chrono::Utc::now().timestamp_millis()),
        };

        let new_tab_id = workspace_ops::replace_or_open_tab(
            &mut self.workspace_root,
            pane_id,
            preview_target.as_deref(),
            tab,
        );

        let created_at = self
            .preview_tab
            .as_ref()
            .filter(|(tid, _)| preview_target.as_deref() == Some(tid.as_str()))
            .map(|(_, instant)| *instant)
            .unwrap_or_else(std::time::Instant::now);

        self.preview_tab = Some((new_tab_id, created_at));
        self.active_pane_id = Some(pane_id.to_string());
        self.file_tree_selected_path = Some(path.clone());
        self.trim_file_caches(cx);
        self.persist_workspaces();

        // Marked as loading before the tab first renders: a render that finds
        // nothing cached takes the tab for one restored from disk and starts a
        // second, working-tree fetch whose empty result would replace this one.
        self.open_diff_contents
            .entry(path.clone())
            .or_insert_with(|| (console_core::DiffResult::default(), String::new()));
        self.fetch_diff_into_cache(&lookup_pane, path, turn_index, diff_override, cx);
        cx.notify();
    }

    /// Load the diff for a tab restored from disk: the working-tree diff, as
    /// the turn it came from is not saved with the tab.
    pub(crate) fn load_diff_for_tab(
        &mut self,
        pane_id: &str,
        path: String,
        cx: &mut Context<Self>,
    ) {
        let lookup_pane = self
            .workspace_chat()
            .map(|(pane, _)| pane.to_owned())
            .unwrap_or_else(|| pane_id.to_owned());
        self.fetch_diff_into_cache(&lookup_pane, path, None, None, cx);
    }

    fn fetch_diff_into_cache(
        &mut self,
        pane_id: &str,
        path: String,
        turn_index: Option<u32>,
        diff_override: Option<String>,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let file_path = path;
        let session_change_diff = diff_override.filter(|d| !d.trim().is_empty());
        // Resolve against the pane's own chat checkout, not the globally
        // last-selected session. `selected_session_id` points at whichever
        // chat was touched last across every pane, so with a main-branch chat
        // and a worktree chat open the diff was fetched against the wrong
        // checkout and a worktree session's changes rendered empty.
        let cwd = self
            .transcript_for_pane(pane_id)
            .read(cx)
            .session_cwd()
            .or_else(|| {
                self.selected_project_for_pane(pane_id)
                    .map(|project| project.path)
            });
        let change_session = turn_index.zip(self.active_session_for_pane(pane_id));

        cx.spawn(async move |entity, cx| {
            let mut diff_raw = session_change_diff.unwrap_or_default();
            if diff_raw.trim().is_empty()
                && let Some((turn_index, session_id)) = change_session
                && let Ok(text) = client
                    .sessions
                    .get_change_diff(&session_id, &file_path, u64::from(turn_index))
                    .await
            {
                diff_raw = text;
            }
            if diff_raw.trim().is_empty()
                && let Ok(resp) = client.git.get_diff(cwd.as_deref(), Some(&file_path)).await
            {
                diff_raw = resp.diff;
            }

            let diff_result = if !diff_raw.trim().is_empty() {
                console_core::utils::diff::parse_unified_diff(&diff_raw)
            } else {
                console_core::DiffResult::default()
            };

            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.open_diff_contents
                            .insert(file_path, (diff_result, diff_raw));
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Maximum retained file/viewer cache entries per map. Bounds long-session
    /// memory; evicted contents are re-fetched or re-rendered on reopen.
    pub(crate) fn trim_file_caches(&mut self, cx: &mut Context<Self>) {
        const MAX_CACHED_FILES: usize = 30;
        let open = self.open_file_paths_everywhere();
        // Media views hold native surfaces; close the ones whose tab is gone,
        // even when no cache is over budget (a closed tab can leave its view
        // behind without tripping the size check).
        let orphaned: Vec<String> = self
            .file_media_views
            .keys()
            .filter(|path| !open.contains(*path))
            .cloned()
            .collect();
        for path in orphaned {
            if let Some(view) = self.file_media_views.remove(&path) {
                view.update(cx, |v, cx| v.close(cx));
            }
        }
        let over = self.open_file_contents.len() > MAX_CACHED_FILES
            || self.open_image_contents.len() > MAX_CACHED_FILES
            || self.open_media_contents.len() > MAX_CACHED_FILES
            || self.open_diff_contents.len() > MAX_CACHED_FILES
            || self.viewer_editor_views.len() > MAX_CACHED_FILES
            || self.viewer_diff_views.len() > MAX_CACHED_FILES
            || self.viewer_cached_markdown_views.len() > MAX_CACHED_FILES;
        if !over {
            return;
        }
        let live_run_ids = self.live_run_script_ids();
        self.open_file_contents
            .retain(|path, _| open.contains(path));
        self.open_image_contents
            .retain(|path, _| open.contains(path));
        self.open_media_contents
            .retain(|path, _| open.contains(path));
        self.svg_preview_mode.retain(|path, _| open.contains(path));
        self.open_diff_contents
            .retain(|path, _| open.contains(path));
        self.viewer_editor_views.retain(|path, (_, _, state, _)| {
            // A dirty buffer is never evicted: dropping it would discard the
            // user's unsaved edits, and there is no file on disk to recover
            // them from.
            open.contains(path) || state.read(cx).is_dirty()
        });
        self.viewer_editor_subscriptions
            .retain(|path, _| self.viewer_editor_views.contains_key(path));
        self.viewer_file_etags
            .retain(|path, _| self.viewer_editor_views.contains_key(path));
        self.dirty_file_paths_cache
            .retain(|path, _| self.viewer_editor_views.contains_key(path));
        self.viewer_diff_views.retain(|path, _| open.contains(path));
        self.viewer_cached_markdown_views
            .retain(|path, _| open.contains(path));
        self.viewer_list_states.retain(|key, _| {
            Self::viewer_key_is_open(key, &open) || Self::run_key_is_live(key, &live_run_ids)
        });
        self.viewer_scrollbar_states.retain(|key, _| {
            Self::viewer_key_is_open(key, &open) || Self::run_key_is_live(key, &live_run_ids)
        });
        self.viewer_markdown_selections.retain(|key, _| {
            Self::viewer_key_is_open(key, &open) || Self::run_key_is_live(key, &live_run_ids)
        });
    }

    /// Script ids still defined in any project's scripts. Bounds the retained
    /// run-output viewer state; scripts are few, so liveness is by existence,
    /// not expansion.
    fn live_run_script_ids(&self) -> std::collections::HashSet<String> {
        self.project_scripts_by_project
            .values()
            .flat_map(|state| state.scripts.iter().map(|script| script.id.clone()))
            .collect()
    }

    /// Viewer state keys for run output are `run:`-prefixed script ids.
    fn run_key_is_live(key: &str, live: &std::collections::HashSet<String>) -> bool {
        key.strip_prefix("run:")
            .is_some_and(|script_id| live.contains(script_id))
    }

    /// Whether `path` has a file tab open in any workspace (current + cached).
    /// Used to guard async view creation against a tab that closed meanwhile.
    pub(crate) fn file_path_is_open(&self, path: &str) -> bool {
        self.open_file_paths_everywhere().contains(path)
    }

    /// File paths with a tab open in any workspace (current + cached).
    fn open_file_paths_everywhere(&self) -> std::collections::HashSet<String> {
        let mut open = std::collections::HashSet::new();
        for path in workspace_ops::open_file_paths(&self.workspace_root) {
            open.insert(path);
        }
        for root in self.project_workspace_roots.values() {
            for path in workspace_ops::open_file_paths(root) {
                open.insert(path);
            }
        }
        open
    }

    /// Viewer state keys are raw paths or `file:`/`diff:`/`md:`-prefixed paths.
    fn viewer_key_is_open(key: &str, open: &std::collections::HashSet<String>) -> bool {
        if open.contains(key) {
            return true;
        }
        ["file:", "diff:", "md:"]
            .iter()
            .find_map(|prefix| key.strip_prefix(prefix))
            .is_some_and(|path| open.contains(path))
    }

    /// Drop cached contents + viewer state for one closed file path.
    ///
    /// A dirty editor survives: the whole point of the close confirmation is
    /// that unsaved edits are never dropped behind the user's back, and this
    /// is the last place they could be. Its cache entry is reclaimed by
    /// `trim_file_caches` once the buffer is saved.
    pub(crate) fn evict_file_caches_for_path(&mut self, path: &str, cx: &mut Context<Self>) {
        self.open_file_contents.remove(path);
        self.open_file_errors.remove(path);
        self.open_image_contents.remove(path);
        self.svg_preview_mode.remove(path);
        self.open_diff_contents.remove(path);
        let dirty = self
            .viewer_editor_views
            .get(path)
            .is_some_and(|(_, _, state, _)| state.read(cx).is_dirty());
        if !dirty {
            self.viewer_editor_views.remove(path);
            self.viewer_editor_subscriptions.remove(path);
            self.viewer_file_etags.remove(path);
            self.dirty_file_paths_cache.remove(path);
        }
        self.viewer_diff_views.remove(path);
        self.viewer_cached_markdown_views.remove(path);
        for key in [
            format!("file:{path}"),
            format!("diff:{path}"),
            format!("md:{path}"),
        ] {
            self.viewer_list_states.remove(&key);
            self.viewer_scrollbar_states.remove(&key);
            self.viewer_markdown_selections.remove(&key);
        }
        self.open_media_contents.remove(path);
        if let Some(view) = self.file_media_views.remove(path) {
            view.update(cx, |v, cx| v.close(cx));
        }
    }
}

/// Map a classified file kind to the media player it needs; `Details` gets
/// the metadata card (`None`).
pub(crate) fn media_kind_for(kind: console_core::FileKind) -> Option<console_ui::MediaFileKind> {
    match kind {
        console_core::FileKind::Video => Some(console_ui::MediaFileKind::Video),
        console_core::FileKind::Audio => Some(console_ui::MediaFileKind::Audio),
        console_core::FileKind::Pdf => Some(console_ui::MediaFileKind::Pdf),
        _ => None,
    }
}
