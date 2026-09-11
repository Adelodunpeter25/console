package com.console.mobile.data.model

const val MAX_FILE_PREVIEW_BYTES: Long = 512 * 1024
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

private val BINARY_FILE_EXTENSIONS: Set<String> = setOf(
    ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".icns", ".tiff",
    ".mp4", ".mov", ".avi", ".mkv", ".webm", ".mp3", ".wav", ".flac", ".ogg", ".m4a",
    ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".jar", ".war",
    ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj", ".bin", ".iso", ".dmg",
    ".pkg", ".deb", ".rpm", ".apk", ".ipa", ".node",
    ".ttf", ".otf", ".woff", ".woff2", ".eot",
    ".pdf", ".wasm", ".psd", ".sketch", ".class", ".pyc", ".db", ".sqlite", ".sqlite3"
)

private val MIME_BY_EXTENSION: Map<String, String> = mapOf(
    ".png" to "image/png", ".jpg" to "image/jpeg", ".jpeg" to "image/jpeg",
    ".gif" to "image/gif", ".webp" to "image/webp", ".bmp" to "image/bmp",
    ".ico" to "image/x-icon", ".svg" to "image/svg+xml"
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

fun formatBytes(bytes: Long): String = when {
    bytes < 1024 -> "$bytes B"
    bytes < 1024 * 1024 -> "${bytes / 1024} KB"
    else -> String.format("%.1f MB", bytes / (1024.0 * 1024.0))
}

fun getFilePreviewBlock(fileName: String, sizeBytes: Long? = null): FilePreviewBlock? {
    if (isLockFileName(fileName)) return FilePreviewBlock(
        kind = "LOCKFILE_BLOCKED",
        title = "Lockfiles can't be previewed",
        message = "\"$fileName\" is a generated lockfile — open it on your machine instead."
    )
    if (isBinaryFileName(fileName)) return FilePreviewBlock(
        kind = "BINARY_FILE",
        title = "Binary file",
        message = "\"$fileName\" isn't a text file, so there's nothing to preview here."
    )
    if (sizeBytes != null && sizeBytes > MAX_FILE_PREVIEW_BYTES) return FilePreviewBlock(
        kind = "FILE_TOO_LARGE",
        title = "File too large",
        message = "\"$fileName\" is ${formatBytes(sizeBytes)} — previews are capped at ${formatBytes(MAX_FILE_PREVIEW_BYTES)}."
    )
    return null
}

fun isMarkdownPath(path: String?): Boolean {
    if (path.isNullOrEmpty()) return false
    val lower = path.lowercase()
    return lower.endsWith(".md") || lower.endsWith(".mdx") || lower.endsWith(".markdown") || lower.endsWith(".mkd")
}
