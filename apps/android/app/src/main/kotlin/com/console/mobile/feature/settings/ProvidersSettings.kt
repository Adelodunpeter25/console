package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.Edit
import io.github.lyxnx.compose.ui.tablericons.outline.Login
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.components.ProviderIcon
import kotlinx.coroutines.launch
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.OptionRow
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.SectionSkeleton
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import console.v1.ProviderAccount

/**
 * What an account row shows instead of the raw email: its label when the
 * user named it, else the local part of the email ("adelodunpeter" for
 * "adelodunpeter@example.com") — readable, and the full email never renders
 * unless the user typed it as a name themselves.
 */
internal fun accountDisplayName(acc: ProviderAccount): String =
    acc.label?.ifEmpty { null }
        ?: acc.email?.substringBefore('@')?.ifEmpty { null }
        ?: acc.id

/**
 * Port of screens/settings/account-settings.tsx. Named ProvidersSettings: this
 * screen is about AI provider credentials, one row per provider. Tapping a
 * provider opens a bottom sheet with that provider's accounts (Use / Rename /
 * Remove) and an Add Account row; sign-in itself goes through the OAuth Custom
 * Tab (getLoginUrl → submitCallback).
 */
@Composable
fun ProvidersSettings(onBack: () -> Unit) {
    val context = LocalContext.current
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    var sheetProvider by remember { mutableStateOf<String?>(null) }
    var renameTarget by remember { mutableStateOf<Pair<String, String>?>(null) }
    var loggingIn by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) {
        AppContainer.providerRepository.loadProviders()
        AppContainer.authRepository.loadStatus()
        AppContainer.authRepository.loadProviderCredentials()
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
                        // A pasted API key is not a login: there is no browser
                        // trip and no session to renew, so it gets a field
                        // instead of the account sheet below.
                        if (p.auth_method == "api-key") {
                            ApiKeyProviderRow(
                                providerId = p.name,
                                displayName = p.display_name,
                                description = p.description,
                            )
                            if (i < providers.lastIndex) SectionDivider(startInset = 58.dp)
                            return@forEachIndexed
                        }
                        val status = authState.status?.get(p.name)
                        val accounts = status?.accounts.orEmpty()
                        val activeAccount = accounts.firstOrNull { it.active == true }
                        val loggedIn = status?.logged_in == true
                        val busy = loggingIn == p.name
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { sheetProvider = p.name }
                                .padding(horizontal = 18.dp, vertical = 14.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            ProviderIcon(provider = p.name, sizeDp = 24)
                            Column(modifier = Modifier.weight(1f).padding(start = 16.dp, end = 12.dp)) {
                                Text(p.display_name.ifBlank { p.name }, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                Text(
                                    when {
                                        actuallySignedIn(accounts, loggedIn) -> activeAccount?.let { accountDisplayName(it) } ?: "Connected"
                                        // Zero-active with stored rows means the
                                        // provider is disabled (no credential
                                        // file, no fallback) until one is picked.
                                        accounts.isNotEmpty() -> "No account selected"
                                        else -> "Not connected"
                                    },
                                    color = if (actuallySignedIn(accounts, loggedIn)) NewTheme.Success else NewTheme.TextMuted,
                                    fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp),
                                )
                            }
                            Icon(
                                TablerIcons.Outline.ChevronRight,
                                contentDescription = if (busy) "Signing in…" else "Accounts",
                                tint = if (busy) NewTheme.Success else NewTheme.TextMuted,
                                modifier = Modifier.size(18.dp),
                            )
                        }
                        if (i < providers.lastIndex) SectionDivider(startInset = 58.dp)
                    }
                }
            }
            GitHubAccountSection()
            val err = authState.error
            if (err != null) {
                Text(err, color = NewTheme.Danger, fontSize = 12.sp, modifier = Modifier.padding(horizontal = 4.dp, vertical = 16.dp))
            }
        }
    }

    // Account sheet for the tapped provider: every stored login as a row
    // (named by its label / email name, never the raw email), plus one
    // Add Account row that drives the existing OAuth login flow. Rendered
    // with the shared BaseSheet/OptionRow design system.
    sheetProvider?.let { providerId ->
        val status = authState.status?.get(providerId)
        val accounts = status?.accounts.orEmpty()
        val provider = providerState.providers.firstOrNull { it.name == providerId }
        BaseSheet(
            onDismiss = { sheetProvider = null },
            title = provider?.display_name?.ifBlank { null } ?: providerId,
        ) {
            Text(
                "Signed-in accounts for this provider.",
                color = NewTheme.TextMuted,
                fontSize = 12.sp,
                modifier = Modifier.padding(start = 4.dp, bottom = 8.dp),
            )
            if (accounts.isEmpty()) {
                Text("No accounts yet — sign in below.", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(start = 4.dp, top = 8.dp, bottom = 8.dp))
            }
            accounts.forEach { acc ->
                AccountSheetRow(
                    providerId = providerId,
                    account = acc,
                    onRename = { renameTarget = providerId to acc.id },
                )
            }
            // Add account always row-1-ably available: same OAuth flow the
            // Login button used, landing in a NEW account that never steals
            // the active one (server decision).
            OptionRow(
                title = if (loggingIn == providerId) "Signing in…" else "Add account",
                leading = {
                    Icon(TablerIcons.Outline.Login, contentDescription = null, tint = NewTheme.TextPrimary, modifier = Modifier.size(20.dp))
                },
                onClick = {
                    val current = providerId
                    loggingIn = current
                    scope.launch {
                        try {
                            // LocalContext.current inside the sheet is
                            // still the hosting Activity, which Custom
                            // Tabs requires.
                            OAuthLoginLauncher.login(context, AppContainer.authRepository, current)
                        } catch (e: Exception) {
                            confirmAlert("Login Failed", e.message ?: "Login failed.")
                        } finally {
                            loggingIn = null
                        }
                    }
                },
            )
        }
    }

    // Rename dialog: the label is the account's display name everywhere —
    // the provider row subtitle, the sheet rows, and (server-merge letting)
    // every other session that reads the status.
    renameTarget?.let { (rProvider, rId) ->
        val existing = authState.status?.get(rProvider)?.accounts
            ?.firstOrNull { it.id == rId }
            ?.let { accountDisplayName(it) }
            .orEmpty()
        var name by remember(rProvider, rId) { mutableStateOf(existing) }
        AlertDialog(
            onDismissRequest = { renameTarget = null },
            title = { Text("Rename account") },
            text = {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    singleLine = true,
                    placeholder = { Text("e.g. Work account") },
                )
            },
            confirmButton = {
                TextButton(
                    enabled = name.isNotBlank(),
                    onClick = {
                        renameTarget = null
                        scope.launch {
                            try { AppContainer.authRepository.renameAccount(rProvider, rId, name) }
                            catch (_: Exception) { /* authState.error shows it */ }
                        }
                    },
                ) { Text("Save") }
            },
            dismissButton = {
                TextButton(onClick = { renameTarget = null }) { Text("Cancel") }
            },
        )
    }
}

/**
 * "Connected" means a usable active account, not just the status flag: while
 * an env token owns the provider, loggedIn stays true but no file account is
 * active, and the row must not pretend a file account is running.
 */
private fun actuallySignedIn(accounts: List<ProviderAccount>, loggedIn: Boolean): Boolean =
    loggedIn && accounts.any { it.active == true }

/**
 * One account row inside the sheet: display name (label / email name), the
 * selected-style check for the active account, trailing Rename/Use/Remove for
 * file accounts, and row-tap = Use on an inactive one. The synthetic
 * environment account renders read-only — it always owns the provider while
 * the env token is set, so a Use button would lie.
 */
@Composable
private fun AccountSheetRow(
    providerId: String,
    account: ProviderAccount,
    onRename: () -> Unit,
) {
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val accId = account.id
    val active = account.active == true
    val isEnv = account.env == true
    val busy = authState.accountBusy == (providerId to accId)

    OptionRow(
        title = accountDisplayName(account),
        subtitle = if (isEnv) "Environment token (set on this server)" else null,
        selected = active,
        trailing = if (busy) {
            {
                CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp)
            }
        } else if (!isEnv) {
            {
                Icon(
                    TablerIcons.Outline.Edit,
                    contentDescription = "Rename",
                    tint = NewTheme.TextSecondary,
                    modifier = Modifier
                        .size(18.dp)
                        .clickable(onClick = onRename),
                )
                if (!active) {
                    Text(
                        "Use",
                        color = NewTheme.TextSecondary,
                        fontSize = 13.sp,
                        modifier = Modifier
                            .clickable {
                                scope.launch {
                                    try { AppContainer.authRepository.useAccount(providerId, accId) }
                                    catch (_: Exception) { /* authState.error shows it */ }
                                }
                            }
                            .padding(horizontal = 8.dp, vertical = 4.dp),
                    )
                }
                Text(
                    "Remove",
                    color = NewTheme.Danger,
                    fontSize = 13.sp,
                    modifier = Modifier
                        .clickable {
                            scope.launch {
                                try { AppContainer.authRepository.removeAccount(providerId, accId) }
                                catch (_: Exception) { /* authState.error shows it */ }
                            }
                        }
                        .padding(start = 8.dp, top = 4.dp, bottom = 4.dp),
                )
            }
        } else null,
        onClick = {
            // Tapping the row itself switches: the same action the Use label
            // offers, and a no-op for the already-active or env account.
            if (!isEnv && !active && authState.accountBusy == null) {
                scope.launch {
                    try { AppContainer.authRepository.useAccount(providerId, accId) }
                    catch (_: Exception) { /* authState.error shows it */ }
                }
            }
        },
    )
}
