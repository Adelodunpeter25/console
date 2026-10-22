package com.console.mobile.feature.chat.markdown

import android.content.Context
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
import io.ratex.RaTeXEngine
import io.ratex.RaTeXFontLoader
import io.ratex.RaTeXRenderer

/**
 * LaTeX through RaTeX (https://github.com/erweixin/RaTeX), the same
 * KaTeX-compatible Rust engine the desktop client uses, so a formula typesets
 * identically on both. The AAR carries the native library and KaTeX fonts.
 */

/** Body text size the formulas are typeset against, in sp. Display math is a little larger. */
private const val INLINE_SP = 15f
private const val DISPLAY_SP = 19f

private data class TypesetKey(val latex: String, val display: Boolean, val color: Int, val fontPx: Float)

/** Typeset formulas, so a settled message does not re-typeset on every recomposition. */
private object TypesetCache {
    private const val LIMIT = 128
    private val entries = object : LinkedHashMap<TypesetKey, RaTeXRenderer?>(LIMIT, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<TypesetKey, RaTeXRenderer?>?) = size > LIMIT
    }

    @Synchronized
    fun get(key: TypesetKey, build: () -> RaTeXRenderer?): RaTeXRenderer? {
        if (entries.containsKey(key)) return entries[key]
        return build().also { entries[key] = it }
    }
}

/**
 * Typeset [latex], or null when RaTeX cannot parse it. A half-streamed formula
 * is the common case, so the caller shows the source text instead.
 */
internal fun typeset(context: Context, latex: String, display: Boolean, color: Int, fontPx: Float): RaTeXRenderer? =
    TypesetCache.get(TypesetKey(latex.trim(), display, color, fontPx)) {
        runCatching {
            // Idempotent; loads the bundled KaTeX faces on first use.
            RaTeXFontLoader.ensureLoaded(context)
            RaTeXRenderer(RaTeXEngine.parseBlocking(latex.trim(), display, color), fontPx) { RaTeXFontLoader.getTypeface(it) }
        }.getOrNull()
    }

/** A formula drawn at its own size. */
@Composable
internal fun Latex(expression: String, display: Boolean) {
    val context = LocalContext.current
    val density = LocalDensity.current
    val color = NewTheme.TextPrimary.toArgb()
    val fontPx = with(density) { (if (display) DISPLAY_SP else INLINE_SP).sp.toPx() }
    val renderer = remember(expression, display, color, fontPx) { typeset(context, expression, display, color, fontPx) }
    if (renderer == null) {
        Text(
            expression,
            color = NewTheme.TextSecondary,
            fontFamily = ConsoleMonoFamily,
            fontSize = 13.sp,
            modifier = Modifier.padding(vertical = if (display) 6.dp else 0.dp),
        )
        return
    }
    val width = with(density) { renderer.widthPx.toDp() }
    val height = with(density) { renderer.totalHeightPx.toDp() }
    val canvas = @Composable {
        Canvas(Modifier.size(width, height)) { drawIntoCanvas { renderer.draw(it.nativeCanvas) } }
    }
    if (display) {
        // A wide equation scrolls sideways inside its own block instead of overflowing.
        Box(
            Modifier.fillMaxWidth().padding(vertical = 8.dp).horizontalScroll(rememberScrollState()),
            contentAlignment = Alignment.Center,
        ) { canvas() }
    } else {
        canvas()
    }
}

/** Reserved size of an inline formula's placeholder, from the typeset result. */
internal data class InlineMathSize(val widthSp: Float, val heightSp: Float)

@Composable
internal fun rememberInlineMathSize(expression: String): InlineMathSize {
    val context = LocalContext.current
    val density = LocalDensity.current
    val color = NewTheme.TextPrimary.toArgb()
    val fontPx = with(density) { INLINE_SP.sp.toPx() }
    return remember(expression, color, fontPx) {
        val renderer = typeset(context, expression, false, color, fontPx)
        with(density) {
            if (renderer != null) {
                InlineMathSize(renderer.widthPx.toSp().value, renderer.totalHeightPx.toSp().value)
            } else {
                // Unparseable (still streaming): reserve roughly the source's width in mono 13sp.
                InlineMathSize(expression.length * 7.8f + 2f, 18f)
            }
        }
    }
}

internal val InlineMathColor: Color get() = NewTheme.TextPrimary
