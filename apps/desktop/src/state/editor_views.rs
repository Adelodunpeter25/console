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
        cx: &mut App,
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
            self.viewer_editor_views
                .insert(path.to_string(), (len, hash, state.clone(), view_clone.clone()));
            return view_clone;
        }

        let lang = syntax::LanguageRegistry::for_path(std::path::Path::new(path));
        let state = cx.new(|cx| {
            let mut s = editor_ui::EditorState::readonly(content, lang);
            s.set_theme(theme_preset, cx);
            s.set_wrap_enabled(false);
            s.set_font(font_config);
            s
        });
        let view = cx.new(|cx| editor_ui::EditorView::new(&state, cx));
        self.viewer_editor_views
            .insert(path.to_string(), (len, hash, state, view.clone()));
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
                    console_core::DiffLineKind::Added => (editor_ui::DiffLineKind::Added, true, false),
                    console_core::DiffLineKind::Removed => (editor_ui::DiffLineKind::Removed, false, true),
                    console_core::DiffLineKind::Context => (editor_ui::DiffLineKind::Context, false, false),
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
                    (diff_state.theme(), diff_state.wrap_enabled(), diff_state.font().clone())
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
            self.viewer_diff_views
                .insert(path.to_string(), (len, hash, state.clone(), view_clone.clone()));
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
