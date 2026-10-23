package com.console.mobile

import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * The logic both clients share, implemented once in Rust (`console-shared`)
 * and reached over JNI. The desktop imports the same crate directly, so a
 * diff, a pairing URL or a tool label cannot come out differently on the two.
 *
 * This is the *only* surface app code uses: the `external` functions are
 * private, and every public function checks [available] first.
 *
 * ### The library is optional at build time
 *
 * `libconsole_wt_android.so` is gitignored and built by `rust-wt/build.sh`,
 * so a fresh clone, a JVM unit test, or a 32-bit ABI has none. [available] is
 * then false and every function here answers with its documented *inert*
 * value (`null`, empty, `false`) instead of throwing, which would take the
 * app down from class initialisation. CI asserts the library is packaged in
 * the release APK, so an inert build is a development condition, not
 * something users ship with.
 *
 * ### A stale library is detected, not trusted
 *
 * [EXPECTED_ABI] must equal the Rust `ABI_VERSION`. An APK carrying a `.so`
 * from a different commit reports a different number, [available] turns
 * false, and no symbol is called. That is the whole defence against a missing
 * or reshaped function, so bump both numbers together.
 */
object SharedNative {
    /** Must match `ABI_VERSION` in `rust-wt/src/shared.rs`. */
    const val EXPECTED_ABI = 1

    /** Framed payloads start with `"CSHR"` and this version. */
    private const val MAGIC = 0x4353_4852
    private const val VERSION: Short = 1

    /** True when the library loaded *and* speaks the ABI this build expects. */
    val available: Boolean = try {
        System.loadLibrary("console_wt_android")
        abiVersion() == EXPECTED_ABI
    } catch (_: UnsatisfiedLinkError) {
        false
    }

    private external fun abiVersion(): Int
    private external fun extractHtmlTitle(html: String): String?
    private external fun closeHanging(text: String): String?
    private external fun toolLabel(name: String): String?
    private external fun toolFlags(name: String): Int
    private external fun fileIconKey(path: String): String?
    private external fun languageIconKey(tag: String): String?
    private external fun formatContextWindow(tokens: Int): String?
    private external fun formatModelName(id: String): String?
    private external fun formatTokensPerSecond(rate: Double): String?
    private external fun parsePairingUrl(raw: String): ByteArray?
    private external fun splitPairingLine(raw: String): ByteArray?
    private external fun hostPort(address: String): String?
    private external fun diffLines(old: String, new: String, context: Int): ByteArray?
    private external fun parseUnifiedDiff(raw: String): ByteArray?
    private external fun fileCallDiffs(name: String, argumentsJson: String): ByteArray?

    // ── Text ──────────────────────────────────────────────────────────────

    /** The page title of an HTML document, cleaned for display, or null. */
    fun pageTitle(html: String): String? = if (available) extractHtmlTitle(html) else null

    /**
     * Repair a streaming Markdown tail so unfinished emphasis, code spans and
     * links render stably. Null when nothing hangs (the common case).
     */
    fun mendMarkdown(text: String): String? = if (available) closeHanging(text) else null

    // ── Tools ─────────────────────────────────────────────────────────────

    /** Display label for a tool call name; the raw name when unavailable. */
    fun label(toolName: String): String =
        (if (available) toolLabel(toolName) else null) ?: toolName

    /** What a tool call touches. All false when the library is unavailable. */
    data class ToolKind(
        val edit: Boolean,
        val write: Boolean,
        val read: Boolean,
        val fileTarget: Boolean,
        val subagent: Boolean,
    )

    fun kind(toolName: String): ToolKind {
        val flags = if (available) toolFlags(toolName) else 0
        return ToolKind(
            edit = flags and 1 != 0,
            write = flags and 2 != 0,
            read = flags and 4 != 0,
            fileTarget = flags and 8 != 0,
            subagent = flags and 16 != 0,
        )
    }

    // ── Icons ─────────────────────────────────────────────────────────────

    /** Icon key (`rust`, `typescript`, ...) for a path or file name; `file` when unknown. */
    fun fileIcon(pathOrName: String): String =
        (if (available) fileIconKey(pathOrName) else null) ?: "file"

    /** Icon key for a code-fence language tag, or null when there is none. */
    fun languageIcon(tag: String): String? = if (available) languageIconKey(tag) else null

    // ── Formatting ────────────────────────────────────────────────────────

    fun contextWindow(tokens: Int): String? = if (available) formatContextWindow(tokens) else null

    fun modelName(id: String): String? = if (available) formatModelName(id) else null

    /** "42.1 tok/s", or null when the rate is missing, non-finite or not positive. */
    fun tokensPerSecond(rate: Double?): String? =
        if (available) formatTokensPerSecond(rate ?: Double.NaN) else null

    // ── Pairing ───────────────────────────────────────────────────────────

    data class Pairing(val baseUrl: String, val serverId: String, val code: String)

    /**
     * Parse a pairing URL. [Result.failure] carries the user-facing reason, or
     * a generic one when the library is unavailable, so a paste is never
     * accepted without the strict parser having looked at it.
     */
    fun pairing(raw: String): Result<Pairing> {
        val bytes = if (available) parsePairingUrl(raw) else null
        val reader = bytes?.let(::Reader)?.takeIf { it.valid }
            ?: return Result.failure(IllegalStateException("Pairing is unavailable on this build."))
        return if (reader.u8() == 1) {
            Result.success(Pairing(reader.str(), reader.str(), reader.str()))
        } else {
            Result.failure(IllegalArgumentException(reader.str()))
        }
    }

    /** Split pasted text into `(address, code)`; `(raw, "")` when unavailable. */
    fun splitPairing(raw: String): Pair<String, String> {
        val reader = (if (available) splitPairingLine(raw) else null)?.let(::Reader)?.takeIf { it.valid }
            ?: return raw.trim() to ""
        return reader.str() to reader.str()
    }

    /** `host:port` for display; the input unchanged when unavailable. */
    fun displayHost(address: String): String =
        (if (available) hostPort(address) else null) ?: address

    // ── Diffs ─────────────────────────────────────────────────────────────

    enum class DiffKind { Context, Added, Removed }

    data class DiffLine(val kind: DiffKind, val text: String, val oldNo: Int?, val newNo: Int?)

    data class Diff(val lines: List<DiffLine>, val added: Int, val removed: Int) {
        companion object {
            val EMPTY = Diff(emptyList(), 0, 0)
        }
    }

    data class FileDiff(val path: String, val diff: Diff)

    /** Diff of two texts with [context] unchanged lines around each change. */
    fun diff(old: String, new: String, context: Int = 3): Diff =
        decodeDiff(if (available) diffLines(old, new, context) else null)

    /** Parse the unified diff the server produced (Changes tab, review sheet). */
    fun parseDiff(raw: String): Diff =
        decodeDiff(if (available) parseUnifiedDiff(raw) else null)

    /**
     * The per-file diffs a file tool call describes, in call order; empty for
     * any other tool. [argumentsJson] is the call's arguments as JSON.
     */
    fun callDiffs(toolName: String, argumentsJson: String): List<FileDiff> {
        val reader = (if (available) fileCallDiffs(toolName, argumentsJson) else null)
            ?.let(::Reader)?.takeIf { it.valid } ?: return emptyList()
        return List(reader.u32()) { FileDiff(reader.str(), reader.diff()) }
    }

    private fun decodeDiff(bytes: ByteArray?): Diff {
        val reader = bytes?.let(::Reader)?.takeIf { it.valid } ?: return Diff.EMPTY
        return reader.diff()
    }

    /** Big-endian reader over a framed payload; [valid] is false on a bad preamble. */
    private class Reader(bytes: ByteArray) {
        private val buffer = ByteBuffer.wrap(bytes).order(ByteOrder.BIG_ENDIAN)
        val valid: Boolean =
            bytes.size >= 6 && buffer.int == MAGIC && buffer.short == VERSION

        fun u8(): Int = buffer.get().toInt() and 0xFF
        fun u32(): Int = buffer.int
        fun str(): String {
            val bytes = ByteArray(buffer.int)
            buffer.get(bytes)
            return String(bytes, Charsets.UTF_8)
        }

        fun diff(): Diff {
            val added = u32()
            val removed = u32()
            val lines = List(u32()) {
                val kind = when (u8()) {
                    1 -> DiffKind.Added
                    2 -> DiffKind.Removed
                    else -> DiffKind.Context
                }
                val old = buffer.int
                val new = buffer.int
                DiffLine(kind, str(), old.takeIf { it >= 0 }, new.takeIf { it >= 0 })
            }
            return Diff(lines, added, removed)
        }
    }
}
