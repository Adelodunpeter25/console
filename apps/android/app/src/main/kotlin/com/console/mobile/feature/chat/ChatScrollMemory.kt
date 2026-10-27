package com.console.mobile.feature.chat

/**
 * Remembers where each session's transcript was scrolled to, so leaving the
 * chat screen (Files, Changes, Home) and coming back resumes the same place
 * instead of jumping to the newest message.
 *
 * Process-lifetime only: a fresh app launch starts every chat at the bottom.
 */
internal object ChatScrollMemory {
    internal data class Position(
        val index: Int,
        val offset: Int,
        /** Whether the transcript was at the end, so new messages keep following it. */
        val atBottom: Boolean,
    )

    private val positions = mutableMapOf<String, Position>()

    fun get(sessionId: String): Position? = positions[sessionId]

    fun save(sessionId: String, index: Int, offset: Int, atBottom: Boolean) {
        positions[sessionId] = Position(index, offset, atBottom)
    }
}
