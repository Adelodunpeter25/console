package com.console.mobile.data.local

import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.UserMessage
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest

private class FakeSharedPreferences : android.content.SharedPreferences {
    private val data = mutableMapOf<String, Any?>()

    override fun getAll(): Map<String, *> = data
    override fun getString(key: String, defValue: String?): String? = data[key] as? String ?: defValue
    override fun getStringSet(key: String, defValues: Set<String>?): Set<String>? =
        @Suppress("UNCHECKED_CAST") (data[key] as? Set<String> ?: defValues)
    override fun getInt(key: String, defValue: Int): Int = data[key] as? Int ?: defValue
    override fun getLong(key: String, defValue: Long): Long = data[key] as? Long ?: defValue
    override fun getFloat(key: String, defValue: Float): Float = data[key] as? Float ?: defValue
    override fun getBoolean(key: String, defValue: Boolean): Boolean = data[key] as? Boolean ?: defValue
    override fun contains(key: String): Boolean = data.containsKey(key)
    override fun edit(): android.content.SharedPreferences.Editor = Editor(this)
    override fun registerOnSharedPreferenceChangeListener(listener: android.content.SharedPreferences.OnSharedPreferenceChangeListener?) {}
    override fun unregisterOnSharedPreferenceChangeListener(listener: android.content.SharedPreferences.OnSharedPreferenceChangeListener?) {}

    class Editor(private val parent: FakeSharedPreferences) : android.content.SharedPreferences.Editor {
        private val pending = mutableMapOf<String, Any?>()
        private var clear = false

        override fun putString(key: String, value: String?): android.content.SharedPreferences.Editor { pending[key] = value; return this }
        override fun putStringSet(key: String, values: Set<String>?): android.content.SharedPreferences.Editor { pending[key] = values; return this }
        override fun putInt(key: String, value: Int): android.content.SharedPreferences.Editor { pending[key] = value; return this }
        override fun putLong(key: String, value: Long): android.content.SharedPreferences.Editor { pending[key] = value; return this }
        override fun putFloat(key: String, value: Float): android.content.SharedPreferences.Editor { pending[key] = value; return this }
        override fun putBoolean(key: String, value: Boolean): android.content.SharedPreferences.Editor { pending[key] = value; return this }
        override fun remove(key: String): android.content.SharedPreferences.Editor { pending[key] = null; return this }
        override fun clear(): android.content.SharedPreferences.Editor { clear = true; return this }
        override fun commit(): Boolean { apply(); return true }
        override fun apply() {
            if (clear) parent.data.clear()
            for ((k, v) in pending) {
                if (v == null) parent.data.remove(k) else parent.data[k] = v
            }
        }
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class ChatPersistenceTest {
    @Test fun roundTripMessagesAndDraft() = runTest {
        val prefs = FakeSharedPreferences()
        val p = ChatPersistence(prefs, this)

        val session = createChatSessionState().copy(
            messages = listOf(UserMessage(id = "m1", content = "hello")),
            input = "draft",
            attachments = listOf(ImageAttachment("b64", "image/png")),
            draftUpdatedAt = 12345L,
        )
        p.saveNow(mapOf("s1" to session))

        val loaded = p.load()
        assertEquals(1, loaded.size)
        val s1 = loaded["s1"]!!
        assertEquals("draft", s1.input)
        assertEquals(1, s1.messages.size)
        assertEquals("hello", (s1.messages[0] as UserMessage).content)
        assertEquals(1, s1.attachments.size)
        assertEquals(12345L, s1.draftUpdatedAt)
    }

    @Test fun capsMessagesAndSessions() = runTest {
        val prefs = FakeSharedPreferences()
        val p = ChatPersistence(prefs, this)

        // build 30 sessions, each with 60 messages
        val bigMap = (1..30).associate { idx ->
            "s$idx" to createChatSessionState().copy(
                messages = (1..60).map { m -> UserMessage(id = "m$m", content = "text$m") },
            )
        }
        p.saveNow(bigMap)

        val loaded = p.load()
        assertEquals(ChatPersistence.MAX_PERSISTED_SESSIONS, loaded.size)
        val s = loaded.values.first()
        assertEquals(ChatPersistence.MAX_PERSISTED_MESSAGES, s.messages.size)
    }

    @Test fun clearWipesCache() = runTest {
        val prefs = FakeSharedPreferences()
        val p = ChatPersistence(prefs, this)

        p.saveNow(mapOf("s1" to createChatSessionState().copy(input = "hi")))
        assertTrue(p.load().isNotEmpty())

        p.clear()
        assertTrue(p.load().isEmpty())
    }
}
