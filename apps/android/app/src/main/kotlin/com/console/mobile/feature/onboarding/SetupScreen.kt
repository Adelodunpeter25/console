package com.console.mobile.feature.onboarding

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.data.store.SetupState
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.theme.NewTheme

/**
 * First-run setup checklist for a server the user has just connected to.
 *
 * Distinct from [OnboardingScreen], which is the *connect* step: this assumes a
 * backend already exists and walks through setting it up.
 *
 * Every step is skippable and nothing is required to dismiss. The three states
 * shown are read live from the server, so a server that is already half-configured
 * does not make the user redo work — and if they delete their last project later,
 * that row correctly goes back to unmet without the panel reopening, because only
 * the dismissal is remembered.
 */
@Composable
fun SetupScreen(state: SetupState, onStep: (SetupStep) -> Unit, onSkip: () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(NewTheme.Background)
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 40.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Text(
            "Set up ${state.serverName ?: "this server"}",
            color = NewTheme.TextPrimary,
            fontSize = 22.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth(),
        )
        Text(
            "None of these are required to get started.",
            color = NewTheme.TextSecondary,
            fontSize = 14.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth().padding(top = 6.dp, bottom = 24.dp),
        )

        SetupRow(
            title = "Add a project",
            detail = "A project is the folder Console works in.",
            done = state.hasProject,
            loading = state.loading,
            action = "Add project",
            onClick = { onStep(SetupStep.AddProject) },
        )
        SetupRow(
            title = "Sign in to an AI provider",
            detail = "Needed before a run can talk to a model.",
            done = state.hasProvider,
            loading = state.loading,
            action = "Open providers",
            onClick = { onStep(SetupStep.Providers) },
        )
        SetupRow(
            title = "Add a GitHub key",
            detail = "Lets the server clone your private repositories.",
            done = state.hasGithub,
            loading = state.loading,
            action = "Open account",
            onClick = { onStep(SetupStep.Github) },
        )

        Text(
            if (state.isComplete) "This server is ready to use." else "Skip for now — you can do this any time in Settings.",
            color = NewTheme.TextMuted,
            fontSize = 12.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth().padding(top = 24.dp),
        )
        ActionButton(
            text = "Got it",
            onClick = onSkip,
            kind = ActionButtonKind.Secondary,
            surface = NewTheme.Raised,
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
        )
    }
}

/** The three setup steps, so callers navigate without matching on strings. */
enum class SetupStep { AddProject, Providers, Github }

@Composable
private fun SetupRow(
    title: String,
    detail: String,
    done: Boolean,
    loading: Boolean,
    action: String,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp)
            .background(NewTheme.Card, RoundedCornerShape(12.dp))
            .padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier
                .size(8.dp)
                .clip(CircleShape)
                .background(if (done) NewTheme.Success else NewTheme.TextMuted),
        )
        Column(
            modifier = Modifier
                .weight(1f)
                .padding(start = 12.dp, end = 10.dp),
        ) {
            Text(title, color = NewTheme.TextPrimary, fontSize = 15.sp)
            Text(
                detail,
                color = NewTheme.TextMuted,
                fontSize = 12.sp,
                modifier = Modifier.padding(top = 1.dp),
            )
        }
        when {
            // While the checks are in flight the state is unknown, not false.
            // A grey dot plus a live button would read as "not done" for a
            // server that may already be fully set up.
            done -> Text("Done", color = NewTheme.Success, fontSize = 12.sp)
            loading -> Text("Checking…", color = NewTheme.TextMuted, fontSize = 12.sp)
            else -> ActionButton(
                text = action,
                onClick = onClick,
                kind = ActionButtonKind.Primary,
                surface = NewTheme.Raised,
                compact = true,
            )
        }
    }
}
