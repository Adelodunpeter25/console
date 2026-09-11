package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Login
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Port of screens/settings/account-settings.tsx.
 * Provider list with login/re-login via OAuth Custom Tab (getLoginUrl → submitCallback).
 */
@Composable
fun AccountSettings(onBack: () -> Unit) {
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    var loggingIn by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) {
        AppContainer.providerRepository.loadProviders()
        AppContainer.authRepository.loadStatus()
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(title = "Account", onBack = onBack)
        Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
            Text("Sign in to AI providers to use their models in chat.", color = ConsoleColors.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 4.dp).padding(top = 8.dp, bottom = 16.dp))
            if (providerState.loadingProviders && providerState.providers.isEmpty()) {
                Box(modifier = Modifier.fillMaxWidth().padding(vertical = 32.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                }
            } else {
                val cardShape = RoundedCornerShape(16.dp)
                Column(modifier = Modifier.fillMaxWidth().clip(cardShape).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, cardShape).padding(horizontal = 20.dp, vertical = 8.dp)) {
                    val providers = providerState.providers.filter { it.authMethod != "none" }
                    if (providers.isEmpty()) {
                        Text("No providers available.", color = ConsoleColors.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(vertical = 16.dp))
                    }
                    providers.forEachIndexed { i, p ->
                        val status = authState.status?.get(p.name)
                        val loggedIn = status?.loggedIn == true
                        val busy = loggingIn == p.name
                        Column(modifier = Modifier.fillMaxWidth()) {
                            Row(modifier = Modifier.fillMaxWidth().padding(vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                                com.console.mobile.ui.components.ProviderIcon(provider = p.name, sizeDp = 18)
                                Column(modifier = Modifier.weight(1f).padding(start = 10.dp).padding(end = 12.dp)) {
                                    Text(p.displayName.ifBlank { p.name }, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                                    Text(if (loggedIn) (status?.email ?: "Connected") else "Not connected", color = ConsoleColors.TextSecondary, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
                                }
                                val label = when {
                                    busy -> "Wait"
                                    loggedIn -> "Re-login"
                                    p.authMethod == "device-code" -> "Pair"
                                    else -> "Login"
                                }
                                TextButton(
                                    onClick = {
                                        loggingIn = p.name
                                        scope.launch {
                                            try {
                                                OAuthLoginLauncher.login(AppContainer.appContext, AppContainer.authRepository, p.name)
                                            } catch (e: Exception) {
                                                confirmAlert("Login Failed", e.message ?: "Login failed.")
                                            } finally {
                                                loggingIn = null
                                            }
                                        }
                                    },
                                    enabled = loggingIn == null,
                                    modifier = Modifier.clip(RoundedCornerShape(999.dp)).then(if (loggedIn) Modifier.border(1.dp, ConsoleColors.Border, RoundedCornerShape(999.dp)) else Modifier.background(Color.White)).padding(horizontal = 14.dp, vertical = 8.dp),
                                ) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        if (busy) CircularProgressIndicator(color = if (loggedIn) Color.White else Color.Black, strokeWidth = 2.dp, modifier = Modifier.size(13.dp))
                                        else if (loggedIn) Icon(Icons.Filled.Refresh, contentDescription = null, tint = Color.White, modifier = Modifier.size(13.dp))
                                        else Icon(Icons.Filled.Login, contentDescription = null, tint = Color.Black, modifier = Modifier.size(13.dp))
                                        Text(label, color = if (loggedIn) Color.White else Color.Black, fontSize = 12.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(start = 6.dp))
                                    }
                                }
                            }
                            if (i < providers.lastIndex) {
                                Box(modifier = Modifier.fillMaxWidth().height(1.dp).background(ConsoleColors.BorderSubtle))
                            }
                        }
                    }
                }
            }
            val err = authState.error
            if (err != null) {
                Text(err, color = ConsoleColors.Destructive, fontSize = 12.sp, modifier = Modifier.padding(horizontal = 4.dp, vertical = 16.dp))
            }
        }
    }
}
