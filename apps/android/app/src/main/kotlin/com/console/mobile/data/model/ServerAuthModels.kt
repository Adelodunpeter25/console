package com.console.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
data class PairingRequest(
    val grant: String,
    val deviceName: String,
    val platform: String = "android",
)

@Serializable
data class PairingData(
    val device: PairedDevice,
    val token: String,
    val expiresAt: Long,
)

@Serializable
data class PairedDevice(
    val id: String,
    val name: String,
    val platform: String,
)

@Serializable
data class PairingGrantRequest(
    val deviceName: String,
    val platform: String = "android",
)

@Serializable
data class PairingGrantData(
    val grant: String,
    val expiresAt: Long,
)

@Serializable
data class DeviceInfo(
    val id: String,
    val name: String,
    val platform: String,
    val createdAt: Long? = null,
    val lastSeenAt: Long? = null,
    val revokedAt: Long? = null,
)

@Serializable
data class DeviceListData(val devices: List<DeviceInfo> = emptyList())
