package com.console.mobile.feature.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.ConsoleColors

@Composable
fun OnboardingPlaceholderScreen(onConnected: () -> Unit) {
    var url by remember { mutableStateOf("") }
    var name by remember { mutableStateOf("") }
    Column(modifier = Modifier.fillMaxSize().padding(32.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Text("Console", color = ConsoleColors.TextPrimary)
        Text("Enter your Console server URL to connect.", color = ConsoleColors.TextSecondary, modifier = Modifier.padding(bottom = 24.dp))
        OutlinedTextField(value = name, onValueChange = { name = it }, label = { Text("Name") }, modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp))
        OutlinedTextField(value = url, onValueChange = { url = it }, label = { Text("http://192.168.1.X:3000") }, modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp))
        Button(onClick = onConnected, enabled = url.isNotBlank(), modifier = Modifier.fillMaxWidth()) { Text("Connect (Phase 1 wires real Test Connection)") }
    }
}
