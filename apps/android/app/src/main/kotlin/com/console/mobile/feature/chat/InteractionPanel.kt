package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.HelpOutline
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.PendingPermission
import com.console.mobile.core.chat.PendingQuestion
import com.console.mobile.data.model.AskQuestionRequest
import com.console.mobile.data.model.PermissionRequest
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/**
 * Port of components/chat/interactions/interaction-panel.tsx + question-panel.tsx.
 * Permission allow/deny card; question wizard (single/multi select + custom text).
 */
@Composable
fun InteractionPanel(sessionId: String, permissions: List<PendingPermission>, questions: List<PendingQuestion>) {
    if (permissions.isEmpty() && questions.isEmpty()) return
    if (permissions.isNotEmpty()) {
        val first = permissions.first()
        PermissionPanel(request = first.request, sessionId = sessionId)
        return
    }
    QuestionWizard(questions = questions.map { it.request }, sessionId = sessionId)
}

@Composable
private fun PermissionPanel(request: PermissionRequest, sessionId: String) {
    val scope = rememberCoroutineScope()
    var busy by remember(request.requestId) { mutableStateOf<String?>(null) }
    val argsString = request.args?.toString() ?: ""
    val shape = RoundedCornerShape(16.dp)
    Column(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 8.dp).clip(shape)
            .background(Color(0xFFF59E0B).copy(alpha = 0.1f))
            .border(1.dp, Color(0xFFF59E0B).copy(alpha = 0.3f), shape)
            .padding(16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Filled.Shield, contentDescription = null, tint = Color(0xFFF59E0B), modifier = Modifier.size(18.dp))
            Text(
                buildString {
                    append(if (request.requiresUpgrade) "Upgrade permission required: " else "Permission required: ")
                    append(request.toolName)
                },
                color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Medium,
                modifier = Modifier.weight(1f).padding(start = 10.dp),
            )
        }
        if (!request.reason.isNullOrBlank()) {
            Text(request.reason, color = ConsoleColors.TextSecondary, fontSize = 12.sp, lineHeight = 20.sp, modifier = Modifier.padding(start = 28.dp, top = 8.dp))
        }
        if (argsString.isNotEmpty()) {
            Box(modifier = Modifier.fillMaxWidth().padding(start = 28.dp, top = 8.dp).clip(RoundedCornerShape(12.dp)).background(Color.Black.copy(alpha = 0.4f)).padding(10.dp)) {
                Text(argsString.take(1200), color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontFamily = com.console.mobile.ui.theme.ConsoleMonoFamily, lineHeight = 17.sp, modifier = Modifier.verticalScroll(rememberScrollState()))
            }
        }
        Row(modifier = Modifier.padding(start = 28.dp, top = 12.dp)) {
            val allowLabel = if (request.requiresUpgrade) "Allow once" else "Allow"
            TextButton(
                onClick = {
                    busy = "allow"
                    scope.launch {
                        AppContainer.chatRepository.approvePermission(sessionId, request.requestId, true)
                        busy = null
                    }
                },
                enabled = busy == null,
                modifier = Modifier.clip(RoundedCornerShape(12.dp)).background(Color(0xFF34D399).copy(alpha = 0.15f)).border(1.dp, Color(0xFF34D399).copy(alpha = 0.3f), RoundedCornerShape(12.dp)).padding(horizontal = 16.dp, vertical = 8.dp),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (busy == "allow") CircularProgressIndicator(color = Color(0xFF34D399), strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                    else Icon(Icons.Filled.Shield, contentDescription = null, tint = Color(0xFF34D399), modifier = Modifier.size(14.dp))
                    Text(allowLabel, color = Color(0xFF34D399), fontSize = 12.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(start = 6.dp))
                }
            }
            TextButton(
                onClick = {
                    busy = "deny"
                    scope.launch {
                        AppContainer.chatRepository.approvePermission(sessionId, request.requestId, false)
                        busy = null
                    }
                },
                enabled = busy == null,
                modifier = Modifier.padding(start = 10.dp).clip(RoundedCornerShape(12.dp)).background(Color(0xFFF87171).copy(alpha = 0.15f)).border(1.dp, Color(0xFFF87171).copy(alpha = 0.3f), RoundedCornerShape(12.dp)).padding(horizontal = 16.dp, vertical = 8.dp),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (busy == "deny") CircularProgressIndicator(color = Color(0xFFF87171), strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                    else Icon(Icons.Filled.Shield, contentDescription = null, tint = Color(0xFFF87171), modifier = Modifier.size(14.dp))
                    Text("Deny", color = Color(0xFFF87171), fontSize = 12.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(start = 6.dp))
                }
            }
        }
    }
}

@Composable
private fun QuestionWizard(questions: List<AskQuestionRequest>, sessionId: String) {
    var index by remember(questions.firstOrNull()?.requestId) { mutableStateOf(0) }
    var submitting by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val current = questions.getOrNull(index) ?: questions.firstOrNull() ?: return
    val isLast = index >= questions.size - 1

    fun submit(answer: kotlinx.serialization.json.JsonElement) {
        submitting = true
        scope.launch {
            AppContainer.chatRepository.answerQuestion(sessionId, current.requestId, answer)
            submitting = false
            if (!isLast) index += 1
        }
    }

    QuestionPanel(
        request = current,
        total = questions.size,
        index = (index + 1).coerceAtMost(questions.size),
        isLast = isLast,
        submitting = submitting,
        onAnswerText = { submit(JsonPrimitive(it)) },
        onAnswerList = { submit(JsonArray(it.map { s -> JsonPrimitive(s) })) },
        onSkip = { submit(buildJsonObject { put("skipped", true) }) },
    )
}

@Composable
private fun QuestionPanel(
    request: AskQuestionRequest,
    total: Int,
    index: Int,
    isLast: Boolean,
    submitting: Boolean,
    onAnswerText: (String) -> Unit,
    onAnswerList: (List<String>) -> Unit,
    onSkip: () -> Unit,
) {
    var selected by remember(request.requestId) { mutableStateOf(setOf<String>()) }
    var custom by remember(request.requestId) { mutableStateOf("") }
    val hasOptions = request.options.isNotEmpty()
    val hasAnswer = custom.trim().isNotEmpty() || selected.isNotEmpty()
    val shape = RoundedCornerShape(16.dp)

    Column(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp).padding(bottom = 8.dp).clip(shape)
            .background(Color(0xFF18181B))
            .border(1.dp, Color.White.copy(alpha = 0.15f), shape)
            .padding(16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Filled.HelpOutline, contentDescription = null, tint = Color.White, modifier = Modifier.size(18.dp))
            Text(request.question, color = Color.White, fontSize = 14.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f).padding(start = 10.dp))
            if (total > 1) {
                Text("$index of $total", color = ConsoleColors.TextSecondary, fontSize = 11.sp, fontFamily = com.console.mobile.ui.theme.ConsoleMonoFamily)
            }
        }
        if (hasOptions) {
            Column(modifier = Modifier.padding(start = 28.dp, top = 12.dp)) {
                request.options.forEach { option ->
                    val isSelected = selected.contains(option)
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp).clip(RoundedCornerShape(12.dp))
                            .background(if (isSelected) Color.White.copy(alpha = 0.08f) else Color.Transparent)
                            .border(1.dp, if (isSelected) Color.White.copy(alpha = 0.25f) else Color.White.copy(alpha = 0.1f), RoundedCornerShape(12.dp))
                            .clickable {
                                selected = if (request.isMultiSelect) {
                                    if (isSelected) selected - option else selected + option
                                } else {
                                    setOf(option)
                                }
                            }
                            .padding(horizontal = 12.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(
                            modifier = Modifier.size(16.dp).clip(if (request.isMultiSelect) RoundedCornerShape(4.dp) else CircleShape)
                                .background(if (isSelected) Color.White else Color.Transparent)
                                .border(1.dp, Color.White.copy(alpha = 0.4f), if (request.isMultiSelect) RoundedCornerShape(4.dp) else CircleShape),
                            contentAlignment = Alignment.Center,
                        ) {
                            if (isSelected) {
                                if (request.isMultiSelect) Icon(Icons.Filled.Check, contentDescription = null, tint = Color.Black, modifier = Modifier.size(11.dp))
                                else Box(modifier = Modifier.size(6.dp).clip(CircleShape).background(Color.Black))
                            }
                        }
                        Text(option, color = if (isSelected) Color.White else ConsoleColors.TextSecondary, fontSize = 14.sp, fontWeight = if (isSelected) FontWeight.Medium else FontWeight.Normal, modifier = Modifier.padding(start = 12.dp))
                    }
                }
            }
        }
        OutlinedTextField(
            value = custom,
            onValueChange = { custom = it },
            placeholder = { Text(if (hasOptions) "Or type your own answer…" else "Type your answer…") },
            singleLine = false,
            maxLines = 4,
            colors = OutlinedTextFieldDefaults.colors(focusedContainerColor = Color.Black.copy(alpha = 0.4f), unfocusedContainerColor = Color.Black.copy(alpha = 0.4f), focusedBorderColor = Color.White.copy(alpha = 0.1f), unfocusedBorderColor = Color.White.copy(alpha = 0.1f), focusedTextColor = Color.White, unfocusedTextColor = Color.White, cursorColor = Color.White),
            shape = RoundedCornerShape(12.dp),
            modifier = Modifier.fillMaxWidth().padding(start = 28.dp, top = 4.dp),
        )
        Row(modifier = Modifier.fillMaxWidth().padding(start = 28.dp, top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            if (request.skippable) {
                TextButton(onClick = onSkip, enabled = !submitting, modifier = Modifier.clip(RoundedCornerShape(999.dp)).border(1.dp, Color.White.copy(alpha = 0.15f), RoundedCornerShape(999.dp)).padding(horizontal = 16.dp, vertical = 8.dp)) {
                    Text("Skip", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                }
            } else {
                androidx.compose.foundation.layout.Spacer(modifier = Modifier.weight(1f))
            }
            androidx.compose.foundation.layout.Spacer(modifier = Modifier.weight(1f))
            TextButton(
                onClick = {
                    if (custom.trim().isNotEmpty()) onAnswerText(custom.trim())
                    else if (selected.isNotEmpty()) {
                        if (request.isMultiSelect) onAnswerList(selected.toList()) else onAnswerText(selected.first())
                    }
                },
                enabled = hasAnswer && !submitting,
                modifier = Modifier.clip(RoundedCornerShape(999.dp)).background(if (hasAnswer && !submitting) Color.White else Color.White.copy(alpha = 0.1f)).padding(horizontal = 20.dp, vertical = 10.dp),
            ) {
                if (submitting) CircularProgressIndicator(color = Color.Black, strokeWidth = 2.dp, modifier = Modifier.size(12.dp))
                else Text(if (isLast) (if (total > 1) "Submit all" else "Submit") else "Next", color = if (hasAnswer && !submitting) Color.Black else Color.White.copy(alpha = 0.6f), fontSize = 12.sp, fontWeight = FontWeight.Bold)
            }
        }
    }
}
