package com.console.mobile.feature.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.AppContainer
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.core.util.urlHostPort
import com.console.mobile.data.model.deviceName
import com.console.mobile.data.model.splitHostAndCode
import com.console.mobile.ui.components.confirmAlert
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.ServerCard
import com.console.mobile.ui.theme.NewTheme
import androidx.compose.foundation.background

/**
 * Port of OnboardingScreen in apps/mobile/index.tsx.
 *
 * Host + pairing code, Connect (pair if a code was given, then persist via
 * EnvironmentsRepository) + Test Connection (GET {url}/api/projects with the
 * server's own credential — mirrors useServerConnection.testConnection).
 *
 * The two fields are what the server prints: `console pair` on the server
 * shows a host and a short code. A code means the server wants this phone
 * to pair, so the phone exchanges it for a credential before it starts
 * talking to the server.
 */
@Composable
fun OnboardingScreen(onConnected: () -> Unit) {
    val scope = rememberCoroutineScope()
    var host by remember { mutableStateOf("") }
    var code by remember { mutableStateOf("") }
    var saving by remember { mutableStateOf(false) }
    var testState by remember { mutableStateOf("idle") } // idle|testing|success|error

    Column(
        modifier = Modifier.fillMaxSize().background(NewTheme.Background).verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 48.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Console Mobile", color = NewTheme.TextPrimary, fontSize = 30.sp, fontWeight = FontWeight.Bold, textAlign = TextAlign.Center)
        Text(
            "Enter the host and pairing code from your Console server.",
            color = NewTheme.TextSecondary,
            fontSize = 15.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(top = 8.dp, bottom = 16.dp),
        )
        ServerCard(
            host = host,
            code = code,
            onHostChange = { text ->
                // A pasted pairing URL carries the code in its
                // fragment, so the field splits it into the two.
                val (address, pasted) = splitHostAndCode(text)
                host = address
                if (pasted.isNotBlank()) code = pasted
                if (testState == "success" || testState == "error") testState = "idle"
            },
            onCodeChange = { code = it; if (testState == "success" || testState == "error") testState = "idle" },
        )
        ActionButton(
            text = if (saving) "Connecting…" else "Connect",
            onClick = {
                val normalized = normalizeBackendUrl(host)
                if (normalized == null) {
                    confirmAlert("Invalid host", "The server host cannot be empty.")
                    return@ActionButton
                }
                saving = true
                scope.launch {
                    try {
                        // A code means the server wants this phone to pair:
                        // exchange it for a credential before the app starts
                        // talking to the new address.
                        var serverName = ""
                        if (code.isNotBlank()) {
                            val paired = AppContainer.authRepository.pairWithCode(normalized, code, deviceName())
                            serverName = paired.server?.name?.takeIf { it.isNotBlank() }.orEmpty()
                        }
                        val label = serverName.ifBlank { urlHostPort(normalized) }
                        withContext(Dispatchers.IO) {
                            AppContainer.environmentsRepository.activateUrl(label, normalized)
                        }
                        onConnected()
                    } catch (e: Exception) {
                        confirmAlert("Error", "Failed to connect: ${e.message ?: e}")
                    } finally {
                        saving = false
                    }
                }
            },
            kind = ActionButtonKind.Primary,
            enabled = !saving && host.isNotBlank(),
            loading = saving,
            modifier = Modifier.fillMaxWidth().padding(top = 24.dp),
        )
        ActionButton(
            text = when (testState) {
                "success" -> "✓ Connection successful"
                "error" -> "✕ Connection failed — try again"
                "testing" -> "Testing…"
                else -> "Test connection"
            },
            onClick = {
                if (host.isBlank()) return@ActionButton
                testState = "testing"
                scope.launch {
                    val ok = AppContainer.authRepository.probeServer(host)
                    testState = if (ok) "success" else "error"
                }
            },
            enabled = host.isNotBlank() && testState != "testing",
            loading = testState == "testing",
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
        )
        // The result is also coloured, so a quick glance tells success from failure.
        when (testState) {
            "success" -> Text("Reachable", color = NewTheme.Success, fontSize = 13.sp, modifier = Modifier.padding(top = 10.dp))
            "error" -> Text("Could not reach the server", color = NewTheme.Danger, fontSize = 13.sp, modifier = Modifier.padding(top = 10.dp))
        }
    }
}
