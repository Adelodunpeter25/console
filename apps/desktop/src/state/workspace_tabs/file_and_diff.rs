use console_core::WorkspaceTabConfig;
use console_ui::workspace::ops as workspace_ops;
use gpui::Context;

use crate::state::app::{ConsoleDesktopApp, ImageFileState};

impl ConsoleDesktopApp {
    /// Resolve a clicked transcript file link against the session cwd with a
    /// pane project-path fallback, then open it as a workspace tab. Phase 1
    /// opens the file only; `:line:col` suffixes are stripped by the resolver.
    pub fn open_file_link(&mut self, link: String, pane_id: &str, cx: &mut Context<Self>) {
        let cwd = self.transcript_for_pane(pane_id).read(cx).session_cwd();
        let project_path = self
            .selected_project_for_pane(pane_id)
            .map(|project| project.path.clone());
        let resolved = console_ui::markdown::file_links::resolve_file_link(
            &link,
            cwd.as_deref(),
            project_path.as_deref(),
        );
        self.open_file_tab_in_pane(pane_id, resolved, cx);
    }

    pub fn open_file_tab(&mut self, path: String, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        self.open_file_tab_in_pane(&pane_id, path, cx);
    }

    pub fn open_file_tab_in_pane(&mut self, pane_id: &str, path: String, cx: &mut Context<Self>) {
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
            project_id: self.pane_project_id(pane_id),
            last_active_at_ms: None,
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
        self.inspector_selected_path = Some(path.clone());
        self.trim_file_caches();
        self.sync_workspace_webviews(cx);
        self.persist_workspaces();

        // Fetch file content or image preview
        self.fetch_file_tab_content(path, cx);

        cx.notify();
    }

    /// Fetch tab content depending on file kind (text/markdown vs raster image/SVG vs blocked).
    pub fn fetch_file_tab_content(&mut self, path: String, cx: &mut Context<Self>) {
        let kind = console_core::file_kind_for_path(&path);
        match kind {
            console_core::FileKind::Markdown | console_core::FileKind::Text => {
                if let Ok(content) = std::fs::read_to_string(&path) {
                    self.open_file_contents.insert(path, content);
                    cx.notify();
                    return;
                }

                let client = self.client.clone();
                let file_path = path;
                cx.spawn(
                    async move |entity, cx| match client.fs.read_file(&file_path).await {
                        Ok(resp) => {
                            cx.update(|cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_file_contents.insert(file_path, resp.content);
                                        cx.notify();
                                    });
                                }
                            });
                        }
                        Err(err) => {
                            log::warn!("Failed to read file for tab {}: {}", file_path, err);
                        }
                    },
                )
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

    pub fn open_diff_tab(&mut self, path: String, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        self.open_diff_tab_in_pane(&pane_id, path, cx);
    }

    pub fn open_diff_tab_in_pane(&mut self, pane_id: &str, path: String, cx: &mut Context<Self>) {
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
            project_id: self.pane_project_id(pane_id),
            last_active_at_ms: None,
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
        self.inspector_selected_path = Some(path.clone());
        self.trim_file_caches();
        self.persist_workspaces();

        let client = self.client.clone();
        let file_path = path.clone();
        let session_change_diff = self
            .inspector_session_changes
            .iter()
            .rfind(|c| {
                c.path == file_path
                    && c.diff_text
                        .as_deref()
                        .map(|d| !d.trim().is_empty())
                        .unwrap_or(false)
            })
            .and_then(|c| c.diff_text.clone());
        let cwd = self
            .selected_session_id
            .as_deref()
            .and_then(|id| self.sessions.iter().find(|s| s.id == id))
            .map(|s| s.cwd.clone())
            .or_else(|| {
                self.selected_project_id
                    .as_deref()
                    .and_then(|id| self.projects.iter().find(|p| p.id == id))
                    .map(|p| p.path.clone())
            });

        cx.spawn(async move |entity, cx| {
            let mut diff_raw = session_change_diff.unwrap_or_default();
            if diff_raw.trim().is_empty() {
                if let Ok(resp) = client.git.get_diff(cwd.as_deref(), Some(&file_path)).await {
                    diff_raw = resp.diff;
                }
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

        cx.notify();
    }

    /// Maximum retained file/viewer cache entries per map. Bounds long-session
    /// memory; evicted contents are re-fetched or re-rendered on reopen.
    pub(crate) fn trim_file_caches(&mut self) {
        const MAX_CACHED_FILES: usize = 30;
        let over = self.open_file_contents.len() > MAX_CACHED_FILES
            || self.open_image_contents.len() > MAX_CACHED_FILES
            || self.open_diff_contents.len() > MAX_CACHED_FILES
            || self.viewer_editor_views.len() > MAX_CACHED_FILES
            || self.viewer_diff_views.len() > MAX_CACHED_FILES
            || self.viewer_cached_markdown_views.len() > MAX_CACHED_FILES;
        if !over {
            return;
        }
        let open = self.open_file_paths_everywhere();
        let live_run_ids = self.live_run_script_ids();
        self.open_file_contents
            .retain(|path, _| open.contains(path));
        self.open_image_contents
            .retain(|path, _| open.contains(path));
        self.svg_preview_mode.retain(|path, _| open.contains(path));
        self.open_diff_contents
            .retain(|path, _| open.contains(path));
        self.viewer_editor_views
            .retain(|path, _| open.contains(path));
        self.viewer_diff_views
            .retain(|path, _| open.contains(path));
        self.viewer_cached_markdown_views
            .retain(|path, _| open.contains(path));
        self.viewer_list_states
            .retain(|key, _| Self::viewer_key_is_open(key, &open) || Self::run_key_is_live(key, &live_run_ids));
        self.viewer_scrollbar_states
            .retain(|key, _| Self::viewer_key_is_open(key, &open) || Self::run_key_is_live(key, &live_run_ids));
        self.viewer_markdown_selections
            .retain(|key, _| Self::viewer_key_is_open(key, &open) || Self::run_key_is_live(key, &live_run_ids));
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
    pub(crate) fn evict_file_caches_for_path(&mut self, path: &str) {
        self.open_file_contents.remove(path);
        self.open_image_contents.remove(path);
        self.svg_preview_mode.remove(path);
        self.open_diff_contents.remove(path);
        self.viewer_editor_views.remove(path);
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
    }
}
