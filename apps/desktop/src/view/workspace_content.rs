use console_core::types::ChangesScope;
use console_core::{SelectedModel, SessionHeaderExt, UpdateSessionDto};
use console_ui::workspace::EmptyChatState;
use console_ui::{
    ComposerView, ModelDropdownMenu, PickerTab, QuestionInteractionCard, ShellEntry, ShellsPill,
    SubagentsPill, SuggestionsPill, TasksPill, Theme, WorkspaceFooter, centered_stripe,
    composer_pills, error_banner, notice_banner, queued_prompts_stack,
};
use gpui::{
    App, AppContext, Context, InteractiveElement, IntoElement, ParentElement,
    StatefulInteractiveElement, Styled, Window, div, prelude::FluentBuilder, px,
};
use std::rc::Rc;

use crate::state::PaneChatSnapshot;
use crate::state::{ConsoleDesktopApp, ImageFileState};

impl ConsoleDesktopApp {
    pub(crate) fn render_workspace_content(
        &mut self,
        pane_id: &str,
        active_tab: Option<&console_core::WorkspaceTabConfig>,
        on_new_chat: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        // No chat tab open in this pane — show the centered empty state
        // (no transcript, no composer, no footer) instead of chat chrome.
        if active_tab.is_none() {
            return EmptyChatState::new(on_new_chat).into_any_element();
        }

        // Settings tab: render the embedded settings view for the active pane.
        if let Some(console_core::WorkspaceTabConfig::Settings {
            active_tab: subtab, ..
        }) = active_tab
        {
            let tab = subtab
                .as_deref()
                .and_then(console_ui::settings::SettingsTab::from_id)
                .unwrap_or(console_ui::settings::SettingsTab::Providers);
            let view = self.get_or_create_settings_view(tab, window, cx);
            return div().size_full().child(view).into_any_element();
        }

        // Terminal tab: render the live terminal surface for its id.
        // No cwd fallback: rehydrated tabs without a selected project must
        // not respawn with the process cwd or "." (it may not exist on the
        // current server). Surface a placeholder instead.
        if let Some(console_core::WorkspaceTabConfig::Terminal { terminal_id, .. }) = active_tab {
            if !self.terminals.contains_key(terminal_id) {
                let Some(cwd) = self
                    .selected_project_for_pane(pane_id)
                    .map(|project| project.path.clone())
                else {
                    let theme = Theme::current(cx);
                    return div()
                        .size_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .text_color(theme.text_ghost)
                        .text_size(px(12.0))
                        .child("Select a project to open a terminal")
                        .into_any_element();
                };
                self.get_or_create_terminal_view(terminal_id, cwd, window, cx);
            }
            let theme = Theme::current(cx);
            return match self.terminals.get(terminal_id) {
                Some(view) => div().size_full().child(view.clone()).into_any_element(),
                None => div()
                    .size_full()
                    .flex()
                    .items_center()
                    .justify_center()
                    .text_color(theme.text_ghost)
                    .text_size(px(12.0))
                    .child("Terminal session ended")
                    .into_any_element(),
            };
        }

        // Browser tab: render the native browser surface for its id.
        // Rehydrated tabs (persisted across launches) have no live view
        // yet: recreate it and re-navigate to the stored url.
        if let Some(console_core::WorkspaceTabConfig::Browser {
            browser_id, url, ..
        }) = active_tab
        {
            let view = self.get_or_create_browser_view(
                browser_id,
                if url.is_empty() {
                    None
                } else {
                    Some(url.clone())
                },
                window,
                cx,
            );
            let occluded = self.any_palette_open(cx) || view.read(cx).overlay_open(cx);
            view.update(cx, |v, cx| {
                v.sync_native_state(true, occluded, cx);
            });
            return div().size_full().child(view).into_any_element();
        }
        // File tab: render full-page MarkdownViewer, FileViewer, ImagePreview, or BlockedFilePanel
        if let Some(console_core::WorkspaceTabConfig::File { path, .. }) = active_tab {
            return self.render_file_tab(pane_id, path, window, cx);
        }

        // Diff tab: render full-page DiffViewer
        if let Some(console_core::WorkspaceTabConfig::Diff { path, .. }) = active_tab {
            return self.render_diff_tab(pane_id, path, window, cx);
        }

        // Changes review tab: every changed file in the tab's scope, stacked
        // with per-file mark-as-reviewed state.
        if let Some(console_core::WorkspaceTabConfig::ChangesReview {
            session_id, scope, ..
        }) = active_tab
        {
            return self.render_changes_review_tab(
                session_id.clone(),
                *scope,
                active_tab.map(|tab| tab.id()).unwrap_or_default(),
                cx,
            );
        }

        if let Some(console_core::WorkspaceTabConfig::Device { .. }) = active_tab {
            let device = self.device_view(window, cx);
            let occluded = self.any_palette_open(cx);
            device.update(cx, |view, cx| view.sync_native_state(true, occluded, cx));
            return div().size_full().child(device).into_any_element();
        }

        if let Some(console_core::WorkspaceTabConfig::FileTree { .. }) = active_tab {
            let tab_id = active_tab.map(|tab| tab.id()).unwrap_or_default();
            return self.render_file_tree_tab(tab_id, cx);
        }

        // Chat tab: transcript, pills, banners, composer and footer.
        self.render_chat_tab(pane_id, window, cx)
    }

    /// File tab: full-page MarkdownViewer, FileViewer, ImagePreview, or
    /// BlockedFilePanel, per the file's kind. Split out of
    /// `render_workspace_content`; each arm renders one preview shape.
    fn render_file_tab(
        &mut self,
        pane_id: &str,
        path: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        // A file tab in the chat's subtab split renders in a pane that never
        // hosted a chat: give it its own composer and comment storage, or it
        // would silently borrow the main pane's.
        self.ensure_workspace_pane_state(pane_id, window, cx);
        let path = path.to_owned();

        let theme = Theme::current(cx);
        let kind = console_core::file_kind_for_path(&path);

        // A text/markdown file that could not be read shows why, with a
        // retry, instead of the loading placeholder it would otherwise sit
        // on forever.
        if matches!(
            kind,
            console_core::FileKind::Markdown | console_core::FileKind::Text
        ) && !self.open_file_contents.contains_key(&path)
            && let Some(message) = self.open_file_errors.get(&path).cloned()
        {
            let retry_path = path.clone();
            let entity = cx.entity().downgrade();
            return console_ui::BlockedFilePanel::new("Couldn't open file", message)
                .icon(console_ui::IconName::TriangleAlert)
                .on_retry(move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.open_file_errors.remove(&retry_path);
                            this.fetch_file_tab_content(retry_path.clone(), cx);
                            cx.notify();
                        });
                    }
                })
                .into_any_element();
        }

        match kind {
            console_core::FileKind::RasterImage => {
                let state = self.ensure_image_file_state(&path, cx);
                self.render_image_file(
                    path,
                    state,
                    false,
                    "Loading image...",
                    "Failed to load image",
                    &theme,
                    cx,
                )
            }
            console_core::FileKind::Blocked => {
                // A binary file never becomes an image: a Loaded answer here
                // means the type sniffer could not classify it, so it renders
                // as the blocked panel the other arms never see.
                let state = match self.ensure_image_file_state(&path, cx) {
                    ImageFileState::Loaded { .. } => ImageFileState::Blocked {
                        title: "Binary file".to_string(),
                        message: format!(
                            "\"{path}\" isn't a text file, so there's nothing to preview here."
                        ),
                    },
                    state => state,
                };
                self.render_image_file(
                    path,
                    state,
                    false,
                    "Checking file...",
                    "Preview unavailable",
                    &theme,
                    cx,
                )
            }
            console_core::FileKind::Svg => {
                let mode = self
                    .svg_preview_mode
                    .get(&path)
                    .copied()
                    .unwrap_or(console_ui::SvgViewMode::Preview);
                match mode {
                    console_ui::SvgViewMode::Preview => {
                        let state = self.ensure_image_file_state(&path, cx);
                        self.render_image_file(
                            path,
                            state,
                            true,
                            "Loading SVG preview...",
                            "Failed to render SVG",
                            &theme,
                            cx,
                        )
                    }
                    console_ui::SvgViewMode::Source => {
                        let Some(content) = self.ensure_text_file_state(&path, cx) else {
                            return Self::file_loading_content_panel(&theme);
                        };
                        let file_viewer =
                            self.file_editor_viewer(pane_id, &path, &content, &theme, cx);
                        let header = console_ui::svg_source_header(
                            theme,
                            Some(Rc::new(Self::svg_mode_toggled(path, cx))),
                        );
                        div()
                            .size_full()
                            .flex()
                            .flex_col()
                            .child(header)
                            .child(div().flex_1().min_h_0().size_full().child(file_viewer))
                            .into_any_element()
                    }
                }
            }
            console_core::FileKind::Markdown => {
                let mode = self
                    .svg_preview_mode
                    .get(&path)
                    .copied()
                    .unwrap_or(console_ui::SvgViewMode::Preview);
                let header = console_ui::view_header(
                    theme,
                    "Markdown",
                    mode,
                    Some(Rc::new(Self::svg_mode_toggled(path.clone(), cx))),
                );

                if mode == console_ui::SvgViewMode::Source {
                    let Some(content) = self.ensure_text_file_state(&path, cx) else {
                        return Self::file_loading_content_panel(&theme);
                    };
                    let file_viewer = self.file_editor_viewer(pane_id, &path, &content, &theme, cx);
                    return div()
                        .size_full()
                        .flex()
                        .flex_col()
                        .child(header)
                        .child(div().flex_1().min_h_0().size_full().child(file_viewer))
                        .into_any_element();
                }

                if !self.open_file_contents.contains_key(&path) {
                    self.fetch_file_tab_content(path.clone(), cx);
                }
                let content = self
                    .open_file_contents
                    .get(&path)
                    .cloned()
                    .or_else(|| std::fs::read_to_string(&path).ok())
                    .unwrap_or_else(|| "Loading file content...".to_string());

                let view = self.get_or_build_markdown_view(&path, &content);
                let selection = self.viewer_markdown_selection(&path);
                let block_count = view.borrow().block_count();
                let list_state = self.viewer_list_state(&format!("md:{}", path), block_count, 60.0);
                let scrollbar_state = self.viewer_scrollbar_state(&format!("md:{}", path));
                // Clicked links route like the transcript's: file paths open
                // as workspace tabs, web URLs open in the embedded browser.
                // Without a handler links fall through to the OS URL opener,
                // which rejects bare file paths (macOS error -50).
                let pane = pane_id.to_owned();
                let entity: gpui::WeakEntity<ConsoleDesktopApp> = cx.entity().downgrade();
                let link_handler: console_ui::markdown::render::LinkHandler =
                    std::rc::Rc::new(move |url: &str, window, cx| {
                        if let Some(app) = entity.upgrade() {
                            if console_shared::markdown::file_links::is_http_url(url) {
                                app.update(cx, |this, cx| {
                                    this.open_chat_url_in_browser(url.to_owned(), window, cx);
                                });
                            } else if console_shared::markdown::file_links::is_file_link(url) {
                                app.update(cx, |this, cx| {
                                    this.open_file_link(url.to_owned(), &pane.clone(), cx);
                                });
                            }
                        }
                    });
                let viewer = console_ui::MarkdownViewer::new(path, view, list_state)
                    .selection(selection)
                    .scrollbar_state(scrollbar_state)
                    .link_handler(link_handler);
                div()
                    .size_full()
                    .flex()
                    .flex_col()
                    .child(header)
                    .child(div().flex_1().min_h_0().size_full().child(viewer))
                    .into_any_element()
            }
            console_core::FileKind::Video
            | console_core::FileKind::Audio
            | console_core::FileKind::Pdf
            | console_core::FileKind::Details => {
                let Some(details) = self.ensure_media_file_state(&path, cx) else {
                    if self.open_file_errors.contains_key(&path) {
                        let message = self
                            .open_file_errors
                            .get(&path)
                            .cloned()
                            .unwrap_or_default();
                        let retry_path = path.clone();
                        let entity = cx.entity().downgrade();
                        return console_ui::BlockedFilePanel::new("Couldn't open file", message)
                            .icon(console_ui::IconName::TriangleAlert)
                            .on_retry(move |_w, cx| {
                                if let Some(app) = entity.upgrade() {
                                    app.update(cx, |this, cx| {
                                        this.open_file_errors.remove(&retry_path);
                                        this.fetch_file_tab_content(retry_path.clone(), cx);
                                        cx.notify();
                                    });
                                }
                            })
                            .into_any_element();
                    }
                    return Self::file_loading_panel("Loading file…", &theme);
                };

                let file_name = std::path::Path::new(&path)
                    .file_name()
                    .and_then(|n| n.to_str())
                    .unwrap_or(&path)
                    .to_string();

                // Details files get a metadata card — no webview needed.
                let Some(player_kind) = details.player_kind() else {
                    let size_label = details
                        .size_bytes
                        .map(console_ui::format_bytes)
                        .unwrap_or_else(|| "Unknown".to_string());
                    let open_path = path.clone();
                    let reveal_path = path.clone();
                    let entity = cx.entity().downgrade();
                    return div()
                        .size_full()
                        .flex()
                        .items_center()
                        .justify_center()
                        .bg(theme.canvas)
                        .child(
                            div()
                                .w(px(360.0))
                                .p(px(24.0))
                                .rounded(px(10.0))
                                .bg(theme.surface)
                                .border_1()
                                .border_color(theme.border)
                                .flex()
                                .flex_col()
                                .items_center()
                                .gap(px(10.0))
                                .child(console_ui::file_type_icon(&path, 40.0))
                                .child(
                                    div()
                                        .text_size(px(14.0))
                                        .font_weight(gpui::FontWeight::SEMIBOLD)
                                        .text_color(theme.text)
                                        .child(file_name.clone()),
                                )
                                .child(
                                    div()
                                        .flex()
                                        .gap(px(16.0))
                                        .text_size(px(12.0))
                                        .text_color(theme.text_secondary)
                                        .child(details.mime_label.clone())
                                        .child(size_label),
                                )
                                .child(
                                    div()
                                        .flex()
                                        .gap(px(8.0))
                                        .mt(px(6.0))
                                        .child(
                                            div()
                                                .id("details-open-external")
                                                .px(px(14.0))
                                                .py(px(6.0))
                                                .rounded(px(6.0))
                                                .bg(theme.inverse)
                                                .text_color(theme.on_inverse)
                                                .text_size(px(12.0))
                                                .font_weight(gpui::FontWeight::MEDIUM)
                                                .cursor_pointer()
                                                .hover(|s| s.opacity(0.9))
                                                .on_click({
                                                    let open_path = open_path.clone();
                                                    let entity = entity.clone();
                                                    move |_, _window, cx| {
                                                        if let Some(app) = entity.upgrade() {
                                                            app.update(cx, |this, cx| {
                                                                this.open_file_externally(
                                                                    &open_path, cx,
                                                                );
                                                            });
                                                        }
                                                    }
                                                })
                                                .child("Open externally"),
                                        )
                                        .child(
                                            div()
                                                .id("details-reveal")
                                                .px(px(14.0))
                                                .py(px(6.0))
                                                .rounded(px(6.0))
                                                .border_1()
                                                .border_color(theme.border)
                                                .text_color(theme.text)
                                                .text_size(px(12.0))
                                                .cursor_pointer()
                                                .hover(|s| s.bg(theme.overlay))
                                                .on_click(move |_event, _window, cx| {
                                                    cx.reveal_path(std::path::Path::new(
                                                        &reveal_path,
                                                    ));
                                                })
                                                .child("Reveal in Finder"),
                                        ),
                                ),
                        )
                        .into_any_element();
                };

                // Video/audio/PDF: reuse or create the webview surface. The
                // view builds its page from the media URL, which needs the
                // auth token — fetched async, so the entity is created by a
                // task and this render shows the loading panel until it lands.
                if !self.file_media_views.contains_key(&path) {
                    self.spawn_media_view(&path, file_name, player_kind, window, cx);
                    return Self::file_loading_panel("Loading media…", &theme);
                }
                let view = self.file_media_views.get(&path).cloned().unwrap();
                let occluded = self.any_palette_open(cx);
                view.update(cx, |v, cx| v.sync_native_state(true, occluded, cx));
                div().size_full().child(view).into_any_element()
            }
            console_core::FileKind::Text => {
                let Some(content) = self.ensure_text_file_state(&path, cx) else {
                    return Self::file_loading_content_panel(&theme);
                };

                let comment_cards = self.file_comment_cards(pane_id, &path, window, cx);
                let viewer = self
                    .file_editor_viewer(pane_id, &path, &content, &theme, cx)
                    .comments(comment_cards);
                // No composer here: with one chat per desk the chat pane is
                // always beside the file, and that is where prompts — and the
                // staged comments collected from every pane — are sent from.
                div().size_full().child(viewer).into_any_element()
            }
        }
    }
    /// Create the `FileMediaView` for a video/audio/PDF file tab. The media
    /// URL embeds the device auth token (webviews cannot send the bearer
    /// header on subresource requests), so it is resolved asynchronously and
    /// the entity is inserted once it lands; renders show a loading panel
    /// until then.
    fn spawn_media_view(
        &mut self,
        path: &str,
        title: String,
        kind: console_ui::MediaFileKind,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let client = self.client.clone();
        let file_path = path.to_owned();
        let window_handle = window.window_handle();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_, cx| {
            let token = client.transport().auth_token().await;
            let url = client.fs.media_url(&file_path, token.as_deref()).await;
            let _ = cx.update(|cx| {
                let _ = window_handle.update(cx, |_, window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            // A tab closed while the URL resolved must not
                            // resurrect its surface.
                            if !this.file_media_views.contains_key(&file_path)
                                && this.file_path_is_open(&file_path)
                            {
                                let view = cx.new(|cx| {
                                    console_ui::FileMediaView::new(
                                        file_path.clone(),
                                        title.clone(),
                                        kind,
                                        url.clone(),
                                        window,
                                        cx,
                                    )
                                });
                                this.file_media_views.insert(file_path.clone(), view);
                                cx.notify();
                            }
                        });
                    }
                });
            });
        })
        .detach();
    }

    /// Open a file with the OS default handler (details card's
    /// "Open externally").
    pub(crate) fn open_file_externally(&mut self, path: &str, cx: &mut Context<Self>) {
        cx.open_with_system(std::path::Path::new(path));
    }

    /// Fetch-on-demand the decoded image for `path`, answering the current
    /// state (Loading until the fetch lands).
    fn ensure_image_file_state(&mut self, path: &str, cx: &mut Context<Self>) -> ImageFileState {
        if !self.open_image_contents.contains_key(path) {
            self.fetch_file_tab_content(path.to_owned(), cx);
        }
        self.open_image_contents
            .get(path)
            .cloned()
            .unwrap_or(ImageFileState::Loading)
    }

    /// Fetch-on-demand the probed media metadata for `path`; None while the
    /// fetch is in flight (or failed outside the read-error path).
    fn ensure_media_file_state(
        &mut self,
        path: &str,
        cx: &mut Context<Self>,
    ) -> Option<crate::state::FileMediaDetails> {
        if !self.open_media_contents.contains_key(path) && !self.open_file_errors.contains_key(path)
        {
            self.fetch_file_tab_content(path.to_owned(), cx);
        }
        self.open_media_contents.get(path).cloned()
    }

    /// Fetch-on-demand the decoded text for `path`; None while the fetch is
    /// in flight (or failed outside the read-error path the caller handles).
    fn ensure_text_file_state(&mut self, path: &str, cx: &mut Context<Self>) -> Option<String> {
        if !self.open_file_contents.contains_key(path) {
            self.fetch_file_tab_content(path.to_owned(), cx);
        }
        self.open_file_contents.get(path).cloned()
    }

    /// One image-file preview covering every [`ImageFileState`]: the loading
    /// spinner, the zoomable ImagePreview (SVG-decorated when `is_svg`), the
    /// retry panel, and the blocked panel. The kind-specific bits — labels
    /// and titles — arrive as parameters.
    fn render_image_file(
        &mut self,
        path: String,
        state: ImageFileState,
        is_svg: bool,
        loading_label: &str,
        failed_title: &str,
        theme: &Theme,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        match state {
            ImageFileState::Loading => Self::file_loading_panel(loading_label, theme),
            ImageFileState::Loaded {
                image,
                w,
                h,
                size_bytes,
                mime,
            } => {
                let meta = console_ui::ImageMeta {
                    width: w,
                    height: h,
                    size_bytes: Some(size_bytes),
                    mime_type: Some(mime),
                };
                let zoom_img = image.clone();
                let toggle_path = path.clone();
                let mut preview = console_ui::ImagePreview::new(path, image).meta(meta);
                if is_svg {
                    preview = preview
                        .is_svg(true)
                        .svg_mode(console_ui::SvgViewMode::Preview)
                        .on_toggle_svg_mode(Self::svg_mode_toggled(toggle_path, cx));
                }
                let entity = cx.entity().downgrade();
                preview
                    .on_zoom(move |_w, cx| {
                        let img = zoom_img.clone();
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.zoomed_image = Some(img);
                                cx.notify();
                            });
                        }
                    })
                    .into_any_element()
            }
            ImageFileState::Failed { message } => {
                Self::file_retry_panel(failed_title, message, path, cx)
            }
            ImageFileState::Blocked { title, message } => {
                console_ui::BlockedFilePanel::new(title, message)
                    .icon(console_ui::IconName::File)
                    .into_any_element()
            }
        }
    }

    /// The spinner row every file-preview loading state shares.
    fn file_loading_panel(label: &str, theme: &Theme) -> gpui::AnyElement {
        div()
            .size_full()
            .flex()
            .items_center()
            .justify_center()
            .gap(px(8.0))
            .text_color(theme.text_tertiary)
            .text_size(px(13.0))
            .child(console_ui::primitives::motion::spin_slow(
                console_ui::app_icon(
                    console_ui::IconName::LoaderCircle,
                    16.0,
                    theme.text_tertiary,
                ),
            ))
            .child(label.to_owned())
            .into_any_element()
    }

    /// The "still fetching" panel shown while a text/markdown fetch is in
    /// flight: the same centered spinner row as the image states.
    fn file_loading_content_panel(theme: &Theme) -> gpui::AnyElement {
        Self::file_loading_panel("Loading file…", theme)
    }

    /// The "failed" panel whose retry refetches the tab content.
    fn file_retry_panel(
        title: &str,
        message: String,
        path: String,
        cx: &Context<Self>,
    ) -> gpui::AnyElement {
        let entity = cx.entity().downgrade();
        console_ui::BlockedFilePanel::new(title, message)
            .icon(console_ui::IconName::TriangleAlert)
            .on_retry(move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.fetch_file_tab_content(path.clone(), cx);
                    });
                }
            })
            .into_any_element()
    }

    /// The store-then-notify closure every SVG preview-mode toggle shares
    /// (preview header, source header, markdown header). Callers wrap it in
    /// `Rc` where the builder wants a trait object.
    fn svg_mode_toggled(
        path: String,
        cx: &Context<Self>,
    ) -> impl Fn(console_ui::SvgViewMode, &mut Window, &mut App) + 'static {
        let entity = cx.entity().downgrade();
        move |new_mode, _w, cx| {
            if let Some(app) = entity.upgrade() {
                app.update(cx, |this, cx| {
                    this.svg_preview_mode.insert(path.clone(), new_mode);
                    cx.notify();
                });
            }
        }
    }

    /// The editor-backed FileViewer the SVG-source, Markdown-source, and
    /// text tabs share.
    fn file_editor_viewer(
        &mut self,
        pane_id: &str,
        path: &str,
        content: &str,
        theme: &Theme,
        cx: &mut Context<Self>,
    ) -> console_ui::FileViewer {
        let editor_view = self.get_or_build_editor_view(path, content, theme, cx);
        console_ui::FileViewer::new(editor_view).path(self.comment_display_path(pane_id, path))
    }

    /// Diff tab: full-page DiffViewer.
    fn render_diff_tab(
        &mut self,
        pane_id: &str,
        path: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        // Same as the file tab: the subtab split needs its own pane state.
        self.ensure_workspace_pane_state(pane_id, window, cx);
        let path = path.to_owned();

        // A tab restored from disk has no diff loaded; fetch it once.
        if !self.open_diff_contents.contains_key(&path) {
            self.open_diff_contents.insert(
                path.clone(),
                (console_core::DiffResult::default(), String::new()),
            );
            self.load_diff_for_tab(pane_id, path.clone(), cx);
        }

        let (diff_result, raw_diff) = self
            .open_diff_contents
            .get(&path)
            .cloned()
            .unwrap_or_else(|| (console_core::DiffResult::default(), String::new()));
        let theme = Theme::current(cx);
        let diff_view = self.get_or_build_diff_view(&path, &diff_result, &raw_diff, &theme, cx);
        self.sync_diff_comments(pane_id, &path, &diff_view, window, cx);
        // No composer here either — same as the file tab: the chat pane
        // beside this split is where prompts are sent from.
        div()
            .size_full()
            .child(
                console_ui::DiffViewer::new(diff_view)
                    .path(self.comment_display_path(pane_id, &path)),
            )
            .into_any_element()
    }

    /// Files tab: the project's file tree; selecting a file opens it as a tab.
    fn render_file_tree_tab(&mut self, tab_id: String, cx: &mut Context<Self>) -> gpui::AnyElement {
        // A tab restored from disk has no tree yet; load it once. The fetch
        // fills every open Files tab's entry.
        if !self.file_tree_data.contains_key(&tab_id) {
            self.file_tree_data
                .insert(tab_id.clone(), self.file_tree_nodes.clone());
            self.refresh_file_tree(cx);
        }
        let tree = self
            .file_tree_data
            .get(&tab_id)
            .cloned()
            .unwrap_or_else(|| self.file_tree_nodes.clone());
        let expanded = self
            .file_tree_expanded
            .get(&tab_id)
            .cloned()
            .unwrap_or_default();
        let entity = cx.entity().downgrade();
        let on_toggle_folder: Rc<dyn Fn(String, &mut Window, &mut App)> = {
            let entity = entity.clone();
            let tab_id = tab_id.clone();
            Rc::new(move |path, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.toggle_file_tree_folder(&tab_id, path, cx);
                    });
                }
            })
        };
        let entity_browser = entity.clone();
        let on_open_browser: Rc<dyn Fn(String, &mut Window, &mut App)> =
            Rc::new(move |path, _w, cx| {
                if let Some(app) = entity_browser.upgrade() {
                    app.update(cx, |this, cx| {
                        let target_pane = this.target_subtab_pane_id();
                        this.open_html_in_browser_tab(&target_pane, path, cx);
                    });
                }
            });
        let on_select_file: Rc<dyn Fn(String, &mut Window, &mut App)> =
            Rc::new(move |path, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| this.open_file_tab(path, cx));
                }
            });
        let scroll_handle = self.file_tree_scrolls.entry(tab_id).or_default().clone();
        div()
            .size_full()
            .min_h_0()
            .child(
                console_ui::FileTreeView::new(
                    tree,
                    expanded,
                    self.file_tree_selected_path.clone(),
                    String::new(),
                    on_toggle_folder,
                    self.git_badges.clone(),
                    on_select_file,
                )
                .on_open_browser(Some(on_open_browser))
                .track_scroll(scroll_handle),
            )
            .into_any_element()
    }

    /// Changes review tab: every changed file in the tab's scope, stacked
    /// with per-file mark-as-reviewed state.
    fn render_changes_review_tab(
        &mut self,
        session_id: String,
        scope: ChangesScope,
        tab_id: String,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        // A tab restored from disk has no data yet; fetch it once.
        if !self.changes_review_data.contains_key(&tab_id) {
            self.changes_review_data
                .insert(tab_id.clone(), Rc::new(Vec::new()));
            self.fetch_changes_review_data(&tab_id, session_id.clone(), cx);
        }
        let changes = self
            .changes_review_data
            .get(&tab_id)
            .cloned()
            .unwrap_or_else(|| Rc::new(Vec::new()));
        let collapsed = self
            .changes_review_collapsed
            .get(&tab_id)
            .cloned()
            .unwrap_or_default();

        let entity = cx.entity().downgrade();
        let tab_id_for_collapse = tab_id.clone();
        let on_toggle_collapsed: Rc<dyn Fn(String, &mut Window, &mut gpui::App)> = {
            let entity = entity.clone();
            let tab_id = tab_id_for_collapse;
            Rc::new(
                move |path: String, _window: &mut Window, cx: &mut gpui::App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.toggle_changes_review_collapsed(&tab_id, &path, cx);
                        });
                    }
                },
            )
        };
        let tab_id_for_reviewed = tab_id.clone();
        let session_id_for_reviewed = session_id.clone();
        let on_toggle_reviewed: Rc<dyn Fn(String, u32, &mut Window, &mut gpui::App)> = {
            let entity = entity.clone();
            let tab_id = tab_id_for_reviewed;
            let session_id = session_id_for_reviewed;
            Rc::new(
                move |path: String, turn_index: u32, _window: &mut Window, cx: &mut gpui::App| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.toggle_change_reviewed(
                                &tab_id,
                                session_id.clone(),
                                path,
                                turn_index,
                                cx,
                            );
                        });
                    }
                },
            )
        };

        let list_key = format!("changes-review:{}", tab_id);
        let count = console_core::types::count_changes_for_scope(&changes, scope);
        let list_state = self.viewer_list_states.entry(list_key).or_insert_with(|| {
            gpui::ListState::new(count, gpui::ListAlignment::Top, gpui::px(120.0))
        });
        if list_state.item_count() != count {
            list_state.reset(count);
        }
        let list_state = list_state.clone();

        console_ui::ReviewTab::new(
            scope,
            changes,
            collapsed,
            on_toggle_collapsed,
            on_toggle_reviewed,
        )
        .list_state(list_state)
        .into_any_element()
    }
    /// The chat tab: transcript, status pills, banners, and the docked
    /// composer + workspace footer. All state derivation lives in
    /// `state::workspace_snapshot` (`ConsoleDesktopApp::pane_chat_snapshot`);
    /// this maps the snapshot to elements and wires the element callbacks.
    fn render_chat_tab(
        &mut self,
        pane_id: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        self.ensure_workspace_pane_state(pane_id, window, cx);
        let pane_transcript = self.transcript_for_pane(pane_id);

        let theme = Theme::current(cx);
        let entity = cx.entity().downgrade();
        let pills_scroll = self.pills_scroll_for_pane(pane_id);
        // One pass over app state for everything the chat chrome needs.
        let snapshot = self.pane_chat_snapshot(pane_id);
        // Popovers for pills that just vanished cannot close themselves.
        self.gc_pane_pill_menus(pane_id, &snapshot, window, cx);

        let pane_session_id = snapshot.session_id.clone();
        let selected_sid = pane_session_id.clone();
        let changes_summary = pane_transcript.read(cx).changes_summary();
        let on_open_changes: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>> =
            pane_session_id.as_ref().map(|session_id| {
                let entity = entity.clone();
                let session_id = session_id.clone();
                let handler: Rc<dyn Fn(&mut Window, &mut App) + 'static> =
                    Rc::new(move |_window, cx| {
                        if let Some(app) = entity.upgrade() {
                            app.update(cx, |this, cx| {
                                this.open_changes_review_tab(
                                    session_id.clone(),
                                    ChangesScope::AllTurns,
                                    cx,
                                );
                            });
                        }
                    });
                handler
            });

        let tasks = snapshot.todos_active.then(|| TasksPill {
            completed: snapshot.todos_completed,
            total: snapshot.todos.len(),
            menu: self.pane_tasks_menu(pane_id),
            items: snapshot.todos.clone(),
        });
        let subagents_pill = (!snapshot.subagents.is_empty()).then(|| SubagentsPill {
            running: snapshot.subagents_running,
            menu: self.pane_subagents_menu(pane_id),
            items: snapshot.subagents.clone(),
        });
        let shells_pill = (!snapshot.shells.is_empty()).then(|| {
            let pane_session_id = pane_session_id.clone();
            let entity = entity.clone();
            let on_stop: Rc<dyn Fn(String, &mut Window, &mut App) + 'static> =
                Rc::new(move |work_id, _window, cx| {
                    if let (Some(app), Some(sid)) = (entity.upgrade(), pane_session_id.clone()) {
                        app.update(cx, |this, cx| this.stop_shell(sid, work_id, cx));
                    }
                });
            ShellsPill {
                menu: self.pane_shells_menu(pane_id),
                shells: Rc::new(
                    snapshot
                        .shells
                        .iter()
                        .map(|work| ShellEntry {
                            id: work.id.clone(),
                            command: work.shell_command(),
                        })
                        .collect(),
                ),
                on_stop,
            }
        });
        // Chips yield to queued prompts, which the user has already committed.
        let suggestions_pill =
            (!snapshot.suggestions.is_empty() && snapshot.queued_prompts.is_empty()).then(|| {
                let entity_pick = entity.clone();
                let pane_for_pick = pane_id.to_owned();
                SuggestionsPill {
                    items: snapshot.suggestions.clone(),
                    on_pick: Rc::new(move |prompt, _window, cx| {
                        if let Some(app) = entity_pick.upgrade() {
                            let pid = pane_for_pick.clone();
                            app.update(cx, |this, cx| {
                                this.send_suggestion_for_pane(&pid, prompt, cx);
                            });
                        }
                    }),
                }
            });
        let pills_key = format!("composer-pills-{pane_id}");
        let composer_pills_row = composer_pills(
            &pills_key,
            tasks,
            subagents_pill,
            shells_pill,
            changes_summary.zip(on_open_changes),
            suggestions_pill,
            &pills_scroll,
            theme,
        );

        // Built before the composer's `.when` closure so the closure only
        // captures the finished element, not `&mut self`.
        let pane_composer_element = self.pane_composer_view(pane_id, window, cx);

        div()
            .flex_1()
            .min_h_0()
            .w_full()
            .bg(theme.chat_canvas)
            .flex()
            .flex_col()
            .overflow_hidden()
            .child(
                div()
                    .relative()
                    .flex_1()
                    .w_full()
                    .overflow_hidden()
                    // Clicking an image in a message opens the app's
                    // image preview modal with its data URI. The handler is wired
                    // once during app initialization, not mutated from this
                    // render path.
                    .child(pane_transcript)
                    // Status pills float over the transcript's bottom edge rather
                    // than occupying a band of their own; the transcript keeps a
                    // matching bottom inset so the last row can scroll clear.
                    .when_some(composer_pills_row, |el, pills| el.child(pills)),
            )
            .when_some(
                snapshot
                    .error
                    .as_ref()
                    .map(|error| (error.message.clone(), error.persistent)),
                |el, error| {
                    let (error, error_persistent) = error;
                    let entity = entity.clone();
                    let error_pane_id = pane_id.to_owned();
                    el.child(error_banner(
                        error,
                        theme,
                        self.error_selection.clone(),
                        Some(Rc::new(move |cx| {
                            if let Some(app) = entity.upgrade() {
                                app.update(cx, |this, cx| {
                                    this.clear_error_for_pane(&error_pane_id, cx)
                                });
                            }
                        })),
                        // Retry/Discard only make sense on a persistent error
                        // (the failed worktree launch), not on every
                        // transient one.
                        if error_persistent {
                            Self::worktree_launch_actions(pane_session_id.clone(), cx)
                        } else {
                            Vec::new()
                        },
                        cx,
                    ))
                },
            )
            .when(
                snapshot.launch_state.as_deref() == Some("provisioning"),
                |el| {
                    // The server is cutting the worktree for this brand-new chat;
                    // the prompt auto-runs the moment it exists.
                    el.child(centered_stripe(
                        div()
                            .w_full()
                            .max_w(px(768.0))
                            .px(px(10.0))
                            .py(px(7.0))
                            .rounded(px(6.0))
                            .bg(theme.composer)
                            .border_1()
                            .border_color(theme.border)
                            .text_size(px(11.5))
                            .text_color(theme.text_secondary)
                            .child("Creating worktree…"),
                        6.0,
                    ))
                },
            )
            .when_some(snapshot.notice.clone(), |el, notice| {
                el.child(notice_banner(notice, theme))
            })
            .when(!snapshot.queued_prompts.is_empty(), |el| {
                let theme = Theme::current(cx);
                el.child(queued_prompts_stack(
                    snapshot.queued_prompts.clone(),
                    Some(Self::queued_prompt_action(
                        pane_id.to_owned(),
                        ConsoleDesktopApp::edit_queued_prompt_for_pane,
                        cx,
                    )),
                    Some(Self::queued_prompt_action(
                        pane_id.to_owned(),
                        ConsoleDesktopApp::delete_queued_prompt_for_pane,
                        cx,
                    )),
                    Some(Self::queued_prompt_action(
                        pane_id.to_owned(),
                        ConsoleDesktopApp::steer_queued_prompt_for_pane,
                        cx,
                    )),
                    theme,
                ))
            })
            .when_some(snapshot.pending_question.clone(), |el, question| {
                let card = self.build_question_card(pane_id, question, selected_sid.clone(), cx);
                el.child(centered_stripe(card, 8.0))
            })
            .when(selected_sid.is_some(), |el| {
                el.child({
                    let footer = self.build_workspace_footer(pane_id, &snapshot, cx);
                    div()
                        .relative()
                        .w_full()
                        .bg(theme.chat_canvas)
                        .flex()
                        .flex_col()
                        .items_center()
                        .child(pane_composer_element)
                        .child(footer)
                })
            })
            .into_any_element()
    }
    /// The docked workspace footer: project/branch pickers, usage meter, and
    /// its eight callbacks. Owns every read it needs, so `render_chat_tab`
    /// stays element assembly.
    fn build_workspace_footer(
        &mut self,
        pane_id: &str,
        snapshot: &PaneChatSnapshot,
        cx: &mut Context<Self>,
    ) -> WorkspaceFooter {
        let entity = cx.entity().downgrade();
        let pane_workspace_key = self.pane_real_project_id(pane_id);
        let pane_branches = self.pane_branches(pane_id);
        let pane_branch_loaded = self.pane_branch_loaded(pane_id);
        let pane_is_git_repository = self.pane_is_git_repository(pane_id);
        let pane_branch_pending = self.pane_branch_pending(pane_id);
        let pane_project_menu = self.pane_project_menu(pane_id);
        let pane_branch_menu = self.pane_branch_menu(pane_id);
        let pane_usage_menu = self.pane_usage_menu(pane_id);
        let project_pane_id = pane_id.to_owned();
        let clear_project_pane_id = pane_id.to_owned();
        let branch_pane_id = pane_id.to_owned();
        let new_worktree_pane_id = pane_id.to_owned();

        WorkspaceFooter::new(
            self.projects.clone(),
            pane_workspace_key,
            pane_branches,
            pane_branch_loaded,
            pane_is_git_repository,
            pane_branch_pending,
            pane_project_menu,
            pane_branch_menu,
            pane_usage_menu,
            snapshot.usage_report.clone(),
            snapshot.usage_loading,
            snapshot.context_snapshot.clone(),
            {
                let entity = entity.clone();
                move |id: String, _w, cx| {
                    if let Some(app) = entity.upgrade() {
                        let pane_id = project_pane_id.clone();
                        app.update(cx, |this, cx| this.select_project_for_pane(pane_id, id, cx));
                    }
                }
            },
            {
                let entity = entity.clone();
                move |window, cx| {
                    if let Some(app) = entity.upgrade() {
                        // Open the ⌘O directory browser palette — it
                        // works against remote backends, where the
                        // native dialog can't see the host filesystem.
                        app.update(cx, |this, cx| this.open_project_browse(window, cx));
                    }
                }
            },
            {
                let entity = entity.clone();
                move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        let pane_id = clear_project_pane_id.clone();
                        app.update(cx, |this, cx| this.clear_project_for_pane(pane_id, cx));
                    }
                }
            },
            {
                let entity = entity.clone();
                move |repo: console_core::GithubRepo, _w, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.open_repo_task(repo, cx));
                    }
                }
            },
            {
                let entity = entity.clone();
                move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| this.refresh_github_repos(cx));
                    }
                }
            },
            {
                let entity = entity.clone();
                move |window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            // Not open_settings: it toggles, and
                            // closing settings the user already had
                            // open would be a surprise from a menu
                            // row.
                            this.open_settings_tab(
                                console_ui::settings::SettingsTab::Providers,
                                window,
                                cx,
                            );
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                move |name: String, _w, cx| {
                    if let Some(app) = entity.upgrade() {
                        let pane_id = branch_pane_id.clone();
                        app.update(cx, |this, cx| {
                            this.checkout_branch_for_pane(pane_id, name, cx)
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        let pane_id = new_worktree_pane_id.clone();
                        app.update(cx, |this, cx| this.new_worktree_for_pane(pane_id, cx));
                    }
                }
            },
        )
        .project_locked(snapshot.session_has_messages)
        .github_section(
            self.github_repos.clone(),
            self.github_repos_loading,
            self.github_repos_error.clone(),
        )
        .repo_cloning(self.repo_task_cloning.clone())
    }

    /// The agent's pending question card, wired to the session that owns it
    /// (not whichever pane holds focus when an option is clicked).
    fn build_question_card(
        &mut self,
        pane_id: &str,
        question: console_core::types::agent::AskQuestionRequest,
        session_id: Option<String>,
        cx: &mut Context<Self>,
    ) -> QuestionInteractionCard {
        let entity = cx.entity().downgrade();
        let question_input = self.question_input_for_pane(pane_id);
        let selected = self.question_selected_for_pane(pane_id);
        let is_multi = question.is_multi_select.unwrap_or(false);
        let is_approve_deny = !is_multi
            && question.options.as_ref().map(|o| o.len()).unwrap_or(0) == 2
            && question.options.as_ref().unwrap().iter().any(|o| o.eq_ignore_ascii_case("approve") || o.eq_ignore_ascii_case("allow"))
            && question.options.as_ref().unwrap().iter().any(|o| o.eq_ignore_ascii_case("deny") || o.eq_ignore_ascii_case("decline"));
        let placeholder = if is_approve_deny {
            "Optional: reason or feedback to stay in plan mode..."
        } else {
            "Type your answer..."
        };
        question_input.update(cx, |input, cx| input.set_placeholder(placeholder, cx));
        let skippable = question.skippable != Some(false);
        let answer_sid = session_id.clone();
        let skip_sid = session_id.clone();
        let entity_for_answer = entity.clone();
        let entity_for_skip = entity.clone();
        let question_card =
            QuestionInteractionCard::new(question, true, false, move |answer, _window, cx| {
                let Some(sid) = answer_sid.clone() else {
                    return;
                };
                if let Some(app) = entity_for_answer.upgrade() {
                    app.update(cx, |this, cx| {
                        this.answer_pending_question_for_session(sid, answer, cx);
                    });
                }
            })
            .custom_input(question_input)
            .selected(selected);
        let question_card = if skippable {
            question_card.on_skip(move |_window, cx| {
                let Some(sid) = skip_sid.clone() else {
                    return;
                };
                if let Some(app) = entity_for_skip.upgrade() {
                    app.update(cx, |this, cx| {
                        this.skip_pending_question_for_session(sid, cx);
                    });
                }
            })
        } else {
            question_card
        };
        question_card.on_select(move |option, _window, cx| {
            let Some(sid) = session_id.clone() else {
                return;
            };
            if let Some(app) = entity.upgrade() {
                app.update(cx, |this, cx| {
                    if is_multi {
                        let mut sel = this.question_selected_for_session(&sid);
                        if !sel.insert(option.clone()) {
                            sel.remove(&option);
                        }
                        this.set_question_selected_for_session(&sid, sel);
                    } else {
                        let mut sel = std::collections::HashSet::new();
                        sel.insert(option);
                        this.set_question_selected_for_session(&sid, sel);
                    }
                    cx.notify();
                });
            }
        })
    }

    /// The Retry/Discard pair a persistent (failed worktree launch) banner
    /// carries; empty for transient errors.
    fn worktree_launch_actions(
        session_id: Option<String>,
        cx: &Context<Self>,
    ) -> Vec<console_ui::common::BannerAction> {
        let session_id = session_id.unwrap_or_default();
        let entity = cx.entity().downgrade();
        let retry_entity = entity.clone();
        let retry_session = session_id.clone();
        let discard_entity = entity;
        let discard_session = session_id;
        vec![
            console_ui::common::BannerAction {
                label: "Retry".into(),
                on_click: Rc::new(move |cx: &mut App| {
                    if let Some(app) = retry_entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.retry_worktree_launch(retry_session.clone(), cx);
                        });
                    }
                }),
            },
            console_ui::common::BannerAction {
                label: "Discard".into(),
                on_click: Rc::new(move |cx: &mut App| {
                    if let Some(app) = discard_entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.discard_worktree_launch(discard_session.clone(), cx);
                        });
                    }
                }),
            },
        ]
    }

    /// The upgrade-entity ceremony every queued-prompt callback shares; the
    /// three actions differ only in which state method they call.
    fn queued_prompt_action(
        pane_id: String,
        action: fn(&mut ConsoleDesktopApp, String, String, &mut Context<ConsoleDesktopApp>),
        cx: &Context<Self>,
    ) -> Rc<dyn Fn(String, &mut Window, &mut App) + 'static> {
        let entity = cx.entity().downgrade();
        Rc::new(move |prompt_id, _window, cx| {
            if let Some(app) = entity.upgrade() {
                let pid = pane_id.clone();
                app.update(cx, |this, cx| action(this, pid, prompt_id, cx));
            }
        })
    }

    fn pane_composer_view(
        &mut self,
        pane_id: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        let pane_composer = self.composer_for_pane(pane_id);
        let pane_selected_model = self.effective_model_for_pane(pane_id);
        let pane_picker_tab = self.pane_picker_tab(pane_id);
        let pane_approval_mode = self.pane_approval_mode(pane_id);
        let pane_thinking_level = self.pane_thinking_level(pane_id);
        let pane_supported_thinking_levels = self.supported_thinking_levels_for_pane(pane_id);
        let pane_model_menu = self.pane_model_menu(pane_id);
        let pane_thinking_menu = self.pane_thinking_menu(pane_id);
        let pane_model_search = self.pane_model_search(pane_id);
        let pane_model_search_query = pane_model_search.read(cx).content().to_owned();
        let pane_session_id = self.active_session_for_pane(pane_id);
        let autocomplete =
            self.composer_autocomplete_for_pane(pane_id, pane_session_id.as_deref(), window, cx);
        let comments = self.composer_comments_for_pane(pane_id, cx);
        let annotations = self.composer_annotations_for_pane(pane_id, cx);
        let attachments = self.attachments_for_pane(pane_id);

        let entity = cx.entity().downgrade();
        let client = self.client.clone();
        let providers = self.providers.clone();
        let favorites = self.favorites.clone();
        let models_by_provider = self.models_by_provider.clone();

        let model_dropdown = ModelDropdownMenu::new(
            providers,
            pane_selected_model.clone(),
            pane_picker_tab,
            favorites,
            models_by_provider,
            pane_model_search.clone(),
            pane_model_search_query,
            pane_model_menu.clone(),
            {
                let entity = entity.clone();
                let model_handle = pane_model_menu.clone();
                let pane_id = pane_id.to_string();
                move |prov: String, m_id: String, window, cx| {
                    model_handle.close(window, cx);
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let model = SelectedModel {
                                provider: prov.clone(),
                                model_id: m_id.clone(),
                            };
                            this.set_pane_model(&pane_id, Some(model));
                            this.update_session_settings_for_pane(
                                pane_id.clone(),
                                UpdateSessionDto {
                                    title: None,
                                    cwd: None,
                                    project_id: None,
                                    model_id: Some(m_id.clone()),
                                    provider: Some(prov.clone()),
                                    approval_mode: None,
                                    thinking_level: None,
                                    pinned: None,
                                },
                                cx,
                            );
                            cx.notify();
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                let search = pane_model_search.clone();
                let pane_id = pane_id.to_string();
                move |tab: PickerTab, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            if let PickerTab::Provider(ref name) = tab {
                                this.load_models_for_provider(name, cx);
                            }
                            this.set_pane_picker_tab(&pane_id, tab);
                            search.update(cx, |input, cx| input.clear(cx));
                            cx.notify();
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                move |prov: String, mid: String, _window, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.toggle_model_favorite(prov, mid, cx);
                        });
                    }
                }
            },
        );

        let composer = ComposerView::new(
            pane_composer.clone(),
            {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            let prompt = this
                                .composer_for_pane(&pane_id)
                                .read(cx)
                                .content()
                                .to_string();
                            let prompt = this.prompt_with_staged_comments(&pane_id, &prompt, cx);
                            if prompt.trim().is_empty() {
                                return;
                            }
                            let attachments = (*this.attachments_for_pane(&pane_id)).clone();
                            let annotations = (*this.annotations_for_pane(&pane_id)).clone();
                            let context_files =
                                console_shared::composer::mentions::submitted_mention_paths(
                                    this.composer_for_pane(&pane_id).read(cx).mentions(),
                                );
                            if this.is_active_session_running_for_pane(&pane_id) {
                                this.queue_prompt_for_pane(
                                    pane_id.clone(),
                                    prompt,
                                    attachments,
                                    annotations,
                                    context_files,
                                    cx,
                                );
                                return;
                            }
                            this.active_pane_id = Some(pane_id.clone());
                            this.selected_session_id = this.active_session_for_pane(&pane_id);
                            this.submit_prompt_with_context(
                                prompt,
                                attachments,
                                annotations,
                                context_files,
                                cx,
                            );
                        });
                    }
                }
            },
            {
                let client = client.clone();
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_w, cx| {
                    let client = client.clone();
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.active_pane_id = Some(pane_id.clone());
                            this.selected_session_id = this.active_session_for_pane(&pane_id);
                            if let Some(sid) = &this.selected_session_id {
                                let sid_clone = sid.clone();
                                this.set_session_running(&sid_clone, None);
                                let entity_for_error = entity.clone();
                                cx.spawn(async move |_entity, cx| {
                                    if let Err(error) = client.runs.abort(&sid_clone).await {
                                        let message =
                                            format!("Unable to stop the agent run: {error}");
                                        cx.update(|cx| {
                                            if let Some(app) = entity_for_error.upgrade() {
                                                app.update(cx, |this, cx| {
                                                    this.set_error_for_session(
                                                        &sid_clone, message, cx,
                                                    )
                                                });
                                            }
                                        });
                                    }
                                })
                                .detach();
                            }
                            cx.notify();
                        });
                    }
                }
            },
            {
                let entity = entity.clone();
                let pane_id = pane_id.to_string();
                move |_w, cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.pick_image(&pane_id, cx);
                        });
                    }
                }
            },
        )
        .running(self.is_active_session_running_for_pane(pane_id))
        .has_queued_prompt(self.has_queued_prompt_for_pane(pane_id))
        .on_queue({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        let prompt = this
                            .composer_for_pane(&pane_id)
                            .read(cx)
                            .content()
                            .to_string();
                        let prompt = this.prompt_with_staged_comments(&pane_id, &prompt, cx);
                        if prompt.trim().is_empty() {
                            return;
                        }
                        let attachments = (*this.attachments_for_pane(&pane_id)).clone();
                        let annotations = (*this.annotations_for_pane(&pane_id)).clone();
                        let context_files =
                            console_shared::composer::mentions::submitted_mention_paths(
                                this.composer_for_pane(&pane_id).read(cx).mentions(),
                            );
                        this.queue_prompt_for_pane(
                            pane_id.clone(),
                            prompt,
                            attachments,
                            annotations,
                            context_files,
                            cx,
                        );
                    });
                }
            }
        })
        .selected_model(pane_selected_model)
        .approval_mode(pane_approval_mode)
        .thinking_level(pane_thinking_level)
        .supported_thinking_levels(pane_supported_thinking_levels)
        .thinking_menu(pane_thinking_menu)
        .on_choose_thinking({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |level, window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.pane_thinking_menu(&pane_id).close(window, cx);
                        this.choose_thinking_level_for_pane(&pane_id, level, cx);
                    });
                }
            }
        })
        .model_dropdown(model_dropdown, pane_model_menu)
        .autocomplete(autocomplete)
        .on_autocomplete_next({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.move_autocomplete_for_pane(&pane_id, true, cx);
                    });
                }
            }
        })
        .on_autocomplete_previous({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.move_autocomplete_for_pane(&pane_id, false, cx);
                    });
                }
            }
        })
        .on_autocomplete_confirm({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.accept_highlighted_autocomplete_for_pane(&pane_id, cx);
                    });
                }
            }
        })
        .on_autocomplete_dismiss({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.dismiss_autocomplete_for_pane(&pane_id, cx);
                    });
                }
            }
        })
        .attachments(attachments)
        .comments(comments)
        .annotations(annotations)
        .mic_enabled(self.stt_enabled())
        .mic_recording(self.stt_recorder.is_some())
        .mic_transcribing(self.stt_transcribing)
        .mic_elapsed_secs(self.stt_recording_secs)
        .mic_error(self.stt_mic_error.clone())
        .on_mic_click({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |_w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.toggle_stt_recording(pane_id.clone(), cx);
                    });
                }
            }
        })
        .on_remove_attachment({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |index, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.remove_attachment(&pane_id, index, cx);
                    });
                }
            }
        })
        .on_remove_annotation({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |index, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.remove_annotation(&pane_id, index, cx);
                    });
                }
            }
        })
        .on_preview_attachment({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |index, _w, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.preview_attachment(&pane_id, index, cx);
                    });
                }
            }
        })
        .on_drop_files({
            let entity = entity.clone();
            let pane_id = pane_id.to_string();
            move |paths, window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.stage_dropped_files(&pane_id, paths, window, cx);
                    });
                }
            }
        });

        composer.into_any_element()
    }

    /// Bring the pane's active chat to the front after a file-tab send so the
    /// streamed answer is visible. A brand-new session is opened by the run
    /// path itself.
    pub(crate) fn reveal_chat_for_pane(&mut self, pane_id: &str, cx: &mut Context<Self>) {
        let Some(session_id) = self.active_session_for_pane(pane_id) else {
            return;
        };
        let title = self
            .sessions
            .iter()
            .find(|session| session.id == session_id)
            .map(|session| session.display_title().to_string())
            .unwrap_or_else(|| "Chat".to_string());
        self.open_chat_tab_in_pane(pane_id, session_id, title, cx);
        cx.notify();
    }
}
