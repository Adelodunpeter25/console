package com.console.mobile.ui.code

import android.content.Context
import android.graphics.Typeface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.core.content.res.ResourcesCompat
import com.console.mobile.R

/**
 * JetBrains Mono, shared by the Sora code viewer and by Compose text so both
 * render identical glyphs. The files are copied from apps/desktop/assets/fonts
 * so the two clients match.
 *
 * These live in `res/font` rather than `assets` because Compose's `Font(...)`
 * takes a resource id, while the Sora path resolves a Typeface through
 * `ResourcesCompat`. The terminal's Meslo Nerd Font stays in assets — nothing
 * but the terminal canvas reads it.
 */
val JetBrainsMono: FontFamily = FontFamily(
    Font(R.font.jetbrains_mono_regular, FontWeight.Normal),
    Font(R.font.jetbrains_mono_medium, FontWeight.Medium),
    Font(R.font.jetbrains_mono_bold, FontWeight.Bold),
)

/**
 * The same face as an Android [Typeface] for the Sora editor, which paints with
 * `android.graphics.Paint` and cannot see a Compose `FontFamily`.
 *
 * Falls back to the platform monospace if the resource cannot be loaded, so a
 * bad font degrades the glyphs rather than breaking the viewer.
 */
@Composable
fun rememberJetBrainsMonoTypeface(): Typeface {
    val context = LocalContext.current
    return remember(context) { loadJetBrainsMono(context) }
}

private fun loadJetBrainsMono(context: Context): Typeface =
    ResourcesCompat.getFont(context, R.font.jetbrains_mono_regular) ?: Typeface.MONOSPACE
