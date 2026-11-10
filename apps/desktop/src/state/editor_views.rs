//! Management of `editor-ui` `EditorView` and `DiffView` entities for file and diff tabs.

use std::collections::hash_map::DefaultHasher;
use std::hash::{Hash, Hasher};

use gpui::{App, Entity, prelude::*, px};

use super::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Standard font configuration for all code and diff viewers.
    pub fn editor_font_config() -> editor_ui::FontConfig {
        editor_ui::FontConfig {
            family: "JetBrains Mono".into(),
            size: px(12.0),
            line_height: px(20.0),
        }
    }

    /// Resolve syntax ThemePreset based on whether the app theme is dark or light.
    pub fn editor_theme_preset(theme: &console_ui::Theme) -> syntax::ThemePreset {
        if theme.is_dark {
            syntax::ThemePreset::GitHubDark
        } else {
            syntax::ThemePreset::GitHubLight
        }
    }

    /// Get or build a cached `EditorView` entity for a file path.
    pub fn get_or_build_editor_view(
        &mut self,
        path: &str,
        content: &str,
        theme: &console_ui::Theme,
        cx: &mut gpui::Context<Self>,
    ) -> Entity<editor_ui::EditorView> {
        let mut hasher = DefaultHasher::new();
        content.hash(&mut hasher);
        let hash = hasher.finish();
        let len = content.len();

        let theme_preset = Self::editor_theme_preset(theme);
        let font_config = Self::editor_font_config();

        if let Some((cached_len, cached_hash, state, view)) = self.viewer_editor_views.get(path) {
            if *cached_len == len && *cached_hash == hash {
                let (current_theme, current_wrap, current_font) = {
                    let editor = state.read(cx);
                    (editor.theme(), editor.wrap_enabled(), editor.font().clone())
                };
                if current_theme != theme_preset || current_wrap || current_font != font_config {
                    state.update(cx, |editor, cx| {
                        if editor.theme() != theme_preset {
                            editor.set_theme(theme_preset, cx);
                        }
                        if editor.wrap_enabled() {
                            editor.set_wrap_enabled(false);
                        }
                        if editor.font() != &font_config {
                            editor.set_font(font_config);
                        }
                    });
                }
                if let Some(line) = self.pending_file_line.remove(path) {
                    view.update(cx, |view, cx| {
                        view.select_line(line, cx);
                    });
                }
                return view.clone();
            }
            // A refresh must never replace edits that have not been saved. The
            // next render will retry once the host supplies matching content.
            if state.read(cx).is_dirty() {
                return view.clone();
            }
            state.update(cx, |editor, cx| {
                editor.set_text(content, cx);
                let lang = syntax::LanguageRegistry::for_path(std::path::Path::new(path));
                editor.set_language(lang, cx);
                if editor.theme() != theme_preset {
                    editor.set_theme(theme_preset, cx);
                }
                editor.set_wrap_enabled(false);
                if editor.font() != &font_config {
                    editor.set_font(font_config);
                }
            });
            let view_clone = view.clone();
            if let Some(line) = self.pending_file_line.remove(path) {
                view.update(cx, |view, cx| {
                    view.select_line(line, cx);
                });
            }
            self.viewer_editor_views.insert(
                path.to_string(),
                (len, hash, state.clone(), view_clone.clone()),
            );
            return view_clone;
        }

        let lang = syntax::LanguageRegistry::for_path(std::path::Path::new(path));
        let comment_tx = self.file_comment_tx.clone();
        let comment_path = path.to_string();
        let save_tx = self.file_save_tx.clone();
        let save_path = path.to_string();
        let state = cx.new(|cx| {
            // Editable mode: typing, caret motion, undo/redo, clipboard. The
            // comment gutter still works alongside editing. No `with_path`:
            // this client never writes workspace files on its own disk, so
            // every save goes through the API handler below.
            let mut s = editor_ui::EditorState::editable(content, lang);
            s.set_theme(theme_preset, cx);
            s.set_wrap_enabled(false);
            s.set_font(font_config);
            s.set_comment_gutter(true, cx);
            s.set_comment_target_handler(Some(Box::new(move |target| {
                let _ = comment_tx.send(super::types::FileCommentRequest {
                    path: comment_path.clone(),
                    start_line: target.start_line,
                    end_line: target.end_line,
                });
            })));
            // The handler must not block, so it only enqueues the API write and
            // reports it as deferred; the host task owns the outcome.
            s.set_save_handler(Some(Box::new(move |request: editor_ui::SaveRequest| {
                let _ = save_tx.send(super::types::FileSaveRequest {
                    path: save_path.clone(),
                    text: request.text,
                    revision: request.revision,
                });
                Ok(editor_ui::SaveStatus::Deferred)
            })));
            s
        });
        let view = cx.new(|cx| editor_ui::EditorView::new(&state, cx));
        if let Some(line) = self.pending_file_line.remove(path) {
            view.update(cx, |view, cx| {
                view.select_line(line, cx);
            });
        }
        self.viewer_editor_views
            .insert(path.to_string(), (len, hash, state.clone(), view.clone()));
        let file_path = path.to_string();
        let subscription = cx.subscribe(&state, move |app, _state, event: &editor_ui::EditorEvent, cx| {
            match event {
                // `mark_saved` emits this after the API write lands, so the
                // cache is refreshed from the editor's own text. The text is
                // `\n`-normalized, so apply the document's line ending to get
                // the bytes the file actually holds; hashing the normalized
                // form instead would make every CRLF file look changed on the
                // next render.
                editor_ui::EditorEvent::Saved => {
                    let (text, line_ending) = app
                        .viewer_editor_views
                        .get(&file_path)
                        .map(|(_, _, state, _)| {
                            let state = state.read(cx);
                            (state.text(), state.line_ending())
                        })
                        .unwrap_or_default();
                    let saved = line_ending.apply(&text).into_owned();
                    let mut hasher = DefaultHasher::new();
                    saved.hash(&mut hasher);
                    if let Some(entry) = app.viewer_editor_views.get_mut(&file_path) {
                        entry.0 = saved.len();
                        entry.1 = hasher.finish();
                    }
                    app.open_file_contents.insert(file_path.clone(), saved);
                }
                // Only the package's own on-disk writer emits this; API saves
                // report through the host task instead.
                editor_ui::EditorEvent::SaveFailed(message) => {
                    log::error!("Failed to save {}: {}", file_path, message);
                }
            }
        });
        self.viewer_editor_subscriptions
            .entry(path.to_string())
            .or_default()
            .push(subscription);
        // Every edit notifies the state, so an observer on it is enough to
        // repaint the app — which is what keeps the tab's dirty dot current.
        let observed = state.clone();
        let observed_path = path.to_string();
        let observer = cx.observe(&state, move |app, _state, cx| {
            let dirty = observed.read(cx).is_dirty();
            if app.dirty_file_paths_cache.get(&observed_path) != Some(&dirty) {
                app.dirty_file_paths_cache.insert(observed_path.clone(), dirty);
                cx.notify();
            }
        });
        self.viewer_editor_subscriptions
            .entry(path.to_string())
            .or_default()
            .push(observer);
        view
    }

    /// Get or build a cached `DiffView` entity for a diff tab.
    pub fn get_or_build_diff_view(
        &mut self,
        path: &str,
        diff_result: &console_core::DiffResult,
        raw_diff: &str,
        theme: &console_ui::Theme,
        cx: &mut App,
    ) -> Entity<editor_ui::DiffView> {
        let mut hasher = DefaultHasher::new();
        raw_diff.hash(&mut hasher);
        diff_result.lines.len().hash(&mut hasher);
        let hash = hasher.finish();
        let len = raw_diff.len() + diff_result.lines.len();

        let theme_preset = Self::editor_theme_preset(theme);
        let font_config = Self::editor_font_config();

        let convert_result = || {
            let mut diff_lines = Vec::with_capacity(diff_result.lines.len());
            let mut added = 0;
            let mut removed = 0;
            for line in &diff_result.lines {
                let (kind, is_add, is_rem) = match line.kind {
                    console_core::DiffLineKind::Added => {
                        (editor_ui::DiffLineKind::Added, true, false)
                    }
                    console_core::DiffLineKind::Removed => {
                        (editor_ui::DiffLineKind::Removed, false, true)
                    }
                    console_core::DiffLineKind::Context => {
                        (editor_ui::DiffLineKind::Context, false, false)
                    }
                };
                if is_add {
                    added += 1;
                }
                if is_rem {
                    removed += 1;
                }
                diff_lines.push(editor_ui::DiffLine {
                    kind,
                    text: line.text.clone(),
                    old_line: line.old_no.map(|n| n as u32),
                    new_line: line.new_no.map(|n| n as u32),
                });
            }
            editor_ui::DiffResult {
                lines: diff_lines,
                added,
                removed,
            }
        };

        if let Some((cached_len, cached_hash, state, view)) = self.viewer_diff_views.get(path) {
            if *cached_len == len && *cached_hash == hash {
                let (current_theme, current_wrap, current_font) = {
                    let diff_state = state.read(cx);
                    (
                        diff_state.theme(),
                        diff_state.wrap_enabled(),
                        diff_state.font().clone(),
                    )
                };
                if current_theme != theme_preset || current_wrap || current_font != font_config {
                    state.update(cx, |diff_state, _cx| {
                        if diff_state.theme() != theme_preset {
                            diff_state.set_theme(theme_preset);
                        }
                        if diff_state.wrap_enabled() {
                            diff_state.set_wrap_enabled(false);
                        }
                        if diff_state.font() != &font_config {
                            diff_state.set_font(font_config);
                        }
                    });
                }
                return view.clone();
            }
            state.update(cx, |diff_state, _cx| {
                diff_state.set_result(convert_result());
                let lang = syntax::LanguageRegistry::for_path(std::path::Path::new(path));
                diff_state.set_language(lang);
                if diff_state.theme() != theme_preset {
                    diff_state.set_theme(theme_preset);
                }
                diff_state.set_wrap_enabled(false);
                if diff_state.font() != &font_config {
                    diff_state.set_font(font_config);
                }
            });
            let view_clone = view.clone();
            self.viewer_diff_views.insert(
                path.to_string(),
                (len, hash, state.clone(), view_clone.clone()),
            );
            return view_clone;
        }

        let lang = syntax::LanguageRegistry::for_path(std::path::Path::new(path));
        let state = cx.new(|_| {
            let mut s = editor_ui::DiffState::from_result(convert_result(), lang);
            s.set_theme(theme_preset);
            s.set_wrap_enabled(false);
            s.set_font(font_config);
            s
        });
        let view = cx.new(|_| editor_ui::DiffView::new(&state));
        self.viewer_diff_views
            .insert(path.to_string(), (len, hash, state, view.clone()));
        view
    }

    pub fn viewer_list_state(
        &mut self,
        id: &str,
        count: usize,
        row_height: f32,
    ) -> gpui::ListState {
        let state = self
            .viewer_list_states
            .entry(id.to_string())
            .or_insert_with(|| {
                gpui::ListState::new(count, gpui::ListAlignment::Top, gpui::px(120.0))
                    .with_uniform_item_height(gpui::px(row_height))
            });
        if state.item_count() != count {
            state.reset_with_uniform_height(count, gpui::px(row_height));
        }
        state.clone()
    }

    pub fn viewer_scrollbar_state(&mut self, id: &str) -> std::rc::Rc<console_ui::ScrollbarState> {
        self.viewer_scrollbar_states
            .entry(id.to_string())
            .or_insert_with(console_ui::ScrollbarState::new)
            .clone()
    }

    pub fn get_or_build_markdown_view(
        &mut self,
        path: &str,
        content: &str,
    ) -> std::rc::Rc<std::cell::RefCell<console_ui::MarkdownView>> {
        let mut hasher = DefaultHasher::new();
        content.hash(&mut hasher);
        let hash = hasher.finish();
        let len = content.len();

        if let Some((cached_len, cached_hash, view)) = self.viewer_cached_markdown_views.get(path) {
            if *cached_len == len && *cached_hash == hash {
                return view.clone();
            }
        }

        let mut view = console_ui::MarkdownView::new();
        view.set_text(content, false);
        let rc = std::rc::Rc::new(std::cell::RefCell::new(view));
        self.viewer_cached_markdown_views
            .insert(path.to_string(), (len, hash, rc.clone()));
        rc
    }

    pub fn viewer_markdown_selection(
        &mut self,
        path: &str,
    ) -> console_ui::markdown::render::TranscriptSelection {
        self.viewer_markdown_selections
            .entry(path.to_string())
            .or_default()
            .clone()
    }
}
