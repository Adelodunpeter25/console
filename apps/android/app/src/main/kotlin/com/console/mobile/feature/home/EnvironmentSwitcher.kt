package com.console.mobile.feature.home

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
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
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.core.util.urlHost
import com.console.mobile.data.store.Environment
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit

/**
 * Port of components/environments/environment-switcher.tsx.
 * Server-icon trigger → bottom sheet with env list + probe dots, inline add form.
 * Probes are local UI state (GET {url}/api/projects, 6s) — mirrors probeEnvironment.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EnvironmentSwitcher(modifier: Modifier = Modifier) {
    val scope = rememberCoroutineScope()
    val envState by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    var sheetOpen by remember { mutableStateOf(false) }
    var creating by remember { mutableStateOf(false) }
    var probes by remember { mutableStateOf<Map<String, Boolean>>(emptyMap()) }

    fun probeAll(envs: List<Environment>) {
        scope.launch {
            val results = withContext(Dispatchers.IO) {
                envs.associate { env ->
                    env.id to try {
                        val client = OkHttpClient.Builder()
                            .connectTimeout(6, TimeUnit.SECONDS)
                            .readTimeout(6, TimeUnit.SECONDS)
                            .callTimeout(6, TimeUnit.SECONDS)
                            .build()
                        val req = Request.Builder().url("${env.url.trimEnd('/')}/api/projects").get().build()
                        client.newCall(req).execute().use { it.isSuccessful }
                    } catch (_: Exception) {
                        false
                    }
                }
            }
            probes = results
        }
    }

    IconButton(
        onClick = {
            creating = false
            sheetOpen = true
            probeAll(envState.environments)
        },
        modifier = modifier.size(40.dp),
    ) {
        Box(
            modifier = Modifier.size(40.dp).clip(CircleShape)
                .background(ConsoleColors.Card)
                .border(1.dp, ConsoleColors.Border, CircleShape),
            contentAlignment = Alignment.Center,
        ) {
            Icon(Icons.Filled.Dns, contentDescription = "Switch environment", tint = ConsoleColors.TextPrimary)
        }
    }

    if (sheetOpen) {
        ModalBottomSheet(
            onDismissRequest = { sheetOpen = false },
            sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
            containerColor = ConsoleColors.Background,
        ) {
            if (creating) {
                var newName by remember { mutableStateOf("") }
                var newUrl by remember { mutableStateOf("") }
                Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(bottom = 16.dp)) {
                        IconButton(onClick = { creating = false }, modifier = Modifier.size(32.dp)) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", tint = ConsoleColors.TextPrimary)
                        }
                        Text("Add environment", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 12.dp))
                    }
                    val fieldColors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = ConsoleColors.Card,
                        unfocusedContainerColor = ConsoleColors.Card,
                        focusedBorderColor = ConsoleColors.Border,
                        unfocusedBorderColor = ConsoleColors.Border,
                        focusedTextColor = ConsoleColors.TextPrimary,
                        unfocusedTextColor = ConsoleColors.TextPrimary,
                        cursorColor = ConsoleColors.TextPrimary,
                    )
                    OutlinedTextField(value = newName, onValueChange = { newName = it }, label = { Text("Name") }, singleLine = true, colors = fieldColors, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp))
                    OutlinedTextField(value = newUrl, onValueChange = { newUrl = it }, label = { Text("http://192.168.1.X:3000") }, singleLine = true, colors = fieldColors, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp))
                    TextButton(
                        onClick = {
                            val normalized = normalizeBackendUrl(newUrl) ?: return@TextButton
                            scope.launch {
                                withContext(Dispatchers.IO) {
                                    AppContainer.environmentsRepository.addEnvironment(newName.ifBlank { "Default" }, normalized)
                                }
                                creating = false
                                sheetOpen = false
                            }
                        },
                        enabled = newUrl.isNotBlank(),
                        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(999.dp)).background(if (newUrl.isNotBlank()) Color.White else Color.White.copy(alpha = 0.4f)).padding(vertical = 12.dp),
                    ) {
                        Text("Save", color = Color.Black, fontWeight = FontWeight.Bold)
                    }
                }
            } else {
                Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
                    Text("Environments", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(bottom = 12.dp))
                    LazyColumn(modifier = Modifier.fillMaxWidth()) {
                        items(envState.environments, key = { it.id }) { env ->
                            val isActive = env.id == envState.activeId
                            val probe = probes[env.id]
                            Row(
                                modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp)
                                    .clip(RoundedCornerShape(16.dp))
                                    .background(if (isActive) ConsoleColors.CardAlt else ConsoleColors.Card)
                                    .border(1.dp, ConsoleColors.Border, RoundedCornerShape(16.dp))
                                    .clickable {
                                        if (env.id != envState.activeId) {
                                            scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.activateEnvironment(env.id) } }
                                        }
                                        sheetOpen = false
                                    }
                                    .padding(horizontal = 16.dp, vertical = 14.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Box(
                                    modifier = Modifier.size(8.dp).clip(CircleShape).background(
                                        when (probe) {
                                            null -> Color(0xFF52525B)
                                            true -> Color(0xFF34D399)
                                            false -> Color(0xFFF87171)
                                        },
                                    ),
                                )
                                Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
                                    Text(env.name, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                                    Text(urlHost(env.url), color = ConsoleColors.TextSecondary, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
                                }
                                if (isActive) {
                                    Icon(Icons.Filled.Check, contentDescription = "Active", tint = Color(0xFF34D399))
                                }
                            }
                        }
                        item {
                            Row(
                                modifier = Modifier.fillMaxWidth()
                                    .clip(RoundedCornerShape(16.dp))
                                    .border(1.dp, ConsoleColors.Border, RoundedCornerShape(16.dp))
                                    .clickable { creating = true }
                                    .padding(vertical = 14.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        Icon(Icons.Filled.Add, contentDescription = null, tint = ConsoleColors.TextSecondary)
                                        Text("Add environment", color = ConsoleColors.TextSecondary, fontSize = 14.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 8.dp))
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
