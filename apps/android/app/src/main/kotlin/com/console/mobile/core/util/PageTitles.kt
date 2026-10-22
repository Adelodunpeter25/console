package com.console.mobile.core.util

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request

private const val MAX_BODY_BYTES = 96 * 1024
private const val MAX_TITLE_CHARS = 90

/**
 * The page title from an HTML document, cleaned for display: entities
 * decoded, whitespace collapsed, length capped. Null when there is none.
 */
fun extractHtmlTitle(html: String): String? {
    val lower = html.lowercase()
    val open = lower.indexOf("<title")
    if (open < 0) return null
    val gt = lower.indexOf('>', open)
    if (gt < 0) return null
    val close = lower.indexOf("</title", gt)
    if (close < 0) return null
    val collapsed = decodeEntities(html.substring(gt + 1, close)).trim().split(Regex("\\s+")).joinToString(" ")
    if (collapsed.isEmpty()) return null
    return if (collapsed.length > MAX_TITLE_CHARS) collapsed.take(MAX_TITLE_CHARS - 1).trimEnd() + "…" else collapsed
}

private val ENTITY = Regex("&(#x[0-9a-fA-F]+|#\\d+|[a-zA-Z]+);")

private fun decodeEntities(text: String): String = ENTITY.replace(text) { m ->
    val e = m.groupValues[1]
    when {
        e == "amp" -> "&"
        e == "lt" -> "<"
        e == "gt" -> ">"
        e == "quot" -> "\""
        e == "apos" -> "'"
        e == "nbsp" -> " "
        e.startsWith("#x") -> e.drop(2).toIntOrNull(16)?.let { String(Character.toChars(it)) } ?: m.value
        e.startsWith("#") -> e.drop(1).toIntOrNull()?.let { String(Character.toChars(it)) } ?: m.value
        else -> m.value
    }
}

/** Bare http(s) URLs in [text] worth looking up a title for. */
fun bareWebUrls(text: String): List<String> =
    Regex("https?://[^\\s<>\"`)\\]]+").findAll(text)
        .map { it.value.trimEnd('.', ',', ';', ':', '!', '?') }
        .filter { faviconHostOrNull(it) != null }
        .distinct()
        .toList()

/**
 * Page titles for bare links, fetched once each in the background. Reading
 * [version] in composition re-runs the reader when a title arrives.
 */
object PageTitles {
    private val titles = ConcurrentHashMap<String, String>()
    private val requested = ConcurrentHashMap.newKeySet<String>()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val client by lazy {
        OkHttpClient.Builder()
            .callTimeout(5, TimeUnit.SECONDS)
            .build()
    }

    var version by mutableIntStateOf(0)
        private set

    fun get(url: String): String? = titles[url]

    /** Starts a fetch for [url] unless one already ran. */
    fun request(url: String) {
        if (faviconHostOrNull(url) == null || !requested.add(url)) return
        scope.launch {
            val title = runCatching { fetch(url) }.getOrNull() ?: return@launch
            titles[url] = title
            version++
        }
    }

    private fun fetch(url: String): String? {
        val request = Request.Builder().url(url)
            .header("User-Agent", "Mozilla/5.0 (compatible; Console/1.0)")
            .build()
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) return null
            val type = response.header("Content-Type").orEmpty().lowercase()
            if (type.isNotEmpty() && !type.contains("html")) return null
            val source = response.body?.source() ?: return null
            val buffer = StringBuilder()
            val chunk = ByteArray(8 * 1024)
            val stream = source.inputStream()
            var total = 0
            while (total < MAX_BODY_BYTES) {
                val n = stream.read(chunk)
                if (n < 0) break
                total += n
                buffer.append(String(chunk, 0, n, Charsets.UTF_8))
                if (buffer.contains("</title", ignoreCase = true)) break
            }
            return extractHtmlTitle(buffer.toString())
        }
    }
}
