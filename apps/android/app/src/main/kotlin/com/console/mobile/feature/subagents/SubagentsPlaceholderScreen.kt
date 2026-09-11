package com.console.mobile.feature.subagents

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
import com.console.mobile.ui.navigation.PlaceholderScreen

@Composable
fun SubagentsPlaceholderScreen(onOpenDetails: (String) -> Unit) {
    Column(modifier = Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Text("Subagents — Phase 6")
        Button(onClick = { onOpenDetails("demo-id") }, modifier = Modifier.padding(top = 12.dp)) { Text("Open details") }
    }
}

@Composable
fun SubagentDetailsPlaceholderScreen() {
    PlaceholderScreen("Subagent Details — Phase 6")
}
