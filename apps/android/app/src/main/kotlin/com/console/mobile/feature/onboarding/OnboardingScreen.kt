package com.console.mobile.feature.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.AppContainer
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit

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

    val fieldColors = OutlinedTextFieldDefaults.colors(
        focusedContainerColor = ConsoleColors.Card,
        unfocusedContainerColor = ConsoleColors.Card,
        focusedBorderColor = ConsoleColors.Border,
        unfocusedBorderColor = ConsoleColors.Border,
        focusedTextColor = ConsoleColors.TextPrimary,
        unfocusedTextColor = ConsoleColors.TextPrimary,
        cursorColor = ConsoleColors.TextPrimary,
        focusedPlaceholderColor = ConsoleColors.TextMuted,
        unfocusedPlaceholderColor = ConsoleColors.TextMuted,
        focusedLabelColor = ConsoleColors.TextSecondary,
        unfocusedLabelColor = ConsoleColors.TextSecondary,
    )

    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 32.dp, vertical = 48.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Console Mobile", color = ConsoleColors.TextPrimary, fontSize = 30.sp, fontWeight = FontWeight.Bold, textAlign = TextAlign.Center)
        Text(
            "Enter your Console server URL to connect.",
            color = ConsoleColors.TextSecondary,
            fontSize = 14.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(top = 8.dp, bottom = 40.dp),
        )
        Text("Name", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp))
        OutlinedTextField(
            value = name,
            onValueChange = { name = it },
            placeholder = { Text("My server") },
            singleLine = true,
            colors = fieldColors,
            shape = RoundedCornerShape(12.dp),
            modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp),
        )
        OutlinedTextField(
            value = url,
            onValueChange = { url = it; if (testState == "success" || testState == "error") testState = "idle" },
            placeholder = { Text("http://192.168.1.X:3000") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            colors = fieldColors,
            shape = RoundedCornerShape(12.dp),
            modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp),
        )
        Button(
            onClick = {
                val normalized = normalizeBackendUrl(url)
                if (normalized == null) {
                    confirmAlert("Invalid URL", "Backend server endpoint cannot be empty.")
                    return@Button
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
            enabled = !saving && url.isNotBlank(),
            shape = RoundedCornerShape(999.dp),
            colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black, disabledContainerColor = Color.White.copy(alpha = 0.5f)),
            modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp),
        ) {
            if (saving) CircularProgressIndicator(color = Color.Black, strokeWidth = 2.dp, modifier = Modifier.padding(4.dp))
            else Text("Connect", fontWeight = FontWeight.Bold, fontSize = 14.sp)
        }
        OutlinedButton(
            onClick = {
                val normalized = normalizeBackendUrl(url) ?: return@OutlinedButton
                testState = "testing"
                scope.launch {
                    val ok = withContext(Dispatchers.IO) { probeBackend(normalized) }
                    testState = if (ok) "success" else "error"
                }
            },
            enabled = url.isNotBlank() && testState != "testing",
            shape = RoundedCornerShape(999.dp),
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (testState == "testing") CircularProgressIndicator(color = ConsoleColors.TextSecondary, strokeWidth = 2.dp, modifier = Modifier.padding(4.dp))
            else Text(
                when (testState) {
                    "success" -> "✓ Connection successful"
                    "error" -> "✕ Connection failed — try again"
                    else -> "Test Connection"
                },
                color = ConsoleColors.TextSecondary,
                fontWeight = FontWeight.SemiBold,
                fontSize = 14.sp,
            )
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
