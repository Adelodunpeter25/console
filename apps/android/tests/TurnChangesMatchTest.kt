package com.console.mobile

import com.console.mobile.core.chat.TranscriptTurn
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.UserMessage
import com.console.mobile.feature.chat.buildTurnChangesMap
import console.v1.SessionFileChange
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TurnChangesMatchTest {

    @Test
    fun matchesTurnChangesToClosingTurn() {
        val t1 = TranscriptTurn(
            id = "t1",
            userMessage = UserMessage(id = "user-1", content = "Edit file a"),
            responses = listOf(AssistantMessage(id = "asst-1", content = listOf(TextPart("Done editing file a")))),
        )
        val t2 = TranscriptTurn(
            id = "t2",
            userMessage = UserMessage(id = "user-2", content = "Edit file b"),
            responses = listOf(AssistantMessage(id = "asst-2", content = listOf(TextPart("Done editing file b")))),
        )

        val changeA = SessionFileChange(path = "src/a.kt", turn_index = 1, user_message_id = "user-1", additions = 5, deletions = 1)
        val changeB = SessionFileChange(path = "src/b.kt", turn_index = 2, user_message_id = "user-2", additions = 10, deletions = 0)

        val turnMap = buildTurnChangesMap(listOf(t1, t2), listOf(changeB, changeA))

        assertEquals(listOf(changeA), turnMap[0])
        assertEquals(listOf(changeB), turnMap[1])
    }

    @Test
    fun returnsEmptyWhenNoChanges() {
        val t1 = TranscriptTurn(
            id = "t1",
            userMessage = UserMessage(id = "user-1", content = "Hello"),
            responses = listOf(AssistantMessage(id = "asst-1", content = listOf(TextPart("Hi there")))),
        )
        val turnMap = buildTurnChangesMap(listOf(t1), emptyList())
        assertTrue(turnMap.isEmpty())
    }
}
