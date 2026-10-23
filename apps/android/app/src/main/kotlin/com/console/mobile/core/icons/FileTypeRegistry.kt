package com.console.mobile.core.icons

import com.console.mobile.SharedNative

/**
 * Which file-type icon a path or code-fence language gets.
 *
 * The tables live in Rust (`console-shared::icons`) and are the same ones the
 * desktop uses, so a file looks the same on both. The SVGs are
 * `assets/icons/file-types/<key>.svg`; this layer only picks the key. Without
 * the native library every file gets the generic `file` icon rather than a
 * second, drifting copy of the table.
 */

/** Filename or path -> file-type icon key (fallback "file"). */
fun getFileIconKey(pathOrName: String): String = SharedNative.fileIcon(pathOrName)

/** Code-fence language id -> file-type icon key (fallback "file"). */
fun getLanguageIconKey(language: String): String = SharedNative.languageIcon(language) ?: "file"

/** Provider name -> provider icon asset key, or null when unknown. */
fun getProviderIconKey(provider: String?): String? {
    if (provider.isNullOrBlank()) return null
    return when (provider.lowercase()) {
        "antigravity" -> "antigravity"
        "openai", "codex" -> "openai"
        "opencode" -> "opencode"
        "claude", "anthropic" -> "claude"
        else -> null
    }
}
