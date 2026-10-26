package com.console.mobile.ui.components

import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import io.github.lyxnx.compose.ui.tablericons.outline.Globe
import coil3.svg.SvgDecoder
import com.console.mobile.core.icons.getFileIconKey
import com.console.mobile.core.icons.getLanguageIconKey
import com.console.mobile.core.icons.getProviderIconKey


/**
 * Generic skill sparkle icon matching desktop's Sparkle glyph.
 */
@Composable
fun SkillIcon(
    sizeDp: Int = 12,
    tint: androidx.compose.ui.graphics.Color = com.console.mobile.ui.theme.NewTheme.Accent,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    AsyncImage(
        model = ImageRequest.Builder(context)
            .data("file:///android_asset/icons/sparkle.svg")
            .decoderFactory(SvgDecoder.Factory())
            .build(),
        contentDescription = null,
        colorFilter = androidx.compose.ui.graphics.ColorFilter.tint(tint),
        contentScale = ContentScale.Fit,
        modifier = modifier.size(sizeDp.dp),
    )
}

private fun assetUri(kind: String, key: String): String =
    "file:///android_asset/icons/$kind/$key.svg"

/**
 * File-type icon from the bundled SVG set (assets/icons/file-types).
 * Port of apps/mobile/components/icons/file-icon.tsx (SvgXml → Coil SVG).
 *
 * @param filename resolved via extension / exact-name rules (getFileIconKey)
 * @param iconKey explicit registry key, bypassing filename resolution (e.g. code-fence languages)
 */
@Composable
fun FileIcon(
    filename: String = "",
    iconKey: String? = null,
    sizeDp: Int = 17,
    modifier: Modifier = Modifier,
) {
    val key = iconKey ?: getFileIconKey(filename)
    val context = LocalContext.current
    AsyncImage(
        model = ImageRequest.Builder(context)
            .data(assetUri("file-types", key))
            .decoderFactory(SvgDecoder.Factory())
            .build(),
        contentDescription = null,
        contentScale = ContentScale.Fit,
        modifier = modifier.size(sizeDp.dp),
    )
}

/** Code-fence language → file-type icon (mirrors getLanguageIconKey). */
@Composable
fun FileLanguageIcon(
    language: String,
    sizeDp: Int = 13,
    modifier: Modifier = Modifier,
) {
    FileIcon(iconKey = getLanguageIconKey(language), sizeDp = sizeDp, modifier = modifier)
}

/**
 * Provider logo (antigravity, openai/codex, opencode). Null box when unknown —
 * mirrors ProviderIcon returning null for unknown providers.
 */
@Composable
fun ProviderIcon(
    provider: String,
    sizeDp: Int = 14,
    modifier: Modifier = Modifier,
) {
    val key = getProviderIconKey(provider) ?: return
    val context = LocalContext.current
    AsyncImage(
        model = ImageRequest.Builder(context)
            .data(assetUri("providers", key))
            .decoderFactory(SvgDecoder.Factory())
            .build(),
        contentDescription = provider,
        contentScale = ContentScale.Fit,
        modifier = modifier.size(sizeDp.dp),
    )
}

/**
 * Site favicon from Google's favicon service, with a globe while it loads and
 * when the fetch fails (offline, or a host with no icon).
 */
@Composable
fun Favicon(
    host: String,
    sizeDp: Int = 14,
    modifier: Modifier = Modifier,
) {
    val fallback: @Composable () -> Unit = {
        androidx.compose.material3.Icon(
            io.github.lyxnx.compose.ui.tablericons.TablerIcons.Outline.Globe,
            contentDescription = null,
            tint = com.console.mobile.ui.theme.NewTheme.TextMuted,
            modifier = Modifier.size(sizeDp.dp),
        )
    }
    coil3.compose.SubcomposeAsyncImage(
        model = com.console.mobile.core.util.faviconUrl(host),
        contentDescription = null,
        contentScale = ContentScale.Fit,
        loading = { fallback() },
        error = { fallback() },
        modifier = modifier.size(sizeDp.dp),
    )
}
