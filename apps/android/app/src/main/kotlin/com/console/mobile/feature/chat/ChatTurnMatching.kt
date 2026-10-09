package com.console.mobile.feature.chat

import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.UserMessage
import console.v1.SessionFileChange

/**
 * Maps each visible assistant message index that closes a turn to the files
 * changed during that user turn (in reverse server order = chronological edit order).
 */
fun buildAssistantTurnChangesMap(
    displayMessages: List<AgentMessage>,
    sessionChanges: List<SessionFileChange>,
): Map<Int, List<SessionFileChange>> {
    val map = mutableMapOf<Int, List<SessionFileChange>>()
    if (sessionChanges.isEmpty()) return map

    displayMessages.forEachIndexed { i, msg ->
        if (msg is AssistantMessage) {
            val closesTurn = displayMessages.subList(i + 1, displayMessages.size)
                .takeWhile { it !is UserMessage }
                .none { it is AssistantMessage }
            if (closesTurn) {
                val userMsg = displayMessages.subList(0, i).findLast { it is UserMessage } as? UserMessage
                val userId = userMsg?.id
                if (!userId.isNullOrEmpty()) {
                    val turnChanges = sessionChanges
                        .filter { it.user_message_id == userId }
                        .reversed()
                    if (turnChanges.isNotEmpty()) {
                        map[i] = turnChanges
                    }
                }
            }
        }
    }
    return map
}
