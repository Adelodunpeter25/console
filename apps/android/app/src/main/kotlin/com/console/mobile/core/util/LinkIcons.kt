package com.console.mobile.core.util

import com.console.mobile.core.icons.getFileIconKey

/**
 * Decides which inline icon (file-type or favicon) the markdown renderer
 * draws in front of a path or link. Pure string logic so it stays testable.
 */

private val PATH_CHARS = Regex("^[A-Za-z0-9._\\-/@+~\\\\]+$")
private val LINE_SUFFIX = Regex("(:\\d+){1,2}$")

/** `foo.kt:12:3` -> `foo.kt`. */
fun stripLineSuffix(path: String): String = path.replace(LINE_SUFFIX, "")

/**
 * The file path inside [text] when it plainly is one, else null.
 *
 * Conservative on purpose, since this runs on every inline code span: no
 * whitespace, no call/brackets, and either a directory separator or a
 * extension the icon registry knows. That keeps `1.2.3`, `e.g.` and
 * `foo()` as plain code.
 */
fun filePathOrNull(text: String): String? {
    val path = stripLineSuffix(text.trim())
    if (path.isEmpty() || path.length > 240) return null
    if (path.contains("://") || !PATH_CHARS.matches(path)) return null
    val name = path.trimEnd('/', '\\').substringAfterLast('/').substringAfterLast('\\')
    if (name.isEmpty() || name.endsWith(".")) return null
    if (path.endsWith("/") || path.endsWith("\\")) return null
    val hasSeparator = path.contains('/') || path.contains('\\')
    val dot = name.lastIndexOf('.')
    val hasExtension = dot > 0 && dot < name.length - 1 && name.substring(dot + 1).any { it.isLetter() }
    val known = getFileIconKey(name) != "file"
    return if (known || (hasSeparator && hasExtension)) path else null
}

/** Host of an http(s) URL worth fetching a favicon for; null for local or private targets. */
fun faviconHostOrNull(url: String): String? {
    val trimmed = url.trim()
    val rest = when {
        trimmed.startsWith("https://", ignoreCase = true) -> trimmed.substring(8)
        trimmed.startsWith("http://", ignoreCase = true) -> trimmed.substring(7)
        trimmed.startsWith("www.", ignoreCase = true) -> trimmed
        else -> return null
    }
    val host = rest.substringBefore('/').substringBefore('?').substringBefore('#')
        .substringAfter('@').substringBefore(':').lowercase()
    if (host.isEmpty() || !host.contains('.')) return null
    if (host == "localhost" || host.endsWith(".local") || host.endsWith(".internal")) return null
    if (host.all { it.isDigit() || it == '.' }) return null
    return host
}

/** Google's favicon service, sized for high-density screens. */
fun faviconUrl(host: String): String =
    "https://www.google.com/s2/favicons?domain=$host&sz=64"
