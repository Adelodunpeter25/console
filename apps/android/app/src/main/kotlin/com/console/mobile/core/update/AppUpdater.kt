package com.console.mobile.core.update

import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.core.content.FileProvider
import com.console.mobile.AppContainer
import com.console.mobile.BuildConfig
import java.io.File
import java.io.IOException
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.Request

private const val RELEASE_BASE = "https://github.com/Adelodunpeter25/console/releases/download/console-android"

/** `latest.json` published next to the APK by the console-android workflow. */
@Serializable
data class UpdateManifest(val versionCode: Int, val versionName: String)

private val json = Json { ignoreUnknownKeys = true }

fun parseUpdateManifest(text: String): UpdateManifest = json.decodeFromString(text)

fun UpdateManifest.isNewerThan(installedVersionCode: Int): Boolean = versionCode > installedVersionCode

object AppUpdater {
    private val installedVersionCode get() = BuildConfig.VERSION_CODE

    /** The published manifest when it is newer than this install, else null. */
    fun checkForUpdate(): UpdateManifest? {
        val request = Request.Builder().url("$RELEASE_BASE/latest.json").build()
        AppContainer.httpCallClient.newCall(request).execute().use { response ->
            if (!response.isSuccessful) throw IOException("Update check failed (HTTP ${response.code})")
            val manifest = parseUpdateManifest(response.body!!.string())
            return manifest.takeIf { it.isNewerThan(installedVersionCode) }
        }
    }

    /** Downloads the APK, reporting 0..1 progress when the size is known. */
    fun download(onProgress: (Float) -> Unit): File {
        val dir = File(AppContainer.appContext.cacheDir, "updates").apply { mkdirs() }
        val target = File(dir, "app-release.apk")
        val client = AppContainer.httpCallClient.newBuilder().callTimeout(0, java.util.concurrent.TimeUnit.SECONDS).build()
        client.newCall(Request.Builder().url("$RELEASE_BASE/app-release.apk").build()).execute().use { response ->
            if (!response.isSuccessful) throw IOException("Download failed (HTTP ${response.code})")
            val body = response.body!!
            val total = body.contentLength()
            var read = 0L
            body.byteStream().use { input ->
                target.outputStream().use { output ->
                    val buffer = ByteArray(64 * 1024)
                    while (true) {
                        val n = input.read(buffer)
                        if (n < 0) break
                        output.write(buffer, 0, n)
                        read += n
                        if (total > 0) onProgress(read.toFloat() / total)
                    }
                }
            }
        }
        return target
    }

    /**
     * Hands the APK to the system installer. Returns false when the user has
     * not yet allowed installs from this app; the settings page for that is
     * opened instead and they tap Install again afterwards.
     */
    fun install(apk: File): Boolean {
        val context = AppContainer.appContext
        if (!context.packageManager.canRequestPackageInstalls()) {
            context.startActivity(
                Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            )
            return false
        }
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.updates", apk)
        context.startActivity(
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK),
        )
        return true
    }
}
