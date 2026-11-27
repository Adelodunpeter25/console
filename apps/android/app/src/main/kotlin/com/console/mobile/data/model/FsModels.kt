package com.console.mobile.data.model

const val IMAGE_MAX_BYTES: Long = 10 * 1024 * 1024
const val SVG_EXTENSION = ".svg"

val IMAGE_PREVIEW_EXTENSIONS: Set<String> = setOf(
    ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico"
)

typealias FilePreviewBlockKind = String

data class FilePreviewBlock(val kind: String, val title: String, val message: String)

private val LOCK_FILE_BASENAMES: Set<String> = setOf(
    "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "composer.lock"
)
private val LOCK_FILE_SUFFIXES = listOf(".lock", ".lockb")

private val VIDEO_EXTENSIONS: Set<String> = setOf(
    ".mp4", ".mov", ".avi", ".mkv", ".webm"
)
private val AUDIO_EXTENSIONS: Set<String> = setOf(
    ".mp3", ".wav", ".flac", ".ogg", ".m4a"
)
private val PDF_EXTENSIONS: Set<String> = setOf(".pdf")
private val HTML_EXTENSIONS: Set<String> = setOf(".html", ".htm")

private val BINARY_FILE_EXTENSIONS: Set<String> = VIDEO_EXTENSIONS + AUDIO_EXTENSIONS + PDF_EXTENSIONS + setOf(
    ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico",
    ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".jar", ".war",
    ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj", ".bin", ".iso", ".dmg",
    ".pkg", ".deb", ".rpm", ".apk", ".ipa", ".node",
    ".ttf", ".otf", ".woff", ".woff2", ".eot",
    ".wasm", ".psd", ".sketch", ".class", ".pyc", ".db", ".sqlite", ".sqlite3",
    ".icns", ".tiff"
)

private val MIME_BY_EXTENSION: Map<String, String> = mapOf(
    ".png" to "image/png", ".jpg" to "image/jpeg", ".jpeg" to "image/jpeg",
    ".gif" to "image/gif", ".webp" to "image/webp", ".bmp" to "image/bmp",
    ".ico" to "image/x-icon", ".svg" to "image/svg+xml",
    ".mp4" to "video/mp4", ".mov" to "video/quicktime", ".avi" to "video/x-msvideo",
    ".mkv" to "video/x-matroska", ".webm" to "video/webm",
    ".mp3" to "audio/mpeg", ".wav" to "audio/wav", ".flac" to "audio/flac",
    ".ogg" to "audio/ogg", ".m4a" to "audio/mp4",
    ".pdf" to "application/pdf",
    ".zip" to "application/zip", ".gz" to "application/gzip", ".tgz" to "application/gzip",
    ".tar" to "application/x-tar", ".bz2" to "application/x-bzip2",
    ".xz" to "application/x-xz", ".7z" to "application/x-7z-compressed",
    ".rar" to "application/vnd.rar", ".jar" to "application/java-archive",
    ".war" to "application/java-archive", ".apk" to "application/vnd.android.package-archive",
    ".ipa" to "application/octet-stream", ".dmg" to "application/x-apple-diskimage",
    ".iso" to "application/x-iso9660-image", ".exe" to "application/vnd.microsoft.portable-executable",
    ".msi" to "application/x-msi", ".deb" to "application/vnd.debian.binary-package",
    ".rpm" to "application/x-rpm", ".pkg" to "application/octet-stream",
    ".ttf" to "font/ttf", ".otf" to "font/otf", ".woff" to "font/woff",
    ".woff2" to "font/woff2", ".eot" to "application/vnd.ms-fontobject",
    ".wasm" to "application/wasm", ".db" to "application/octet-stream",
    ".sqlite" to "application/vnd.sqlite3", ".sqlite3" to "application/vnd.sqlite3",
    ".class" to "application/java-vm", ".psd" to "image/vnd.adobe.photoshop",
    ".sketch" to "application/zip", ".icns" to "image/icns", ".tiff" to "image/tiff"
)

fun extensionOf(fileName: String): String {
    val idx = fileName.lastIndexOf('.')
    return if (idx == -1) "" else fileName.substring(idx).lowercase()
}

fun imageMimeForExtension(ext: String): String? {
    val normalized = (if (ext.startsWith('.')) ext else ".$ext").lowercase()
    return MIME_BY_EXTENSION[normalized]
}

fun isLockFileName(fileName: String): Boolean {
    val lower = fileName.lowercase()
    if (LOCK_FILE_BASENAMES.contains(lower)) return true
    return LOCK_FILE_SUFFIXES.any { lower.endsWith(it) }
}

fun isBinaryFileName(fileName: String): Boolean = BINARY_FILE_EXTENSIONS.contains(extensionOf(fileName))
fun isPreviewableImageName(fileName: String): Boolean = IMAGE_PREVIEW_EXTENSIONS.contains(extensionOf(fileName))
fun isSvgFileName(fileName: String): Boolean = extensionOf(fileName) == SVG_EXTENSION
fun isHtmlFileName(fileName: String): Boolean = HTML_EXTENSIONS.contains(extensionOf(fileName))

/** How the file viewer renders a file: fetched image bytes, a media player, HTML preview, a details card, or text. */
enum class FilePreviewKind { IMAGE, VIDEO, AUDIO, PDF, HTML, DETAILS, BLOCKED, TEXT }

fun filePreviewKind(fileName: String): FilePreviewKind = when {
    isPreviewableImageName(fileName) || isSvgFileName(fileName) -> FilePreviewKind.IMAGE
    VIDEO_EXTENSIONS.contains(extensionOf(fileName)) -> FilePreviewKind.VIDEO
    AUDIO_EXTENSIONS.contains(extensionOf(fileName)) -> FilePreviewKind.AUDIO
    PDF_EXTENSIONS.contains(extensionOf(fileName)) -> FilePreviewKind.PDF
    isHtmlFileName(fileName) -> FilePreviewKind.HTML
    isLockFileName(fileName) -> FilePreviewKind.BLOCKED
    isBinaryFileName(fileName) -> FilePreviewKind.DETAILS
    else -> FilePreviewKind.TEXT
}

fun formatBytes(bytes: Long): String = when {
    bytes < 1024 -> "$bytes B"
    bytes < 1024 * 1024 -> "${bytes / 1024} KB"
    else -> String.format("%.1f MB", bytes / (1024.0 * 1024.0))
}

fun getFilePreviewBlock(fileName: String): FilePreviewBlock? {
    // Only lockfiles dead-end: video/audio/PDF have players and every other
    // opaque format renders the details card via fileDetails().
    if (isLockFileName(fileName)) return FilePreviewBlock(
        kind = "LOCKFILE_BLOCKED",
        title = "Lockfiles can't be previewed",
        message = "\"$fileName\" is a generated lockfile — open it on your machine instead."
    )
    return null
}

/** MIME label for a file name, or null when the extension is unknown. */
fun mimeTypeForName(fileName: String): String? =
    MIME_BY_EXTENSION[extensionOf(fileName)]

/** Metadata shown on the opaque-file (DETAILS) card; mirrors the desktop details panel. */
data class FileDetails(
    val fileName: String,
    val mimeType: String?,
    val sizeBytes: Long?,
    val width: Int? = null,
    val height: Int? = null,
    val durationMs: Long? = null,
) {
    /** "Video · 12.4 MB" style subtitle for the card header. */
    val label: String get() = mimeType ?: "File"

    /** Human-readable rows for the card, in display order. */
    val fields: List<Pair<String, String>>
        get() = buildList {
            mimeType?.let { add("Type" to it) }
            sizeBytes?.let { add("Size" to formatBytes(it)) }
            if (width != null && height != null) add("Dimensions" to "$width × $height")
            durationMs?.let { add("Duration" to formatDurationMs(it)) }
        }
}

fun fileDetails(fileName: String, sizeBytes: Long? = null): FileDetails = FileDetails(
    fileName = fileName,
    mimeType = mimeTypeForName(fileName),
    sizeBytes = sizeBytes,
)

fun formatDurationMs(ms: Long): String {
    val totalSeconds = ms.coerceAtLeast(0) / 1000
    val hours = totalSeconds / 3600
    val minutes = (totalSeconds % 3600) / 60
    val seconds = totalSeconds % 60
    return if (hours > 0) {
        "%d:%02d:%02d".format(hours, minutes, seconds)
    } else {
        "%d:%02d".format(minutes, seconds)
    }
}

fun isMarkdownPath(path: String?): Boolean {
    if (path.isNullOrEmpty()) return false
    val lower = path.lowercase()
    return lower.endsWith(".md") || lower.endsWith(".mdx") || lower.endsWith(".markdown") || lower.endsWith(".mkd")
}
