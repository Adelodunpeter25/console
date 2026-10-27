package com.console.mobile.core.util

/**
 * Remembers list scroll positions for the lifetime of the process, so leaving a
 * screen and coming back resumes the same place. Not persisted across launches:
 * a fresh start begins at the natural position (top for Home, latest for a chat).
 */
internal object ScrollMemory {
    data class Position(
        val index: Int,
        val offset: Int,
        /** Whether the list was at its end, so new content keeps following it. */
        val atEnd: Boolean = false,
    )

    private val positions = mutableMapOf<String, Position>()

    fun get(key: String): Position? = positions[key]

    fun save(key: String, index: Int, offset: Int, atEnd: Boolean = false) {
        positions[key] = Position(index, offset, atEnd)
    }
}
