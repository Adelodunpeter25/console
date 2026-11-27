package com.console.mobile.feature.chat

import com.console.mobile.core.chat.TranscriptTurn
import console.v1.SessionFileChange

/**
 * Maps each turn index that closes a user turn (the last turn with responses
 * before the next user turn) to the files changed during that user turn.
 */
fun buildTurnChangesMap(
    turns: List<TranscriptTurn>,
    sessionChanges: List<SessionFileChange>,
): Map<Int, List<SessionFileChange>> {
    val map = mutableMapOf<Int, List<SessionFileChange>>()
    if (sessionChanges.isEmpty()) return map

    turns.forEachIndexed { i, turn ->
        if (turn.responses.isEmpty()) return@forEachIndexed
        val closesUserTurn = turns.subList(i + 1, turns.size)
            .takeWhile { it.userMessage == null }
            .all { it.responses.isEmpty() }
        if (!closesUserTurn) return@forEachIndexed
        val userId = turns.subList(0, i + 1)
            .lastOrNull { it.userMessage != null }
            ?.userMessage
            ?.id
        if (!userId.isNullOrEmpty()) {
            val turnChanges = sessionChanges
                .filter { it.user_message_id == userId }
                .reversed()
            if (turnChanges.isNotEmpty()) {
                map[i] = turnChanges
            }
        }
    }
    return map
}
