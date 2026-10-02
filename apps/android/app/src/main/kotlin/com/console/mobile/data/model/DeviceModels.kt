package com.console.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
data class DeviceDescriptor(
    val id: String,
    val name: String,
    val platform: String,
    val state: String,
    val model: String? = null,
    val osVersion: String? = null,
    val isAvailable: Boolean = true,
) {
    val isBooted: Boolean get() = state.equals("booted", ignoreCase = true)
    val isBooting: Boolean get() = state.equals("booting", ignoreCase = true)
    val isIos: Boolean get() = platform.equals("ios", ignoreCase = true)
    val displayName: String get() = name.ifEmpty { id }
}

@Serializable
data class DeviceDiagnostics(
    val xcodeInstalled: Boolean = false,
    val xcodeVersion: String? = null,
    val simctlAvailable: Boolean = false,
    val androidSdkFound: Boolean = false,
    val adbAvailable: Boolean = false,
    val emulatorAvailable: Boolean = false,
    val diskFreeBytes: Long = 0,
    val hasEnoughDiskSpace: Boolean = false,
    val errors: List<String> = emptyList(),
)

@Serializable
data class DeviceActionRequest(
    val action: String,
    val x: Double? = null,
    val y: Double? = null,
    val endX: Double? = null,
    val endY: Double? = null,
    val durationMs: Int? = null,
    val text: String? = null,
    val key: String? = null,
    val appearance: String? = null,
)
