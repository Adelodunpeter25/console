use std::rc::Rc;

use console_core::{ApprovalMode, ImageAttachment, SelectedModel, ThinkingLevel};
use gpui::{
    App, Div, ElementId, Entity, ExternalPaths, FontWeight, InteractiveElement, IntoElement,
    ParentElement, RenderOnce, StatefulInteractiveElement, Styled, Window, div, img,
    prelude::FluentBuilder, px,
};

use crate::common::{
    AutocompleteConfirm, AutocompleteDismiss, AutocompleteNext, AutocompletePrevious,
    AutocompleteView, ModelDropdownMenu, attachment_image, file_mention_chip,
    format_model_name, provider_svg_path,
};
use crate::input::ComposerInput;
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{
    app_icon, dropdown_menu, ContextMenuHandle, IconName, MenuAlign, MenuChip, MenuItem, popover,
};
use crate::theme::Theme;

/// A staged file comment shown as a chip above the composer's text field.
///
/// The chip mirrors the inline file-mention pill (`address.rs +23-35`) and
/// keeps the two actions the inline card also offers: reopen for editing and
/// remove the comment from the message.
#[derive(Clone)]
pub struct ComposerComment {
    pub id: u64,
    pub path: String,
    pub label: String,
    pub on_edit: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_remove: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl ComposerComment {
    pub fn new(
        id: u64,
        path: impl Into<String>,
        label: impl Into<String>,
        on_edit: impl Fn(&mut Window, &mut App) + 'static,
        on_remove: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            id,
            path: path.into(),
            label: label.into(),
            on_edit: Rc::new(on_edit),
            on_remove: Rc::new(on_remove),
        }
    }
}

/// A staged browser element annotation shown as a chip above the composer.
/// Label is `Component • file:line` (or selector fallback); tooltip carries
/// the user comment.
#[derive(Clone)]
pub struct ComposerAnnotation {
    pub id: String,
    pub label: String,
    pub tooltip: String,
    pub on_remove: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl ComposerAnnotation {
    pub fn new(
        id: impl Into<String>,
        label: impl Into<String>,
        tooltip: impl Into<String>,
        on_remove: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            tooltip: tooltip.into(),
            on_remove: Rc::new(on_remove),
        }
    }
}

/// The composer state that is meaningful to the backend-facing shell.
///
/// The component only paints this state and emits callbacks. It deliberately
/// does not own a `ConsoleClient`, so it can be previewed independently and
/// later mounted by any session implementation.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum ComposerRunState {
    #[default]
    Ready,
    Preparing,
    Running,
}

impl ComposerRunState {
    pub fn is_running(self) -> bool {
        matches!(self, Self::Preparing | Self::Running)
    }
}

/// Waku-shaped prompt composer with callbacks at every backend seam.
///
/// `ImageAttachment`, `SlashCommandInfo`, `SelectedModel`, and `ApprovalMode`
/// are the same data concepts used by `console-core`; the component never
/// invents a parallel transport model. The parent remains responsible for
/// turning a submitted draft into `RunPromptDto` and for calling `RunService`.
/// Plan mode reads as faint accent stripes laid over a composer card. It sits
/// first among the card's children, so everything else paints above it.
pub(crate) fn plan_mode_stripes(theme: Theme) -> Div {
    div()
        .absolute()
        .inset_0()
        .bg(gpui::pattern_slash(theme.accent.opacity(0.24), 1.5, 7.0))
}

#[derive(IntoElement)]
pub struct ComposerView {
    pub composer_input: Entity<ComposerInput>,
    pub run_state: ComposerRunState,
    pub selected_model: Option<SelectedModel>,
    pub approval_mode: ApprovalMode,
    pub thinking_level: Option<ThinkingLevel>,
    pub supported_thinking_levels: Vec<ThinkingLevel>,
    pub attachments: Rc<Vec<ImageAttachment>>,
    /// File line comments staged for this pane. Rendered as chips above the
    /// input; they are part of the outgoing prompt without occupying the text
    /// field.
    pub comments: Rc<Vec<ComposerComment>>,
    pub annotations: Rc<Vec<ComposerAnnotation>>,
    /// A floating overlay (file/diff tabs) rather than the docked chat footer.
    floating: bool,
    pub has_queued_prompt: bool,
    /// Speech-to-text dictation button. `mic_enabled` is false until the
    /// server has a credential, so the button renders disabled rather than
    /// failing on tap; `mic_recording` swaps it for the stop control with
    /// the live elapsed label, and `mic_transcribing` for the spinner.
    pub mic_enabled: bool,
    pub mic_recording: bool,
    pub mic_transcribing: bool,
    pub mic_elapsed_secs: u32,
    /// Failure text for the dictation flow (mic denied, transcribe failed),
    /// shown next to the button.
    pub mic_error: Option<String>,
    model_menu: Option<(ModelDropdownMenu, ContextMenuHandle)>,
    thinking_menu: Option<ContextMenuHandle>,
    autocomplete: Option<AutocompleteView>,
    on_send: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_queue: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_abort: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_pick_image: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_choose_thinking: Option<Rc<dyn Fn(ThinkingLevel, &mut Window, &mut App) + 'static>>,
    on_mic_click: Option<Rc<dyn Fn(&mut Window, &mut App) + 'static>>,
    on_remove_attachment: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_preview_attachment: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_remove_annotation: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_drop_files: Rc<dyn Fn(&ExternalPaths, &mut Window, &mut App) + 'static>,
    on_autocomplete_next: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_autocomplete_previous: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_autocomplete_confirm: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_autocomplete_dismiss: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl ComposerView {
    pub fn new(
        composer_input: Entity<ComposerInput>,
        on_send: impl Fn(&mut Window, &mut App) + 'static,
        on_abort: impl Fn(&mut Window, &mut App) + 'static,
        on_pick_image: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        let on_send = Rc::new(on_send);
        Self {
            composer_input,
            run_state: ComposerRunState::Ready,
            selected_model: None,
            approval_mode: ApprovalMode::default(),
            thinking_level: None,
            supported_thinking_levels: Vec::new(),
            attachments: Rc::new(Vec::new()),
            comments: Rc::new(Vec::new()),
            annotations: Rc::new(Vec::new()),
            floating: false,
            has_queued_prompt: false,
            mic_enabled: false,
            mic_recording: false,
            mic_transcribing: false,
            mic_elapsed_secs: 0,
            mic_error: None,
            model_menu: None,
            thinking_menu: None,
            autocomplete: None,
            on_queue: on_send.clone(),
            on_send,
            on_abort: Rc::new(on_abort),
            on_pick_image: Rc::new(on_pick_image),
            on_choose_thinking: None,
            on_mic_click: None,
            on_remove_attachment: Rc::new(|_: usize, _: &mut Window, _: &mut App| {}),
            on_preview_attachment: Rc::new(|_: usize, _: &mut Window, _: &mut App| {}),
            on_remove_annotation: Rc::new(|_: usize, _: &mut Window, _: &mut App| {}),
            on_drop_files: Rc::new(|_: &ExternalPaths, _: &mut Window, _: &mut App| {}),
            on_autocomplete_next: Rc::new(|_, _| {}),
            on_autocomplete_previous: Rc::new(|_, _| {}),
            on_autocomplete_confirm: Rc::new(|_, _| {}),
            on_autocomplete_dismiss: Rc::new(|_, _| {}),
        }
    }

    /// Compatibility builder for the existing app shell.
    pub fn running(mut self, running: bool) -> Self {
        self.run_state = if running {
            ComposerRunState::Running
        } else {
            ComposerRunState::Ready
        };
        self
    }

    pub fn run_state(mut self, state: ComposerRunState) -> Self {
        self.run_state = state;
        self
    }

    pub fn preparing(mut self, preparing: bool) -> Self {
        if preparing {
            self.run_state = ComposerRunState::Preparing;
        } else if self.run_state == ComposerRunState::Preparing {
            self.run_state = ComposerRunState::Ready;
        }
        self
    }

    pub fn selected_model(mut self, model: Option<SelectedModel>) -> Self {
        self.selected_model = model;
        self
    }

    pub fn approval_mode(mut self, mode: ApprovalMode) -> Self {
        self.approval_mode = mode;
        self
    }

    pub fn thinking_level(mut self, level: Option<ThinkingLevel>) -> Self {
        self.thinking_level = level;
        self
    }

    pub fn supported_thinking_levels(mut self, levels: Vec<ThinkingLevel>) -> Self {
        self.supported_thinking_levels = levels;
        self
    }

    pub fn thinking_menu(mut self, handle: ContextMenuHandle) -> Self {
        self.thinking_menu = Some(handle);
        self
    }

    pub fn on_choose_thinking(
        mut self,
        handler: impl Fn(ThinkingLevel, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_choose_thinking = Some(Rc::new(handler));
        self
    }

    pub fn mic_enabled(mut self, enabled: bool) -> Self {
        self.mic_enabled = enabled;
        self
    }

    pub fn mic_recording(mut self, recording: bool) -> Self {
        self.mic_recording = recording;
        self
    }

    pub fn mic_transcribing(mut self, transcribing: bool) -> Self {
        self.mic_transcribing = transcribing;
        self
    }

    pub fn mic_elapsed_secs(mut self, secs: u32) -> Self {
        self.mic_elapsed_secs = secs;
        self
    }

    pub fn mic_error(mut self, error: Option<String>) -> Self {
        self.mic_error = error;
        self
    }

    pub fn on_mic_click(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_mic_click = Some(Rc::new(handler));
        self
    }

    pub fn model_dropdown(
        mut self,
        dropdown: ModelDropdownMenu,
        handle: ContextMenuHandle,
    ) -> Self {
        self.model_menu = Some((dropdown, handle));
        self
    }

    pub fn attachments(mut self, attachments: Rc<Vec<ImageAttachment>>) -> Self {
        self.attachments = attachments;
        self
    }

    pub fn comments(mut self, comments: Rc<Vec<ComposerComment>>) -> Self {
        self.comments = comments;
        self
    }

    pub fn annotations(mut self, annotations: Rc<Vec<ComposerAnnotation>>) -> Self {
        self.annotations = annotations;
        self
    }

    pub fn floating(mut self, floating: bool) -> Self {
        self.floating = floating;
        self
    }

    pub fn has_queued_prompt(mut self, queued: bool) -> Self {
        self.has_queued_prompt = queued;
        self
    }

    pub fn on_queue(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_queue = Rc::new(handler);
        self
    }

    pub fn with_queue_handler(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_queue = Rc::new(handler);
        self
    }

    pub fn on_remove_attachment(
        mut self,
        handler: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_remove_attachment = Rc::new(handler);
        self
    }

    pub fn on_preview_attachment(
        mut self,
        handler: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_preview_attachment = Rc::new(handler);
        self
    }

    pub fn on_remove_annotation(
        mut self,
        handler: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_remove_annotation = Rc::new(handler);
        self
    }

    pub fn on_drop_files(
        mut self,
        handler: impl Fn(&ExternalPaths, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_drop_files = Rc::new(handler);
        self
    }

    pub fn autocomplete(mut self, view: Option<AutocompleteView>) -> Self {
        self.autocomplete = view;
        self
    }

    pub fn on_autocomplete_next(
        mut self,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_autocomplete_next = Rc::new(handler);
        self
    }

    pub fn on_autocomplete_previous(
        mut self,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_autocomplete_previous = Rc::new(handler);
        self
    }

    pub fn on_autocomplete_confirm(
        mut self,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_autocomplete_confirm = Rc::new(handler);
        self
    }

    pub fn on_autocomplete_dismiss(
        mut self,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_autocomplete_dismiss = Rc::new(handler);
        self
    }
}

/// The staged-image chip row. Public so the floating composer renders the same
/// chips the docked one does instead of keeping a second copy.
pub fn composer_attachment_row(
    attachments: Rc<Vec<ImageAttachment>>,
    on_remove: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_preview: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    theme: Theme,
) -> impl IntoElement {
    div()
        .flex()
        .flex_wrap()
        .gap(px(8.0))
        .px(px(12.0))
        .pb(px(8.0))
        .children(
            attachments
                .iter()
                .cloned()
                .enumerate()
                .filter_map(move |(index, attachment)| {
                    // Decode to bytes so gpui renders the thumbnail; a `data:`
                    // URI string would be fetched as a URL and never show.
                    let image = attachment_image(&attachment)?;
                    let on_remove = on_remove.clone();
                    let on_preview = on_preview.clone();
                    Some(
                        div()
                            .id(ElementId::Name(
                                format!("composer-attachment-{index}").into(),
                            ))
                            .relative()
                            .size(px(56.0))
                            .rounded(px(8.0))
                            .border_1()
                            .border_color(theme.border)
                            .bg(gpui::rgb(0x000000))
                            .overflow_hidden()
                            .cursor_default()
                            .group(format!("composer-attachment-{index}"))
                            // Clicking the thumbnail opens the image preview modal.
                            .on_click(move |_, window, cx| {
                                (on_preview)(index, window, cx);
                            })
                            .child(img(image).size_full())
                            .child(
                                div()
                                    .id(ElementId::Name(
                                        format!("remove-attachment-{index}").into(),
                                    ))
                                    .absolute()
                                    .top(px(3.0))
                                    .right(px(3.0))
                                    .size(px(16.0))
                                    .rounded_full()
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .bg(theme.inverse.opacity(0.88))
                                    .cursor_default()
                                    .hover(|s| s.bg(theme.danger))
                                    .on_click(move |_, window, cx| {
                                        (on_remove)(index, window, cx);
                                        cx.stop_propagation();
                                    })
                                    .child(app_icon(IconName::X, 9.0, theme.on_inverse)),
                            ),
                    )
                }),
        )
}

impl ComposerView {
    fn render_attachments(
        attachments: Rc<Vec<ImageAttachment>>,
        on_remove: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
        on_preview: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
        theme: Theme,
    ) -> impl IntoElement {
        composer_attachment_row(attachments, on_remove, on_preview, theme)
    }

    fn render_comments(comments: Rc<Vec<ComposerComment>>, theme: Theme) -> impl IntoElement {
        div()
            .flex()
            .flex_wrap()
            .gap(px(6.0))
            .px(px(12.0))
            .pb(px(7.0))
            .children(comments.iter().map(move |comment| {
                let edit = comment.on_edit.clone();
                let remove = comment.on_remove.clone();
                let chip_path = comment.path.clone();
                let chip_label = comment.label.clone();
                div()
                    .id(ElementId::Name(
                        format!("composer-comment-{}", comment.id).into(),
                    ))
                    .flex()
                    .items_center()
                    .gap(px(1.0))
                    .child(
                        div()
                            .id(ElementId::Name(
                                format!("edit-comment-{}", comment.id).into(),
                            ))
                            .cursor_default()
                            .on_click(move |_, window, cx| (edit)(window, cx))
                            .child(file_mention_chip(&chip_path, chip_label, theme)),
                    )
                    .child(
                        div()
                            .id(ElementId::Name(
                                format!("remove-comment-{}", comment.id).into(),
                            ))
                            .size(px(16.0))
                            .rounded_full()
                            .flex()
                            .items_center()
                            .justify_center()
                            .cursor_default()
                            .hover(|style| style.bg(theme.overlay_strong))
                            .on_click(move |_, window, cx| {
                                (remove)(window, cx);
                                cx.stop_propagation();
                            })
                            .child(app_icon(IconName::X, 9.0, theme.text_tertiary)),
                    )
            }))
    }

    fn render_annotations(
        annotations: Rc<Vec<ComposerAnnotation>>,
        on_remove: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
        theme: Theme,
    ) -> impl IntoElement {
        div()
            .flex()
            .flex_wrap()
            .gap(px(6.0))
            .px(px(12.0))
            .pb(px(7.0))
            .children(
                annotations
                    .iter()
                    .cloned()
                    .enumerate()
                    .map(move |(index, ann)| {
                        let on_remove = on_remove.clone();
                        let label = ann.label.clone();
                        let tooltip = ann.tooltip.clone();
                        div()
                            .id(ElementId::Name(
                                format!("composer-annotation-{}", ann.id).into(),
                            ))
                            .flex()
                            .items_center()
                            .gap(px(4.0))
                            .px(px(8.0))
                            .py(px(3.0))
                            .rounded(px(12.0))
                            .border_1()
                            .border_color(theme.border)
                            .bg(theme.overlay)
                            .tooltip(Tooltip::text(tooltip))
                            .child(
                                div()
                                    .text_size(px(11.5))
                                    .text_color(theme.text_secondary)
                                    .child(label),
                            )
                            .child(
                                div()
                                    .id(ElementId::Name(
                                        format!("remove-annotation-{}", ann.id).into(),
                                    ))
                                    .size(px(16.0))
                                    .rounded_full()
                                    .flex()
                                    .items_center()
                                    .justify_center()
                                    .cursor_default()
                                    .hover(|s| s.bg(theme.overlay_strong))
                                    .on_click(move |_, window, cx| {
                                        (on_remove)(index, window, cx);
                                        cx.stop_propagation();
                                    })
                                    .child(app_icon(IconName::X, 9.0, theme.text_tertiary)),
                            )
                    }),
            )
    }
}
impl RenderOnce for ComposerView {
    fn render(self, window: &mut Window, cx: &mut App) -> impl gpui::IntoElement {
        let theme = Theme::current(cx);
        let run_state = self.run_state;
        let is_running = run_state.is_running();
        let focused = self.composer_input.read(cx).is_visually_focused(window);

        let (model_label, model_provider) = self
            .selected_model
            .as_ref()
            .map(|model| (format_model_name(&model.model_id), model.provider.clone()))
            .unwrap_or_else(|| ("Select Model".to_owned(), "antigravity".to_owned()));
        let model_is_open = self
            .model_menu
            .as_ref()
            .map(|(_, handle)| handle.is_open())
            .unwrap_or(false);
        let model_trigger = MenuChip::new("model-picker-chip")
            .height(px(24.0))
            .outlined()
            .background(theme.composer)
            .selected(model_is_open)
            .icon(provider_svg_path(&model_provider), theme.text)
            .label(model_label);
        let model_control = if let Some((dropdown, handle)) = self.model_menu.clone() {
            popover(
                model_trigger,
                &handle,
                MenuAlign::AboveLeft,
                move |_handle, _window, _cx| dropdown.clone().into_any_element(),
            )
        } else {
            model_trigger.into_any_element()
        };

        let plan_mode = self.approval_mode == ApprovalMode::PlanMode;

        let thinking_control = if !self.supported_thinking_levels.is_empty() {
            let level = self
                .thinking_level
                .unwrap_or(self.supported_thinking_levels[0]);
            let thinking_is_open = self
                .thinking_menu
                .as_ref()
                .map(|h| h.is_open())
                .unwrap_or(false);
            let trigger = MenuChip::new("thinking-level-chip")
                .height(px(24.0))
                .outlined()
                .background(theme.composer)
                .selected(thinking_is_open)
                .label(level.label());
            let on_choose_thinking = self.on_choose_thinking.clone();
            let levels = self.supported_thinking_levels.clone();
            let control = if let Some(handle) = self.thinking_menu.clone() {
                dropdown_menu(
                    trigger,
                    "thinking-level-menu",
                    &handle,
                    MenuAlign::AboveLeft,
                    move |_| {
                        levels
                            .iter()
                            .map(|candidate| {
                                let on_choose_thinking = on_choose_thinking.clone();
                                let value = *candidate;
                                MenuItem::new(value.label(), move |window, cx| {
                                    if let Some(ref cb) = on_choose_thinking {
                                        cb(value, window, cx);
                                    }
                                })
                                .selected(value == level)
                            })
                            .collect()
                    },
                )
            } else {
                trigger.into_any_element()
            };
            Some(control)
        } else {
            None
        };

        let has_draft = !self.composer_input.read(cx).content().trim().is_empty()
            || !self.attachments.is_empty()
            || !self.comments.is_empty()
            || !self.annotations.is_empty();
        let attachment_data = self.attachments.clone();
        let attachment_on_remove = self.on_remove_attachment.clone();
        let attachment_on_preview = self.on_preview_attachment.clone();
        let attachments = (!attachment_data.is_empty()).then(|| {
            Self::render_attachments(
                attachment_data,
                attachment_on_remove,
                attachment_on_preview,
                theme,
            )
        });
        let comments = (!self.comments.is_empty())
            .then(|| Self::render_comments(self.comments.clone(), theme));
        let annotations = (!self.annotations.is_empty()).then(|| {
            Self::render_annotations(
                self.annotations.clone(),
                self.on_remove_annotation.clone(),
                theme,
            )
        });
        let floating = self.floating;
        let autocomplete = self.autocomplete;
        let autocomplete_anchor = autocomplete.as_ref().map(AutocompleteView::anchor_cell);
        let autocomplete_open = autocomplete.is_some();
        let on_autocomplete_next = self.on_autocomplete_next.clone();
        let on_autocomplete_previous = self.on_autocomplete_previous.clone();
        let on_autocomplete_confirm = self.on_autocomplete_confirm.clone();
        let on_autocomplete_dismiss = self.on_autocomplete_dismiss.clone();
        let on_pick = self.on_pick_image.clone();
        let on_drop_files = self.on_drop_files.clone();

        // Over a file/diff the composer rests as a one-line bar and grows to
        // the full card once it is focused or has something staged.
        // An open model dropdown means the user is still working in
        // the composer, even though the menu holds keyboard focus.
        let menu_open = self.model_menu.as_ref().is_some_and(|(_, h)| h.is_open());
        if floating
            && !focused
            && !menu_open
            && !has_draft
            && !autocomplete_open
            && matches!(run_state, ComposerRunState::Ready)
        {
            let focus = self.composer_input.read(cx).focus();
            let placeholder = self.composer_input.read(cx).placeholder.clone();
            return div()
                .id("composer-wrapper")
                .w_full()
                .px(px(24.0))
                .pt(px(12.0))
                .pb(px(18.0))
                .flex()
                .flex_col()
                .items_center()
                .child(
                    div()
                        .id("composer-compact")
                        .w_full()
                        .max_w(px(768.0))
                        .h(px(48.0))
                        .px(px(14.0))
                        .rounded(px(13.0))
                        .bg(theme.composer)
                        .border_1()
                        .border_color(theme.border_strong)
                        .shadow_lg()
                        .flex()
                        .items_center()
                        .gap(px(10.0))
                        .cursor_text()
                        .on_click(move |_, window, cx| window.focus(&focus, cx))
                        .child(
                            div()
                                .flex_1()
                                .text_size(px(14.0))
                                .text_color(theme.text_ghost)
                                .child(placeholder),
                        )
                        .child(
                            div()
                                .text_size(px(11.5))
                                .text_color(theme.text_ghost)
                                .child("\u{2318}L to focus"),
                        )
                        .child(
                            div()
                                .size(px(26.0))
                                .rounded_full()
                                .flex()
                                .items_center()
                                .justify_center()
                                .bg(theme.overlay_strong)
                                .child(app_icon(IconName::ArrowUp, 15.0, theme.text_ghost)),
                        ),
                )
                .into_any_element();
        }

        div()
            .id("composer-wrapper")
            .w_full()
            .px(if floating { px(24.0) } else { px(20.0) })
            .pt(px(if floating { 12.0 } else { 4.0 }))
            .pb(px(if floating { 18.0 } else { 9.0 }))
            .flex()
            .flex_col()
            .items_center()
            .child(
                div()
                    .w_full()
                    .max_w(px(768.0))
                    .relative()
                    .child(
                        div()
                            .id("composer-box")
                            .relative()
                            .when_some(autocomplete_anchor, |element, anchor_bounds| {
                                element.child(AutocompleteView::bounds_probe(anchor_bounds))
                            })
                            .when(autocomplete_open, |element| {
                                let on_next = on_autocomplete_next.clone();
                                let on_previous = on_autocomplete_previous.clone();
                                let on_confirm = on_autocomplete_confirm.clone();
                                let on_dismiss = on_autocomplete_dismiss.clone();
                                element
                                    .key_context(crate::common::autocomplete::AUTOCOMPLETE_CONTEXT)
                                    .on_action(move |_: &AutocompleteNext, window, cx| {
                                        (on_next)(window, cx);
                                        cx.stop_propagation();
                                    })
                                    .on_action(move |_: &AutocompletePrevious, window, cx| {
                                        (on_previous)(window, cx);
                                        cx.stop_propagation();
                                    })
                                    .on_action(move |_: &AutocompleteConfirm, window, cx| {
                                        (on_confirm)(window, cx);
                                        cx.stop_propagation();
                                    })
                                    .on_action(move |_: &AutocompleteDismiss, window, cx| {
                                        (on_dismiss)(window, cx);
                                        cx.stop_propagation();
                                    })
                            })
                            .w_full()
                            .rounded(px(13.0))
                            .bg(theme.composer)
                            .border_1()
                            .border_color(if is_running {
                                theme.accent.opacity(0.32)
                            } else if focused {
                                theme.accent
                            } else {
                                theme.border_strong
                            })
                            .when(floating, |el| el.shadow_lg())
                            .when(!floating, |el| el.shadow_sm())
                            .overflow_hidden()
                            .py(px(10.0))
                            .flex()
                            .flex_col()
                            .when(plan_mode, |el| el.child(plan_mode_stripes(theme)))
                            // Dropping image files onto the composer stages
                            // them as attachment chips; the card highlights
                            // while a drag hovers over it.
                            .drag_over::<ExternalPaths>(move |style, _, _, _| {
                                style
                                    .bg(theme.composer.opacity(1.0))
                                    .border_color(theme.accent)
                            })
                            .on_drop(move |paths: &ExternalPaths, window, cx| {
                                (on_drop_files)(paths, window, cx);
                            })
                            .when_some(comments, |element, comments| element.child(comments))
                            .when_some(annotations, |element, annotations| {
                                element.child(annotations)
                            })
                            .when_some(attachments, |element, attachments| {
                                element.child(attachments)
                            })
                            .child(
                                div()
                                    .w_full()
                                    .min_h(px(36.0))
                                    .max_h(crate::input::COMPOSER_MAX_HEIGHT)
                                    .px(px(12.0))
                                    .pt(px(2.0))
                                    .pb(px(2.0))
                                    .child(self.composer_input.clone()),
                            )
                            .child(
                                div()
                                    .mt(px(8.0))
                                    .px(px(10.0))
                                    .flex()
                                    .items_center()
                                    .gap(px(4.0))
                                    .text_size(px(11.5))
                                    .line_height(px(14.0))
                                    .child(
                                        div()
                                            .id("btn-attach")
                                            .p(px(5.0))
                                            .rounded(px(6.0))
                                            .cursor_default()
                                            .hover(|style| style.bg(theme.overlay))
                                            .on_click(move |_, window, cx| {
                                                (on_pick)(window, cx);
                                            })
                                            .child(app_icon(
                                                IconName::Plus,
                                                13.0,
                                                theme.text_tertiary,
                                            )),
                                    )
                                    .child(model_control)
                                    .when_some(thinking_control, |el, thinking| el.child(thinking))
                                    .child(div().flex_1())
                                    .child({
                                        // Speech-to-text dictation. Idle: the mic
                                        // starts a take. Recording: the stop
                                        // control takes its place with the live
                                        // elapsed label. Unconfigured: disabled
                                        // with a tooltip naming the fix.
                                        let on_mic = self.on_mic_click.clone();
                                        if self.mic_recording {
                                            div()
                                                .id("btn-mic-stop")
                                                .flex()
                                                .items_center()
                                                .gap(px(4.0))
                                                .p(px(4.0))
                                                .rounded(px(6.0))
                                                .bg(theme.danger_soft)
                                                .cursor_default()
                                                .hover(|style| style.opacity(0.8))
                                                .on_click(move |_, window, cx| {
                                                    if let Some(on_mic) = &on_mic {
                                                        (on_mic)(window, cx);
                                                    }
                                                })
                                                .child(app_icon(
                                                    IconName::StopFilled,
                                                    13.0,
                                                    theme.danger,
                                                ))
                                                .child(
                                                    div()
                                                        .text_size(px(11.5))
                                                        .font_weight(FontWeight::MEDIUM)
                                                        .text_color(theme.danger)
                                                        .child(format!(
                                                            "{}:{:02}",
                                                            self.mic_elapsed_secs / 60,
                                                            self.mic_elapsed_secs % 60
                                                        )),
                                                )
                                                .into_any_element()
                                        } else if self.mic_transcribing {
                                            div()
                                                .id("btn-mic-transcribing")
                                                .p(px(5.0))
                                                .rounded(px(6.0))
                                                .child(crate::primitives::motion::spin(app_icon(
                                                    IconName::LoaderCircle,
                                                    13.0,
                                                    theme.text_tertiary,
                                                )))
                                                .into_any_element()
                                        } else {
                                            let button = div()
                                                .id("btn-mic")
                                                .p(px(5.0))
                                                .rounded(px(6.0))
                                                .when(self.mic_enabled, |el| {
                                                    el.cursor_default()
                                                        .hover(|style| style.bg(theme.overlay))
                                                        .on_click(move |_, window, cx| {
                                                            if let Some(on_mic) = &on_mic {
                                                                (on_mic)(window, cx);
                                                            }
                                                        })
                                                })
                                                .when(!self.mic_enabled, |el| {
                                                    el.opacity(0.35)
                                                        .tooltip(Tooltip::text(
                                                            "Set up Speech to Text in Settings › Providers",
                                                        ))
                                                })
                                                .child(app_icon(
                                                    IconName::Mic,
                                                    13.0,
                                                    theme.text_tertiary,
                                                ));
                                            button.into_any_element()
                                        }
                                    })
                                    .when_some(self.mic_error.clone(), |el, error| {
                                        el.child(
                                            div()
                                                .text_size(px(11.5))
                                                .text_color(theme.danger)
                                                .child(error),
                                        )
                                    })
                                    .child(match run_state {
                                        ComposerRunState::Preparing => div()
                                            .id("btn-composer-preparing")
                                            .size(px(26.0))
                                            .rounded_full()
                                            .flex()
                                            .items_center()
                                            .justify_center()
                                            .bg(theme.overlay_strong)
                                            .child(crate::primitives::motion::spin(app_icon(
                                                IconName::LoaderCircle,
                                                14.0,
                                                theme.text_secondary,
                                            ))),
                                        ComposerRunState::Running => {
                                            let on_abort = self.on_abort.clone();
                                            let on_queue = self.on_queue.clone();
                                            div()
                                                .id("composer-running-actions")
                                                .flex()
                                                .items_center()
                                                .gap(px(6.0))
                                                .child(
                                                    div()
                                                        .id("btn-abort-prompt")
                                                        .size(px(26.0))
                                                        .rounded_full()
                                                        .flex()
                                                        .items_center()
                                                        .justify_center()
                                                        .bg(theme.overlay_strong)
                                                        .cursor_default()
                                                        .hover(|style| style.bg(theme.danger_soft))
                                                        .active(|style| style.opacity(0.8))
                                                        .on_click(move |_, window, cx| {
                                                            (on_abort)(window, cx);
                                                        })
                                                        .child(app_icon(
                                                            IconName::Stop,
                                                            16.0,
                                                            theme.text,
                                                        )),
                                                )
                                                .when(has_draft, |element| {
                                                    element.child(
                                                        div()
                                                            .id("btn-queue-follow-up")
                                                            .size(px(26.0))
                                                            .rounded_full()
                                                            .flex()
                                                            .items_center()
                                                            .justify_center()
                                                            .bg(theme.inverse)
                                                            .cursor_default()
                                                            .hover(|style| style.opacity(0.9))
                                                            .active(|style| style.opacity(0.8))
                                                            .on_click(move |_, window, cx| {
                                                                (on_queue)(window, cx);
                                                            })
                                                            .child(app_icon(
                                                                IconName::ArrowUp,
                                                                15.0,
                                                                theme.on_inverse,
                                                            )),
                                                    )
                                                })
                                        }
                                        ComposerRunState::Ready => {
                                            let on_send = self.on_send.clone();
                                            div()
                                                .id("btn-send-prompt")
                                                .size(px(26.0))
                                                .rounded_full()
                                                .flex()
                                                .items_center()
                                                .justify_center()
                                                .bg(if has_draft {
                                                    theme.inverse
                                                } else {
                                                    theme.overlay_strong
                                                })
                                                .when(has_draft, |element| {
                                                    element
                                                        .cursor_default()
                                                        .hover(|style| style.opacity(0.9))
                                                        .active(|style| style.opacity(0.8))
                                                        .on_click(move |_, window, cx| {
                                                            (on_send)(window, cx);
                                                        })
                                                })
                                                .child(app_icon(
                                                    IconName::ArrowUp,
                                                    15.0,
                                                    if has_draft {
                                                        theme.on_inverse
                                                    } else {
                                                        theme.text_ghost
                                                    },
                                                ))
                                        }
                                    }),
                            ),
                    )
                    .when_some(autocomplete, |element, autocomplete| {
                        element.child(autocomplete)
                    }),
            )
            .into_any_element()
    }
}
