//! Row assembly and message bubble rendering for the transcript list.

use console_core::{ActivityEvent, AgentMessage, AssistantContentPart};
use gpui::{div, prelude::*, px, AnyElement, App, IntoElement, WeakEntity};

use crate::chat::message_bubble::{AssistantMessageBubble, UserMessageBubble};
use crate::chat::toolcalls::ToolCalls;
use crate::chat::transcript_view::TranscriptView;
use crate::chat::working_indicator::WorkingIndicator;
use crate::theme::Theme;

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

    // These messages are transport-only after RunActivity takes ownership of
    // their visual output. Return before the normal row padding is applied;
    // rendering an empty child inside the wrapper still reserves vertical
    // space, which creates a gap between the run summary and final response.
    if is_hidden_tool_transport(message) {
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
            ..
        } => {
            let expand_key = id.clone().unwrap_or_else(|| format!("message-{index}"));
            let user_expanded = view_ref.user_message_expanded(&expand_key);
            let expand_entity = entity.clone();
            let mut bubble = UserMessageBubble::new(content.clone())
                .created_at(*created_at)
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
            let activity = view_ref.tool_activity_for_user(index);
            let activity_state = view_ref.tool_calls_state.clone();
            let activity_entity = entity.clone();
            div()
                .w_full()
                .flex()
                .flex_col()
                .gap(px(6.0))
                .child(bubble)
                .when_some(
                    activity,
                    move |element, (entries, started_at, elapsed, working)| {
                        // Visual artifacts render as their own item after the
                        // run block, so folding the run never hides the card.
                        let visuals: Vec<_> = entries
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
                        let visual_selection = view_ref.selection.clone();
                        let activity = ToolCalls::new(
                            format!("run-{index}"),
                            entries,
                            working,
                            started_at,
                            elapsed,
                            activity_state,
                        )
                        .cwd(view_ref.session_cwd.clone())
                        .selection(view_ref.selection.clone())
                        .link_handler(link_handler.clone())
                        .on_action(move |action, window, cx| {
                            if let Some(view) = activity_entity.upgrade() {
                                view.update(cx, |view, cx| {
                                    view.apply_tool_calls_action(action, window, cx);
                                });
                            }
                        });
                        element.child(activity).children(visuals.into_iter().map(
                            |(call_id, visual)| {
                                crate::chat::visual::VisualCard::new(
                                    call_id,
                                    visual,
                                    visual_selection.clone(),
                                )
                            },
                        ))
                    },
                )
                .into_any_element()
        }
        AgentMessage::Assistant {
            content,
            created_at,
            id,
            tok_per_sec,
            ..
        } => {
            let presentation = view_ref.assistant_presentation(index, content);
            let markdown_views = presentation
                .content_parts
                .iter()
                .enumerate()
                .map(|(part_index, part)| match part {
                    AssistantContentPart::Text { .. } | AssistantContentPart::Thinking { .. } => {
                        Some(view_ref.markdown_view_for(index, part_index))
                    }
                    AssistantContentPart::ToolCall { .. } | AssistantContentPart::Image { .. } => {
                        None
                    }
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
            let on_fork = view_ref.on_fork.clone();
            let toggle_entity = entity.clone();
            AssistantMessageBubble::new(presentation.content_parts)
                .file_changes(turn_changes)
                .changes_expanded(changes_expanded)
                .on_toggle_changes(move |_window, cx| {
                    if let Some(view) = toggle_entity.upgrade() {
                        let turn_id = turn_id.clone();
                        view.update(cx, |view, cx| view.toggle_turn_changes(index, turn_id, cx));
                    }
                })
                .on_open_change(on_open_change)
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
                .into_any_element()
        }
        // Tool-result messages are persistence transport. Their results are
        // joined to the calls and rendered in the owning user's run activity.
        AgentMessage::ToolResult { .. } => div().into_any_element(),
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

pub fn is_hidden_tool_transport(message: &AgentMessage) -> bool {
    match message {
        AgentMessage::ToolResult { .. } => true,
        AgentMessage::Assistant { content, .. } => content
            .iter()
            .any(|part| matches!(part, AssistantContentPart::ToolCall { .. })),
        AgentMessage::User { .. } => false,
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
        .child(crate::primitives::app_icon(crate::primitives::IconName::Bot, 32.0, theme.text_ghost))
        .child(
            div()
                .text_size(px(14.0))
                .text_color(theme.text_ghost)
                .child("Start a conversation"),
        )
}
