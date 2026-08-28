use std::rc::Rc;

use console_core::{ApprovalMode, ImageAttachment, SelectedModel};
use gpui::{
    App, ElementId, Entity, ExternalPaths, InteractiveElement, IntoElement, ParentElement,
    RenderOnce, StatefulInteractiveElement, Styled, Window, div, img, prelude::FluentBuilder, px,
};

use crate::common::{
    ApprovalModeDropdown, ApprovalModeIconExt, AutocompleteConfirm, AutocompleteDismiss,
    AutocompleteNext, AutocompletePrevious, AutocompleteView, ModelDropdownMenu, attachment_image,
    format_model_name, provider_svg_path,
};
use crate::input::ComposerInput;
use crate::primitives::{ContextMenuHandle, IconName, MenuAlign, MenuChip, app_icon, popover};
use crate::theme::Theme;

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
#[derive(IntoElement)]
pub struct ComposerView {
    pub composer_input: Entity<ComposerInput>,
    pub run_state: ComposerRunState,
    pub selected_model: Option<SelectedModel>,
    pub approval_mode: ApprovalMode,
    pub attachments: Rc<Vec<ImageAttachment>>,
    model_menu: Option<(ModelDropdownMenu, ContextMenuHandle)>,
    approval_menu: Option<(ApprovalModeDropdown, ContextMenuHandle)>,
    autocomplete: Option<AutocompleteView>,
    on_send: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_queue: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_abort: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_pick_image: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_remove_attachment: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
    on_preview_attachment: Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>,
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
            approval_mode: ApprovalMode::AlwaysAsk,
            attachments: Rc::new(Vec::new()),
            model_menu: None,
            approval_menu: None,
            autocomplete: None,
            on_queue: on_send.clone(),
            on_send,
            on_abort: Rc::new(on_abort),
            on_pick_image: Rc::new(on_pick_image),
            on_remove_attachment: Rc::new(|_: usize, _: &mut Window, _: &mut App| {}),
            on_preview_attachment: Rc::new(|_: usize, _: &mut Window, _: &mut App| {}),
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

    pub fn model_dropdown(
        mut self,
        dropdown: ModelDropdownMenu,
        handle: ContextMenuHandle,
    ) -> Self {
        self.model_menu = Some((dropdown, handle));
        self
    }

    pub fn approval_dropdown(
        mut self,
        dropdown: ApprovalModeDropdown,
        handle: ContextMenuHandle,
    ) -> Self {
        self.approval_menu = Some((dropdown, handle));
        self
    }

    pub fn attachments(mut self, attachments: Rc<Vec<ImageAttachment>>) -> Self {
        self.attachments = attachments;
        self
    }

    pub fn on_queue(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
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

    fn render_attachments(
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
            .children(attachments.iter().cloned().enumerate().filter_map(
                move |(index, attachment)| {
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
                },
            ))
    }
}

impl RenderOnce for ComposerView {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let run_state = self.run_state;
        let is_running = run_state.is_running();
        let focused = self.composer_input.read(cx).is_visually_focused(window);

        let (model_label, model_provider) = self
            .selected_model
            .as_ref()
            .map(|model| (format_model_name(&model.model_id), model.provider.clone()))
            .unwrap_or_else(|| ("Select Model".to_owned(), "gemini".to_owned()));
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

        let approval_is_open = self
            .approval_menu
            .as_ref()
            .map(|(_, handle)| handle.is_open())
            .unwrap_or(false);
        let approval_trigger = MenuChip::new("approval-mode-chip")
            .height(px(26.0))
            .outlined()
            .background(theme.composer)
            .selected(approval_is_open)
            .icon(
                self.approval_mode.icon().path(),
                if self.approval_mode == ApprovalMode::FullAccess {
                    theme.warning
                } else {
                    theme.text_tertiary
                },
            )
            .label(self.approval_mode.label());
        let approval_control = if let Some((dropdown, handle)) = self.approval_menu.clone() {
            popover(
                approval_trigger,
                &handle,
                MenuAlign::AboveLeft,
                move |_handle, _window, _cx| dropdown.clone().into_any_element(),
            )
        } else {
            approval_trigger.into_any_element()
        };

        let has_draft = !self.composer_input.read(cx).content().trim().is_empty()
            || !self.attachments.is_empty();
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
        let autocomplete = self.autocomplete;
        let autocomplete_anchor = autocomplete.as_ref().map(AutocompleteView::anchor_cell);
        let autocomplete_open = autocomplete.is_some();
        let on_autocomplete_next = self.on_autocomplete_next.clone();
        let on_autocomplete_previous = self.on_autocomplete_previous.clone();
        let on_autocomplete_confirm = self.on_autocomplete_confirm.clone();
        let on_autocomplete_dismiss = self.on_autocomplete_dismiss.clone();
        let on_pick = self.on_pick_image.clone();
        let on_drop_files = self.on_drop_files.clone();

        div()
            .id("composer-wrapper")
            .w_full()
            .px(px(20.0))
            .pt(px(4.0))
            .pb(px(9.0))
            .bg(theme.composer)
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
                            .shadow_sm()
                            .overflow_hidden()
                            .py(px(10.0))
                            .flex()
                            .flex_col()
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
                            .when_some(attachments, |element, attachments| {
                                element.child(attachments)
                            })
                            .child(
                                div()
                                    .w_full()
                                    .min_h(px(36.0))
                                    .max_h(px(220.0))
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
                                    .child(approval_control)
                                    .child(div().flex_1())
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
    }
}
