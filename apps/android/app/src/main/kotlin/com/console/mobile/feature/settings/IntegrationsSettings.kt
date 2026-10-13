package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.SectionSkeleton
import com.console.mobile.ui.components.common.new.SkeletonLoader
import console.v1.IntegrationStatus

/**
 * Helper binaries the server shells out to (gh CLI today). Read-only: the
 * page reports live installed/missing status and the command to install what
 * is missing — the install itself happens on the box, outside this UI.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun IntegrationsSettings(onBack: () -> Unit) {
    val integrationsState by AppContainer.integrationsStateHolder.state.collectAsStateWithLifecycle()
    var refreshing by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { AppContainer.integrationsRepository.loadIntegrations() }

    val isLoading = integrationsState.loading && integrationsState.integrations.isEmpty()

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(
            title = "Integrations",
            onBack = onBack,
            actions = {
                IconButton(onClick = { AppContainer.integrationsRepository.loadIntegrations() }, modifier = Modifier.size(36.dp)) {
                    Icon(TablerIcons.Outline.Refresh, contentDescription = "Refresh", tint = NewTheme.TextPrimary, modifier = Modifier.size(16.dp))
                }
            },
        )
        SkeletonLoader(
            loading = isLoading,
            modifier = Modifier.fillMaxSize(),
            skeleton = {
                Column(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                    SectionSkeleton(rows = 2)
                }
            },
        ) {
            PullToRefreshBox(
                isRefreshing = refreshing,
                onRefresh = {
                    refreshing = true
                    AppContainer.integrationsRepository.loadIntegrations()
                    refreshing = false
                },
                modifier = Modifier.fillMaxSize(),
            ) {
                Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
                    Text("Helper tools the server uses on your behalf. Status is checked live on the server — install anything missing, then refresh.", color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 12.dp).padding(top = 8.dp))
                    integrationsState.error?.let { Banner(it, modifier = Modifier.padding(horizontal = 16.dp).padding(top = 12.dp)) }
                    if (integrationsState.integrations.isEmpty() && !isLoading) {
                        Note("No integrations reported.")
                    }
                    integrationsState.integrations.forEach { IntegrationCard(it) }
                }
            }
        }
    }
}

@Composable
private fun IntegrationCard(row: IntegrationStatus) {
    // `installed` has explicit presence on the wire: absent reads as not
    // installed, matching the GitHub `connected` idiom.
    val installed = row.installed == true
    Section(row.name) {
        Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
                Text(
                    if (installed) "Installed" else "Not installed",
                    color = if (installed) NewTheme.Success else NewTheme.TextMuted,
                    fontSize = 15.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
                Text(row.description, color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(top = 2.dp))
                // Version and path are optional on the wire: a binary that
                // resolves but whose probe fails still reads installed, just
                // without details.
                val details = listOfNotNull(
                    row.version?.takeIf { it.isNotBlank() }?.let { "v$it" },
                    row.binary_path?.takeIf { it.isNotBlank() },
                ).joinToString(" · ")
                if (installed && details.isNotEmpty()) {
                    Text(details, color = NewTheme.TextMuted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
                }
            }
        }
        if (!installed) {
            SectionDivider()
            Note("Install with: ${row.install_hint}")
            Text(row.install_url, color = NewTheme.TextMuted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp).padding(bottom = 16.dp))
        }
    }
}
