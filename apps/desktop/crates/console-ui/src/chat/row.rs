//! Row assembly and message bubble rendering for the transcript list.

use console_core::{ActivityEvent, AgentMessage, ApprovalMode, AssistantContentPart};
use gpui::{
    Animation, AnimationExt, AnyElement, App, ElementId, IntoElement, WeakEntity, div, prelude::*,
    px,
};
use std::time::Duration;

use crate::chat::message_bubble::{AssistantMessageBubble, UserMessageBubble};
use crate::chat::toolcalls::ToolCalls;
use crate::chat::transcript_view::{RunActivityBlock, TranscriptView};
use crate::chat::working_indicator::WorkingIndicator;
use crate::markdown::render::LinkHandler;
use crate::primitives::ease_out_cubic;
use crate::theme::Theme;

const SEND_ANIMATION: Duration = Duration::from_millis(140);
/// How far below its final spot the bubble starts before settling.
const SEND_RISE: f32 = 10.0;

/// Builds one transcript row for the list element.
pub fn transcript_row(
    entity: WeakEntity<TranscriptView>,
    index: usize,
    cx: &mut App,
) -> AnyElement {
    let Some(view) = entity.upgrade() else {
        return div().into_any_element();
    };
    let view_ref = view.read(cx);

    // While a run is streaming, the trailing row is the live working
    // indicator — pulsing dots plus "Working for Ns" — pinned right below
    // whatever content has arrived.
    if index >= view_ref.messages.len() {
        let theme = Theme::current(cx);
        let started_at = view_ref
            .streaming_started_at
            .unwrap_or_else(|| chrono::Utc::now().timestamp());
        return div()
            .w_full()
            .px(px(20.0))
            .py(px(4.0))
            .flex()
            .justify_center()
            .child(
                div()
                    .w_full()
                    .max_w(px(768.0))
                    .child(WorkingIndicator::new(started_at, theme)),
            )
            .into_any_element();
    }

    let Some(message) = view_ref.messages.get(index) else {
        return div().into_any_element();
    };

    let wake_boundary_label = view_ref.wake_boundary_at(index).map(str::to_string);

    // These messages are transport-only after RunActivity takes ownership of
    // their visual output, unless a reconstructed wake boundary starts here.
    // Return before the normal row padding is applied; rendering an empty child
    // inside the wrapper still reserves vertical space, which creates a gap
    // between the run summary and final response.
    if is_hidden_tool_transport(message) && wake_boundary_label.is_none() {
        return div().into_any_element();
    }

    let preview_handler = view_ref.on_preview_image.clone();
    let link_handler = view_ref.link_handler.clone();
    let row = match message {
        AgentMessage::User {
            id,
            content,
            attachments,
            annotations,
            context_files,
            created_at,
            approval_mode,
        } => {
            let expand_key = id.clone().unwrap_or_else(|| format!("message-{index}"));
            let user_expanded = view_ref.user_message_expanded(&expand_key);
            let expand_entity = entity.clone();
            let mut bubble = UserMessageBubble::new(content.clone())
                .created_at(*created_at)
                .plan_mode(approval_mode.as_deref() == Some(ApprovalMode::PlanMode.value()))
                .selection(view_ref.selection.clone(), format!("user-{index}"))
                .expanded(user_expanded)
                .on_toggle_expand(move |_window, cx| {
                    if let Some(view) = expand_entity.upgrade() {
                        let expand_key = expand_key.clone();
                        view.update(cx, |view, cx| {
                            view.toggle_user_message_expanded(expand_key, index, cx)
                        });
                    }
                });
            if let Some(attachments) = attachments {
                bubble = bubble.attachments(attachments.clone());
            }
            if let Some(annotations) = annotations {
                bubble = bubble.annotations(annotations.clone());
            }
            if let Some(files) = context_files {
                bubble = bubble.context_files(files.clone());
            }
            if let Some(handler) = &preview_handler {
                bubble = bubble.on_preview_image(handler.clone());
            }
            // Only the prompt sent a moment ago animates. The window outlives the
            // animation so a mid-flight re-render keeps its progress; afterwards
            // the bubble is no longer wrapped, so rows rebuilt by scrolling stay still.
            let just_sent = view_ref
                .just_sent
                .is_some_and(|(sent, at)| sent == index && at.elapsed() < SEND_ANIMATION * 3);
            let bubble = if just_sent {
                div()
                    .w_full()
                    .child(bubble)
                    .with_animation(
                        ElementId::Name(format!("user-send-{index}").into()),
                        Animation::new(SEND_ANIMATION).with_easing(ease_out_cubic),
                        |element, delta| element.opacity(delta).mt(px((1.0 - delta) * SEND_RISE)),
                    )
                    .into_any_element()
            } else {
                bubble.into_any_element()
            };
            let activity = view_ref.tool_activity_for_user(index);
            div()
                .w_full()
                .flex()
                .flex_col()
                .gap(px(6.0))
                .child(bubble)
                .when_some(activity, |mut element, blocks| {
                    for block in blocks
                        .into_iter()
                        .filter(|block| block.boundary_index.is_none())
                    {
                        element = element.child(render_run_block(
                            block,
                            view_ref,
                            entity.clone(),
                            link_handler.clone(),
                            cx,
                        ));
                    }
                    element
                })
                .into_any_element()
        }
        AgentMessage::Assistant {
            content,
            created_at,
            id,
            tok_per_sec,
            ..
        } => {
            if is_hidden_tool_transport(message) {
                let label = wake_boundary_label.unwrap_or_default();
                render_wake_boundary_row(
                    label,
                    wake_block_for_boundary(view_ref, index),
                    view_ref,
                    entity.clone(),
                    link_handler.clone(),
                    cx,
                )
            } else {
                let presentation = view_ref.assistant_presentation(index, content);
                let markdown_views = presentation
                    .content_parts
                    .iter()
                    .enumerate()
                    .map(|(part_index, part)| match part {
                        AssistantContentPart::Text { .. }
                        | AssistantContentPart::Thinking { .. } => {
                            Some(view_ref.markdown_view_for(index, part_index))
                        }
                        AssistantContentPart::ToolCall { .. }
                        | AssistantContentPart::Image { .. } => None,
                    })
                    .collect();
                let thinking_state = view_ref.thinking_expanded.clone();
                let thinking_entity = entity.clone();
                let turn_changes = view_ref.turn_changes_for_assistant(index);
                let turn_id = turn_changes
                    .as_ref()
                    .map(|turn| turn.user_message_id.clone())
                    .unwrap_or_default();
                let changes_expanded = view_ref.expanded_turn_changes.borrow().contains(&turn_id);
                let on_open_change = view_ref.on_open_change.clone();
                let load_diff = view_ref.load_diff.clone();
                let on_fork = view_ref.on_fork.clone();
                let toggle_entity = entity.clone();
                let bubble = AssistantMessageBubble::new(presentation.content_parts)
                    .cwd(view_ref.session_cwd.clone())
                    .file_changes(turn_changes)
                    .changes_expanded(changes_expanded)
                    .on_toggle_changes(move |_window, cx| {
                        if let Some(view) = toggle_entity.upgrade() {
                            let turn_id = turn_id.clone();
                            view.update(cx, |view, cx| {
                                view.toggle_turn_changes(index, turn_id, cx)
                            });
                        }
                    })
                    .on_open_change(on_open_change)
                    .load_diff(load_diff)
                    .fork_message_id(id.clone())
                    .on_fork(on_fork)
                    .copy_content(presentation.copy_content)
                    .link_handler(link_handler.clone())
                    .thinking_expanded(thinking_state)
                    .on_thinking_toggle(move |thinking_id, expanded, _window, cx| {
                        if let Some(view) = thinking_entity.upgrade() {
                            view.update(cx, |view, cx| {
                                view.apply_thinking_toggle(thinking_id, expanded, cx);
                            });
                        }
                    })
                    .created_at(*created_at)
                    .tok_per_sec(*tok_per_sec)
                    .selection(view_ref.selection.clone(), format!("message-{index}"))
                    .markdown_views(markdown_views)
                    .streaming(view_ref.is_streaming && index + 1 == view_ref.messages.len())
                    .when_some(preview_handler.clone(), |bubble, handler| {
                        bubble.on_preview_image(handler)
                    })
                    .into_any_element();
                if let Some(label) = wake_boundary_label {
                    div()
                        .w_full()
                        .flex()
                        .flex_col()
                        .gap(px(6.0))
                        .child(render_wake_divider(label, cx))
                        .child(bubble)
                        .into_any_element()
                } else {
                    bubble
                }
            }
        }
        // Tool-result messages are persistence transport unless a reconstructed
        // wake boundary starts on this row.
        AgentMessage::ToolResult { .. } => {
            if let Some(label) = wake_boundary_label {
                render_wake_boundary_row(
                    label,
                    wake_block_for_boundary(view_ref, index),
                    view_ref,
                    entity.clone(),
                    link_handler.clone(),
                    cx,
                )
            } else {
                div().into_any_element()
            }
        }
        // Client-side wake-turn marker: renders the wake divider and any
        // wake-turn tool activity at the top of the wake turn.
        AgentMessage::RunBoundary { label, .. } => render_wake_boundary_row(
            label.clone(),
            wake_block_for_boundary(view_ref, index),
            view_ref,
            entity.clone(),
            link_handler.clone(),
            cx,
        ),
    };

    div()
        .w_full()
        .px(px(20.0))
        .py(px(4.0))
        .flex()
        .justify_center()
        .child(div().w_full().max_w(px(768.0)).child(row))
        .into_any_element()
}

fn wake_block_for_boundary(
    view_ref: &TranscriptView,
    boundary_index: usize,
) -> Option<RunActivityBlock> {
    let user_index = view_ref.messages[..boundary_index]
        .iter()
        .rposition(|message| matches!(message, AgentMessage::User { .. }))?;
    view_ref
        .tool_activity_for_user(user_index)?
        .into_iter()
        .find(|block| block.boundary_index == Some(boundary_index))
}

fn render_wake_divider(label: String, cx: &App) -> AnyElement {
    let theme = Theme::current(cx);
    div()
        .px(px(10.0))
        .py(px(5.0))
        .text_size(px(11.5))
        .text_color(theme.text_tertiary)
        .child(label)
        .into_any_element()
}

fn render_wake_boundary_row(
    label: String,
    block: Option<RunActivityBlock>,
    view_ref: &TranscriptView,
    activity_entity: WeakEntity<TranscriptView>,
    link_handler: Option<LinkHandler>,
    cx: &App,
) -> AnyElement {
    if let Some(block) = block {
        render_run_block(block, view_ref, activity_entity, link_handler, cx)
    } else {
        render_wake_divider(label, cx)
    }
}

fn render_run_block(
    block: RunActivityBlock,
    view_ref: &TranscriptView,
    activity_entity: WeakEntity<TranscriptView>,
    link_handler: Option<LinkHandler>,
    cx: &App,
) -> AnyElement {
    // Visual artifacts render as their own item after the run block, so
    // folding the run never hides the card. Events is consumed by
    // `ToolCalls::new` below, so visual items must be collected first.
    #[expect(clippy::needless_collect)]
    let visuals: Vec<_> = block
        .events
        .iter()
        .filter_map(|event| match event {
            ActivityEvent::ToolCall(entry) => entry
                .result
                .as_ref()
                .and_then(|result| result.visual.clone())
                .map(|visual| (entry.call.id.clone(), visual)),
            _ => None,
        })
        .collect();
    #[expect(clippy::needless_collect)]
    let htmls: Vec<_> = block
        .events
        .iter()
        .filter_map(|event| match event {
            ActivityEvent::ToolCall(entry) => entry
                .result
                .as_ref()
                .and_then(|result| result.html.clone())
                .map(|html| (entry.call.id.clone(), html)),
            _ => None,
        })
        .collect();
    let visual_selection = view_ref.selection.clone();
    let on_open_browser = view_ref.on_open_url_raw.clone();
    let on_open_editor = view_ref.on_open_html_editor.clone();
    let run_id = block.run_id.clone();
    let mut segment = div().w_full().flex().flex_col().gap(px(6.0));
    if let Some(label) = block.divider_label {
        segment = segment.child(render_wake_divider(label, cx));
    }
    segment = segment.child(
        ToolCalls::new(
            run_id,
            block.events,
            block.working,
            block.started_at,
            block.elapsed,
            view_ref.tool_calls_state.clone(),
        )
        .cwd(view_ref.session_cwd.clone())
        .selection(view_ref.selection.clone())
        .link_handler(link_handler)
        .on_action(move |action, window, cx| {
            if let Some(view) = activity_entity.upgrade() {
                view.update(cx, |view, cx| {
                    view.apply_tool_calls_action(action, window, cx);
                });
            }
        }),
    );
    div()
        .w_full()
        .flex()
        .flex_col()
        .gap(px(6.0))
        .child(segment)
        .children(visuals.into_iter().map(|(call_id, visual)| {
            crate::chat::visual::VisualCard::new(call_id, visual, visual_selection.clone())
        }))
        .children(htmls.into_iter().map(|(call_id, html)| {
            crate::chat::html::HtmlCard::new(call_id, html)
                .on_open_browser(on_open_browser.clone())
                .on_open_editor(on_open_editor.clone())
        }))
        .into_any_element()
}

pub fn is_hidden_tool_transport(message: &AgentMessage) -> bool {
    match message {
        AgentMessage::ToolResult { .. } => true,
        AgentMessage::Assistant { content, .. } => content
            .iter()
            .any(|part| matches!(part, AssistantContentPart::ToolCall { .. })),
        AgentMessage::User { .. } | AgentMessage::RunBoundary { .. } => false,
    }
}

pub fn empty_state(theme: Theme) -> impl IntoElement {
    div()
        .size_full()
        .flex()
        .flex_col()
        .items_center()
        .justify_center()
        .gap(px(8.0))
        .child(crate::primitives::app_icon(
            crate::primitives::IconName::Bot,
            32.0,
            theme.text_ghost,
        ))
        .child(
            div()
                .text_size(px(14.0))
                .text_color(theme.text_ghost)
                .child("Start a conversation"),
        )
}
