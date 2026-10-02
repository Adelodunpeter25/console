package com.console.mobile.feature.devices

import android.annotation.SuppressLint
import android.util.Log
import android.webkit.ConsoleMessage
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
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
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.data.model.DeviceActionRequest
import com.console.mobile.data.model.DeviceDescriptor
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronLeft
import io.github.lyxnx.compose.ui.tablericons.outline.DeviceMobile
import io.github.lyxnx.compose.ui.tablericons.outline.Home
import io.github.lyxnx.compose.ui.tablericons.outline.Lock
import io.github.lyxnx.compose.ui.tablericons.outline.Moon
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerPlay
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerStop
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import io.github.lyxnx.compose.ui.tablericons.outline.Volume
import io.github.lyxnx.compose.ui.tablericons.outline.Volume2
import java.io.ByteArrayInputStream
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

private class AndroidBridge(private val onStatus: (String) -> Unit) {
    @JavascriptInterface
    fun postMessage(json: String) {
        try {
            val root = kotlinx.serialization.json.Json.parseToJsonElement(json) as? kotlinx.serialization.json.JsonObject ?: return
            val type = root["type"]?.toString()?.trim('"')
            if (type == "status") {
                val status = root["status"]?.toString()?.trim('"') ?: ""
                onStatus(status)
            }
        } catch (_: Exception) {}
    }
}

@SuppressLint("SetJavaScriptEnabled")
@Composable
fun DevicesScreen(
    modifier: Modifier = Modifier,
    onBack: (() -> Unit)? = null,
) {
    val repo = AppContainer.deviceRepository
    val devices by repo.devices.collectAsStateWithLifecycle()
    val isLoading by repo.isLoading.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    var selectedDevice by remember { mutableStateOf<DeviceDescriptor?>(null) }
    var streamStatus by remember { mutableStateOf("idle") }
    var isBooting by remember { mutableStateOf(false) }
    var isStopping by remember { mutableStateOf(false) }
    var webViewRef by remember { mutableStateOf<WebView?>(null) }
    // The player page defines window.__consoleDevice only once it has loaded;
    // calling start() before then is a silent no-op, so wait for this.
    var pageReady by remember { mutableStateOf(false) }
    var dropdownExpanded by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        repo.refreshDevices()
    }

    // Keep selected device reference updated from polled device list
    LaunchedEffect(devices) {
        selectedDevice?.let { cur ->
            val updated = devices.find { it.id == cur.id || it.name == cur.name }
            if (updated != null) {
                selectedDevice = updated
                if (updated.isBooted && isBooting) {
                    isBooting = false
                }
            }
        }
    }

    // Auto-poll while booting
    LaunchedEffect(isBooting) {
        if (isBooting) {
            while (isActive && isBooting) {
                delay(3000)
                repo.refreshDevices()
            }
        }
    }

    // Stream start / stop sync with WebView
    LaunchedEffect(selectedDevice?.id, selectedDevice?.state, webViewRef, pageReady) {
        val wv = webViewRef ?: return@LaunchedEffect
        if (!pageReady) return@LaunchedEffect
        val dev = selectedDevice
        if (dev != null && dev.isBooted) {
            val streamUrl = repo.getStreamUrl(dev.id, dev.platform)
            val config = buildJsonObject {
                put("streamUrl", streamUrl)
            }.toString()
            val escaped = config.replace("\\", "\\\\").replace("'", "\\'")
            wv.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.start('$escaped')", null)
        } else {
            wv.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.stop()", null)
            streamStatus = "idle"
        }
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .background(ConsoleColors.Background),
    ) {
        ScreenHeader(title = "Devices", onBack = onBack)
        // Top Toolbar
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(ConsoleColors.Surface)
                .border(1.dp, ConsoleColors.BorderSubtle)
                .padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            // Dropdown trigger
            Box(modifier = Modifier.weight(1f)) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(34.dp)
                        .clip(RoundedCornerShape(6.dp))
                        .background(ConsoleColors.SurfaceElevated)
                        .border(1.dp, ConsoleColors.Border, RoundedCornerShape(6.dp))
                        .clickable { dropdownExpanded = true }
                        .padding(horizontal = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(
                        imageVector = TablerIcons.Outline.DeviceMobile,
                        contentDescription = null,
                        tint = ConsoleColors.TextMuted,
                        modifier = Modifier.size(15.dp),
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = selectedDevice?.let { "${it.displayName} · ${it.platform}" } ?: "Select device",
                        color = ConsoleColors.TextPrimary,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f),
                    )
                    selectedDevice?.let { dev ->
                        val badgeColor = when {
                            isBooting -> ConsoleColors.StatusRunning
                            dev.isBooted -> ConsoleColors.StatusReady
                            else -> ConsoleColors.TextMuted
                        }
                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(999.dp))
                                .background(badgeColor.copy(alpha = 0.12f))
                                .padding(horizontal = 6.dp, vertical = 2.dp),
                        ) {
                            Text(
                                text = if (isBooting) "Booting" else if (dev.isBooted) "Ready" else dev.state,
                                color = badgeColor,
                                fontSize = 9.sp,
                                fontWeight = FontWeight.Medium,
                            )
                        }
                        Spacer(Modifier.width(6.dp))
                    }
                    Icon(
                        imageVector = TablerIcons.Outline.ChevronDown,
                        contentDescription = null,
                        tint = ConsoleColors.TextMuted,
                        modifier = Modifier.size(13.dp),
                    )
                }

                DropdownMenu(
                    expanded = dropdownExpanded,
                    onDismissRequest = { dropdownExpanded = false },
                    modifier = Modifier.background(ConsoleColors.SurfaceElevated),
                ) {
                    if (devices.isEmpty()) {
                        DropdownMenuItem(
                            text = { Text("No devices found", color = ConsoleColors.TextMuted, fontSize = 12.sp) },
                            onClick = { dropdownExpanded = false },
                        )
                    } else {
                        devices.forEach { dev ->
                            DropdownMenuItem(
                                text = {
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.SpaceBetween,
                                        verticalAlignment = Alignment.CenterVertically,
                                    ) {
                                        Text(
                                            text = "${dev.displayName} · ${dev.platform}",
                                            color = ConsoleColors.TextPrimary,
                                            fontSize = 12.sp,
                                            fontWeight = if (selectedDevice?.id == dev.id) FontWeight.Bold else FontWeight.Normal,
                                        )
                                        Text(
                                            text = dev.state,
                                            color = if (dev.isBooted) ConsoleColors.StatusReady else ConsoleColors.TextMuted,
                                            fontSize = 10.sp,
                                        )
                                    }
                                },
                                onClick = {
                                    selectedDevice = dev
                                    dropdownExpanded = false
                                },
                            )
                        }
                    }
                }
            }

            // Refresh button
            IconButton(
                onClick = { scope.launch { repo.refreshDevices() } },
                modifier = Modifier.size(34.dp),
            ) {
                if (isLoading) {
                    CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp, color = ConsoleColors.TextPrimary)
                } else {
                    Icon(TablerIcons.Outline.Refresh, contentDescription = "Refresh", tint = ConsoleColors.TextSecondary, modifier = Modifier.size(15.dp))
                }
            }

            // Boot (Play) button
            val canBoot = selectedDevice != null && !selectedDevice!!.isBooted && !isBooting && !isStopping
            IconButton(
                onClick = {
                    selectedDevice?.let { dev ->
                        isBooting = true
                        scope.launch {
                            repo.bootDevice(dev.id, dev.platform)
                        }
                    }
                },
                enabled = canBoot,
                modifier = Modifier.size(34.dp),
            ) {
                Icon(
                    TablerIcons.Outline.PlayerPlay,
                    contentDescription = "Boot device",
                    tint = if (canBoot) ConsoleColors.TextPrimary else ConsoleColors.TextMuted.copy(alpha = 0.4f),
                    modifier = Modifier.size(15.dp),
                )
            }

            // Stop button
            val canStop = selectedDevice != null && selectedDevice!!.isBooted && !isStopping
            IconButton(
                onClick = {
                    selectedDevice?.let { dev ->
                        isStopping = true
                        webViewRef?.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.stop()", null)
                        scope.launch {
                            repo.shutdownDevice(dev.id, dev.platform)
                            isStopping = false
                        }
                    }
                },
                enabled = canStop,
                modifier = Modifier.size(34.dp),
            ) {
                Icon(
                    TablerIcons.Outline.PlayerStop,
                    contentDescription = "Shut down simulator",
                    tint = if (canStop) ConsoleColors.Destructive else ConsoleColors.TextMuted.copy(alpha = 0.4f),
                    modifier = Modifier.size(15.dp),
                )
            }
        }

        // Main content area
        val dev = selectedDevice
        if (dev == null) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(32.dp),
                contentAlignment = Alignment.Center,
            ) {
                EmptyState(
                    title = "No device selected",
                    description = "Pick an iOS simulator or Android emulator from the dropdown above.",
                    icon = {
                        Icon(
                            imageVector = TablerIcons.Outline.DeviceMobile,
                            contentDescription = null,
                            tint = ConsoleColors.TextMuted,
                            modifier = Modifier.size(32.dp),
                        )
                    },
                )
            }
        } else {
            Row(
                modifier = Modifier
                    .fillMaxSize()
                    .background(ConsoleColors.Background),
            ) {
                // Video stream surface (WebView)
                Box(
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxHeight(),
                    contentAlignment = Alignment.Center,
                ) {
                    AndroidView(
                        factory = { ctx ->
                            WebView(ctx).apply {
                                settings.javaScriptEnabled = true
                                settings.domStorageEnabled = true
                                settings.mediaPlaybackRequiresUserGesture = false
                                settings.cacheMode = WebSettings.LOAD_NO_CACHE
                                // The page is https (secure context, needed for WebCodecs)
                                // but the stream socket is usually plain ws:// on a LAN.
                                settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
                                setBackgroundColor(0xFF0A0A0B.toInt())
                                addJavascriptInterface(AndroidBridge { s ->
                                    streamStatus = s
                                }, "AndroidBridge")
                                webChromeClient = object : WebChromeClient() {
                                    override fun onConsoleMessage(consoleMessage: ConsoleMessage): Boolean {
                                        Log.d("DevicesPlayer", "${consoleMessage.message()} -- From line ${consoleMessage.lineNumber()} of ${consoleMessage.sourceId()}")
                                        return true
                                    }
                                }
                                webViewClient = object : WebViewClient() {
                                    override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                                        if (request.url.toString() != DEVICES_PLAYER_URL) return null
                                        return WebResourceResponse("text/html", "utf-8", ByteArrayInputStream(DEVICES_PLAYER_HTML.toByteArray()))
                                    }

                                    override fun onPageFinished(view: WebView, url: String?) {
                                        if (url == DEVICES_PLAYER_URL) pageReady = true
                                    }
                                }
                                loadUrl(DEVICES_PLAYER_URL)
                                webViewRef = this
                            }
                        },
                        onRelease = { wv ->
                            wv.destroy()
                            if (webViewRef === wv) webViewRef = null
                            pageReady = false
                            streamStatus = "idle"
                        },
                        modifier = Modifier.fillMaxSize(),
                    )
                    StreamStatusOverlay(device = dev, status = streamStatus, isBooting = isBooting)
                }

                // Right Hardware Controls Rail (when booted)
                if (dev.isBooted) {
                    Box(
                        modifier = Modifier
                            .fillMaxHeight()
                            .padding(end = 8.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        Card(
                            shape = RoundedCornerShape(10.dp),
                            colors = CardDefaults.cardColors(containerColor = ConsoleColors.SurfaceElevated),
                            border = androidx.compose.foundation.BorderStroke(1.dp, ConsoleColors.Border),
                            modifier = Modifier.padding(vertical = 12.dp),
                        ) {
                            Column(
                                modifier = Modifier.padding(4.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(4.dp),
                            ) {
                                RailButton(
                                    icon = TablerIcons.Outline.Home,
                                    tooltip = "Home",
                                    onClick = {
                                        webViewRef?.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.button('home')", null)
                                    },
                                )
                                if (!dev.isIos) {
                                    RailButton(
                                        icon = TablerIcons.Outline.ChevronLeft,
                                        tooltip = "Back",
                                        onClick = {
                                            webViewRef?.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.button('back')", null)
                                        },
                                    )
                                }
                                RailButton(
                                    icon = TablerIcons.Outline.Volume,
                                    tooltip = "Volume up",
                                    onClick = {
                                        webViewRef?.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.button('volume-up')", null)
                                    },
                                )
                                RailButton(
                                    icon = TablerIcons.Outline.Volume2,
                                    tooltip = "Volume down",
                                    onClick = {
                                        webViewRef?.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.button('volume-down')", null)
                                    },
                                )
                                RailButton(
                                    icon = TablerIcons.Outline.Lock,
                                    tooltip = "Lock / Power",
                                    onClick = {
                                        webViewRef?.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.button('power')", null)
                                    },
                                )
                                RailButton(
                                    icon = TablerIcons.Outline.Moon,
                                    tooltip = "Toggle dark / light",
                                    onClick = {
                                        scope.launch {
                                            repo.interact(dev.id, dev.platform, DeviceActionRequest(action = "appearance", appearance = "toggle"))
                                        }
                                    },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            webViewRef?.evaluateJavascript("window.__consoleDevice && window.__consoleDevice.stop()", null)
        }
    }
}

/** Human-readable stream state, so a dead stream isn't just a blank panel. */
@Composable
private fun StreamStatusOverlay(device: DeviceDescriptor, status: String, isBooting: Boolean) {
    val message = when {
        isBooting -> "Booting ${device.displayName}…"
        !device.isBooted -> "${device.displayName} is not running. Tap play to boot it."
        status == "streaming" -> null
        status == "idle" || status == "ready" || status == "connecting" -> "Connecting…"
        status == "waiting for video" -> "Waiting for video…"
        status == "disconnected" -> "Stream disconnected. Retrying…"
        else -> status
    } ?: return
    val isError = status == "disconnected" || status.contains("unavailable") || status.contains("error")
    Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
        Text(
            text = message,
            color = if (isError) ConsoleColors.Destructive else ConsoleColors.TextSecondary,
            fontSize = 13.sp,
        )
    }
}

@Composable
private fun RailButton(
    icon: ImageVector,
    tooltip: String,
    onClick: () -> Unit,
) {
    Box(
        modifier = Modifier
            .size(32.dp)
            .clip(RoundedCornerShape(6.dp))
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = tooltip,
            tint = ConsoleColors.TextSecondary,
            modifier = Modifier.size(15.dp),
        )
    }
}
