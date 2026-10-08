package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import console.v1.DeviceActionRequest
import console.v1.DeviceDescriptor
import console.v1.DeviceDiagnostics
import java.net.URLEncoder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext

class DeviceRepository(
    private val api: ConsoleApi,
    private val apiClient: ConsoleApiClient,
) {
    private val _devices = MutableStateFlow<List<DeviceDescriptor>>(emptyList())
    val devices: StateFlow<List<DeviceDescriptor>> = _devices.asStateFlow()

    private val _isLoading = MutableStateFlow(false)
    val isLoading: StateFlow<Boolean> = _isLoading.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    suspend fun refreshDevices(): Result<List<DeviceDescriptor>> = withContext(Dispatchers.IO) {
        _isLoading.value = true
        _error.value = null
        try {
            val list = api.getDevices()
            _devices.value = list
            _isLoading.value = false
            Result.success(list)
        } catch (e: Exception) {
            val msg = e.message ?: "Failed to load devices"
            _error.value = msg
            _isLoading.value = false
            Result.failure(e)
        }
    }

    suspend fun getDiagnostics(): Result<DeviceDiagnostics> = withContext(Dispatchers.IO) {
        try {
            Result.success(api.getDeviceDiagnostics())
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun bootDevice(id: String, platform: String): Result<Unit> = withContext(Dispatchers.IO) {
        try {
            api.bootDevice(id, platform)
            Result.success(Unit)
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun shutdownDevice(id: String, platform: String): Result<Unit> = withContext(Dispatchers.IO) {
        try {
            api.shutdownDevice(id, platform)
            refreshDevices()
            Result.success(Unit)
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun interact(id: String, platform: String, action: DeviceActionRequest): Result<Unit> =
        withContext(Dispatchers.IO) {
            try {
                api.interactDevice(id, platform, action)
                Result.success(Unit)
            } catch (e: Exception) {
                Result.failure(e)
            }
        }

    /**
     * Builds the WebSocket URL for live H.264 streaming.
     */
    fun getStreamUrl(id: String, platform: String): String {
        val base = apiClient.baseUrl.trimEnd('/')
        val wsBase = if (base.startsWith("https://", ignoreCase = true)) {
            "wss://" + base.substring(8)
        } else if (base.startsWith("http://", ignoreCase = true)) {
            "ws://" + base.substring(7)
        } else {
            base
        }
        val encId = URLEncoder.encode(id, "UTF-8")
        val encPlat = URLEncoder.encode(platform, "UTF-8")
        return "$wsBase/api/devices/$encId/stream?platform=$encPlat"
    }

    val serverBaseUrl: String
        get() = apiClient.baseUrl.trimEnd('/')
}
