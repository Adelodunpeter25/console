package com.console.mobile.core.util

import androidx.compose.ui.graphics.Color
import com.console.mobile.SharedNative
import com.console.mobile.SharedNative.GitBadgeInfo
import com.console.mobile.SharedNative.GitRole
import com.console.mobile.ui.theme.NewTheme

/**
 * Git badges, as the shared rules describe them.
 *
 * The rules — which code means what, which letter is drawn, how loudly a
 * badge reports itself, and which badge a folder inherits — live in
 * `console-shared`, which the desktop imports directly and this app reaches
 * over JNI. Only the palette is the phone's own, so a file cannot be green on
 * one client and grey on the other.
 */

/** The badge for a git status code, or null when the code has no badge. */
fun gitBadge(status: String?): GitBadgeInfo? = SharedNative.gitBadge(status ?: "")

/** The letter drawn beside a changed file. */
fun statusLetter(status: String?): String = gitBadge(status)?.label ?: "M"

/** The color a git badge is drawn in: the app's palette, by the shared role. */
fun gitBadgeColor(status: String?): Color =
    gitBadge(status)?.let { gitRoleColor(it.role) } ?: NewTheme.TextSecondary

/** The color for a role. */
fun gitRoleColor(role: GitRole): Color = when (role) {
    GitRole.SUCCESS -> NewTheme.Success
    GitRole.WARNING -> NewTheme.Warning
    GitRole.DANGER -> NewTheme.Danger
    GitRole.MUTED -> NewTheme.TextSecondary
}

/**
 * The badge for every path a changed file sits under: the file itself and
 * each folder above it, a folder carrying the strongest badge beneath it.
 *
 * [paths] and [statuses] are parallel arrays, one entry per changed file.
 */
fun gitBadges(paths: List<String>, statuses: List<String>): Map<String, GitBadgeInfo> =
    SharedNative.gitBadges(paths, statuses)
