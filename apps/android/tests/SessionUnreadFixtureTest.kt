package com.console.mobile

import com.console.mobile.data.model.SessionStatus
import com.console.mobile.feature.home.ReadAction
import com.console.mobile.feature.home.readAction
import com.console.mobile.feature.home.showsUnreadDot
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SessionHeader
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Unread chats: the wire fixture, the row's dot, and when "Mark as unread" is offered. */
class SessionUnreadFixtureTest {
    private val moshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    private fun header(
        status: String = "done",
        unread: Boolean = false,
        settledAt: Long? = null,
    ) = SessionHeader(
        id = "s1",
        title = "Demo",
        cwd = "",
        model_id = "",
        provider = "",
        created_at = 1_000L,
        updated_at = 1_000L,
        message_count = 1,
        status = status,
        settled_at = settledAt,
        unread = unread,
    )

    @Test
    fun decodesGoldenUnreadHeader() {
        val adapter = moshi.adapter(SessionHeader::class.java)
        assertTrue(adapter.fromJson(fixture("header_unread.json"))!!.unread)
        // Older fixtures have no unread field: a header without it reads as read.
        assertFalse(adapter.fromJson(fixture("header.json"))!!.unread)
    }

    @Test
    fun aFinishedUnreadChatShowsTheDot() {
        assertTrue(showsUnreadDot(header(unread = true), SessionStatus.Done))
    }

    @Test
    fun aReadChatShowsNoDot() {
        assertFalse(showsUnreadDot(header(unread = false), SessionStatus.Done))
    }

    @Test
    fun runningAndAttentionChatsKeepTheirPillInsteadOfTheDot() {
        assertFalse(showsUnreadDot(header(status = "working", unread = true), SessionStatus.Working))
        assertFalse(showsUnreadDot(header(status = "needs_attention", unread = true), SessionStatus.NeedsAttention))
    }

    @Test
    fun anUnreadChatOffersMarkAsRead() {
        assertEquals(ReadAction.MarkRead, readAction(header(unread = true)))
        // Even a chat waiting on the user can be marked read: they have seen it.
        assertEquals(ReadAction.MarkRead, readAction(header(status = "needs_attention", unread = true)))
    }

    @Test
    fun aReadIdleChatOffersMarkAsUnread() {
        assertEquals(ReadAction.MarkUnread, readAction(header()))
        assertEquals(ReadAction.MarkUnread, readAction(header(status = "needs_attention")))
    }

    @Test
    fun runningAndSettledChatsOfferNoReadEntry() {
        assertNull("running", readAction(header(status = "working")))
        assertNull("settled", readAction(header(settledAt = 5_000L)))
    }
}
