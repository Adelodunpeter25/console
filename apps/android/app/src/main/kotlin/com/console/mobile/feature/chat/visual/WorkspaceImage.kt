package com.console.mobile.feature.chat.visual

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import coil3.svg.SvgDecoder
import com.console.mobile.AppContainer
import com.console.mobile.core.util.chatImagePath
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * A workspace image for chat: `path` is resolved against the session cwd and
 * fetched through the authenticated fs raw endpoint, then decoded by Coil.
 * Bounded height, alt caption, and a quiet fallback when the fetch fails.
 */
@Composable
fun WorkspaceImage(path: String, alt: String?, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val absPath = chatImagePath(path) ?: path
    val bytes by produceState<ByteArray?>(initialValue = null, absPath) {
        value = withContext(Dispatchers.IO) {
            runCatching { AppContainer.fsRepository.readFileRaw(absPath) }.getOrNull()
        }
    }
    val caption = alt?.takeIf { it.isNotBlank() }
        ?: absPath.substringAfterLast('/')

    Column(modifier = modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        val data = bytes
        if (data != null) {
            AsyncImage(
                model = ImageRequest.Builder(context)
                    .data(data)
                    .decoderFactory(SvgDecoder.Factory())
                    .build(),
                contentDescription = caption,
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxWidth().heightIn(max = 360.dp),
            )
        } else {
            // Still loading or failed: keep the reference visible.
            Text(caption, color = NewTheme.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(vertical = 8.dp))
        }
        if (caption.isNotBlank()) {
            Text(caption, color = NewTheme.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
        }
    }
}
