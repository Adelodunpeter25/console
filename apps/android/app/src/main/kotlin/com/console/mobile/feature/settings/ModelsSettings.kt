package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
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
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.X
import com.console.mobile.AppContainer
import com.console.mobile.core.util.formatModelName
import com.console.mobile.data.model.MODEL_ROLES
import com.console.mobile.data.model.ROLE_SMOL
import com.console.mobile.data.model.ROLE_VISION
import com.console.mobile.ui.components.picker.ModelPickerSheet
import com.console.mobile.ui.components.ProviderIcon
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.launch

/** Role -> (label, description), mirroring the desktop Models page. */
private data class RoleSpec(val role: String, val label: String, val description: String)

private val ROLE_SPECS = listOf(
    RoleSpec(ROLE_VISION, "Vision", "Image and screenshot inspection model"),
    RoleSpec(ROLE_SMOL, "Smol", "Fast summaries and session titles model"),
)

/**
 * Port of settings/models_page.rs — model roles (vision / smol) resolved
 * against the provider catalog, saved through PATCH /api/settings.
 * Unset roles fall back to the session's chat model.
 */
@Composable
fun ModelsSettings(onBack: () -> Unit) {
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    // Pending edits, keyed by role. Null means "unset" — distinct from absent
    // (not yet loaded), so a load never clobbers a choice made before it lands.
    var pending by remember { mutableStateOf<Map<String, String?>?>(null) }
    var sheetRole by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) { AppContainer.providerRepository.loadSettings() }

    val loaded = providerState.modelRoles
    val drafts = pending ?: loaded
    val dirty = pending != null && MODEL_ROLES.any { role -> drafts[role] != loaded[role] }

    fun refFor(role: String): String? = drafts[role]
    fun setRef(role: String, ref: String?) {
        val next = drafts.toMutableMap()
        if (ref == null) next.remove(role) else next[role] = ref
        pending = next
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(title = "Models", onBack = onBack)

        Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
            Text("Model roles", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 4.dp, vertical = 8.dp))
            Text(
                "Choose the model used for each harness role. Unset roles use the chat model.",
                color = ConsoleColors.TextSecondary, fontSize = 12.sp,
                modifier = Modifier.padding(horizontal = 4.dp).padding(bottom = 16.dp),
            )

            if (providerState.loadingRoles && loaded.isEmpty()) {
                Box(modifier = Modifier.fillMaxWidth().padding(vertical = 40.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(20.dp))
                }
                return@Column
            }

            val shape = RoundedCornerShape(16.dp)
            Column(
                modifier = Modifier.fillMaxWidth().clip(shape)
                    .background(ConsoleColors.Surface)
                    .border(1.dp, ConsoleColors.Border, shape)
                    .padding(16.dp),
            ) {
                ROLE_SPECS.forEach { spec ->
                    RolePickerRow(
                        spec = spec,
                        ref = refFor(spec.role),
                        onOpen = { sheetRole = spec.role },
                        onClear = { setRef(spec.role, null) },
                    )
                }
                SaveButton(
                    enabled = dirty,
                    saving = providerState.savingRoles,
                    onClick = {
                        scope.launch {
                            val saved = AppContainer.providerRepository.saveModelRoles(
                                MODEL_ROLES.associateWith { drafts[it] },
                            )
                            // Only drop the pending overlay once the server has
                            // accepted it; a failed save keeps the user's edits.
                            if (saved) pending = null
                        }
                    },
                )
                if (providerState.savingRoles) {
                    Text("Saving…", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 8.dp))
                }
                providerState.rolesError?.let { error ->
                    Text(error, color = ConsoleColors.Destructive, fontSize = 12.sp, modifier = Modifier.padding(top = 8.dp))
                }
            }
        }
    }

    val activeSpec = ROLE_SPECS.firstOrNull { it.role == sheetRole }
    if (activeSpec != null) {
        val ref = drafts[activeSpec.role]
        val (refProvider, refModel) = splitRef(ref)
        ModelPickerSheet(
            title = "Select ${activeSpec.label} Model",
            selectedModel = refModel,
            selectedProvider = refProvider,
            onDismiss = { sheetRole = null },
            onSelect = { modelId, providerId ->
                sheetRole = null
                // The server stores "provider/model"; fall back to the loaded
                // provider when the sheet reports a bare model id.
                val provider = providerId ?: refProvider
                setRef(activeSpec.role, if (provider != null) "$provider/$modelId" else modelId)
            },
        )
    }
}

@Composable
private fun RolePickerRow(
    spec: RoleSpec,
    ref: String?,
    onOpen: () -> Unit,
    onClear: () -> Unit,
) {
    val (provider, modelId) = splitRef(ref)
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(spec.label, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
            Text(spec.description, color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 2.dp))
            Row(
                modifier = Modifier.padding(top = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (modelId != null) {
                    ProviderIcon(provider = provider ?: "", sizeDp = 14)
                    Text(
                        formatModelName(modelId),
                        color = ConsoleColors.TextPrimary, fontSize = 12.sp, maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(start = 6.dp),
                    )
                } else {
                    Text("Uses chat model", color = ConsoleColors.TextMuted, fontSize = 12.sp, fontStyle = androidx.compose.ui.text.font.FontStyle.Italic)
                }
            }
        }
        if (ref != null) {
            IconButton(onClick = onClear, modifier = Modifier.size(32.dp)) {
                Icon(TablerIcons.Outline.X, contentDescription = "Clear ${spec.label}", tint = ConsoleColors.TextMuted, modifier = Modifier.size(16.dp))
            }
        }
    }
}

@Composable
private fun SaveButton(enabled: Boolean, saving: Boolean, onClick: () -> Unit) {
    val shape = RoundedCornerShape(12.dp)
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 8.dp)
            .clip(shape)
            .background(if (enabled && !saving) ConsoleColors.Primary else Color.White.copy(alpha = 0.08f))
            .clickable(enabled = enabled && !saving, onClick = onClick)
            .padding(vertical = 12.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            "Save model roles",
            color = if (enabled && !saving) ConsoleColors.TextDark else ConsoleColors.TextMuted,
            fontSize = 14.sp, fontWeight = FontWeight.SemiBold,
        )
    }
}

/** "provider/model" -> (provider, modelId); a bare id yields a null provider. */
private fun splitRef(ref: String?): Pair<String?, String?> {
    if (ref.isNullOrBlank()) return null to null
    val i = ref.indexOf('/')
    if (i <= 0) return null to ref
    return ref.substring(0, i) to ref.substring(i + 1)
}
