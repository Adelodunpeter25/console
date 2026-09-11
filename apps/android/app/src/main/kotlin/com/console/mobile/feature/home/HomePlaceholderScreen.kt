package com.console.mobile.feature.home

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.ConsoleColors

@Composable
fun HomePlaceholderScreen(
    onOpenSettings: () -> Unit,
    onOpenSubagents: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Home", color = ConsoleColors.TextPrimary)
        Button(onClick = onOpenSettings, modifier = Modifier.padding(top = 16.dp)) { Text("Settings") }
        Button(onClick = onOpenSubagents, modifier = Modifier.padding(top = 8.dp)) { Text("Subagents") }
    }
}
