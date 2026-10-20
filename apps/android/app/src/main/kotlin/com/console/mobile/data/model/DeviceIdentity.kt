package com.console.mobile.data.model

import android.os.Build

/**
 * The name this device calls itself in the server's device list.
 *
 * Derived rather than typed: a phone that asks the user to name itself
 * before it can connect is a phone that does not get connected. The model
 * name is what a person would call the device anyway ("Pixel 7"), and it
 * survives a reinstall — the server keeps the name it was given.
 */
fun deviceName(): String {
    val model = Build.MODEL?.trim().orEmpty()
    if (model.isNotEmpty()) return model
    val manufacturer = Build.MANUFACTURER?.trim().orEmpty()
    return manufacturer.ifEmpty { "Android device" }
}

/**
 * The platform id the server records for this device.
 */
const val PLATFORM_ANDROID = "android"
