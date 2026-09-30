package com.console.mobile.ui.components.picker

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.StarBorder
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.util.formatContextWindow
import com.console.mobile.core.util.formatModelName
import com.console.mobile.data.model.ApprovalMode
import com.console.mobile.data.model.ApprovalModeOption
import com.console.mobile.data.model.Model
import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.data.model.favoriteKey
import com.console.mobile.ui.components.ConsoleSearchField
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.launch

/**
 * Selection sheets shared by the composer strip and settings. These are
 * generic "pick one of N" surfaces — no chat state, no session mutation — so
 * they live with the other shared UI rather than inside a feature.
 */

/** Sheet title row. */
@Composable
fun PickerSheetTitle(title: String) {
    Text(title, color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(bottom = 12.dp))
}

/** One selectable row: title, optional subtitle, and a check on the selection.
 * [trailing] renders beside the check — the model picker's star. */
@Composable
fun PickerRow(
    title: String,
    subtitle: String? = null,
    selected: Boolean = false,
    monoSubtitle: Boolean = false,
    trailing: (@Composable () -> Unit)? = null,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(if (selected) ConsoleColors.CardAlt else Color.Transparent)
            .border(1.dp, if (selected) ConsoleColors.Border else Color.Transparent, RoundedCornerShape(12.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (!subtitle.isNullOrBlank()) {
                Text(
                    subtitle, color = ConsoleColors.TextSecondary, fontSize = if (monoSubtitle) 11.sp else 12.sp,
                    maxLines = 1, overflow = TextOverflow.Ellipsis,
                    fontFamily = if (monoSubtitle) ConsoleMonoFamily else null,
                    modifier = Modifier.padding(top = 2.dp),
                )
            }
        }
        if (trailing != null) {
            trailing()
            Spacer(modifier = Modifier.size(8.dp))
        }
        if (selected) Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF34D399), modifier = Modifier.size(16.dp))
    }
}

/** Centered placeholder for a sheet's loading / empty / no-match state. */
@Composable
fun PickerPlaceholder(message: String? = null, spinner: Boolean = false) {
    Box(modifier = Modifier.fillMaxWidth().padding(vertical = 32.dp), contentAlignment = Alignment.Center) {
        if (spinner) {
            CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
        } else if (!message.isNullOrBlank()) {
            Text(message, color = ConsoleColors.TextSecondary, fontSize = 12.sp)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProjectPickerSheet(projects: List<ProjectInfo>, selectedId: String?, locked: Boolean, onDismiss: () -> Unit, onSelect: (ProjectInfo) -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            PickerSheetTitle("Select Folder")
            if (locked) {
                Text("Folder cannot be changed once a chat has started.", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(bottom = 12.dp))
            } else {
                projects.forEach { p ->
                    PickerRow(title = p.name, subtitle = p.path, selected = p.id == selectedId, monoSubtitle = true) { onSelect(p) }
                }
            }
        }
    }
}

/** A starred model, resolved far enough to render a row in the favourites tab. */
private data class FavoriteEntry(
    val provider: String,
    val model: Model,
    val providerLabel: String,
)

/** Provider → model browser, with the desktop's favourites tab and per-row star. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ModelPickerSheet(
    title: String = "Select Model",
    selectedModel: String?,
    selectedProvider: String?,
    onDismiss: () -> Unit,
    onSelect: (String, String?) -> Unit,
) {
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var search by remember { mutableStateOf("") }
    var showFavorites by remember { mutableStateOf(false) }
    var activeProvider by remember(providerState.providers) {
        mutableStateOf(selectedProvider ?: providerState.providers.firstOrNull()?.name)
    }
    LaunchedEffect(Unit) {
        AppContainer.providerRepository.loadProviders()
        AppContainer.providerRepository.loadFavorites()
    }
    // Favourite rows can live under any provider, so load them all before the
    // favourites tab can render an empty list it does not deserve.
    LaunchedEffect(showFavorites) {
        if (showFavorites) providerState.providers.forEach { AppContainer.providerRepository.loadModels(it.name) }
    }

    val modelsByProvider = providerState.modelsByProvider
    val favorites = providerState.favorites
    val favoriteEntries: List<FavoriteEntry> = favorites.mapNotNull { key ->
        val sep = key.indexOf(':')
        if (sep <= 0) return@mapNotNull null
        val provider = key.substring(0, sep)
        val modelId = key.substring(sep + 1)
        val model = modelsByProvider[provider]?.firstOrNull { it.id == modelId } ?: return@mapNotNull null
        val entry = providerState.providers.firstOrNull { it.name == provider }
        FavoriteEntry(provider, model, entry?.displayName?.ifBlank { provider } ?: provider)
    }

    fun toggleFavorite(providerId: String, modelId: String) {
        val isFav = favoriteKey(providerId, modelId) in favorites
        scope.launch { AppContainer.providerRepository.setFavorite(providerId, modelId, !isFav) }
    }

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            PickerSheetTitle(title)
            if (providerState.providers.isNotEmpty()) {
                LazyRow(modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp)) {
                    item {
                        // Favourites tab: a star button ahead of the provider
                        // chips, matching the desktop's tab bar.
                        Box(
                            modifier = Modifier.padding(end = 8.dp).size(34.dp).clip(RoundedCornerShape(8.dp))
                                .background(if (showFavorites) Color.White.copy(alpha = 0.12f) else ConsoleColors.CardAlt)
                                .clickable { showFavorites = true },
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(
                                Icons.Filled.Star,
                                contentDescription = "Favorites",
                                tint = if (showFavorites) ConsoleColors.TextPrimary else ConsoleColors.TextSecondary,
                                modifier = Modifier.size(16.dp),
                            )
                        }
                    }
                    items(providerState.providers) { p ->
                        val sel = !showFavorites && p.name == activeProvider
                        Box(
                            modifier = Modifier.padding(end = 8.dp).clip(RoundedCornerShape(8.dp))
                                .background(if (sel) Color.White.copy(alpha = 0.12f) else ConsoleColors.CardAlt)
                                .clickable {
                                    showFavorites = false
                                    activeProvider = p.name
                                }
                                .padding(horizontal = 10.dp, vertical = 6.dp),
                        ) {
                            Text(p.displayName.ifBlank { p.name }, color = if (sel) ConsoleColors.TextPrimary else ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium)
                        }
                    }
                }
            }
            ConsoleSearchField(
                value = search,
                onValueChange = { search = it },
                placeholder = "Search models…",
                modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp),
            )
            val q = search.trim().lowercase()
            val activeLoading = activeProvider?.let { providerState.loadingModels[it] } == true || providerState.loadingProviders
            // The favourites tab resolves entries against every provider's model
            // list, so it has to wait on all of them — not just the favourites
            // request, which finishes long before the models arrive.
            val anyModelLoading = providerState.loadingProviders ||
                providerState.providers.any { providerState.loadingModels[it.name] == true }
            // On the first frame the provider list is still in flight, so no
            // per-provider flag is set yet. Without this the sheet flashes
            // "No models available" before the spinner ever appears.
            val bootstrapping = providerState.providers.isEmpty() && providerState.error == null

            // Star button for a row. Filled once starred, outline otherwise.
            @Composable
            fun starFor(providerId: String, modelId: String) {
                val isFav = favoriteKey(providerId, modelId) in favorites
                Icon(
                    if (isFav) Icons.Filled.Star else Icons.Outlined.StarBorder,
                    contentDescription = if (isFav) "Remove from favorites" else "Add to favorites",
                    tint = if (isFav) Color(0xFFFACC15) else ConsoleColors.TextMuted,
                    modifier = Modifier.size(18.dp).clickable { toggleFavorite(providerId, modelId) },
                )
            }

            if (showFavorites) {
                val visibleFavorites = if (q.isEmpty()) favoriteEntries else favoriteEntries.filter { it.model.id.lowercase().contains(q) }
                when {
                    (providerState.loadingFavorites || anyModelLoading) && favoriteEntries.isEmpty() -> PickerPlaceholder(spinner = true)
                    visibleFavorites.isEmpty() -> PickerPlaceholder(if (search.isNotEmpty()) "No matching favorites" else "No favorites yet — tap a star to pin a model")
                    else -> LazyColumn(modifier = Modifier.weight(1f, fill = false)) {
                        items(visibleFavorites, key = { favoriteKey(it.provider, it.model.id) }) { entry ->
                            PickerRow(
                                title = formatModelName(entry.model.id),
                                // Provider is not shown as a tab here, so it
                                // belongs on the row (desktop parity).
                                subtitle = "${entry.providerLabel} · ${formatContextWindow(entry.model.contextWindow)} context",
                                selected = entry.model.id == selectedModel && entry.provider == selectedProvider,
                                trailing = { starFor(entry.provider, entry.model.id) },
                            ) { onSelect(entry.model.id, entry.provider) }
                        }
                    }
                }
            } else {
                val models: List<Model> = activeProvider?.let { providerState.modelsByProvider[it] } ?: emptyList()
                val filtered = if (q.isEmpty()) models else models.filter { it.id.lowercase().contains(q) }
                when {
                    (activeLoading || bootstrapping) && models.isEmpty() -> PickerPlaceholder(spinner = true)
                    filtered.isEmpty() -> PickerPlaceholder(if (search.isNotEmpty()) "No matching models found" else "No models available")
                    else ->
                        // The list must own the remaining sheet height and scroll within it.
                        // A plain Column here grew past the sheet on long provider lists
                        // (antigravity) with no way to reach the overflow items.
                        LazyColumn(modifier = Modifier.weight(1f, fill = false)) {
                            items(filtered.take(100), key = { it.id }) { m ->
                                val providerId = activeProvider.orEmpty()
                                PickerRow(
                                    title = formatModelName(m.id),
                                    // The formatted name is already the row title, so
                                    // the id under it repeated itself — the context
                                    // window is the useful second line (desktop parity).
                                    subtitle = "${formatContextWindow(m.contextWindow)} context",
                                    selected = m.id == selectedModel,
                                    trailing = { starFor(providerId, m.id) },
                                ) { onSelect(m.id, activeProvider) }
                            }
                        }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ApprovalModePickerSheet(current: String, onDismiss: () -> Unit, onSelect: (String) -> Unit) {
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { AppContainer.providerRepository.loadApprovalModes() }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            PickerSheetTitle("Approval Mode")
            if (providerState.loadingApprovalModes && providerState.approvalModes.isEmpty()) {
                PickerPlaceholder(spinner = true)
            } else {
                val modes = providerState.approvalModes.ifEmpty {
                    ApprovalMode.entries.map { ApprovalModeOption(it, it.value, "") }
                }
                modes.forEach { m ->
                    PickerRow(
                        title = m.label.ifBlank { m.value.value },
                        subtitle = m.description.takeIf { it.isNotBlank() },
                        selected = m.value.value == current,
                    ) { onSelect(m.value.value) }
                }
            }
        }
    }
}
