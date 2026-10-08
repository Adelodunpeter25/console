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
import com.console.mobile.ui.components.confirmAlert
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.TextInput
import com.console.mobile.ui.theme.NewTheme
import androidx.compose.foundation.background
import androidx.compose.ui.text.input.KeyboardType

/**
 * Port of OnboardingScreen in apps/mobile/index.tsx.
 * Name + backend URL, Connect (persist via EnvironmentsRepository) + Test Connection
 * (GET {url}/api/projects, 6s timeout — mirrors useServerConnection.testConnection).
 */
@Composable
fun OnboardingScreen(onConnected: () -> Unit) {
    val scope = rememberCoroutineScope()
    var name by remember { mutableStateOf("") }
    var url by remember { mutableStateOf("") }
    var saving by remember { mutableStateOf(false) }
    var testState by remember { mutableStateOf("idle") } // idle|testing|success|error

    Column(
        modifier = Modifier.fillMaxSize().background(NewTheme.Background).verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 48.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Console Mobile", color = NewTheme.TextPrimary, fontSize = 30.sp, fontWeight = FontWeight.Bold, textAlign = TextAlign.Center)
        Text(
            "Enter your Console server URL to connect.",
            color = NewTheme.TextSecondary,
            fontSize = 15.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(top = 8.dp, bottom = 16.dp),
        )
        TextInput("Name", name, { name = it }, "My server")
        TextInput(
            label = "Server URL",
            value = url,
            onValueChange = { url = it; if (testState == "success" || testState == "error") testState = "idle" },
            placeholder = "http://192.168.1.X:3000",
            keyboardType = KeyboardType.Uri,
        )
        ActionButton(
            text = "Connect",
            onClick = {
                val normalized = normalizeBackendUrl(url)
                if (normalized == null) {
                    confirmAlert("Invalid URL", "Backend server endpoint cannot be empty.")
                    return@ActionButton
                }
                saving = true
                scope.launch {
                    try {
                        withContext(Dispatchers.IO) {
                            AppContainer.environmentsRepository.addEnvironment(name.ifBlank { "Default" }, normalized)
                        }
                        onConnected()
                    } catch (e: Exception) {
                        confirmAlert("Error", "Failed to save endpoint URL: ${e.message ?: e}")
                    } finally {
                        saving = false
                    }
                }
            },
            kind = ActionButtonKind.Primary,
            enabled = !saving && url.isNotBlank(),
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
                val normalized = normalizeBackendUrl(url) ?: return@ActionButton
                testState = "testing"
                scope.launch {
                    val ok = withContext(Dispatchers.IO) { probeBackend(normalized) }
                    testState = if (ok) "success" else "error"
                }
            },
            enabled = url.isNotBlank() && testState != "testing",
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

private fun probeBackend(baseUrl: String): Boolean {
    return try {
        val client = OkHttpClient.Builder()
            .connectTimeout(6, TimeUnit.SECONDS)
            .readTimeout(6, TimeUnit.SECONDS)
            .callTimeout(6, TimeUnit.SECONDS)
            .build()
        val req = Request.Builder().url("${baseUrl.trimEnd('/')}/api/projects").get().build()
        client.newCall(req).execute().use { it.isSuccessful }
    } catch (_: Exception) {
        false
    }
}
