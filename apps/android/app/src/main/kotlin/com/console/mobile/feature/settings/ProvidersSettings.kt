package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
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
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Circle
import io.github.lyxnx.compose.ui.tablericons.outline.Login
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.PillButton
import com.console.mobile.ui.components.PillButtonVariant
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.components.ProviderIcon
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.SectionSkeleton

/**
 * Port of screens/settings/account-settings.tsx. Named ProvidersSettings: this
 * screen is about AI provider credentials, one per row.
 * Provider list with login/re-login via OAuth Custom Tab (getLoginUrl → submitCallback).
 */
@Composable
fun ProvidersSettings(onBack: () -> Unit) {
    val context = LocalContext.current
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    var loggingIn by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) {
        AppContainer.providerRepository.loadProviders()
        AppContainer.authRepository.loadStatus()
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(title = "Account", onBack = onBack)
        Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
            Text("Sign in to AI providers to use their models in chat.", color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 12.dp).padding(top = 8.dp))
            if (providerState.loadingProviders && providerState.providers.isEmpty()) {
                SectionSkeleton(rows = 4)
            } else {
                val providers = providerState.providers.filter { it.auth_method != "none" }
                Section("Providers") {
                    if (providers.isEmpty()) Note("No providers available.")
                    providers.forEachIndexed { i, p ->
                        val status = authState.status?.get(p.name)
                        val loggedIn = status?.logged_in == true
                        val busy = loggingIn == p.name
                        Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                            ProviderIcon(provider = p.name, sizeDp = 24)
                            Column(modifier = Modifier.weight(1f).padding(start = 16.dp).padding(end = 12.dp)) {
                                Text(p.display_name.ifBlank { p.name }, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                Text(
                                    if (loggedIn) (status.email ?: "Connected") else "Not connected",
                                    color = if (loggedIn) NewTheme.Success else NewTheme.TextMuted,
                                    fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp),
                                )
                            }
                            val label = when {
                                busy -> "Wait"
                                loggedIn -> "Re-login"
                                p.auth_method == "device-code" -> "Pair"
                                else -> "Login"
                            }
                            PillButton(
                                text = label,
                                onClick = {
                                    loggingIn = p.name
                                    scope.launch {
                                        try {
                                            // LocalContext.current here is the hosting Activity,
                                            // which Custom Tabs requires — the app context can't
                                            // start activities without FLAG_ACTIVITY_NEW_TASK.
                                            OAuthLoginLauncher.login(context, AppContainer.authRepository, p.name)
                                        } catch (e: Exception) {
                                            confirmAlert("Login Failed", e.message ?: "Login failed.")
                                        } finally {
                                            loggingIn = null
                                        }
                                    }
                                },
                                enabled = loggingIn == null,
                                loading = busy,
                                icon = if (loggedIn) TablerIcons.Outline.Refresh else TablerIcons.Outline.Login,
                                variant = if (loggedIn) PillButtonVariant.Outline else PillButtonVariant.Filled,
                            )
                        }
                        if (i < providers.lastIndex) SectionDivider(startInset = 58.dp)
                    }
                }
            }
            val err = authState.error
            if (err != null) {
                Text(err, color = NewTheme.Danger, fontSize = 12.sp, modifier = Modifier.padding(horizontal = 4.dp, vertical = 16.dp))
            }
        }
    }
}
