//! Floating scroll pill and prompt jump popup controls for the chat transcript.

use gpui::{
    AnyElement, Context, IntoElement, KeyDownEvent, ScrollStrategy, WeakEntity, div, prelude::*,
    px, uniform_list,
};

use crate::chat::transcript_view::TranscriptView;
use crate::primitives::tooltip::Tooltip;
use crate::primitives::{IconName, app_icon};
use crate::theme::Theme;

pub const PROMPT_ROW_HEIGHT: f32 = 34.0;
pub const PROMPT_POPUP_MAX_ROWS: usize = 7;

/// One row in the prompt jump popup.
pub fn prompt_row(
    prompt: &console_core::SessionPrompt,
    from_end: usize,
    theme: Theme,
    entity: WeakEntity<TranscriptView>,
) -> AnyElement {
    div()
        .id(gpui::ElementId::Name(
            format!("prompt-jump-{}", prompt.id).into(),
        ))
        .w_full()
        .h(px(PROMPT_ROW_HEIGHT))
        .flex()
        .flex_none()
        .items_center()
        .gap(px(12.0))
        .px(px(12.0))
        .rounded(px(8.0))
        .cursor_pointer()
        .hover(|element| element.bg(theme.overlay_strong))
        .on_click(move |_, window, cx| {
            let Some(view) = entity.upgrade() else {
                return;
            };
            let jump = view.update(cx, |this, cx| {
                this.close_prompts(cx);
                this.jump_callback()
            });
            if let Some(jump) = jump {
                jump(from_end, window, cx);
            }
        })
        .child(
            div()
                .flex_1()
                .min_w_0()
                .truncate()
                .text_size(px(13.0))
                .text_color(theme.text)
                .child(prompt.preview.clone()),
        )
        .when(prompt.created_at > 0, |element| {
            // "just now" and dates ("Oct 29") already read fine; only "3h" needs "ago".
            let age = crate::utils::format_time_ago(prompt.created_at);
            let age = if age.contains(' ') {
                age
            } else {
                format!("{age} ago")
            };
            element.child(
                div()
                    .flex_none()
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child(age),
            )
        })
        .into_any_element()
}

impl TranscriptView {
    /// Floating pill at the bottom-right: a prompt list button (opens the jump
    /// popup) and, while scrolled away from the tail, a scroll-to-bottom arrow.
    pub(crate) fn render_scroll_controls(
        &self,
        theme: Theme,
        cx: &mut Context<Self>,
    ) -> AnyElement {
        let has_prompts = !self.messages.is_empty();
        let show_down = !self.list_state.is_following_tail();
        if (!has_prompts && !show_down) || !(self.hovered || self.prompts_open) {
            return div().into_any_element();
        }
        let both = has_prompts && show_down;
        let button = |id: &'static str, icon: IconName, is_left: bool| {
            let mut el = div()
                .id(id)
                .w(px(32.0))
                .h(px(32.0))
                .flex()
                .items_center()
                .justify_center()
                .cursor_pointer()
                .hover(|element| element.bg(theme.overlay_strong))
                .child(app_icon(icon, 15.0, theme.text_secondary));
            if both {
                if is_left {
                    el = el.rounded_tl(px(16.0)).rounded_bl(px(16.0));
                } else {
                    el = el.rounded_tr(px(16.0)).rounded_br(px(16.0));
                }
            } else {
                el = el.rounded(px(16.0));
            }
            el
        };

        let mut pill = div()
            .flex()
            .items_center()
            .overflow_hidden()
            .rounded(px(17.0))
            .border_1()
            .border_color(theme.border)
            .bg(theme.raised);

        if has_prompts {
            pill = pill.child(
                button("transcript-prompts", IconName::List, true)
                    .tooltip(Tooltip::text("Jump to prompt"))
                    .on_click(cx.listener(|this, _, window, cx| {
                        this.prompts_open = !this.prompts_open;
                        if this.prompts_open {
                            window.focus(&this.prompts_focus, cx);
                            if let Some(last) = this.prompts.len().checked_sub(1) {
                                this.prompts_scroll
                                    .scroll_to_item_strict(last, ScrollStrategy::Bottom);
                            }
                            if this.prompts_stale || this.prompts.is_empty() {
                                this.prompts_stale = false;
                                if let Some(request) = this.on_request_prompts.clone() {
                                    request(window, cx);
                                }
                            }
                        }
                        cx.notify();
                    })),
            );
        }
        if show_down {
            if has_prompts {
                pill = pill.child(div().w(px(1.0)).h(px(16.0)).flex_none().bg(theme.border));
            }
            pill = pill.child(
                button("transcript-scroll-bottom", IconName::ArrowDown, false)
                    .tooltip(Tooltip::text("Scroll to bottom"))
                    .on_click(cx.listener(|this, _, _, cx| this.scroll_to_bottom(cx))),
            );
        }

        let mut layer = div().absolute().inset_0();
        if self.prompts_open && has_prompts {
            let prompts = self.prompts.clone();
            let entity = cx.entity().downgrade();
            let total = prompts.len();
            let visible_rows = total.min(PROMPT_POPUP_MAX_ROWS);
            let body = if prompts.is_empty() {
                div()
                    .px(px(12.0))
                    .py(px(7.0))
                    .text_size(px(13.0))
                    .text_color(theme.text_tertiary)
                    .child("Loading…")
                    .into_any_element()
            } else {
                uniform_list(
                    "transcript-prompts-list",
                    prompts.len(),
                    move |range, _window, _cx| {
                        range
                            .map(|ix| {
                                prompt_row(&prompts[ix], total - 1 - ix, theme, entity.clone())
                            })
                            .collect::<Vec<_>>()
                    },
                )
                .track_scroll(&self.prompts_scroll)
                .size_full()
                .into_any_element()
            };

            layer = layer
                .child(
                    div()
                        .id("transcript-prompts-backdrop")
                        .absolute()
                        .inset_0()
                        .on_click(cx.listener(|this, _, _, cx| {
                            this.close_prompts(cx);
                        })),
                )
                .child(
                    div()
                        .id("transcript-prompts-popup")
                        .track_focus(&self.prompts_focus)
                        .on_key_down(cx.listener(|this, event: &KeyDownEvent, _, cx| {
                            if event.keystroke.key == "escape" {
                                this.close_prompts(cx);
                                cx.stop_propagation();
                            }
                        }))
                        .absolute()
                        .right(px(16.0))
                        .bottom(px(54.0))
                        .w(px(340.0))
                        .max_w_full()
                        .h(px(visible_rows.max(1) as f32 * PROMPT_ROW_HEIGHT + 8.0))
                        .occlude()
                        .on_scroll_wheel(|_, _, cx| cx.stop_propagation())
                        .p(px(4.0))
                        .rounded(px(12.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.raised)
                        .child(body),
                );
        }

        layer
            .child(
                div()
                    .absolute()
                    .right(px(16.0))
                    .bottom(px(10.0))
                    .child(pill),
            )
            .into_any_element()
    }
}
