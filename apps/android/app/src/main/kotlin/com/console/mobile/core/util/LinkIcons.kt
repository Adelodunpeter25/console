package com.console.mobile.core.util

import com.console.mobile.core.icons.getFileIconKey

/**
 * Decides which inline icon (file-type or favicon) the markdown renderer
 * draws in front of a path or link. Pure string logic so it stays testable.
 */

private val PATH_CHARS = Regex("^[A-Za-z0-9._\\-/@+~\\\\]+$")
/** Extensionless names common enough to be files when they stand alone. */
private val BARE_FILE_NAMES = setOf(
    "makefile", "dockerfile", "justfile", "license", "licence", "gemfile",
    "rakefile", "procfile", "brewfile", "vagrantfile", "codeowners",
)
private val LINE_SUFFIX = Regex("(:\\d+){1,2}$")

/**
 * Bare paths in prose, same grammar as the desktop parser: `~/`, `/`, `./`,
 * `../` or `.dir/` prefixed, or at least one directory, ending in a file
 * extension, with an optional `:line:col`.
 */
val BARE_FILE_PATH_REGEX = Regex(
    "(?:(?:~/|/|\\.\\./|\\./|\\.[\\w\\-.]+/)(?:[\\w\\-.]+/)*[\\w\\-.]*[\\w]\\.[A-Za-z0-9]{1,10}(?::\\d+(?::\\d+)?)?" +
        "|\\b(?:[\\w\\-.]+/)+[\\w\\-.]*[\\w]\\.[A-Za-z0-9]{1,10}(?::\\d+(?::\\d+)?)?)",
)
val BARE_FILE_URL_REGEX = Regex("""\bfile://[^\s<>"`\\]+""", RegexOption.IGNORE_CASE)

/** `/`, `~/` and `.` paths must start a token, so `$0.0005/audio-minute` and `env/x.y` stay prose. */
fun isBareFilePathMatch(text: String, start: Int, matched: String): Boolean {
    if ((matched.startsWith("/") || matched.startsWith("~/") || matched.startsWith(".")) && start > 0) {
        val prev = text[start - 1]
        if (!prev.isWhitespace() && prev !in "([{<\"'`") return false
    }
    return isFileLink(matched)
}

/** True when [link] should open as a workspace file: `file://`, or a path whose name has a lettered extension. */
fun isFileLink(link: String): Boolean {
    val l = link.trim()
    if (l.isEmpty() || l.startsWith("http://", true) || l.startsWith("https://", true)) return false
    if (l.startsWith("file://", true) || l.lowercase() in BARE_FILE_NAMES) return true
    val file = stripLineSuffix(l).substringAfterLast('/').substringAfterLast('\\')
    val dot = file.lastIndexOf('.')
    if (dot < 0 || dot + 1 >= file.length) return false
    val ext = file.substring(dot + 1)
    return ext.length <= 10 && ext.all { it.isLetterOrDigit() && it.code < 128 } && ext.any { it.isLetter() }
}

/** A link that is just a file name: no directory, not `~`, not `file://`. */
fun isBareFileName(link: String): Boolean {
    val name = stripLineSuffix(link.trim())
    return name.isNotEmpty() && '/' !in name && '\\' !in name && !name.startsWith("~") && !link.trim().startsWith("file://", true)
}

/** Of several same-named files, the one sharing the longest leading path with [cwd]; ties go shallower, then alphabetical. */
fun closestMatch(cwd: String?, candidates: List<String>): String? {
    val base = cwd.orEmpty().split('/').filter { it.isNotEmpty() }
    return candidates.minWithOrNull(
        compareByDescending<String> { c ->
            c.split('/').filter { it.isNotEmpty() }.zip(base).takeWhile { (a, b) -> a == b }.size
        }.thenBy { it.split('/').count { p -> p.isNotEmpty() } }.thenBy { it },
    )
}

/** `foo.kt:12:3` -> 12; null without a suffix. */
fun parseLineSuffix(path: String): Int? =
    LINE_SUFFIX.find(path.trim())?.value?.trimStart(':')?.substringBefore(':')?.toIntOrNull()?.takeIf { it > 0 }

/** [target] with the `:line:col` suffix [source] carried, so a link keeps where it pointed. */
fun withLineSuffix(target: String, source: String): String =
    target + (LINE_SUFFIX.find(source.trim().substringBefore('#'))?.value ?: "")

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
    val known = getFileIconKey(name) != "file" || name.lowercase() in BARE_FILE_NAMES
    val isHomePath = path.startsWith("~/")
    return if (known || (hasSeparator && hasExtension) || isHomePath) path else null
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

/** Scheme that marks a markdown link as "open this workspace file". */
const val FILE_LINK_PREFIX = "console-file:"

/** The workspace path a link points at (`foo.kt`, `./a/b.rs:12`, `file:///x`), or null for anything else. */
fun fileLinkTarget(url: String): String? {
    val trimmed = url.trim()
    if (trimmed.startsWith("file://", ignoreCase = true)) {
        return cleanFileUrlPath(trimmed)
    }
    if (trimmed.contains("://") || trimmed.startsWith("mailto:", ignoreCase = true)) return null
    return filePathOrNull(trimmed)
}

/** Inverse of the [FILE_LINK_PREFIX] encoding: the path inside a file link, else null. */
fun filePathFromLink(uri: String): String? =
    if (uri.startsWith(FILE_LINK_PREFIX)) uri.removePrefix(FILE_LINK_PREFIX).ifBlank { null } else null

/**
 * Absolute path for [path] as written by the agent: absolute stays, `~` is
 * left to the server, and anything relative joins [root] (the project folder).
 */
fun resolveFilePath(path: String, root: String?): String {
    val clean = if (path.trim().startsWith("file://", true)) cleanFileUrlPath(path.trim()).orEmpty() else stripLineSuffix(path.trim())
    if (clean.startsWith("/") || clean.startsWith("~") || root.isNullOrBlank()) return clean
    return root.trimEnd('/') + "/" + clean.removePrefix("./")
}

/** `file:///a/b%20c.kt:3` -> `/a/b c.kt`: scheme, `localhost`, fragment, line suffix and percent-escapes removed. */
fun cleanFileUrlPath(url: String): String? {
    var raw = url.substring(7).substringBefore('#')
    if (raw.startsWith("localhost")) raw = raw.removePrefix("localhost")
    val path = stripLineSuffix(raw).trim()
    if (path.isEmpty()) return null
    return runCatching { java.net.URLDecoder.decode(path.replace("+", "%2B"), "UTF-8") }.getOrDefault(path)
}

/**
 * The session's working directory for resolving agent-written relative
 * paths (the chat the user is looking at), or null when unknown.
 */
fun chatWorkspaceCwd(): String? {
    val sessionId = com.console.mobile.AppContainer.appStateHolder.state.value.selectedSessionId ?: return null
    return com.console.mobile.AppContainer.sessionStateHolder.views.value[sessionId]?.sessionCwd?.takeIf { it.isNotBlank() }
}

/** Absolute path for a chat image target, or null for remote/data URLs. */
fun chatImagePath(url: String, cwd: String? = null): String? {
    val trimmed = url.trim()
    if (trimmed.isEmpty() || trimmed.startsWith("data:", ignoreCase = true)) return null
    if (trimmed.startsWith("http://", ignoreCase = true) || trimmed.startsWith("https://", ignoreCase = true)) return null
    if (trimmed.contains("://", ignoreCase = true) && !trimmed.startsWith("file://", ignoreCase = true)) return null
    val root = cwd ?: chatWorkspaceCwd()
    return resolveFilePath(trimmed, root)
}
