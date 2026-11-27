package com.console.mobile.feature.devices

/**
 * Virtual https origin the player page is served from (intercepted in the
 * WebViewClient, never hits the network). WebCodecs only exists in secure
 * contexts, and the server's own http:// address isn't one.
 *
 * The page itself is the server's device_player.html, copied into the app's
 * assets at build time (see copyDevicePlayer in app/build.gradle.kts).
 */
internal const val DEVICES_PLAYER_URL = "https://appassets.androidplatform.net/devices/player.html"

internal const val DEVICES_PLAYER_ASSET = "devices/player.html"
