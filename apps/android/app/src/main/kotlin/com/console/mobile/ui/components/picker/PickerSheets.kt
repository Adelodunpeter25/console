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
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
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
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.filled.Star
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Star
import com.console.mobile.AppContainer
import com.console.mobile.core.util.formatContextWindow
import com.console.mobile.core.util.formatModelName
import com.console.mobile.data.model.ApprovalMode
import com.console.mobile.data.model.ApprovalModeOption
import console.v1.Model
import console.v1.ProjectInfo
import com.console.mobile.data.model.favoriteKey
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.launch
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.IconTabPill
import com.console.mobile.ui.components.common.new.OptionRow
import com.console.mobile.ui.components.common.new.SearchInput
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TabPill
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.theme.NewTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.lazy.itemsIndexed
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.SectionCard

/**
 * Selection sheets shared by the composer strip and settings. These are
 * generic "pick one of N" surfaces — no chat state, no session mutation — so
 * they live with the other shared UI rather than inside a feature.
 */

@Composable
fun ProjectPickerSheet(
    projects: List<ProjectInfo>,
    selectedId: String?,
    locked: Boolean,
    onDismiss: () -> Unit,
    onSelect: (ProjectInfo) -> Unit,
    onSelectNone: () -> Unit,
    onAddProject: () -> Unit,
) {
    BaseSheet(onDismiss = onDismiss, title = "Select Folder") {
        if (locked) {
            Text("Folder cannot be changed once a chat has started.", color = NewTheme.TextMuted, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 4.dp, vertical = 8.dp))
        } else {
            SectionCard {
                projects.forEach { p ->
                    OptionRow(title = p.name, subtitle = p.path, selected = p.id == selectedId, monoSubtitle = true) { onSelect(p) }
                    SectionDivider()
                }
                OptionRow(title = "No project", subtitle = "Work in a scratch folder", selected = selectedId == null) { onSelectNone() }
            }
            if (projects.isEmpty()) {
                ActionButton(
                    text = "Add new project",
                    onClick = onAddProject,
                    kind = ActionButtonKind.Primary,
                    modifier = Modifier.fillMaxWidth().padding(top = 20.dp),
                )
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
    // Load the models for whichever provider tab is showing. Without this the
    // provider tabs never fetch anything — only the favourites tab does, which
    // is why it appeared to be the only one with a working loading state.
    LaunchedEffect(activeProvider) {
        activeProvider?.let { AppContainer.providerRepository.loadModels(it) }
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
        FavoriteEntry(provider, model, entry?.display_name?.ifBlank { provider } ?: provider)
    }

    fun toggleFavorite(providerId: String, modelId: String) {
        val isFav = favoriteKey(providerId, modelId) in favorites
        scope.launch { AppContainer.providerRepository.setFavorite(providerId, modelId, !isFav) }
    }

    // The list scrolls itself (LazyColumn), so the sheet must not also scroll.
    BaseSheet(onDismiss = onDismiss, title = title, scrollable = false) {
        if (providerState.providers.isNotEmpty()) {
            LazyRow(modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                // Favourites tab: a star ahead of the provider tabs, matching the desktop's tab bar.
                item { IconTabPill(TablerIcons.Outline.Star, "Favorites", selected = showFavorites) { showFavorites = true } }
                items(providerState.providers) { p ->
                    TabPill(
                        label = p.display_name.ifBlank { p.name },
                        selected = !showFavorites && p.name == activeProvider,
                    ) {
                        showFavorites = false
                        activeProvider = p.name
                    }
                }
            }
        }
        SearchInput(
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
                if (isFav) TablerIcons.Filled.Star else TablerIcons.Outline.Star,
                contentDescription = if (isFav) "Remove from favorites" else "Add to favorites",
                tint = if (isFav) NewTheme.Warning else NewTheme.TextMuted,
                modifier = Modifier.size(22.dp).clickable { toggleFavorite(providerId, modelId) },
            )
        }

        if (showFavorites) {
            val visibleFavorites = if (q.isEmpty()) favoriteEntries else favoriteEntries.filter { it.model.id.lowercase().contains(q) }
            when {
                (providerState.loadingFavorites || anyModelLoading) && favoriteEntries.isEmpty() -> LoadingState()
                visibleFavorites.isEmpty() -> ModelPlaceholder(if (search.isNotEmpty()) "No matching favorites" else "No favorites yet — tap a star to pin a model")
                else -> ModelListCard(visibleFavorites, key = { favoriteKey(it.provider, it.model.id) }) { entry ->
                    OptionRow(
                        title = formatModelName(entry.model.id),
                        // Provider is not shown as a tab here, so it belongs on the row (desktop parity).
                        subtitle = "${entry.providerLabel} · ${formatContextWindow(entry.model.context_window)} context",
                        selected = entry.model.id == selectedModel && entry.provider == selectedProvider,
                        trailing = { starFor(entry.provider, entry.model.id) },
                    ) { onSelect(entry.model.id, entry.provider) }
                }
            }
        } else {
            val models: List<Model> = activeProvider?.let { providerState.modelsByProvider[it] } ?: emptyList()
            val filtered = if (q.isEmpty()) models else models.filter { it.id.lowercase().contains(q) }
            when {
                (activeLoading || bootstrapping) && models.isEmpty() -> LoadingState()
                filtered.isEmpty() -> ModelPlaceholder(if (search.isNotEmpty()) "No matching models found" else "No models available")
                else -> ModelListCard(filtered.take(100), key = { it.id }) { m ->
                    val providerId = activeProvider.orEmpty()
                    OptionRow(
                        title = formatModelName(m.id),
                        // The formatted name is already the row title, so the id under it
                        // repeated itself — the context window is the useful second line.
                        subtitle = "${formatContextWindow(m.context_window)} context",
                        selected = m.id == selectedModel,
                        trailing = { starFor(providerId, m.id) },
                    ) { onSelect(m.id, activeProvider) }
                }
            }
        }
    }
}

/**
 * The model rows in one rounded card. The list owns the remaining sheet height
 * and scrolls within it: a plain Column here grew past the sheet on long
 * provider lists (antigravity) with no way to reach the overflow items.
 */
@Composable
private fun <T> ColumnScope.ModelListCard(items: List<T>, key: (T) -> Any, row: @Composable (T) -> Unit) {
    LazyColumn(
        modifier = Modifier.fillMaxWidth().weight(1f, fill = false)
            .clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card),
    ) {
        itemsIndexed(items, key = { _, item -> key(item) }) { index, item ->
            row(item)
            if (index < items.lastIndex) SectionDivider()
        }
    }
}

@Composable
private fun ModelPlaceholder(message: String) {
    Box(modifier = Modifier.fillMaxWidth().padding(vertical = 32.dp), contentAlignment = Alignment.Center) {
        Text(message, color = NewTheme.TextSecondary, fontSize = 14.sp)
    }
}

@Composable
fun ApprovalModePickerSheet(current: String, onDismiss: () -> Unit, onSelect: (String) -> Unit) {
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { AppContainer.providerRepository.loadApprovalModes() }
    BaseSheet(onDismiss = onDismiss, title = "Approval Mode") {
        if (providerState.loadingApprovalModes && providerState.approvalModes.isEmpty()) {
            LoadingState()
        } else {
            val modes = providerState.approvalModes.ifEmpty {
                ApprovalMode.entries.map { ApprovalModeOption(it, it.value, "") }
            }
            SectionCard {
                modes.forEachIndexed { index, m ->
                    OptionRow(
                        title = m.label.ifBlank { m.value.value },
                        subtitle = m.description.takeIf { it.isNotBlank() },
                        selected = m.value.value == current,
                    ) { onSelect(m.value.value) }
                    if (index < modes.lastIndex) SectionDivider()
                }
            }
        }
    }
}

/**
 * Branch picker for a chat's project. "New worktree" is an action, not a
 * selection, so it sits in its own card above the branch list.
 *
 * [locked] covers a run in flight and a chat that already has messages: a
 * branch switch mid-stream would only land on the next turn, and a worktree
 * can only be attached to a message-less session.
 */
@Composable
fun BranchPickerSheet(
    branches: List<console.v1.GitBranchInfo>?,
    loading: Boolean,
    locked: Boolean,
    lockedReason: String?,
    worktreeBranch: String?,
    canStartWorktree: Boolean,
    /** Branch a switch is in flight to; rows are inert and this one shows progress. */
    switchingTo: String? = null,
    /** Why the last switch was refused, shown above the list. */
    error: String? = null,
    onDismiss: () -> Unit,
    onSelect: (String) -> Unit,
    onNewWorktree: () -> Unit,
) {
    // The branch list scrolls itself, so the sheet must not also scroll.
    BaseSheet(onDismiss = onDismiss, title = "Branch", scrollable = false) {
        if (locked && !lockedReason.isNullOrBlank()) {
            Text(lockedReason, color = NewTheme.TextMuted, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 4.dp, vertical = 4.dp))
        }
        if (!error.isNullOrBlank()) {
            Banner(error, modifier = Modifier.padding(vertical = 8.dp))
        }
        Column(modifier = Modifier.alpha(if (locked) 0.45f else 1f)) {
            if (worktreeBranch == null && canStartWorktree) {
                SectionCard(modifier = Modifier.padding(bottom = 12.dp)) {
                    OptionRow(title = "New worktree", subtitle = "Work in an isolated copy on its own branch") {
                        if (!locked) onNewWorktree()
                    }
                }
            }
            when {
                loading && branches == null -> LoadingState()
                branches.isNullOrEmpty() -> Text("No branches found", color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.fillMaxWidth().padding(vertical = 32.dp), textAlign = androidx.compose.ui.text.style.TextAlign.Center)
                else -> LazyColumn(
                    modifier = Modifier.fillMaxWidth().weight(1f, fill = false)
                        .clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card),
                ) {
                    itemsIndexed(branches, key = { _, b -> b.name }) { index, b ->
                        OptionRow(
                            title = b.name,
                            selected = b.current,
                            trailing = if (switchingTo == b.name) {
                                { CircularProgressIndicator(color = NewTheme.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(18.dp)) }
                            } else null,
                        ) { if (!locked && switchingTo == null) onSelect(b.name) }
                        if (index < branches.lastIndex) SectionDivider()
                    }
                }
            }
        }
    }
}
