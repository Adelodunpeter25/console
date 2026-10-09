package com.console.mobile

import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.UserMessage
import com.console.mobile.feature.chat.buildAssistantTurnChangesMap
import console.v1.SessionFileChange
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TurnChangesMatchTest {

    @Test
    fun matchesTurnChangesToClosingAssistantMessage() {
        val user1 = UserMessage(id = "user-1", content = "Edit file a")
        val asst1 = AssistantMessage(id = "asst-1", content = listOf(TextPart("Done editing file a")))
        val user2 = UserMessage(id = "user-2", content = "Edit file b")
        val asst2 = AssistantMessage(id = "asst-2", content = listOf(TextPart("Done editing file b")))

        val changeA = SessionFileChange(path = "src/a.kt", turn_index = 1, user_message_id = "user-1", additions = 5, deletions = 1)
        val changeB = SessionFileChange(path = "src/b.kt", turn_index = 2, user_message_id = "user-2", additions = 10, deletions = 0)

        val messages = listOf(user1, asst1, user2, asst2)
        val changes = listOf(changeB, changeA) // server returns newest first

        val turnMap = buildAssistantTurnChangesMap(messages, changes)

        // Asst 1 (index 1) should have changeA
        assertEquals(listOf(changeA), turnMap[1])
        // Asst 2 (index 3) should have changeB
        assertEquals(listOf(changeB), turnMap[3])
    }

    @Test
    fun returnsEmptyWhenNoChanges() {
        val user1 = UserMessage(id = "user-1", content = "Hello")
        val asst1 = AssistantMessage(id = "asst-1", content = listOf(TextPart("Hi there")))
        val turnMap = buildAssistantTurnChangesMap(listOf(user1, asst1), emptyList())
        assertTrue(turnMap.isEmpty())
    }
}
